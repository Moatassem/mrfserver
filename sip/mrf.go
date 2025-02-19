package sip

import (
	"encoding/binary"
	"encoding/xml"
	"fmt"
	"math"
	"mrfgo/dtmf"
	. "mrfgo/global"
	"mrfgo/q850"
	"mrfgo/rtp"
	"mrfgo/sdp"
	"mrfgo/sip/state"
	"mrfgo/sip/status"
	"net"
	"slices"
	"strings"
	"time"
)

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

	repo, ok := MRFRepos.GetMRFRepo(upart)
	if !ok {
		ss.RejectMe(trans, status.NotFound, q850.UnallocatedNumber, "MRF Repository not found")
		return
	}

	ss.MRFRepo = repo

	ss.answerMRF(trans, sipmsg1)
}

// ============================================================================
// MRF methods
func (ss *SipSession) buildSDPAnswer(sipmsg *SipMessage) (sipcode, q850code int, warn string) {
	sdpbytes, _ := sipmsg.GetBodyPart(SDP)
	sdpses, err := sdp.Parse(sdpbytes)
	if err != nil {
		sipcode = status.UnsupportedMediaType
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
		if media.Type != sdp.Audio || media.Port == 0 || media.Proto != sdp.RtpAvp || (conn == nil && len(media.Connection) == 0) { //|| media.Mode != sdp.SendRecv
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
		warn = "No media connection found"
		return
	}

	if media == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.BearerCapabilityNotAvailable
		warn = "No SDP audio offer found"
		return
	}

	if audioFormat == nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.IncompatibleDestination
		warn = "No common audio codec found"
		return
	}

	if Str2Int[int](sdpses.GetEffectivePTime()) != PacketizationTime {
		sipcode = status.NotAcceptableHere
		q850code = q850.BearerCapabilityNotImplemented
		warn = "Packetization other than 20ms not supported"
		return
	}

	rmedia, err := BuildUDPAddr(conn.Address, media.Port)
	if err != nil {
		sipcode = status.NotAcceptableHere
		q850code = q850.ChannelUnacceptable
		warn = "Unable to parse received connection IPv4"
		return
	}

	ss.RemoteMedia = rmedia
	ss.IsCallHeld = sdpses.IsCallHeld()

	// TODO need to handle CANCEL (put some delay before answering?)
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
			SessionID:      ss.SDPSessionID,
			SessionVersion: ss.SDPSessionVersion,
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

	if ss.LocalSDP != nil && !mySDP.Equals(ss.LocalSDP) {
		ss.SDPSessionVersion += 1
		mySDP.Origin.SessionVersion = ss.SDPSessionVersion
	}

	ss.LocalSDP = mySDP
	ss.rtpPayloadType = audioFormat.Payload
	ss.WithTeleEvents = dtmfFormat != nil

	if !ss.WithTeleEvents {
		ss.PCMBytes = make([]byte, 0, DTMFPacketsCount*PayloadSize)
	}

	return
}

func (ss *SipSession) answerMRF(trans *Transaction, sipmsg *SipMessage) {
	if sc, qc, wr := ss.buildSDPAnswer(sipmsg); sc != 0 {
		ss.RejectMe(trans, sc, qc, wr)
		return
	}

	// initializations
	ss.rtpSSRC = RandomNum(2000, 9000000)
	ss.rtpSequenceNum = uint16(RandomNum(1000, 2000))
	ss.rtpTimeStmp = 0
	ss.SDPSessionID = int64(RandomNum(1000, 9000))
	ss.SDPSessionVersion = 1

	ss.SendResponse(trans, status.Ringing, EmptyBody())

	<-time.After(AnswerDelay * time.Millisecond)

	if !ss.IsBeingEstablished() {
		return
	}

	ss.SendResponse(trans, status.OK, NewMessageSDPBody(ss.LocalSDP.Bytes()))
}

