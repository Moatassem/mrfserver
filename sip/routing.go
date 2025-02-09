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

	ss1.AnswerMRF(trans1, sipmsg1, upart)

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
// MRF functions
func (ss *SipSession) BuildSDPAnswer(sipmsg *SipMessage) (sipcode, q850code int, warn string) {
	sdpbytes, _ := sipmsg.GetBodyPart(SDP)
	sdpses, err := sdp.Parse(sdpbytes)
	if err != nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.BearerCapabilityNotImplemented
		warn = "Not supported SDP"
		return
	}
	var media *sdp.Media
	var conn *sdp.Connection = sdpses.Connection
	var audioFormat *sdp.Format
	var dtmfFormat *sdp.Format
	for i := 0; i < len(sdpses.Media); i++ {
		media = sdpses.Media[i]
		if media.Type != "audio" || media.Port == 0 || media.Proto != "RTP/AVP" || conn == nil && len(media.Connection) == 0 { //|| media.Mode != sdp.SendRecv
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
		media.Chosen = true
		break
	}
	if conn == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.MandatoryInformationElementIsMissing
		warn = "No available media connection found"
		return
	}
	if media == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.BearerCapabilityNotAvailable
		warn = "No available SDP found"
		return
	}
	if audioFormat == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.IncompatibleDestination
		warn = "No common audio codec found"
		return
	}
	ss.WithTeleEvents = dtmfFormat != nil

	rmedia, err := BuildUDPSocket(conn.Address, media.Port)
	if err != nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.ChannelUnacceptable
		warn = "Unable to parse received connection IPv4"
		return
	}

	ss.RemoteMedia = rmedia

	// reserve a local UDP port
	// TODO need to ask OS if this port is really available!!!
	// TODO need to handle incoming ReINVITE and UPDPATE
	// TODO need to negotiate media-directive properly
	// need to handle CANCEL (put some delay before answering?)
	// need to handle INFO to play the required audio files
	// need to handle DTMF
	// TODO need to build RTP packets and stream media back
	if ss.LocalSocket == nil {
		ss.LocalSocket = MediaPorts.ReserveSocket(ServerIPv4)
	}
	if ss.LocalSocket == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.ResourceUnavailableUnspecified
		warn = "Media pool depleted"
		return
	}

	mySDP := &sdp.Session{
		Origin: &sdp.Origin{
			Username:       "mt",
			SessionID:      1, // will be updated by updateSDPPart in PrepareMessageBytes
			SessionVersion: 1, // will be updated by updateSDPPart in PrepareMessageBytes
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
	}

	for i := 0; i < len(sdpses.Media); i++ {
		media := sdpses.Media[i]
		var newmedia *sdp.Media
		if media.Chosen {
			newmedia = &sdp.Media{
				Chosen:     true,
				Type:       "audio",
				Port:       ss.LocalSocket.Port,
				Proto:      "RTP/AVP",
				Format:     []*sdp.Format{audioFormat},
				Attributes: []*sdp.Attr{{Name: "ptime", Value: "20"}},
				Mode:       sdp.NegotiateMode(sdp.SendRecv, sdpses.GetEffectiveMediaDirective())}
			if dtmfFormat != nil {
				newmedia.Format = append(newmedia.Format, dtmfFormat)
			}
		} else {
			newmedia = &sdp.Media{Type: media.Type, Port: 0, Proto: media.Proto}
		}
		mySDP.Media = append(mySDP.Media, newmedia)
	}
	ss.LocalSDP = mySDP
	return
}

func (ss *SipSession) AnswerMRF(trans *Transaction, sipmsg *SipMessage, upart string) {
	if sc, qc, wr := ss.BuildSDPAnswer(sipmsg); sc != 0 {
		ss.RejectMe(trans, sc, qc, wr)
		return
	}
	ss.SendResponse(trans, status.OK, *NewMessageSDPBody(ss.LocalSDP.Bytes()))
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
