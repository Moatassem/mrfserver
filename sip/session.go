/*
# Software Name : Session Router (SR)
# SPDX-FileCopyrightText: Copyright (c) Orange Business - OINIS/Services/NSF
# SPDX-License-Identifier: Apache-2.0
#
# This software is distributed under the Apache-2.0
# See the "LICENSES" directory for more details.
#
# Authors:
# - Moatassem Talaat <moatassem.talaat@orange.com>

---
*/

package sip

import (
	. "SRGo/global"
	"SRGo/guid"
	"SRGo/q850"
	"SRGo/sip/mode"
	"SRGo/sip/state"
	"SRGo/sip/status"
	"cmp"
	"fmt"
	"log"
	"net"
	"runtime"
	"sync"
	"time"
)

type SipSession struct {
	Direction Direction

	state     state.SessionState
	stateLock sync.RWMutex

	CallID           string
	FromHeader       string
	ToHeader         string
	FromTag          string
	ToTag            string
	RemoteContactURI string
	RecordRouteURI   string

	Forsaken     bool
	Force180Only bool

	Mymode mode.SessionMode

	RoutingData *RoutingData

	dscmutex         sync.RWMutex
	dialogueChanging bool

	IsPRACKSupported   bool
	IsDelayedOfferCall bool

	ReferSubscription bool
	Relayed18xNotify  []int

	RemoteUDP        *net.UDPAddr
	RemoteContactUDP *net.UDPAddr
	RecordRouteUDP   *net.UDPAddr
	UDPListenser     *net.UDPConn
	RemoteUserAgent  *SipUdpUserAgent

	FwdCSeq uint32
	BwdCSeq uint32
	RSeq    uint32

	RemoteBody        MessageBody //to save body if SDP is included
	SDPHashValue      string
	SDPSessionID      uint64
	SDPSessionVersion uint64

	LinkedSession *SipSession

	IsDisposed    bool
	multiUseMutex sync.Mutex // used for synchronizing no18x & noAns timers, probing & max duration, dropping session
	no18xSTimer   *SipTimer
	noAnsSTimer   *SipTimer

	maxDurationTimer *time.Timer  //used on inbound sessions only
	probingTicker    *time.Ticker //used on inbound sessions only
	maxDprobDoneChan chan bool    //to send kill signal to both maxDurationTimer & probingTicker

	Transactions []*Transaction
	TransLock    sync.RWMutex
}

func NewSS(dir Direction) *SipSession {
	ss := &SipSession{
		Direction:        dir,
		maxDprobDoneChan: make(chan bool),
	}
	return ss
}

func NewSIPSession(sipmsg *SipMessage) *SipSession { //used in inbound sessions
	ss := NewSS(INBOUND)
	ss.CallID = sipmsg.CallID
	return ss
}
func (session *SipSession) String() string {
	return fmt.Sprintf("Call-ID: %s, State: %s, Direction: %s, Mode: %s", session.CallID, session.state.String(), session.Direction.String(), session.Mymode)
}

//============================================================

func (session *SipSession) GetTransactionSYNC(SIPMsg *SipMessage) *Transaction {
	session.TransLock.RLock()
	defer session.TransLock.RUnlock()

	var CSeqRT Method
	CSeqNum := SIPMsg.CSeqNum
	if SIPMsg.IsRequest() {
		CSeqRT = SIPMsg.GetMethod()
		return Find(session.Transactions, func(x *Transaction) bool {
			return x.Direction == INBOUND && x.CSeq == CSeqNum &&
				((x.Method == CSeqRT && x.ViaBranch == SIPMsg.ViaBranch) ||
					(CSeqRT == ACK && x.Method.RequiresACK() && x.IsACKed && session.FromTag == SIPMsg.FromTag &&
						(session.ToTag == "" || session.ToTag == SIPMsg.ToTag)))
		})
	} else {
		CSeqRT = SIPMsg.CSeqMethod
		return Find(session.Transactions, func(x *Transaction) bool {
			return x.Direction == OUTBOUND && x.ViaBranch == SIPMsg.ViaBranch && x.CSeq == CSeqNum &&
				(x.Method == CSeqRT || (CSeqRT == INVITE && x.Method == ReINVITE))
		})
	}
}

func (session *SipSession) IsDuplicateMessage(msg *SipMessage) bool {
	sc := msg.StartLine.StatusCode
	if sc == 0 {
		tx := session.GetTransactionSYNC(msg)
		return tx != nil
	}
	if sc <= 199 {
		return false
	}
	trans := session.GetTransactionSYNC(msg)
	if trans == nil {
		return true
	}
	if trans.StatusCodeExistsSYNC(sc) {
		if trans.Method.RequiresACK() && trans.ACKTransaction != nil {
			session.SendSTMessage(trans.ACKTransaction)
		}
		return true
	}
	return false
}

func (session *SipSession) AddIncomingRequest(requestMsg *SipMessage, lt *Transaction) *Transaction {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()

	rType := requestMsg.StartLine.Method

	// Stop retransmitting any pending outgoing requests after receiving BYE
	if rType == BYE {
		for _, pendingST := range session.GetPendingOutgoingTransactions() {
			pendingST.StopTransTimer(true)
		}
		for _, pendingST := range session.GetPendingIncomingTransactions() {
			if pendingST.Method == INVITE && pendingST.IsFinalized && !pendingST.IsACKed {
				pendingST.StopTransTimer(true)
				CheckPendingTransaction(session, pendingST)
			}
		}
	}

	switch rType {
	case ACK:
		reInviteST := session.GetReOrInviteTransaction(requestMsg.CSeqNum, true)
		if reInviteST == nil {
			return nil
		}
		if reInviteST.RequireSameViaBranch() == (reInviteST.ViaBranch == requestMsg.ViaBranch) {
			reInviteST.IsACKed = true
			reInviteST.StopTransTimer(true)
			return reInviteST
		}
		log.Printf("Received ACK with improper Via-Branch for %v – Call-ID [%s]", reInviteST.RequestMessage.StartLine.Method.String(), requestMsg.CallID)
		return nil
	case CANCEL:
		inviteST := session.GetReOrInviteTransaction(requestMsg.CSeqNum, false)
		if inviteST == nil {
			st := NewSIPTransaction_RT(requestMsg, lt, session)
			session.AddTransaction(st)
			return st
		}
		if inviteST.ViaBranch == requestMsg.ViaBranch {
			st := NewSIPTransaction_RT(requestMsg, inviteST, session)
			session.AddTransaction(st)
			return st
		}
		log.Printf("Received CANCEL with improper Via-Branch for INVITE – Call-ID [%s]", requestMsg.CallID)
		return nil
	case PRACK:
		var prackST *Transaction
		if rSeq, cSeq, ok := requestMsg.GetRSeqFromRAck(); ok {
			prackST = session.GetPRACKTransaction(rSeq, cSeq)
			if prackST == nil {
				prackST = NewSIPTransaction_RP(0, PRACKUnexpected)
				LogError(LTSIPStack, fmt.Sprintf("Cannot find unPRACKed 1xx response for the incoming PRACK – Call-ID [%s]", requestMsg.CallID))
			}
		} else {
			prackST = NewSIPTransaction_RP(0, PRACKMissingBadRAck)
			LogError(LTSIPStack, fmt.Sprintf("Cannot parse RAck header or it is missing for the incoming PRACK – Call-ID [%s]", requestMsg.CallID))
		}
		prackST.RequestMessage = requestMsg
		prackST.CSeq = requestMsg.CSeqNum
		prackST.ViaBranch = requestMsg.ViaBranch
		return prackST
	default:
		if rType == INVITE && session.IsDuplicateINVITE(requestMsg) {
			return nil
		}
		st := NewSIPTransaction_RT(requestMsg, lt, session)
		// lastST := session.LastTransaction()
		// if lastST != nil && SIPConfig.BlockFastTransactions {
		// 	st.IsFastTrans = time.Since(lastST.TimeStamp).Milliseconds() <= SIPConfig.FastTransDeltaTime
		// }
		if rType.IsDialogueCreating() && session.Direction == INBOUND {
			session.FromHeader = requestMsg.FromHeader
			session.ToHeader = requestMsg.ToHeader
			session.FromTag = requestMsg.FromTag
			st.From = session.FromHeader
			st.To = requestMsg.ToHeader
			st.RequestMessage = requestMsg
		}
		session.AddTransaction(st)
		return st
	}
}

