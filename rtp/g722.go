package rtp

import (
	"fmt"

	"github.com/gotranspile/g722"
	// "github.com/gotranspile/g722"
	// "github.com/xlab/opus-go/opus"
)

const (
	PCMU byte = 0
	PCMA byte = 8
	G722 byte = 9
)

var codecSilence = map[byte]byte{PCMU: 255, PCMA: 213, G722: 85}

var TranscodingEngine TXEngine

type TXEngine struct {
	G722Encoder *g722.Encoder
	G722Decoder *g722.Decoder
}

func GetPCM(frames []byte, pt byte) []int16 {
	switch pt {
	case PCMU:
		return G711U2PCM(frames)
	case PCMA:
		return G711A2PCM(frames)
	case G722:
		return G722toPCM(frames)
	default:
		return nil
	}
}

func TxPCMnSilence(pcm []int16, pt byte) ([]byte, byte) {
	switch pt {
	case PCMU:
		return PCM2G711U(pcm), codecSilence[pt]
	case PCMA:
		return PCM2G711A(pcm), codecSilence[pt]
	case G722:
		return PCM2G722(pcm), codecSilence[pt]
	default:
		return nil, 0
	}
}

func G722toPCM(frame []byte) []int16 {
	count := len(frame)
	if len(frame) == 0 {
		return nil
	}
	res := make([]int16, count)
	n := TranscodingEngine.G722Decoder.Decode(res, frame)
	if n == 0 {
		fmt.Println(fmt.Errorf("Failed to encode G.722 data"))
		return nil
	}
	return res

}

func PCM2G722(pcm []int16) []byte {
	g722 := make([]byte, len(pcm))
	n := TranscodingEngine.G722Encoder.Encode(g722, pcm)
	if n == 0 {
		fmt.Println(fmt.Errorf("Failed to encode G.722 data"))
		return nil
	}
	return g722
}

func InitializeTX() {
	// Create a G.722 encoder
	g722rate := 64000
	var g722flag g722.Flags = g722.FlagSampleRate8000
	TranscodingEngine.G722Encoder = g722.NewEncoder(g722rate, g722flag)

	// Create a G.722 decoder
	TranscodingEngine.G722Decoder = g722.NewDecoder(g722rate, g722flag)
}
