/*
# Software Name : Media Resource Function Server (SR)
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
	. "MRFGo/global"
	"MRFGo/q850"
	"MRFGo/rtp"
	"MRFGo/sdp"
	"MRFGo/sip/state"
	"MRFGo/sip/status"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"slices"
	"time"
)

func getURIUsername(uri string) string {
	var mtch []string
	if RMatch(uri, NumberOnly, &mtch) {
		return mtch[1]
	}
	return ""
}

func (ss *SipSession) RouteRequestInternal(trans *Transaction, sipmsg1 *SipMessage) {
	defer func() {
		if r := recover(); r != nil {
			LogCallStack(r)
		}
	}()

	upart := sipmsg1.StartLine.UserPart

	if !sipmsg1.Body.ContainsSDP() {
		ss.RejectMe(trans, status.NotAcceptableHere, q850.BearerCapabilityNotImplemented, "Not supported SDP or delayed offer")
		return
	}

	if !MRFRepos.DoesMRFRepoExist(upart) {
		ss.RejectMe(trans, status.NotFound, q850.UnallocatedNumber, "MRF Repository not found")
		return
	}

	ss.answerMRF(trans, sipmsg1)
}

// ============================================================================
// MRF methods
func (ss *SipSession) buildSDPAnswer(sipmsg *SipMessage) (sipcode, q850code int, warn string) {
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
			if connection.Type != sdp.TypeIPv4 || connection.Network != sdp.NetworkInternet { //connection.Address == "0.0.0.0"
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

	rmedia, err := BuildUDPAddr(conn.Address, media.Port)
	if err != nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.ChannelUnacceptable
		warn = "Unable to parse received connection IPv4"
		return
	}

	ss.RemoteMedia = rmedia
	ss.IsCallHeld = sdpses.IsCallHeld()

	// TODO need to handle incoming ReINVITE and UPDPATE
	// need to handle CANCEL (put some delay before answering?)
	// need to handle INFO to play the required audio files
	// need to handle DTMF
	// TODO need to build RTP packets and stream media back
	if ss.MediaListener == nil {
		ss.MediaListener = MediaPorts.ReserveSocket()
	}
	if ss.MediaListener == nil {
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
			Address:        ServerIPv4.String(),
		},
		Name: "MRF",
		// Information: "A Seminar on the session description protocol",
		// URI:         "http://www.example.com/seminars/sdp.pdf",
		// Email:       []string{"j.doe@example.com (Jane Doe)"},
		// Phone:       []string{"+1 617 555-6011"},
		Connection: &sdp.Connection{
			Network: sdp.NetworkInternet,
			Type:    sdp.TypeIPv4,
			Address: ServerIPv4.String(),
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
				Port:       GetUDPortFromConn(ss.MediaListener),
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
	ss.rtpPayload = audioFormat.Payload
	return
}

func (ss *SipSession) answerMRF(trans *Transaction, sipmsg *SipMessage) {
	if sc, qc, wr := ss.buildSDPAnswer(sipmsg); sc != 0 {
		ss.RejectMe(trans, sc, qc, wr)
		return
	}

	ss.rtpSSRC = RandomNum(2000, 9000000)
	ss.rtpSequenceNum = uint16(RandomNum(1000, 2000))
	ss.rtpTimeStmp = 0

	ss.SendResponse(trans, status.OK, *NewMessageSDPBody(ss.LocalSDP.Bytes()))
}

func (ss *SipSession) mediaReceiver() {
	for {
		if ss.MediaListener == nil {
			return
		}
		buf := RTPRXBufferPool.Get().([]byte)
		n, addr, err := ss.MediaListener.ReadFromUDP(buf)
		if err != nil {
			if buf != nil {
				RTPRXBufferPool.Put(buf)
			}
			if opErr, ok := err.(*net.OpError); ok {
				_ = opErr
				return
			}
			fmt.Println(err)
			continue
		}
		bytes := buf[:n]
		RTPRXBufferPool.Put(buf)
		if !AreUAddrsEqual(addr, ss.RemoteMedia) {
			fmt.Println("Received RTP from unknown remote connection")
			continue
		}
		if n == 16 { // TODO check if no RFC 4733 is negotiated - transcode InBand DTMF into teleEvents
			ts := binary.BigEndian.Uint32(bytes[4:8]) //TODO check how to use IsSystemBigEndian
			if ss.rtpRFC4733TS != ts {
				ss.rtpRFC4733TS = ts
				dtmf := DicDTMFEvent[bytes[12]]
				fmt.Printf("RFC 4733 Received: %s\n", dtmf)
				switch dtmf {
				case "DTMF #":
					ss.stopRTPStreaming() // TODO use this if audiofile can be interrupted by any DTMF or a specific DTMF or not at all
				case "DTMF *":

				}
			}
		}
	}
}

func (ss *SipSession) stopRTPStreaming() {
	ss.rtpmutex.Lock()
	if !ss.isrtpstreaming {
		ss.rtpmutex.Unlock()
		return
	}
	ss.rtpmutex.Unlock()

	select {
	case ss.rtpChan <- true:
	default:
		<-ss.rtpChan
	}
}

func (ss *SipSession) startRTPStreaming(filename string) {
	ss.rtpmutex.Lock()
	if ss.isrtpstreaming {
		ss.rtpmutex.Unlock()
		return
	}
	ss.isrtpstreaming = true
	ss.rtpmutex.Unlock()

	// TODO see if i can support more codecs
	pcm, ok := MRFRepos.Get("999", filename) // TODO build repos and manage them from UI
	if !ok {
		fmt.Printf("Cannot find file [%s]\n", filename) // TODO handle that in INFO .. use buffer ..
		goto finish1
	}

	// TODO get SIP Engine PCM2G711A & PCM2G711U

	// TODO only allow ptime:20 .. i.e. 160 bytes/packet/20ms
	{
		data, silence := rtp.TxPCMnSilence(pcm, ss.rtpPayload)
		if data == nil {
			goto finish1
		}

		tckr := time.NewTicker(20 * time.Millisecond)
		defer tckr.Stop()

		Marker := true
		// ss.rtpIndex = 0 // TODO put it zero if file changes

		for {
			select {
			case <-ss.rtpChan:
				goto finish2
			case <-tckr.C:
			}

			if ss.IsCallHeld {
				goto finish1
			}

			ss.rtpTimeStmp += uint32(RTPPayloadSize)
			if ss.rtpSequenceNum == math.MaxUint16 {
				ss.rtpSequenceNum = 0
			} else {
				ss.rtpSequenceNum++
			}

			pkt := RTPTXBufferPool.Get().([]byte)[:0]

			delta := len(data) - ss.rtpIndex
			var payload []byte
			if RTPPayloadSize <= delta {
				payload = (data)[ss.rtpIndex : ss.rtpIndex+RTPPayloadSize]
				ss.rtpIndex += RTPPayloadSize
			} else {
				payload = (data)[ss.rtpIndex : ss.rtpIndex+delta]
				for n := delta; n < RTPPayloadSize; n++ {
					payload = append(payload, silence)
				}
				ss.rtpIndex += delta
			}

			pkt = append(pkt, 128)
			pkt = append(pkt, bool2byte(Marker)*128+ss.rtpPayload)
			pkt = append(pkt, uint16ToBytes(ss.rtpSequenceNum)...)
			pkt = append(pkt, uint32ToBytes(ss.rtpTimeStmp)...)
			pkt = append(pkt, uint32ToBytes(ss.rtpSSRC)...)
			pkt = append(pkt, payload...)

			ss.MediaListener.WriteToUDP(pkt, ss.RemoteMedia)

			Marker = false
			RTPTXBufferPool.Put(pkt[:0])
		}
	}

finish1:
	select {
	case <-ss.rtpChan:
	default:
	}

finish2:
	ss.rtpmutex.Lock()
	ss.isrtpstreaming = false
	ss.rtpmutex.Unlock()
}

func bool2byte(b bool) byte {
	if b {
		return 1
	}
	return 0
}

func uint16ToBytes(num uint16) []byte {
	bytes := make([]byte, 2)
	binary.BigEndian.PutUint16(bytes, num)
	return bytes
}

func uint32ToBytes(num uint32) []byte {
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, num)
	return bytes
}

// ============================================================================

func ProbeUA(conn *net.UDPConn, ua *SipUdpUserAgent) {
	if conn == nil || ua == nil {
		return
	}
	ss := NewSS(OUTBOUND)
	ss.RemoteUDP = ua.UDPAddr
	ss.SIPUDPListenser = conn
	ss.RemoteUserAgent = ua

	hdrs := NewSipHeaders()
	hdrs.AddHeader(Subject, "Out-of-dialogue keep-alive")
	hdrs.AddHeader(Accept, "application/sdp")

	trans := ss.CreateSARequest(RequestPack{Method: OPTIONS, Max70: true, CustomHeaders: hdrs, RUriUP: "ping", FromUP: "ping", IsProbing: true}, EmptyBody())

	ss.SetState(state.BeingProbed)
	ss.AddMe()
	ss.SendSTMessage(trans)
}