func (session *SipSession) AddIncomingResponse(responseMsg *SipMessage) *Transaction {
	st := session.GetTransactionSYNC(responseMsg)
	if st != nil {
		st.Lock.Lock()
		rc := responseMsg.StartLine.StatusCode
		st.StopTransTimer(false)
		st.Responses = append(st.Responses, rc)
		st.IsFinalized = cmp.Or(st.IsFinalized, rc >= 200)
		if st.IsFinalized {
			if st.Method == CANCEL {
				st.LinkedTransaction.StartCancelTimer(session)
			} else if st.Method == INVITE {
				st.StopCancelTimer()
			}
		}
		st.Lock.Unlock()

		// Handle ToTag assignment if the session direction is outbound
		if responseMsg.ToTag != "" && session.Direction == OUTBOUND {
			session.ToTag = responseMsg.ToTag
			session.ToHeader = responseMsg.Headers.ValueHeader(To)
			st.To = session.ToHeader
		}
	}
	return st
}

func (session *SipSession) GenerateOutgoingPRACKST(responseMsg *SipMessage) *Transaction {
	// Parse RSeq from the headers and handle the error
	rSeq := Str2Uint[uint32](responseMsg.Headers.ValueHeader(RSeq))
	cseqHeaderValue := responseMsg.Headers.ValueHeader(CSeq)
	newST := NewSIPTransaction_RC(rSeq, cseqHeaderValue)

	session.TransLock.Lock()
	session.Transactions = append(session.Transactions, newST)
	session.TransLock.Unlock()

	return newST
}

func (session *SipSession) AddTransaction(tx *Transaction) {
	// Add the transaction to the session
	session.Transactions = append(session.Transactions, tx)
}

func (session *SipSession) GetReOrInviteTransaction(cSeqNum uint32, isFinalized bool) *Transaction {
	return Find(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == INBOUND &&
			tx.CSeq == cSeqNum &&
			tx.Method.RequiresACK() &&
			tx.IsFinalized == isFinalized
	})
}

func (session *SipSession) GetPendingOutgoingTransactions() []*Transaction {
	return Filter(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == OUTBOUND && !tx.IsFinalized
	})
}

func (session *SipSession) GetPendingIncomingTransactions() []*Transaction {
	return Filter(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == INBOUND // && !tx.IsFinalized
	})
}

func (session *SipSession) GetPRACKTransaction(rSeqNum, cSeqNum uint32) *Transaction {
	// Retrieve the re-INVITE transaction that matches the CSeq number
	reInvite := session.GetReOrInviteTransaction(cSeqNum, false)
	if reInvite != nil {
		reInvite.StopTransTimer(true)
	}
	return Find(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == INBOUND && tx.RSeq == rSeqNum && tx.Method == PRACK
	})
}

func (session *SipSession) AreTherePendingOutgoingPRACK() bool {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()
	return Any(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == OUTBOUND && tx.Method == PRACK && !tx.IsFinalized
	})
}

func (session *SipSession) IsDuplicateINVITE(incINVITE *SipMessage) bool {
	trans := Find(session.Transactions, func(tx *Transaction) bool {
		return tx.Direction == INBOUND && tx.Method == INVITE &&
			tx.RequestMessage.FromTag == incINVITE.FromTag &&
			tx.ViaBranch == incINVITE.ViaBranch && tx.CSeq == incINVITE.CSeqNum
	})
	return trans != nil
}

func (session *SipSession) UnPRACKed18xCountSYNC() int {
	session.TransLock.RLock()
	defer session.TransLock.RUnlock()
	lst := Filter(session.Transactions, func(x *Transaction) bool {
		return x.Direction == INBOUND && x.Method == PRACK && x.RequestMessage == nil
	})
	return len(lst)
}

func (session *SipSession) CreateLinkedINVITE(userpart string, body MessageBody) (*Transaction, *SipMessage) {
	trans := session.AddOutgoingRequest(INVITE, nil)
	sipmsg := NewRequestMessage(INVITE, userpart)
	sipmsg.Headers = NewSHsPointer(true)
	session.ProxifyRequestHeaders(sipmsg, trans)
	session.ProcessRequestHeaders(trans, sipmsg, RequestPack{Method: INVITE}, body)
	return trans, sipmsg
}

func (session *SipSession) SendRequest(m Method, trans *Transaction, body MessageBody) {
	session.SendRequestDetailed(RequestPack{Method: m}, trans, body)
}

func (session *SipSession) SendRequestDetailed(rqstpk RequestPack, trans *Transaction, body MessageBody) {
	newtrans := session.AddOutgoingRequest(rqstpk.Method, trans)
	sipmsg := NewRequestMessage(rqstpk.Method, "")
	session.PrepareRequestHeaders(newtrans, rqstpk, sipmsg)
	session.ProcessRequestHeaders(newtrans, sipmsg, rqstpk, body)
	sipmsg.Body = &body

	newtrans.IsProbing = rqstpk.IsProbing //set by probing SIP OPTIONS
	newtrans.RequestMessage = sipmsg
	newtrans.SentMessage = sipmsg

	session.SendSTMessage(newtrans)
}

