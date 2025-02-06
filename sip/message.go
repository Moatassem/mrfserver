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
	"SRGo/numtype"
	"bytes"
	"cmp"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
)

type SipMessage struct {
	MsgType   MessageType
	StartLine *SipStartLine
	Headers   *SipHeaders
	Body      *MessageBody

	//all fields below are only set in incoming messages
	FromHeader string
	ToHeader   string
	PAIHeaders []string
	DivHeaders []string

	CallID    string
	FromTag   string
	ToTag     string
	ViaBranch string

	RCURI string
	RRURI string

	MaxFwds       int
	CSeqNum       uint32
	CSeqMethod    Method
	ContentLength uint16 //only set for incoming messages
}

func NewRequestMessage(md Method, up string) *SipMessage {
	sipmsg := &SipMessage{
		MsgType: REQUEST,
		StartLine: &SipStartLine{
			Method:    md,
			UriScheme: "sip",
			UserPart:  up,
		},
	}
	return sipmsg
}

func NewResponseMessage(sc int, rp string) *SipMessage {
	sipmsg := &SipMessage{MsgType: RESPONSE, StartLine: new(SipStartLine)}
	if 100 <= sc && sc <= 699 {
		sipmsg.StartLine.StatusCode = sc
		dfltsc := Str2int[int](fmt.Sprintf("%d00", sc/100))
		sipmsg.StartLine.ReasonPhrase = cmp.Or(rp, DicResponse[sc], DicResponse[dfltsc])
	}
	return sipmsg
}

// ==========================================================================

func (sipmsg *SipMessage) getAddBodyParts() []int {
	hv := ASCIIToLower(sipmsg.Headers.ValueHeader(P_Add_BodyPart))
	hv = strings.ReplaceAll(hv, " ", "")
	partflags := strings.Split(hv, ",")
	var flags []int
	for k, v := range BodyAddParts {
		if slices.Contains(partflags, v) {
			flags = append(flags, k)
		}
	}
	return flags
}

func (sipmsg *SipMessage) AddRequestedBodyParts() {
	pflags := sipmsg.getAddBodyParts()
	if len(pflags) == 0 {
		return
	}
	// if sipmsg.Body
	msgbdy := sipmsg.Body
	hdrs := sipmsg.Headers
	if len(msgbdy.PartsContents) == 1 {
		frstbt := FirstKey(msgbdy.PartsContents)
		cntnthdrsmap := hdrs.ValuesWithHeaderPrefix("Content-", Content_Length.LowerCaseString())
		hdrs.DeleteHeadersWithPrefix("Content-")
		msgbdy.PartsContents[frstbt] = ContentPart{Headers: NewSHsFromMap(cntnthdrsmap), Bytes: msgbdy.PartsContents[frstbt].Bytes}
	}
	for _, pf := range pflags {
		switch pf {
		case AddXMLPIDFLO:
			bt := PIDFXML
			xml := `<?xml version="1.0" encoding="UTF-8"?><presence xmlns="urn:ietf:params:xml:ns:pidf" xmlns:gp="urn:ietf:params:xml:ns:pidf:geopriv10" xmlns:cl="urn:ietf:params:xml:ns:pidf:geopriv10:civicLoc" xmlns:btd="http://btd.orange-business.com" entity="pres:geotarget@btip.orange-business.com"><tuple id="sg89ae"><status><gp:geopriv><gp:location-info><cl:civicAddress><cl:country>FR</cl:country><cl:A2>35</cl:A2><cl:A3>CESSON SEVIGNE</cl:A3><cl:A6>DU CHENE GERMAIN</cl:A6><cl:HNO>9</cl:HNO><cl:STS>RUE</cl:STS><cl:PC>35510</cl:PC><cl:CITYCODE>99996</cl:CITYCODE></cl:civicAddress></gp:location-info><gp:usage-rules></gp:usage-rules></gp:geopriv></status></tuple></presence>`
			xmlbytes := []byte(xml)
			msgbdy.PartsContents[bt] = NewContentPart(bt, xmlbytes)
		case AddINDATA:
			bt := VndOrangeInData
			binbytes, _ := hex.DecodeString("77124700830e8307839069391718068a019288000d0a")
			sh := NewSipHeaders()
			sh.AddHeader(Content_Type, DicBodyContentType[bt])
			sh.AddHeader(Content_Transfer_Encoding, "binary")
			sh.AddHeader(Content_Disposition, "signal;handling=optional")
			msgbdy.PartsContents[bt] = ContentPart{Headers: sh, Bytes: binbytes}
		}
	}
}