func (ss *SipSession) mediaReceiver() {
	for {
		if ss.MediaListener == nil {
			return
		}
		buf := RTPRXBufferPool.Get().(*[]byte)
		n, addr, err := ss.MediaListener.ReadFromUDP(*buf)
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
		bytes := (*buf)[:n]

		if !AreUAddrsEqual(addr, ss.RemoteMedia) {
			fmt.Println("Received RTP from unknown remote connection")
			continue
		}
		if ss.WithTeleEvents {
			if n == 16 { // TODO check if no RFC 4733 is negotiated - transcode InBand DTMF into teleEvents
				ts := binary.BigEndian.Uint32(bytes[4:8]) //TODO check how to use IsSystemBigEndian
				if ss.rtpRFC4733TS != ts {
					ss.rtpRFC4733TS = ts
					dtmf := DicDTMFEvent[bytes[12]]
					ss.processDTMF(dtmf, "Inband - RTP Telephone Event (RFC 4733) - Received: ")
					// switch dtmf {
					// case "DTMF #":
					// 	// ss.stopRTPStreaming() // TODO use this if audiofile can be interrupted by any DTMF or a specific DTMF or not at all
					// case "DTMF *":

					// }
				}
			}
		} else {
			if n == RTPHeadersSize+PayloadSize {
				b1 := bytes[1]
				if b1 >= 128 {
					ss.NewDTMF = true
					ss.PCMBytes = ss.PCMBytes[:0]
				} else if ss.NewDTMF {
					payload := bytes[12:]
					if len(ss.PCMBytes) == DTMFPacketsCount*len(payload) {
						ss.PCMBytes = append(ss.PCMBytes, payload...)
						ss.NewDTMF = false
						pcm := rtp.GetPCM(ss.PCMBytes, ss.rtpPayloadType)
						signal := dtmf.DetectDTMF(pcm)
						if signal != "" {
							dtmf := DicDTMFEvent[DicDTMFSignal[signal]]
							frmt := ss.LocalSDP.GetChosenMedia().FormatByPayload(ss.rtpPayloadType)
							ss.processDTMF(dtmf, fmt.Sprintf("Inband - RTP Audio Tone (%s) - Received: ", frmt.Name))
						}
					} else {
						ss.PCMBytes = append(ss.PCMBytes, payload...)
					}
				}
			}
		}
		RTPRXBufferPool.Put(buf)
	}
}

func (ss *SipSession) parseDTMF(bytes []byte, m Method, bt BodyType) {
	strng := string(bytes)
	var mtch []string
	var signal string
	if bt == DTMFRelay {
		for _, ln := range strings.Split(strng, "\r\n") {
			if RMatch(ln, SignalDTMF, &mtch) {
				signal = mtch[1]
				break
			}
		}
	} else {
		signal = strng
	}
	if signal == "" {
		return
	}
	dtmf := DicDTMFEvent[DicDTMFSignal[signal]]
	ss.processDTMF(dtmf, fmt.Sprintf("OOB - SIP %s (%s) - Received: ", m.String(), DicBodyContentType[bt]))
}

func (ss *SipSession) processDTMF(dtmf, details string) {
	ss.lastDTMF = dtmf
	if ss.bargeEnabled && ss.stopRTPStreaming() {
		LogInfo(LTMediaCapability, "Audio streaming has been interrupted")
	}
	LogInfo(LTDTMF, details+dtmf)
}