func (session *SipSession) CreateSARequest(rqstpk RequestPack, body MessageBody) *Transaction {
	switch rqstpk.Method {
	case OPTIONS:
		session.FwdCSeq = 911
		session.Mymode = mode.KeepAlive
	case INVITE:
		session.Mymode = mode.Multimedia
		fallthrough
	default: // Any other
		session.FwdCSeq = uint32(RandomNum(1, 500))
	}
	st := NewSIPTransaction_CRL(session.FwdCSeq, rqstpk.Method, nil)
	session.PrepareSARequestHeaders(st, rqstpk, body)

	session.TransLock.Lock()
	session.AddTransaction(st)
	session.TransLock.Unlock()
	return st
}

func (session *SipSession) PrepareSARequestHeaders(st *Transaction, rqstpk RequestPack, msgbody MessageBody) {
	st.RequestMessage = NewRequestMessage(rqstpk.Method, rqstpk.RUriUP)
	session.BuildSARequestHeaders(st, rqstpk, st.RequestMessage)
	st.RequestMessage.Body = &msgbody
	st.SentMessage = st.RequestMessage
}

func (session *SipSession) BuildSARequestHeaders(st *Transaction, rqstpk RequestPack, sipmsg *SipMessage) {
	localsocket := GenerateUDPSocket(session.UDPListenser)
	localIP := localsocket.IP
	remoteIP := session.RemoteUDP.IP

	// Set Start line
	sl := sipmsg.StartLine
	sl.HostPart = session.RemoteUDP.String()
	sl.UserPart = rqstpk.RUriUP
	switch rqstpk.Method {
	case INVITE:
		sl.UriParameters = &map[string]string{"user": "phone"}
	}
	sl.Ruri = fmt.Sprintf("%v:%v%v@%v%v", sl.UriScheme, sl.UserPart, GenerateParameters(sl.UserParameters), sl.HostPart, GenerateParameters(sl.UriParameters))

	hdrs := NewSHsPointer(true)

	// Set Call-ID
	session.CallID = guid.NewCallID()
	hdrs.AddHeader(Call_ID, session.CallID)

	// Set Via and Branch
	hdrs.AddHeader(Via, fmt.Sprintf("%s;branch=%s", GenerateViaWithoutBranch(session.UDPListenser), st.ViaBranch))

	// Set From Header with tag
	session.FromTag = guid.NewTag()
	session.FromHeader = fmt.Sprintf("<sip:%s@%s;user=phone>;tag=%s", rqstpk.FromUP, localIP, session.FromTag)
	st.From = session.FromHeader
	hdrs.AddHeader(From, session.FromHeader)

	// Add custom headers if any
	if hmap := rqstpk.CustomHeaders.InternalMap(); hmap != nil {
		for k, vs := range hmap {
			for _, v := range vs {
				hdrs.Add(k, v)
			}
		}
	}

	// Set To
	session.ToHeader = fmt.Sprintf("<sip:%s@%s;user=phone>", rqstpk.RUriUP, remoteIP)
	st.To = session.ToHeader
	hdrs.SetHeader(To, session.ToHeader)

	// Set CSeq
	st.CSeq = session.FwdCSeq
	hdrs.AddHeader(CSeq, fmt.Sprintf("%d %s", session.FwdCSeq, rqstpk.Method.String()))

	// Set Date
	hdrs.AddHeader(Date, time.Now().UTC().Format(DicTFs[Signaling]))

	sipmsg.Headers = hdrs
}

func (session *SipSession) SendResponse(trans *Transaction, sc int, msgbody MessageBody) {
	session.SendResponseDetailed(trans, ResponsePack{StatusCode: sc}, msgbody)
}

func (session *SipSession) SendResponseDetailed(trans *Transaction, rspspk ResponsePack, msgbody MessageBody) {
	if trans == nil {
		trans = session.GetLastUnACKedINVSYNC(INBOUND)
	}
	stc := rspspk.StatusCode
	trans.Lock.Lock()
	trans.Responses = append(trans.Responses, stc)
	trans.IsFinalized = cmp.Or(trans.IsFinalized, stc >= 200)
	trans.Lock.Unlock()

	sipmsg := NewResponseMessage(stc, rspspk.ReasonPhrase)
	sipmsg.Headers = session.CreateHeadersForResponse(trans, rspspk)
	sipmsg.Body = &msgbody
	trans.SentMessage = sipmsg
	session.SendSTMessage(trans)
}

func (session *SipSession) CreateHeadersForResponse(trans *Transaction, rspnspk ResponsePack) *SipHeaders {
	hdrs := NewSHsPointer(true)
	sc := rspnspk.StatusCode
	sipmsg := trans.RequestMessage

	// Add Contact header
	if rspnspk.ContactHeader == "" {
		localsocket := GenerateUDPSocket(session.UDPListenser)
		hdrs.AddHeader(Contact, GenerateContact(localsocket))
	} else {
		hdrs.AddHeader(Contact, rspnspk.ContactHeader)
	}

	// Add Expires header (for registration responses)
	if trans.Method == REGISTER {
		if sipmsg.Headers.ValueHeader(Expires) != "" {
			hdrs.AddHeader(Expires, sipmsg.Headers.ValueHeader(Expires))
		}
	}

	// Add Call-ID header
	hdrs.AddHeader(Call_ID, session.CallID)

	// Add custom headers if present
	if hmap := rspnspk.CustomHeaders.InternalMap(); hmap != nil {
		for k, vs := range hmap {
			for _, v := range vs {
				hdrs.Add(k, v)
			}
		}
	}

	// Add mandatory headers
	hdrs.AddHeader(Via, sipmsg.Headers.ValueHeader(Via))
	hdrs.AddHeader(From, sipmsg.Headers.ValueHeader(From))
	hdrs.AddHeader(To, sipmsg.Headers.ValueHeader(To))
	hdrs.AddHeader(CSeq, sipmsg.Headers.ValueHeader(CSeq))
	hdrs.AddHeader(Record_Route, sipmsg.Headers.ValueHeader(Record_Route))
	hdrs.AddHeader(Date, time.Now().UTC().Format(DicTFs[Signaling]))
	hdrs.AddHeader(Refer_Sub, sipmsg.Headers.ValueHeader(Refer_Sub))

	// Handle Reason header if session is linked and response code >= 400
	// if !rspnspk.IsCancelled && session.LinkedSession != nil && sc >= 400 && !hdrs.HeaderExists("Reason") {
	// 	reason := session.LinkedSession.GetLastMessageHeaderValueSYNC("Reason")
	// 	if reason == "" {
	// 		reason = "Q.850;cause=16"
	// 	}
	// 	hdrs.Add("Reason", reason)
	// }

	// Add tags and PRACK headers for responses > 100
	if sc > 100 {
		if !hdrs.ContainsToTag() && Is18xOrPositive(sc) && session.Direction == INBOUND {
			if session.ToTag == "" {
				session.ToTag = guid.NewTag()
			}
			session.ToHeader = fmt.Sprintf("%s;tag=%s", hdrs.ValueHeader(To), session.ToTag)
			hdrs.SetHeader(To, session.ToHeader)
			trans.To = session.ToHeader
		}
		hdrs.AddHeader(Record_Route, sipmsg.Headers.ValueHeader(Record_Route))

		// remoteses := session.LinkedSession
		// prackRequested := remoteses != nil && remoteses.AreTherePendingOutgoingPRACK()
		prackRequested := rspnspk.PRACKRequested || rspnspk.LinkedPRACKST != nil

		// Add PRACK support for provisional responses if applicable
		if IsProvisional18x(sc) && session.IsPRACKSupported && session.Direction == INBOUND && prackRequested {
			hdrs.SetHeader(RSeq, session.GenerateRSeqCreatePRACKSTSYNC(rspnspk.LinkedPRACKST))
			hdrs.SetHeader(Require, "100rel")
		}
	}

	// Ensure any options in "Require" header are copied to "Supported"
	// if requireOptions, ok := hdrs.TryGetField("Require"); ok {
	// 	hvalues := strings.Split(requireOptions, ",;")
	// 	for _, hv := range hvalues {
	// 		hdrs.AddOrMergeField("Supported", strings.ToLower(strings.TrimSpace(hv)))
	// 	}
	// }

	return hdrs
}

