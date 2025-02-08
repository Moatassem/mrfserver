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
	"slices"
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
	var media *sdp.Media
	var conn *sdp.Connection = sess.Connection
	var audioFormat *sdp.Format
	var dtmfFormat *sdp.Format
	for i := 0; i < len(sess.Media); i++ {
		media = sess.Media[i]
		if media.Type != "audio" || media.Port == 0 || media.Proto != "RTP/AVP" || len(media.Connection) == 0 || media.Mode != sdp.SendRecv {
			continue
		}
		for j := 0; j < len(media.Connection); j++ {
			connection := media.Connection[j]
			if connection.Address == "0.0.0.0" || connection.Type != sdp.TypeIPv4 || connection.Network != sdp.NetworkInternet {
				continue
			}
			conn = connection
			break
		}
		for k := 0; k < len(media.Format); k++ {
			frmt := media.Format[k]
			if frmt.Channels != 1 || frmt.ClockRate != 8000 || !slices.Contains(sdp.SupportedCodecs, frmt.Payload) {
				continue
			}
			audioFormat = frmt
			break
		}
		for k := 0; k < len(media.Format); k++ {
			frmt := media.Format[k]
			if frmt.Name == sdp.TelephoneEvents {
				dtmfFormat = frmt
				break
			}
		}
		break
	}
	if conn == nil {
		ss.RejectMe(trans, status.NotAcceptableHere, q850.CallRejected, "No available media connection found")
		return
	}
	if media == nil {
		ss.RejectMe(trans, status.NotAcceptableHere, q850.CallRejected, "No available SDP found")
		return
	}
	if audioFormat == nil {
		ss.RejectMe(trans, status.NotAcceptableHere, q850.CallRejected, "No common audio codec found")
		return
	}
	if dtmfFormat == nil {

	}
	rmedia, err := BuildUDPSocket(conn.Address, media.Port)
	if err != nil {
		ss.RejectMe(trans, status.ServiceUnavailable, q850.CallRejected, "Unable to parse received connection IPv4")
		return
	}

	ss.RemoteMedia = rmedia
	fmt.Println(audioFormat)
	fmt.Println(dtmfFormat)

	mySDP := &sdp.Session{
		Origin: &sdp.Origin{
			Username:       "mt",
			SessionID:      int64(RandomNum(1000, 9000)),
			SessionVersion: 1,
			Network:        sdp.NetworkInternet,
			Type:           sdp.TypeIPv4,
			Address:        ServerIPv4,
		},
		Name: "MRF",
		// Information: "A Seminar on the session description protocol",
		// URI:         "http://www.example.com/seminars/sdp.pdf",
		// Email:       []string{"j.doe@example.com (Jane Doe)"},
		// Phone:       []string{"+1 617 555-6011"},
		Connection: &sdp.Connection{
			Network: sdp.NetworkInternet,
			Type:    sdp.TypeIPv4,
			Address: ServerIPv4,
			TTL:     0,
		},
		// Bandwidth: []*Bandwidth{
		// 	{"AS", 2000},
		// },
		// Timing: &Timing{
		// 	Start: parseTime("1996-02-27 15:26:59 +0000 UTC"),
		// 	Stop:  parseTime("1996-05-30 16:26:59 +0000 UTC"),
		// },
		// Repeat: []*Repeat{
		// 	{
		// 		Interval: time.Duration(604800) * time.Second,
		// 		Duration: time.Duration(3600) * time.Second,
		// 		Offsets: []time.Duration{
		// 			time.Duration(0),
		// 			time.Duration(90000) * time.Second,
		// 		},
		// 	},
		// },
		// TimeZone: []*TimeZone{
		// 	{Time: parseTime("1996-02-27 15:26:59 +0000 UTC"), Offset: -time.Hour},
		// 	{Time: parseTime("1996-05-30 16:26:59 +0000 UTC"), Offset: 0},
		// },
		Mode: sdp.SendRecv,
	}

	for i := 0; i < len(sess.Media); i++ {
		media = sess.Media[i]
		if media.Type != "audio" || media.Port == 0 || media.Proto != "RTP/AVP" || len(media.Connection) == 0 || media.Mode != sdp.SendRecv {
			mySDP.Media = append(mySDP.Media, &sdp.Media{
				Type:  media.Type,
				Port:  0,
				Proto: media.Proto})
			continue
		}
		mySDP.Media = append(mySDP.Media, &sdp.Media{
			Type:   "audio",
			Port:   5000, //<<<<<<<<<<<<<<<<<<<<<<<
			Proto:  "RTP/AVP",
			Format: []*sdp.Format{audioFormat, dtmfFormat}})
	}
	mySDPBytes := mySDP.Bytes()

	ss.LocalBody = NewMessageBody(true)
	ss.LocalBody.PartsContents[SDP] = ContentPart{Bytes: mySDPBytes}

	ss.SendResponse(trans, status.OK, *ss.LocalBody)
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