// TODO need to check:
// if only one part left: to remove ContentPart object - see KeepOnlyPart
// if nothing left: to nullify parts i.e. sipmsg.Body.PartsBytes = nil
// func (sipmsg *SipMessage) dropBodyPart(bt BodyType) {
// 	delete(messagebody.PartsBytes, bt)
// }

func (sipmsg *SipMessage) KeepOnlyBodyPart(bt BodyType) bool {
	msgbdy := sipmsg.Body
	kys := Keys(msgbdy.PartsContents) //get all keys
	if len(kys) == 1 && kys[0] == bt {
		return true //to avoid removing Content-* headers while there is no Content headers inside the single body part
	}
	for _, ky := range kys {
		if ky == bt {
			continue
		}
		delete(msgbdy.PartsContents, ky) //remove other keys
	}
	if len(msgbdy.PartsContents) == 0 { //return if no remaining parts
		return false
	}
	cntprt := msgbdy.PartsContents[bt]
	smhdrs := sipmsg.Headers
	smhdrs.DeleteHeadersWithPrefix("Content-")            //remove all existing Content-* headers
	for _, hdr := range cntprt.Headers.GetHeaderNames() { //set Content-* headers from kept body part
		smhdrs.Set(hdr, cntprt.Headers.Value(hdr))
	}
	msgbdy.PartsContents[bt] = ContentPart{Bytes: cntprt.Bytes}
	return true
}

func (sipmsg *SipMessage) GetBodyPart(bt BodyType) ([]byte, bool) {
	cntnt, ok := sipmsg.Body.PartsContents[bt]
	return cntnt.Bytes, ok
}

// ===========================================================================

func (sipmsg *SipMessage) IsOutOfDialgoue() bool {
	return sipmsg.ToTag == ""
}

func (sipmsg *SipMessage) GetRSeqFromRAck() (rSeq, cSeq uint32, ok bool) {
	rAck := sipmsg.Headers.ValueHeader(RAck)
	if rAck == "" {
		LogError(LTSIPStack, "Empty RAck header")
		ok = false
		return
	}
	mtch := DicFieldRegEx[RAckHeader].FindStringSubmatch(rAck)
	if mtch == nil { // Ensure we have both RSeq and CSeq from the match
		LogError(LTSIPStack, "Malformed RAck header")
		ok = false
		return
	}
	rSeq = Str2uint[uint32](mtch[1])
	cSeq = Str2uint[uint32](mtch[2])
	ok = true
	return
}

func (sipmsg *SipMessage) IsOptionSupportedOrRequired(opt string) bool {
	hdr := sipmsg.Headers.ValueHeader(Require)
	if strings.Contains(hdr, opt) {
		return true
	}
	hdr = sipmsg.Headers.ValueHeader(Supported)
	return strings.Contains(hdr, opt)
}

func (sipmsg *SipMessage) IsOptionSupported(o string) bool {
	hdr := sipmsg.Headers.ValueHeader(Supported)
	hdr = ASCIIToLower(hdr)
	return hdr != "" && strings.Contains(hdr, o)
}

func (sipmsg *SipMessage) IsOptionRequired(o string) bool {
	hdr := sipmsg.Headers.ValueHeader(Require)
	hdr = ASCIIToLower(hdr)
	return hdr != "" && strings.Contains(hdr, o)
}

func (sipmsg *SipMessage) IsMethodAllowed(m Method) bool {
	hdr := sipmsg.Headers.ValueHeader(Allow)
	hdr = ASCIIToLower(hdr)
	return hdr != "" && strings.Contains(hdr, ASCIIToLower(m.String()))
}

func (sipmsg *SipMessage) IsKnownRURIScheme() bool {
	for _, s := range UriSchemes {
		if s == sipmsg.StartLine.UriScheme {
			return true
		}
	}
	return false
}

func (sipmsg *SipMessage) GetReferToRUIR() (string, error) {
	ok, values := sipmsg.Headers.ValuesHeader(Refer_To)
	if !ok {
		return "", errors.New("No Refer-To header")
	}
	if len(values) > 1 {
		return "", errors.New("Multiple Refer-To headers found")
	}
	value := values[0]
	if strings.Contains(ASCIIToLower(value), "replaces") {
		return "", errors.New("Refer-To with Replaces")
	}
	var mtch []string
	if !RMatch(value, URIFull, &mtch) {
		return "", errors.New("Badly formatted URI")
	}
	return mtch[1], nil
}