func (session *SipSession) GenerateRSeqCreatePRACKSTSYNC(linkedPRACKST *Transaction) string {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()
	if session.RSeq == 0 {
		session.RSeq = uint32(RandomNum(1, 999))
	} else {
		session.RSeq++
	}
	pst := NewSIPTransaction_RP(session.RSeq, PRACKExpected)
	if linkedPRACKST != nil {
		pst.LinkedTransaction = linkedPRACKST
		linkedPRACKST.LinkedTransaction = pst
	}
	session.AddTransaction(pst)
	return fmt.Sprintf("%v", session.RSeq)
}

func (session *SipSession) GetLastMessageHeaderValueSYNC(headerName string) string {
	session.TransLock.RLock()
	defer session.TransLock.RUnlock()

	for i := len(session.Transactions) - 1; i >= 0; i-- {
		trans := (session.Transactions)[i]
		if trans.RequestMessage == nil {
			continue
		}
		if (trans.Method == CANCEL || trans.Method == BYE) && trans.RequestMessage != nil && trans.RequestMessage.Headers.HeaderExists(headerName) {
			return trans.RequestMessage.Headers.Value(headerName)
		}
	}

	// for _, t := range session.Transactions {
	// 	if t.Method != INVITE {
	// 		continue
	// 	}
	// 	for j := len(t.ResponseMsgs) - 1; j >= 0; j-- {
	// 		msg := t.ResponseMsgs[j]
	// 		if msg.StartLine.ResponseCode >= 400 && msg.Headers.FieldExist(sh) {
	// 			return msg.Headers.Field(sh)
	// 		}
	// 	}
	// }
	return ""
}

func (session *SipSession) AddOutgoingRequest(rt Method, lt *Transaction) *Transaction {
	// Reject any pending incoming requests before sending BYE
	if rt == BYE {
		for _, pendingST := range session.GetPendingIncomingTransactionsSYNC() {
			session.SendResponseDetailed(pendingST, ResponsePack{StatusCode: 503, CustomHeaders: NewSHQ850OrSIP(31, "Session being cleared", "")}, EmptyBody())
		}
	}

	session.TransLock.Lock()
	defer session.TransLock.Unlock()

	var st *Transaction

	if session.Direction == OUTBOUND {
		switch rt {
		case ACK:
			if lt == nil {
				lt = session.GetUnACKedINVorReINV()
			}
			lt.IsACKed = true
			st = lt.CreateACKST()
		case CANCEL:
			if lt == nil {
				lt = session.GetLastUnACKedINV(OUTBOUND)
			}
			st = lt.CreateCANCELST()
			session.AddTransaction(st)
		default:
			// Increment forward CSeq
			if session.FwdCSeq == 0 {
				session.FwdCSeq = uint32(RandomNum(0, 500))
			} else {
				session.FwdCSeq += 1
			}
			if rt == PRACK {
				st = lt // LT is already created using GenerateOutgoingPRACKST
				st.CSeq = session.FwdCSeq
			} else {
				st = NewSIPTransaction_CRL(session.FwdCSeq, rt, lt)
				session.AddTransaction(st)
			}
		}
	} else {
		if rt == ACK {
			if lt == nil {
				lt = session.GetUnACKedINVorReINV()
			}
			lt.IsACKed = true
			st = lt.CreateACKST()
		} else {
			// Increment backward CSeq
			if session.BwdCSeq == 0 {
				session.BwdCSeq = uint32(RandomNum(600, 1000))
			} else {
				session.BwdCSeq += 1
			}
			st = NewSIPTransaction_CRL(session.BwdCSeq, rt, lt)
			session.AddTransaction(st)
		}
	}
	return st
}

func (session *SipSession) GetUnACKedINVorReINVSYNC(rqstCSeq uint32) *Transaction {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()
	return session.GetUnACKedINVorReINV()
}

func (session *SipSession) GetUnACKedINVorReINV() *Transaction {
	// Find the first outgoing transaction that requires an ACK and is not ACKed
	for _, tx := range session.Transactions {
		if tx.Direction == OUTBOUND &&
			tx.Method.RequiresACK() &&
			!tx.IsACKed {
			return tx
		}
	}
	return nil
}

func (session *SipSession) GetPendingIncomingTransactionsSYNC() []*Transaction {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()
	var pendingTransactions []*Transaction

	// Find all incoming transactions that are not finalized
	for _, tx := range session.Transactions {
		if tx.Direction == INBOUND && !tx.IsFinalized {
			pendingTransactions = append(pendingTransactions, tx)
		}
	}

	return pendingTransactions
}

func (session *SipSession) GetLastUnACKedINV(dir Direction) *Transaction {
	// Find the last outgoing INVITE transaction that is not ACKed
	for i := len(session.Transactions) - 1; i >= 0; i-- {
		tx := (session.Transactions)[i]
		if tx.Direction == dir && tx.Method == INVITE && !tx.IsACKed {
			return tx
		}
	}
	return nil
}

func (session *SipSession) GetLastUnACKedINVSYNC(dir Direction) *Transaction {
	session.TransLock.Lock()
	defer session.TransLock.Unlock()
	return session.GetLastUnACKedINV(dir)
}

