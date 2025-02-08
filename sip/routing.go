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
	"SRGo/q850"
	"SRGo/sdp"
	"SRGo/sip/state"
	"SRGo/sip/status"
	"fmt"
	"net"
)

func getURIUsername(uri string) string {
	var mtch []string
	if RMatch(uri, NumberOnly, &mtch) {
		return mtch[1]
	}
	return ""
}

func (ss1 *SipSession) RouteRequestInternal(trans1 *Transaction, sipmsg1 *SipMessage) {
	defer func() {
		if r := recover(); r != nil {
			LogCallStack(r)
		}
	}()

	upart := sipmsg1.StartLine.UserPart

	if !sipmsg1.Body.ContainsSDP() {
		ss1.RejectMe(trans1, status.NotAcceptableHere, q850.BearerCapabilityNotImplemented, "Not supported SDP or delayed offer")
		return
	}

	// if ivr, ok := ivr.IVRsRepo.Get(upart); ok {

	ss1.AnswerIVR(trans1, sipmsg1, upart)

	// ss1.RejectMe(trans1, status.NotFound, q850.UnallocatedNumber, "No target found")
}

func (ss1 *SipSession) RerouteRequest(rspnspk ResponsePack) {
	defer func() {
		if r := recover(); r != nil {
			LogCallStack(r)
		}
	}()
	if ss1 == nil {
		return
	}
	// var reason string
	// switch rspnspk.StatusCode {
	// case 487:
	// 	reason = "NOANSWER"
	// case 408:
	// 	reason = "UNREACHABLE"
	// default:
	// 	reason = "REJECTED"
	// }
	trans1 := ss1.GetLastUnACKedINVSYNC(INBOUND)
	if trans1 == nil {
		return
	}
	if ss1.IsBeingEstablished() {
		ss1.LinkedSession = nil
		ss1.RejectMe(trans1, rspnspk.StatusCode, q850.NormalUnspecified, "Rerouting failed")
		return
	}
	// rcv18x := trans1.StatusCodeExistsSYNC(180)
	// if err := failure(reason, rcv18x, ss1.RoutingData); err != nil {
	// 	LogError(LTConfiguration, err.Error())
	// 	if ss1.IsBeingEstablished() {
	// 		ss1.LinkedSession = nil
	// 		ss1.RejectMe(trans1, status.ServiceUnavailable, q850.ExchangeRoutingError, "Rerouting failure")
	// 		return
	// 	}
	// }
	// ss1.RouteRequest(trans1, nil)
}

// ============================================================================
// IVR transfer functions
func (ss *SipSession) AnswerIVR(trans *Transaction, sipmsg *SipMessage, upart string) {
	sdpbytes, _ := sipmsg.GetBodyPart(SDP)
	sess, err := sdp.ParseString(string(sdpbytes))
	if err != nil {
		ss.RejectMe(trans, status.NotAcceptableHere, q850.BearerCapabilityNotImplemented, "Not supported SDP")
		return
	}
	fmt.Println(sess)
	ss.SendResponse(trans, status.OK, EmptyBody())
}

// ==
func (ss *SipSession) HandleRefer(trans *Transaction, sipmsg *SipMessage) {
	referRuri, err := sipmsg.GetReferToRUIR()
	if err != nil {
		ss.SendResponseDetailed(trans, NewResponsePackRFWarning(status.BadRequest, "", err.Error()), EmptyBody())
		return
	}

	ss.ReferSubscription = !sipmsg.WithNoReferSubscription()
	if ss.ReferSubscription {
		ss.Relayed18xNotify = nil
	}

	fmt.Println(referRuri)
	ss.SendResponse(trans, status.OK, EmptyBody())
}

// ============================================================================

func ProbeUA(conn *net.UDPConn, ua *SipUdpUserAgent) {
	if conn == nil || ua == nil {
		return
	}
	ss := NewSS(OUTBOUND)
	ss.RemoteUDP = ua.UDPAddr
	ss.UDPListenser = conn
	ss.RemoteUserAgent = ua

	hdrs := NewSipHeaders()
	hdrs.AddHeader(Subject, "Out-of-dialogue keep-alive")
	hdrs.AddHeader(Accept, "application/sdp")

	trans := ss.CreateSARequest(RequestPack{Method: OPTIONS, Max70: true, CustomHeaders: hdrs, RUriUP: "ping", FromUP: "ping", IsProbing: true}, EmptyBody())

	ss.SetState(state.BeingProbed)
	ss.AddMe()
	ss.SendSTMessage(trans)
}