func (ss *SipSession) parseXMLnPlay(bytes []byte, bt BodyType) (int, string) {
	if bt != MSCXML {
		return 400, "Unsupported Info body"
	}
	var mrqst MSCRequest
	if err := xml.Unmarshal(bytes, &mrqst); err != nil {
		return 400, "Bad XML body"
	}
	var rqstnm string
	pc := mrqst.Request.PlayCollect
	p := mrqst.Request.Play
	var prmpt Prompt
	var loopflag bool
	ss.bargeEnabled = false
	if pc != nil {
		rqstnm = "playcollect"
		prmpt = pc.Prompt
		ss.bargeEnabled = pc.Barge == "yes"
	} else if p != nil {
		rqstnm = "play"
		prmpt = p.Prompt
		loopflag = prmpt.Repeat == "infinite"
	} else {
		return 400, "Bad MSC request"
	}
	audio := prmpt.Audio
	if len(audio) == 0 {
		return 400, "No defined prompt audio in MSC request"
	}
	ss.stopRTPStreaming()
	go func() {
		tmNow := time.Now()
	loop:
		isStopped := false
		for i := 0; i < len(audio); i++ {
			url := audio[i].URL
			pcmbytes, ok := ss.MRFRepo.Get(url)
			if !ok {
				LogWarning(LTConfiguration, fmt.Sprintf("Requested MSC prompt audio [%s] not found or empty in Repo [%s] - Call ID [%s]", url, ss.MRFRepo.name, ss.CallID))
				continue
			}
			if isStopped = ss.startRTPStreaming(pcmbytes, true, loopflag, false); isStopped {
				break
			}
		}
		if ss.IsDisposed {
			return
		}
		if loopflag {
			goto loop
		}
		var txt, dtmf string
		if isStopped {
			txt = "interrupted"
			dtmf = ""
		} else {
			txt = "timeout"
			dtmf = ss.lastDTMF
		}
		mresp := NewMSCResponse(int(time.Since(tmNow).Milliseconds()), 200, txt, "The request has succeeded", rqstnm, dtmf)
		mrespBytes, _ := xml.Marshal(mresp)
		ss.SendRequest(INFO, nil, NewMSCXML(mrespBytes))
	}()
	return 200, ""
}

func (ss *SipSession) stopRTPStreaming() bool {
	ss.rtpmutex.Lock()
	if !ss.isrtpstreaming {
		ss.rtpmutex.Unlock()
		return false
	}
	ss.rtpmutex.Unlock()

	select {
	case ss.rtpChan <- true:
		return true
	default:
		<-ss.rtpChan
	}
	return false
}