func (session *SipSession) ProxifyRequestHeaders(sipmsg *SipMessage, trans *Transaction) {
	lnkdsipmsg := session.LinkedSession.CurrentRequestMessage()
	sipHdrs := sipmsg.Headers

	localsocket := GenerateUDPSocket(session.UDPListenser)
	localIP := localsocket.IP
	remoteIP := session.RemoteUDP.IP

	sl := sipmsg.StartLine
	sl.HostPart = session.RemoteUDP.String()
	sl.UserParameters = &map[string]string{}
	sl.UriParameters = &map[string]string{"user": "phone"}
	sl.Ruri = fmt.Sprintf("%v:%v%v@%v%v", sl.UriScheme, sl.UserPart, GenerateParameters(sl.UserParameters), sl.HostPart, GenerateParameters(sl.UriParameters))

	var nm, nmbr string
	var mtch []string

	// From Header
	var frmHeader string
	if lnkdsipmsg.Headers.DoesValueExistInHeader("Privacy", "user") {
		frmHeader = `"Anonymous" <sip:anonymous@anonymous.invalid>`
	} else {
		if RMatch(lnkdsipmsg.Headers.ValueHeader(From), NameAndNumber, &mtch) {
			nm = DropVisualSeparators(TrimWithSuffix(mtch[1], " "))
			nmbr = DropVisualSeparators(mtch[2])
			frmHeader = fmt.Sprintf("%s<sip:%s@%s;user=phone>", nm, nmbr, localIP)
		} else {
			frmHeader = fmt.Sprintf("<sip:Invalid@%s;user=phone>", localIP)
		}
	}

	session.FromTag = guid.NewTag()
	trans.From = fmt.Sprintf("%s;tag=%s", frmHeader, session.FromTag)
	sipHdrs.SetHeader(From, trans.From)
	session.FromHeader = trans.From

	// To Header
	if RMatch(lnkdsipmsg.Headers.ValueHeader(To), NameAndNumber, &mtch) {
		nm = DropVisualSeparators(TrimWithSuffix(mtch[1], " "))
		nmbr = DropVisualSeparators(mtch[2])
		trans.To = fmt.Sprintf("%s<sip:%s@%s;user=phone>", nm, nmbr, remoteIP)
	}
	sipHdrs.SetHeader(To, trans.To)
	session.ToHeader = trans.To

	// Call-ID Header
	session.CallID = guid.NewCallID()
	sipHdrs.SetHeader(Call_ID, session.CallID)

	// Via Header
	sipHdrs.AddHeader(Via, fmt.Sprintf("%s;branch=%s", GenerateViaWithoutBranch(session.UDPListenser), trans.ViaBranch))

	// Contact Header
	sipHdrs.SetHeader(Contact, GenerateContact(localsocket))

	// Content-Disposition
	sipHdrs.AddHeader(Content_Disposition, lnkdsipmsg.Headers.ValueHeader(Content_Disposition))

	// Forward Diversion header as per RFC 5806
	sipHdrs.AddHeader(Diversion, lnkdsipmsg.Headers.ValueHeader(Diversion))

	// Privacy Header
	sipHdrs.AddHeader(Privacy, lnkdsipmsg.Headers.ValueHeader(Privacy))

	// History-Info Header
	sipHdrs.AddHeader(History_Info, lnkdsipmsg.Headers.ValueHeader(History_Info))

	// P-Headers
	pHeaders := lnkdsipmsg.Headers.ValuesWithHeaderPrefix("P-")
	for k, vs := range pHeaders {
		for _, v := range vs {
			sipHdrs.Add(k, v)
		}
	}

	// Set INVITE message extra headers
	// for _, header := range SIPConfig.ExtraINVITEHeaders {
	// 	sipHdrs.Add(header, sipMsg.Headers.Value(header))
	// }

	// P-Asserted-Identity Header
	if RMatch(lnkdsipmsg.Headers.ValueHeader(P_Asserted_Identity), NameAndNumber, &mtch) {
		nm = DropVisualSeparators(TrimWithSuffix(mtch[1], " "))
		nmbr = DropVisualSeparators(mtch[2])
		sipHdrs.AddHeader(P_Asserted_Identity, fmt.Sprintf("%s<sip:%s@%s;transport=udp>", nm, nmbr, localsocket))
	}

	// Max-Forwards Header
	var maxForwards int
	if trans.ResetMF {
		maxForwards = 70
		trans.ResetMF = false
	} else {
		maxForwards = lnkdsipmsg.MaxFwds - 1
	}
	sipmsg.MaxFwds = maxForwards
	sipHdrs.SetHeader(Max_Forwards, fmt.Sprintf("%d", maxForwards))
}

func (session *SipSession) PrepareRequestHeaders(trans *Transaction, rqstpk RequestPack, sipmsg *SipMessage) {
	hdrs := NewSHsPointer(true)
	sipmsg.Headers = hdrs

	localsocket := GenerateUDPSocket(session.UDPListenser)

	sl := sipmsg.StartLine
	sl.Ruri = session.RemoteContactURI

	// Set To and From headers depending on session direction
	if session.Direction == OUTBOUND {
		hdrs.SetHeader(To, session.ToHeader)
		hdrs.SetHeader(From, session.FromHeader)
	} else {
		hdrs.SetHeader(To, session.FromHeader)
		hdrs.SetHeader(From, session.ToHeader)
	}

	// Add RAck header if the request type is PRACK
	if rqstpk.Method == PRACK {
		hdrs.SetHeader(RAck, trans.RAck)
	}

	// Max-Forwards
	var maxFwds int
	if rqstpk.Max70 || session.LinkedSession == nil || trans.ResetMF {
		maxFwds = 70
	} else {
		if trans.LinkedTransaction == nil {
			maxFwds = 70
		} else {
			if rqstpk.Method == ACK || rqstpk.Method == CANCEL {
				maxFwds = trans.LinkedTransaction.RequestMessage.MaxFwds
			} else {
				maxFwds = trans.LinkedTransaction.RequestMessage.MaxFwds - 1
			}
		}
	}
	hdrs.SetHeader(Max_Forwards, fmt.Sprintf("%d", maxFwds))

	if rqstpk.Method == ReINVITE {
		sipmsg.MaxFwds = maxFwds
	}

	// Add Contact, Call-ID, and Via headers
	hdrs.SetHeader(Contact, GenerateContact(localsocket))
	hdrs.SetHeader(Call_ID, session.CallID)
	hdrs.AddHeader(Via, fmt.Sprintf("%s;branch=%s", GenerateViaWithoutBranch(session.UDPListenser), trans.ViaBranch))
}

