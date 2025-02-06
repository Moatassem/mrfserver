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
	"SRGo/phone"
	"SRGo/q850"
	"SRGo/sip/ivr"
	"SRGo/sip/state"
	"SRGo/sip/status"
	"fmt"
	"net"

	"github.com/pixelbender/go-sdp/sdp"
)

func getURIUsername(uri string) string {
	var mtch []string
	if RMatch(uri, NumberOnly, &mtch) {
		return mtch[1]
	}
	return ""
}

func (ss1 *SipSession) RouteRequest(trans1 *Transaction, sipmsg1 *SipMessage) {
	defer func() {
		if r := recover(); r != nil {
			LogCallStack(r)
		}
	}()

	if ss1.RoutingData == nil { //first invocation
		ss1.RoutingData = &RoutingData{NoAnswerTimeout: 180, No18xTimeout: 60, MaxCallDuration: 0, RURIUsername: sipmsg1.StartLine.UserPart}
		isCallerPhone := phone.Phones.IsPhoneExt(getURIUsername(sipmsg1.FromHeader))
		if isCallerPhone {
			ss1.RoutingData.RemoteUDP = ASUserAgent.UDPAddr
			sipmsg1.AddRequestedBodyParts()
			// if !sipmsg1.KeepOnlyBodyPart(SDP) {
			// 	ss1.RejectMe(trans1, status.NotAcceptableHere, q850.BearerCapabilityNotAvailable, "no remaining body")
			// 	return
			// }
		} else if phone, ok := phone.Phones.Get(ss1.RoutingData.RURIUsername); ok {
			ss1.RoutingData.RemoteUDP = phone.UA.UDPAddr
			if !phone.IsRegistered {
				ss1.RejectMe(trans1, status.TemporarilyUnavailable, q850.NoAnswerFromUser, "target not registered")
				return
			}
			if !phone.IsReachable {
				ss1.RejectMe(trans1, status.DoesNotExistAnywhere, q850.NoRouteToDestination, "target not reachable")
				return
			}
			if !phone.UA.IsAlive {
				ss1.RejectMe(trans1, status.TemporarilyUnavailable, q850.NetworkOutOfOrder, "target not alive")
				return
			}
			if !sipmsg1.KeepOnlyBodyPart(SDP) {
				ss1.RejectMe(trans1, status.NotAcceptableHere, q850.BearerCapabilityNotAvailable, "no remaining body")
				return
			}
		} else {
			ss1.RoutingData.RemoteUDP = ASUserAgent.UDPAddr
			// ss1.RejectMe(trans1, status.NotFound, q850.UnallocatedNumber, "No target found")
			// return
		}
		// if err := initial(sipmsg1, ss1.RoutingData); err != nil {
		// 	LogError(LTConfiguration, err.Error())
		// 	ss1.RejectMe(trans1, status.ServiceUnavailable, q850.ExchangeRoutingError, "Routing failure")
		// 	return
		// }
	}

	// set body in ss1 that will be sent to ss2 after processing
	ss1.RemoteBody = *sipmsg1.Body

	rd := ss1.RoutingData

	// if isMRF && ss1.IsBeingEstablished() && ss1.IsDelayedOfferCall && !trans1.RequestMessage.IsMethodAllowed(UPDATE) {
	// 	ss1.RejectMe(trans1, status.ServiceUnavailable, q850.InterworkingUnspecified, "Delayed offer with no UPDATE support for MRF")
	// 	return
	// }

	ss2 := NewSS(OUTBOUND)
	// ss2.RemoteUDP = ss1.RemoteUDP
	ss2.RemoteUDP = rd.RemoteUDP
	ss2.UDPListenser = ss1.UDPListenser
	ss2.RoutingData = rd
	ss2.IsDelayedOfferCall = ss1.IsDelayedOfferCall

	ss2.LinkedSession = ss1
	ss1.LinkedSession = ss2

	trans2, _ := ss2.CreateLinkedINVITE(rd.RURIUsername, ss1.RemoteBody)

	ss2.IsPRACKSupported = ss1.IsPRACKSupported
	//TODO - return target and prefix .. ex. cdpn:+201223309859, prefix: 042544154
	//To header to contain cdpn & ruri-userpart to contain "+" + prefix + cdpn
	// sipmsg2.TranslateRM(ss2, trans2, numtype.CalledRURI, rd.RURIUsername)

	if !ss1.IsBeingEstablished() {
		return
	}

	ss2.SetState(state.BeingEstablished)
	ss2.AddMe()
	ss2.SendSTMessage(trans2)
}