func (sipmsg *SipMessage) WithNoReferSubscription() bool {
	if sipmsg.Headers.DoesValueExistInHeader(Require.String(), "norefersub") {
		return true
	}
	if sipmsg.Headers.DoesValueExistInHeader(Supported.String(), "norefersub") {
		return true
	}
	if sipmsg.Headers.DoesValueExistInHeader(Refer_Sub.String(), "false") {
		return true
	}
	return false
}

func (sipmsg *SipMessage) IsResponse() bool {
	return sipmsg.MsgType == RESPONSE
}

func (sipmsg *SipMessage) IsRequest() bool {
	return sipmsg.MsgType == REQUEST
}

func (sipmsg *SipMessage) GetMethod() Method {
	return sipmsg.StartLine.Method
}

func (sipmsg *SipMessage) GetStatusCode() int {
	return sipmsg.StartLine.StatusCode
}

func (sipmsg *SipMessage) GetRegistrationData() (contact, ext, ruri, ipport string, expiresInt int) {
	// TODO fix the Regex
	contact = sipmsg.Headers.ValueHeader(Contact)
	contact1 := strings.Replace(contact, "-", ";", 1) // Contact: <sip:12345-0x562f8a9e7390@172.20.40.132:5030>;expires=30;+sip.instance="<urn:uuid:da213fce-693c-3403-8455-a548a10ef970>"
	var mtch []string
	if RMatch(contact1, INVITERURI, &mtch) {
		ruri = mtch[0]
		ext = mtch[2]
		ipport = mtch[4]
	} else {
		expiresInt = -100 // bad contact
		return
	}
	if RMatch(contact, ExpiresParameter, &mtch) {
		expiresInt = Str2int[int](mtch[1])
		return
	}
	expires := sipmsg.Headers.ValueHeader(Expires)
	if expires != "" {
		expiresInt = Str2int[int](expires)
		return
	}
	expires = "3600"
	sipmsg.Headers.SetHeader(Expires, expires)
	expiresInt = Str2int[int](expires)
	return
}

func (sipmsg *SipMessage) TranslateRM(ss *SipSession, tx *Transaction, nt numtype.NumberType, NewNumber string) {
	if NewNumber == "" {
		return
	}
	localsocket := GenerateUDPSocket(ss.UDPListenser)
	rep := fmt.Sprintf("${1}%s$2", NewNumber)

	switch nt {
	case numtype.CalledRURI:
		sipmsg.StartLine.Ruri = RReplaceNumberOnly(sipmsg.StartLine.Ruri, rep)
		sipmsg.StartLine.UserPart = NewNumber
		ss.RemoteContactURI = sipmsg.StartLine.Ruri
	case numtype.CalledTo:
		sipmsg.Headers.SetHeader(To, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(To), rep))
		ss.ToHeader = sipmsg.Headers.ValueHeader(To)
		tx.To = ss.ToHeader
	case numtype.CalledBoth:
		sipmsg.StartLine.Ruri = RReplaceNumberOnly(sipmsg.StartLine.Ruri, rep)
		sipmsg.StartLine.UserPart = NewNumber
		ss.RemoteContactURI = sipmsg.StartLine.Ruri

		sipmsg.Headers.SetHeader(To, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(To), rep))
		ss.ToHeader = sipmsg.Headers.ValueHeader(To)
		tx.To = ss.ToHeader
	case numtype.CallingFrom:
		sipmsg.Headers.SetHeader(From, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(From), rep))
		ss.FromHeader = sipmsg.Headers.ValueHeader(From)
		tx.From = ss.FromHeader
	case numtype.CallingPAI:
		if sipmsg.Headers.HeaderExists(P_Asserted_Identity.String()) {
			sipmsg.Headers.SetHeader(P_Asserted_Identity, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(P_Asserted_Identity), rep))
		} else {
			sipmsg.Headers.SetHeader(P_Asserted_Identity, fmt.Sprintf("<sip:%s@%s;user=phone>", NewNumber, localsocket.IP))
		}
	case numtype.CallingBoth:
		if sipmsg.Headers.HeaderExists(P_Asserted_Identity.String()) {
			sipmsg.Headers.SetHeader(P_Asserted_Identity, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(P_Asserted_Identity), rep))
		} else {
			sipmsg.Headers.SetHeader(P_Asserted_Identity, fmt.Sprintf("<sip:%s@%s;user=phone>", NewNumber, localsocket.IP))
		}

		sipmsg.Headers.SetHeader(From, RReplaceNumberOnly(sipmsg.Headers.ValueHeader(From), rep))
		ss.FromHeader = sipmsg.Headers.ValueHeader(From)
		tx.From = ss.FromHeader
	}
}