func (session *SipSession) ProcessRequestHeaders(trans *Transaction, sipmsg *SipMessage, rqstpk RequestPack, msgBody MessageBody) {
	hdrs := sipmsg.Headers

	// Add Date header
	hdrs.SetHeader(Date, time.Now().UTC().Format(DicTFs[Signaling]))

	// CSeq header
	hdrs.SetHeader(CSeq, fmt.Sprintf("%d %s", trans.CSeq, sipmsg.StartLine.Method.String()))

	// Add custom headers if any
	if hmap := rqstpk.CustomHeaders.InternalMap(); hmap != nil {
		for k, vs := range hmap {
			for _, v := range vs {
				hdrs.Add(k, v)
			}
		}
	}

	// Add Reason header for CANCEL or BYE requests
	if sipmsg.StartLine.Method == CANCEL || sipmsg.StartLine.Method == BYE {
		if !hdrs.HeaderExists("Reason") {
			if session.LinkedSession == nil || trans.LinkedTransaction == nil {
				hdrs.AddHeader(Reason, "Q.850;cause=16")
			} else if reason := trans.LinkedTransaction.RequestMessage.Headers.ValueHeader(Reason); reason != "" {
				hdrs.AddHeader(Reason, reason)
			} else {
				hdrs.AddHeader(Reason, "Q.850;cause=16")
			}
		}
	}

	// Set Content-Type and Content-Length based on the message body
	sipmsg.Body = &msgBody

	// INVITE specific headers
	if sipmsg.StartLine.Method == INVITE {
		trans.RequestMessage = sipmsg
		trans.SentMessage = sipmsg
		session.RemoteContactURI = sipmsg.StartLine.Ruri
		if session.IsPRACKSupported {
			hdrs.AddHeader(Supported, "100rel")
		}
	}

	// NOTIFY specific headers
	// if rqstpk.RequestType == NOTIFY {
	// 	if msgBody.MyBodyType == SIPFragment {
	// 		hdrs.Add("Event", session.Transactions.GenerateReferHeaderForNotifyFromLastREFERCSeqSYNC())
	// 		if msgBody.NotifyResponse < 200 {
	// 			hdrs.Add("Subscription-State", "pending")
	// 		} else {
	// 			hdrs.Add("Subscription-State", "terminated;reason=noresource")
	// 		}
	// 	} else if msgBody.BodyType == SimpleMsgSummary {
	// 		hdrs.Add("Event", session.InitialRequestMessage.Headers.ValueHeader(Event))
	// 		if msgBody.SubscriptionStatusReason == SubsStateReasonNone {
	// 			hdrs.Add("Subscription-State", "active")
	// 			hdrs.Add("Subscription-Expires", fmt.Sprintf("%d", session.MyVoiceMailBox.SubscriptionExpires.Format(DicTFs[TimeFormatSignaling])))
	// 		} else {
	// 			hdrs.Add("Subscription-State", fmt.Sprintf("terminated;reason=%s", msgBody.SubscriptionStatusReason))
	// 		}
	// 		hdrs.Add("Contact", fmt.Sprintf("<%s>", session.MyVoiceMailBox.URI))
	// 	} else if msgBody.BodyType == DTMFRelay {
	// 		hdrs.Add("Event", "telephone-event")
	// 	}
	// }

	// PRACK specific headers
	if sipmsg.StartLine.Method == PRACK && !session.IsPRACKSupported {
		LogWarning(LTSIPStack, fmt.Sprintf("UAS requesting 100rel although not offered - Call ID [%s]", session.CallID))
		hdrs.AddHeader(Warning, `399 Newkah "100rel was not offered, yet it was requested"`)
	}

	// ReINVITE specific headers
	// if sipmsg.StartLine.Method == ReINVITE {
	// 	hdrs.Add("P-Early-Media", "")
	// 	hdrs.Add("Subject", "")
	// 	sipmsg.StartLine.Method = INVITE
	// }

	// OPTIONS specific headers
	// if sipmsg.StartLine.Method == OPTIONS {
	// 	hdrs.Add("Accept", "")
	// }
}

func (session *SipSession) Received1xx() bool {
	trans := session.GetFirstTransaction()
	return trans != nil && trans.Any1xxSYNC()
}

func (session *SipSession) Received200() bool {
	trans := session.GetFirstTransaction()
	return trans != nil && trans.IsFinalResponsePositiveSYNC()
}

func (session *SipSession) StopAllOutTransactions() {
	session.TransLock.RLock()
	defer session.TransLock.RUnlock()
	for _, tx := range session.Transactions {
		tx.StopTransTimer(true)
	}
}

func (session *SipSession) GetFirstTransaction() *Transaction {
	session.TransLock.RLock()
	defer session.TransLock.RUnlock()
	Ts := session.Transactions
	return Ts[0]
}

func (session *SipSession) GetLastTransaction() *Transaction {
	if len(session.Transactions) == 0 {
		return nil
	}
	return (session.Transactions)[len(session.Transactions)-1]
}

func (session *SipSession) CurrentRequestMessage() *SipMessage {
	trans := session.GetFirstTransaction()
	if trans == nil {
		return nil
	}
	return trans.RequestMessage
}

func (session *SipSession) UpdateContactRecordRouteBody(sipmsg *SipMessage) {
	parseURI := func(hv string, uri string) (bool, string, *net.UDPAddr) {
		if hv == uri {
			return false, hv, nil
		}
		var mtch []string
		if !RMatch(hv, FQDNPort, &mtch) {
			return false, hv, nil
		}
		prt := Str2Int[int](mtch[2])
		prt = cmp.Or(prt, 5060)
		ip := net.ParseIP(mtch[1])
		if ip == nil {
			return false, hv, nil
		}
		return true, hv, &net.UDPAddr{IP: ip, Port: prt}
	}

	ok1, RCURI, RCUDP := parseURI(sipmsg.RCURI, session.RemoteContactURI)
	ok2, RRURI, RRUDP := parseURI(sipmsg.RRURI, session.RecordRouteURI)
	if ok1 {
		session.RemoteContactURI = RCURI
		session.RemoteContactUDP = RCUDP
	}
	if ok2 {
		session.RecordRouteURI = RRURI
		session.RecordRouteUDP = RRUDP
	}
	// if sipmsg.Body.ContainsSDP() {
	// 	session.RemoteBody = *sipmsg.Body
	// }
}

func (session *SipSession) SendSTMessage(st *Transaction) {
	st.Lock.Lock()
	defer st.Lock.Unlock()
	var createTimer bool
	if st.Direction == OUTBOUND {
		createTimer = st.Method != ACK
	} else {
		createTimer = (st.IsFinalized && st.Method.RequiresACK()) || session.UnPRACKed18xCountSYNC() > 0
	}
	session.Send(st)
	if createTimer {
		st.StartTransTimer(session)
	}
}