func (ss1 *SipSession) RouteRequestInternal(trans1 *Transaction, sipmsg1 *SipMessage) {
	defer func() {
		if r := recover(); r != nil {
			LogCallStack(r)
		}
	}()

	upart := sipmsg1.StartLine.UserPart

	if phone, ok := phone.Phones.Get(upart); ok {
		ss1.RoutingData = &RoutingData{NoAnswerTimeout: 10, No18xTimeout: 15, MaxCallDuration: 7200, RURIUsername: upart}
		ss1.RoutingData.RemoteUDP = phone.UA.UDPAddr
		if !phone.IsRegistered {
			ss1.RejectMe(trans1, status.TemporarilyUnavailable, q850.NoAnswerFromUser, "target not registered")
			return
		}
		if !phone.IsReachable {
			ss1.RejectMe(trans1, status.DoesNotExistAnywhere, q850.NoRouteToDestination, "target not reachable")
			return
		}
		if !phone.UA.IsAlive {
			ss1.RejectMe(trans1, status.TemporarilyUnavailable, q850.NetworkOutOfOrder, "target not alive")
			return
		}
		if !sipmsg1.KeepOnlyBodyPart(SDP) {
			ss1.RejectMe(trans1, status.NotAcceptableHere, q850.BearerCapabilityNotAvailable, "no remaining body")
			return
		}
		goto routeCall
	}

	if !sipmsg1.Body.ContainsSDP() {
		ss1.RejectMe(trans1, status.NotAcceptableHere, q850.BearerCapabilityNotImplemented, "Not supported SDP or delay offer")
		return
	}

	if ivr, ok := ivr.IVRsRepo.Get(upart); ok {
		ss1.AnswerIVR(trans1, sipmsg1, ivr)
		return
	}

	ss1.RejectMe(trans1, status.NotFound, q850.UnallocatedNumber, "No target found")
	return

routeCall:
	// set body in ss1 that will be sent to ss2 after processing
	ss1.RemoteBody = *sipmsg1.Body

	rd := ss1.RoutingData

	// if isMRF && ss1.IsBeingEstablished() && ss1.IsDelayedOfferCall && !trans1.RequestMessage.IsMethodAllowed(UPDATE) {
	// 	ss1.RejectMe(trans1, status.ServiceUnavailable, q850.InterworkingUnspecified, "Delayed offer with no UPDATE support for MRF")
	// 	return
	// }

	ss2 := NewSS(OUTBOUND)
	// ss2.RemoteUDP = ss1.RemoteUDP
	ss2.RemoteUDP = rd.RemoteUDP
	ss2.UDPListenser = ss1.UDPListenser
	ss2.RoutingData = rd
	ss2.IsDelayedOfferCall = ss1.IsDelayedOfferCall

	ss2.LinkedSession = ss1
	ss1.LinkedSession = ss2

	trans2, _ := ss2.CreateLinkedINVITE(rd.RURIUsername, ss1.RemoteBody)

	ss2.IsPRACKSupported = ss1.IsPRACKSupported
	//TODO - return target and prefix .. ex. cdpn:+201223309859, prefix: 042544154
	//To header to contain cdpn & ruri-userpart to contain "+" + prefix + cdpn
	// sipmsg2.TranslateRM(ss2, trans2, numtype.CalledRURI, rd.RURIUsername)

	if !ss1.IsBeingEstablished() {
		return
	}

	ss2.SetState(state.BeingEstablished)
	ss2.AddMe()
	ss2.SendSTMessage(trans2)
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
func (ss *SipSession) AnswerIVR(trans *Transaction, sipmsg *SipMessage, bytes *[]byte) {
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