func (sipmsg *SipMessage) PrepareMessageBytes(ss *SipSession) {
	var bb bytes.Buffer
	var headers []string

	updateSDPPart := func(sipmsg *SipMessage, ss *SipSession) {
		ct, ok := sipmsg.Body.PartsContents[SDP]
		if !ok {
			return
		}
		hashvalue := HashSDPBytes(ct.Bytes)
		if ss.SDPHashValue != hashvalue {
			ss.SDPHashValue = hashvalue
			ss.SDPSessionVersion += 1
		}
		if ss.SDPSessionID == 0 {
			ss.SDPSessionID = uint64(RandomNum(1000, 9000))
		}
		lines := strings.Split(string(ct.Bytes), "\r\n")
		for i, ln := range lines {
			var mtch []string
			if RMatch(ln, SDPOriginLine, &mtch) {
				lines[i] = fmt.Sprintf("o=- %v %v IN IP4 %v", ss.SDPSessionID, ss.SDPSessionVersion, mtch[3])
				break
			}
		}
		ct.Bytes = []byte(strings.Join(lines, "\r\n"))
		sipmsg.Body.PartsContents[SDP] = ct
	}

	byteschan := make(chan []byte)

	go func(bc chan<- []byte) {
		var bb2 bytes.Buffer
		if sipmsg.Body.PartsContents == nil {
			sipmsg.Headers.SetHeader(Content_Type, "")
			sipmsg.Headers.SetHeader(MIME_Version, "")
		} else {
			updateSDPPart(sipmsg, ss)
			bdyparts := sipmsg.Body.PartsContents
			if len(bdyparts) == 1 {
				k, v := FirstKeyValue(bdyparts)
				sipmsg.Headers.SetHeader(Content_Type, DicBodyContentType[k])
				sipmsg.Headers.SetHeader(MIME_Version, "")
				bb2.Write(v.Bytes)
			} else {
				sipmsg.Headers.SetHeader(Content_Type, fmt.Sprintf("multipart/mixed;boundary=%v", MultipartBoundary))
				sipmsg.Headers.SetHeader(MIME_Version, "1.0")
				isfirstline := true
				for _, ct := range bdyparts {
					if !isfirstline {
						bb2.WriteString("\r\n")
					}
					bb2.WriteString(fmt.Sprintf("--%v\r\n", MultipartBoundary))
					for _, h := range ct.Headers.GetHeaderNames() {
						_, values := ct.Headers.Values(h)
						for _, hv := range values {
							bb2.WriteString(fmt.Sprintf("%v: %v\r\n", HeaderCase(h), hv))
						}
					}
					bb2.WriteString("\r\n")
					bb2.Write(ct.Bytes)
					isfirstline = false
				}
				bb2.WriteString(fmt.Sprintf("\r\n--%v--\r\n", MultipartBoundary))
			}
		}
		bc <- bb2.Bytes()
	}(byteschan)

	//startline
	if sipmsg.IsRequest() {
		sl := sipmsg.StartLine
		bb.WriteString(fmt.Sprintf("%s %s %s\r\n", sl.Method.String(), sl.Ruri, SipVersion))
		headers = DicRequestHeaders[sipmsg.StartLine.Method]
	} else {
		sl := sipmsg.StartLine
		bb.WriteString(fmt.Sprintf("%s %d %s\r\n", SipVersion, sl.StatusCode, sl.ReasonPhrase))
		headers = DicResponseHeaders[sipmsg.StartLine.StatusCode]
	}

	// var bodybytes []byte
	bodybytes := <-byteschan

	//body - build body type, length, multipart and related headers
	cntntlen := len(bodybytes)

	sipmsg.Headers.SetHeader(Content_Length, fmt.Sprintf("%v", cntntlen))

	//headers - build and write
	for _, h := range headers {
		_, values := sipmsg.Headers.Values(h)
		for _, hv := range values {
			if hv != "" {
				bb.WriteString(fmt.Sprintf("%v: %v\r\n", h, hv))
			}
		}
	}

	//P- headers build and write
	pHeaders := sipmsg.Headers.ValuesWithHeaderPrefix("P-")
	for h, hvs := range pHeaders {
		for _, hv := range hvs {
			if hv != "" {
				bb.WriteString(fmt.Sprintf("%v: %v\r\n", h, hv))
			}
		}
	}

	// write separator
	bb.WriteString("\r\n")

	// write body bytes
	bb.Write(bodybytes)

	//save generated bytes for retransmissions
	sipmsg.Body.MessageBytes = bb.Bytes()
}