func (session *SipSession) Send(tx *Transaction) {
	if len(tx.SentMessage.Body.MessageBytes) == 0 {
		tx.SentMessage.PrepareMessageBytes(session)
	}
	if tx.SentMessage.IsRequest() && session.RemoteContactUDP != nil {
		_, err := session.UDPListenser.WriteToUDP(tx.SentMessage.Body.MessageBytes, session.RemoteContactUDP)
		if err != nil {
			LogError(LTSystem, "Failed to send message: "+err.Error())
		}
		return
	}
	_, err := session.UDPListenser.WriteToUDP(tx.SentMessage.Body.MessageBytes, session.RemoteUDP)
	if err != nil {
		LogError(LTSystem, "Failed to send message: "+err.Error())
	}
}

func CheckPendingTransaction(ss *SipSession, tx *Transaction) {
	// TODO: incomplete!!!
	switch tx.Method {
	case OPTIONS:
		if ss.Mymode == mode.KeepAlive {
			ss.SetState(state.TimedOut)
			ss.DropMe()
			return
		}
		if ss.Mymode == mode.Multimedia && ss.Direction == INBOUND && tx.Direction == OUTBOUND && tx.IsProbing { //means my in-dialogue probing OPTIONS
			ss.ReleaseCall("Probing timed-out")
		}
	case INVITE:
		if ss.IsPending() {
			ss.SetState(state.TimedOut)
			ss.DropMe()
		}
		if lnkdss := ss.LinkedSession; lnkdss != nil {
			if lnkdss.Direction == OUTBOUND && lnkdss.Received200() {
				lnkdss.SendRequest(ACK, nil, EmptyBody())
				lnkdss.WaitMS(100)
				lnkdss.SetState(state.BeingDropped)
				lnkdss.SendRequestDetailed(RequestPack{Method: BYE, CustomHeaders: NewSHQ850OrSIP(487, "Inbound INVITE timed-out", "")}, nil, EmptyBody())
				return
			}
			lnkdss.RerouteRequest(NewResponsePackSRW(408, "Outbound INVITE timed-out", ""))
		}
	case CANCEL, BYE:
		ss.FinalizeState()
		ss.DropMe()
	case PRACK:
		ss.StopNoTimers()
		lnkdss := ss.LinkedSession
		lnkdss.RerouteRequest(NewResponsePackSRW(408, "Outbound PRACK timed-out", ""))
		ss.SetState(state.Failed)
		ss.DropMe()
	default:
		lnkdss := ss.LinkedSession
		s1, s2 := ss.ReleaseCall(fmt.Sprintf("In-dialogue %s timed-out", tx.Method.String()))
		if !s1 {
			ss.DropMe()
		}
		if !s2 && lnkdss != nil {
			lnkdss.DropMe()
		}
	}
}

// ==================================================================
// for indialogue change

func (ss *SipSession) IsDialogueChanging() bool {
	ss.dscmutex.RLock()
	defer ss.dscmutex.RUnlock()
	return ss.dialogueChanging
}

func (ss *SipSession) ChecknSetDialogueChanging(newflag bool) bool {
	ss.dscmutex.Lock()
	defer ss.dscmutex.Unlock()
	if newflag != ss.dialogueChanging {
		ss.dialogueChanging = newflag
		return true
	}
	return false
}

// ==================================================================

// Unsafe
func (ss *SipSession) setTimerPointer(tt TimerType, tmr *SipTimer) {
	if tt == NoAnswer {
		ss.noAnsSTimer = tmr
	} else {
		ss.no18xSTimer = tmr
	}
}

// Unsafe
func (ss *SipSession) getTimerPointer(tt TimerType) *SipTimer {
	if tt == NoAnswer {
		return ss.noAnsSTimer
	} else {
		return ss.no18xSTimer
	}
}

func (ss *SipSession) StartTimer(tt TimerType) {
	ss.multiUseMutex.Lock()
	defer ss.multiUseMutex.Unlock()
	if (tt == NoAnswer && ss.noAnsSTimer != nil) || (tt == No18x && ss.no18xSTimer != nil) {
		return
	}
	var delay uint16
	if tt == NoAnswer {
		delay = ss.RoutingData.NoAnswerTimeout
	} else {
		delay = ss.RoutingData.No18xTimeout
	}
	tmr := &SipTimer{
		DoneCh: make(chan bool),
		Tmr:    time.NewTimer(time.Duration(delay) * time.Second),
	}
	ss.setTimerPointer(tt, tmr)
	go ss.TimerHandler(tt)
}

func (ss *SipSession) StopTimer(tt TimerType) {
	ss.multiUseMutex.Lock()
	defer ss.multiUseMutex.Unlock()
	siptmr := ss.getTimerPointer(tt)
	if siptmr == nil {
		return
	}
	if siptmr.Tmr.Stop() {
		close(siptmr.DoneCh)
	}
}

func (ss *SipSession) StopNoTimers() {
	ss.StopTimer(No18x)
	ss.StopTimer(NoAnswer)
}

func (ss *SipSession) TimerHandler(ttt TimerType) {
	tmr := ss.getTimerPointer(ttt)
	select {
	case <-tmr.DoneCh:
		ss.multiUseMutex.Lock()
		defer ss.multiUseMutex.Unlock()
		ss.setTimerPointer(ttt, nil)
		return
	case <-tmr.Tmr.C:
	}
	ss.multiUseMutex.Lock()
	close(tmr.DoneCh)
	ss.setTimerPointer(ttt, nil)
	ss.multiUseMutex.Unlock()
	lnkdss := ss.LinkedSession
	ss.CancelMe(q850.NoAnswerFromUser, ttt.Details())
	lnkdss.RerouteRequest(NewResponsePackSRW(487, ttt.Details(), ""))
}

// ------------------------------------------------------------------------------

func (ss *SipSession) StartInDialogueProbing() {
	if InDialogueProbingSec == 0 {
		LogWarning(LTConfiguration, "Probing duration is set to ZERO - Skipped")
		return
	}
	ss.multiUseMutex.Lock()
	defer ss.multiUseMutex.Unlock()
	ss.probingTicker = time.NewTicker(time.Duration(InDialogueProbingSec) * time.Second)
	go ss.probingTickerHandler(ss.maxDprobDoneChan, ss.probingTicker.C)
}

func (ss *SipSession) StartMaxCallDuration() {
	if ss.RoutingData == nil {
		LogWarning(LTSystem, "Max call duration not started - missing RoutingData")
		return
	}
	mxD := ss.RoutingData.MaxCallDuration
	if mxD == 0 {
		LogWarning(LTConfiguration, "Max call duration is set to ZERO - Skipped")
		return
	}
	ss.multiUseMutex.Lock()
	defer ss.multiUseMutex.Unlock()
	ss.maxDurationTimer = time.NewTimer(time.Duration(mxD) * time.Second)
	go ss.maxDurationTimerHandler(ss.maxDprobDoneChan, ss.maxDurationTimer.C)
}