func (ss *SipSession) startRTPStreaming(pcm []int16, resetflag, loopflag, dropCallflag bool) bool {
	ss.rtpmutex.Lock()
	if ss.isrtpstreaming {
		ss.rtpmutex.Unlock()
		return true
	}
	ss.isrtpstreaming = true
	ss.rtpmutex.Unlock()

	origPayload := ss.rtpPayloadType

	// TODO see if i can support more codecs
	// pcm, ok := ss.MRFRepo.Get(afname) // TODO build repos and manage them from UI
	// if !ok {
	// 	fmt.Printf("Cannot find audio [%s]\n", afname) // TODO handle that in INFO
	// 	goto finish1
	// }

	// { To test transcoding is not corrupting data
	// 	g722 := rtp.PCM2G722(pcm)
	// 	pcm = rtp.G722toPCM(g722)
	// 	ulaw := rtp.PCM2G711U(pcm)
	// 	pcm = rtp.G711U2PCM(ulaw)
	// 	alaw := rtp.PCM2G711A(pcm)
	// 	pcm = rtp.G711A2PCM(alaw)
	// }

	isFinished := true // to know that streaming has reached its end

	{
		data, silence := rtp.TxPCMnSilence(pcm, ss.rtpPayloadType)
		if data == nil {
			goto finish1
		}

		tckr := time.NewTicker(20 * time.Millisecond)
		defer tckr.Stop()

		Marker := true

		if resetflag {
			ss.rtpIndex = 0
		}

		for {
			select {
			case <-ss.rtpChan:
				isFinished = false
				goto finish2
			case <-tckr.C:
			}

			if origPayload != ss.rtpPayloadType {
				defer ss.startRTPStreaming(pcm, false, loopflag, dropCallflag)
				goto finish1
			}

			// TODO uncomment below to allow pausing streaming when call is held
			// if ss.IsCallHeld {
			// 	goto finish1
			// }

			ss.rtpTimeStmp += uint32(RTPPayloadSize)
			if ss.rtpSequenceNum == math.MaxUint16 {
				ss.rtpSequenceNum = 0
			} else {
				ss.rtpSequenceNum++
			}

			delta := len(data) - ss.rtpIndex
			var payload []byte
			if RTPPayloadSize <= delta {
				payload = (data)[ss.rtpIndex : ss.rtpIndex+RTPPayloadSize]
				ss.rtpIndex += RTPPayloadSize
				isFinished = false
			} else {
				payload = (data)[ss.rtpIndex : ss.rtpIndex+delta]
				for n := delta; n < RTPPayloadSize; n++ {
					payload = append(payload, silence)
				}
				ss.rtpIndex += delta
				isFinished = true
			}

			if !ss.IsCallHeld {
				pktptr := RTPTXBufferPool.Get().(*[]byte)
				pkt := (*pktptr)[:0]
				pkt = append(pkt, 128)
				pkt = append(pkt, bool2byte(Marker)*128+ss.rtpPayloadType)
				pkt = append(pkt, uint16ToBytes(ss.rtpSequenceNum)...)
				pkt = append(pkt, uint32ToBytes(ss.rtpTimeStmp)...)
				pkt = append(pkt, uint32ToBytes(ss.rtpSSRC)...)
				pkt = append(pkt, payload...)
				_, err := ss.MediaListener.WriteToUDP(pkt, ss.RemoteMedia)
				if err != nil {
					goto finish1
				}
				RTPTXBufferPool.Put(pktptr)
			}

			Marker = false

			if isFinished {
				if loopflag {
					ss.rtpIndex = 0
					isFinished = false
					Marker = true
					continue
				}
				ss.rtpIndex = 0
				break
			}
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

	if dropCallflag {
		ss.ReleaseMe("audio playback ended")
	}

	return !isFinished
}

// =========================================================================================================================

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
// ============================================================================
// Request:
type MSCRequest struct {
	XMLName xml.Name `xml:"MediaServerControl"`
	Version string   `xml:"version,attr"`
	Request struct {
		Play        *Play        `xml:"play,omitempty"`
		PlayCollect *PlayCollect `xml:"playcollect,omitempty"`
	} `xml:"request"`
}

type Play struct {
	Prompt Prompt `xml:"prompt"`
}

type PlayCollect struct {
	MaxDigits       int    `xml:"maxdigits,attr"`
	Barge           string `xml:"barge,attr"`
	ExtraDigitTimer string `xml:"extradigittimer,attr"`
	FirstDigitTimer string `xml:"firstdigittimer,attr"`
	Prompt          Prompt `xml:"prompt"`
}

type Prompt struct {
	Repeat string  `xml:"repeat,attr,omitempty"`
	Audio  []Audio `xml:"audio"`
}

type Audio struct {
	URL string `xml:"url,attr"`
}

// Response:
type MSCResponse struct {
	XMLName  xml.Name  `xml:"MediaServerControl"`
	Version  string    `xml:"version,attr"`
	Response xResponse `xml:"response"`
}

type xResponse struct {
	PlayDuration int    `xml:"playduration,attr"`
	Reason       string `xml:"reason,attr"`
	PlayOffset   int    `xml:"playoffset,attr"`
	Text         string `xml:"text,attr"`
	Request      string `xml:"request,attr"`
	Code         int    `xml:"code,attr"`
	Digits       string `xml:"digits,attr,omitempty"`
}

func NewMSCResponse(pd, cd int, rsn, txt, rqst, dgts string) MSCResponse {
	return MSCResponse{
		XMLName: xml.Name{Local: "MediaServerControl"},
		Version: "1.0",
		Response: xResponse{
			PlayDuration: pd,
			Reason:       rsn,
			PlayOffset:   pd,
			Text:         txt,
			Request:      rqst,
			Code:         cd,
			Digits:       dgts,
		},
	}
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