func (ss *SipSession) probingTickerHandler(doneChan chan bool, tkChan <-chan time.Time) {
	select {
	case <-doneChan:
	case <-tkChan:
		if ss.IsEstablished() {
			ss.SendRequestDetailed(RequestPack{Method: OPTIONS, Max70: true, IsProbing: true}, nil, EmptyBody())
		}
	}
}

func (ss *SipSession) maxDurationTimerHandler(doneChan chan bool, tmrChan <-chan time.Time) {
	select {
	case <-doneChan:
	case <-tmrChan:
		ss.ReleaseCall("Max call duration reached")
	}
}

// ==============================================================================

func (session *SipSession) GetState() state.SessionState {
	session.stateLock.RLock()
	defer session.stateLock.RUnlock()
	return session.state
}

func (session *SipSession) SetState(ss state.SessionState) {
	session.stateLock.Lock()
	defer session.stateLock.Unlock()
	session.state = ss
}

func (session *SipSession) FinalizeState() {
	session.stateLock.Lock()
	defer session.stateLock.Unlock()
	session.state = session.state.FinalizeMe()
}

func (session *SipSession) IsFinalized() bool {
	session.stateLock.RLock()
	defer session.stateLock.RUnlock()
	return session.state.IsFinalized()
}

func (session *SipSession) IsEstablished() bool {
	session.stateLock.RLock()
	defer session.stateLock.RUnlock()
	return session.state == state.Established
}

func (session *SipSession) IsBeingEstablished() bool {
	session.stateLock.RLock()
	defer session.stateLock.RUnlock()
	return session.state == state.BeingEstablished
}

func (session *SipSession) IsPending() bool {
	session.stateLock.RLock()
	defer session.stateLock.RUnlock()
	return session.state.IsPending()
}

func (session *SipSession) IsLinked() bool {
	return session.LinkedSession != nil
}

// ==============================================================================

func (ss *SipSession) ReleaseCall(details string) (s1 bool, s2 bool) {
	if ss.IsEstablished() {
		ss.SetState(state.BeingDropped)
		ss.SendRequestDetailed(RequestPack{Method: BYE, Max70: true, CustomHeaders: NewSHQ850OrSIP(0, details, "")}, nil, EmptyBody())
		s1 = true
	}
	if lnkss := ss.LinkedSession; lnkss != nil && lnkss.IsEstablished() {
		lnkss.SetState(state.BeingDropped)
		lnkss.SendRequestDetailed(RequestPack{Method: BYE, Max70: true, CustomHeaders: NewSHQ850OrSIP(0, details, "")}, nil, EmptyBody())
		s2 = true
	}
	return
}

func (ss *SipSession) ReleaseMe(details string) bool {
	if ss.IsEstablished() {
		ss.SetState(state.BeingCleared)
		ss.SendRequestDetailed(RequestPack{Method: BYE, Max70: true, CustomHeaders: NewSHQ850OrSIP(0, details, "")}, nil, EmptyBody())
		return true
	}
	return false
}

func (ss *SipSession) ReleaseEarlyFinalCall(details string) (s1 bool, s2 bool) {
	if ss.IsBeingEstablished() {
		ss.SetState(state.BeingFailed)
		ss.SendResponseDetailed(nil, ResponsePack{StatusCode: status.NotAcceptable, CustomHeaders: NewSHQ850OrSIP(0, details, "")}, EmptyBody())
		s1 = true
	}
	if lnkss := ss.LinkedSession; lnkss != nil && lnkss.IsEstablished() {
		lnkss.SetState(state.BeingDropped)
		lnkss.SendRequestDetailed(RequestPack{Method: BYE, Max70: true, CustomHeaders: NewSHQ850OrSIP(0, details, "")}, nil, EmptyBody())
		s2 = true
	}
	return
}

// Cancel outgoing INVITE
func (session *SipSession) CancelMe(q850 int, details string) bool {
	if session.Direction != OUTBOUND {
		return false
	}
	if session.IsBeingEstablished() {
		session.StopNoTimers()
		session.SetState(state.BeingCancelled)
		if q850 == -1 || details == "" {
			session.SendRequest(CANCEL, nil, EmptyBody())
		} else {
			session.SendRequestDetailed(RequestPack{Method: CANCEL, CustomHeaders: NewSHQ850OrSIP(q850, details, "")}, nil, EmptyBody())
		}
		return true
	}
	return false
}

// Reject incoming INVITE
func (session *SipSession) RejectMe(trans *Transaction, stsCode int, q850 int, details string) bool {
	if session.Direction != INBOUND {
		return false
	}
	if session.IsBeingEstablished() {
		session.SetState(state.BeingFailed)
		session.SendResponseDetailed(trans, ResponsePack{StatusCode: stsCode, CustomHeaders: NewSHQ850OrSIP(q850, details, "")}, EmptyBody())
		return true
	}
	return false
}

// ACK redirected/rejected outgoing INVITE
func (session *SipSession) Ack3xxTo6xx(finalstate state.SessionState) {
	if session.Direction != OUTBOUND {
		return
	}
	session.SetState(finalstate)
	session.SendRequest(ACK, nil, EmptyBody())
	session.DropMeTimed()
}

func (session *SipSession) Ack3xxTo6xxFinalize() {
	if session.Direction != OUTBOUND {
		return
	}
	session.FinalizeState()
	session.SendRequest(ACK, nil, EmptyBody())
	session.DropMeTimed()
}

func (session *SipSession) AddMe() {
	Sessions.Store(session.CallID, session)
}

func (session *SipSession) DropMe() {
	session.multiUseMutex.Lock()
	defer session.multiUseMutex.Unlock()
	if session.IsDisposed {
		fmt.Print("Already Disposed Session: ", session.CallID, " State: ", session.state.String())
		pc, f, l, ok := runtime.Caller(1) // pc, _, _, ok := runtime.Caller(1)
		details := runtime.FuncForPC(pc)
		if ok && details != nil { // f, l := details.FileLine(pc)
			fmt.Printf(" >> [func (%s) - file (%s) - line (%d)]\n", details.Name(), f, l)
		}
		return
	}
	fmt.Println("Session:", session.CallID, "State:", session.state.String())
	session.IsDisposed = true
	close(session.maxDprobDoneChan)
	Sessions.Delete(session.CallID)
}

func (ss *SipSession) DropMeTimed() {
	go func() {
		<-time.After(time.Second * time.Duration(SessionDropDelaySec))
		// time.Sleep(time.Second * time.Duration(SessionDropDelaySec))
		ss.DropMe()
	}()
}

func (ss *SipSession) WaitMS(dur int) {
	<-time.After(time.Millisecond * time.Duration(dur))
}
