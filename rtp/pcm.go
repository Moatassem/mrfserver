package rtp

// import (
// 	"bytes"
// 	"fmt"
// 	"log"
// 	"os"
// 	"time"

// 	"github.com/dhowden/tag"
// 	"github.com/faiface/beep/mp3"
// )

// // Convert MP3 to PCM and return as []int16
// func Mp3ToPcm(filePath string) ([]int16, error) {
// 	// Read the MP3 file into a byte slice
// 	fileData, err := os.ReadFile(filePath)
// 	if err != nil {
// 		fmt.Printf("Error reading MP3 file: %v\n", err)
// 		return nil, err
// 	}

// 	// Remove metadata from the MP3 file
// 	reader := bytes.NewReader(fileData)
// 	_, err = tag.ReadFrom(reader)
// 	if err != nil && err != tag.ErrNoTagsFound {
// 		fmt.Printf("Error reading metadata: %v\n", err)
// 		return nil, err
// 	}

// 	// Seek to the beginning of the audio data
// 	reader.Seek(0, 0)

// 	// Decode the MP3 file
// 	streamer, format, err := mp3.Decode(f)
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to decode MP3 file: %v", err)
// 	}
// 	defer streamer.Close()

// 	// Create a buffer to hold the PCM data
// 	var pcmData []int16
// 	buf := make([][2]float64, format.SampleRate.N(time.Second/10)) // Buffer for 100ms of audio
// 	for {
// 		n, ok := streamer.Stream(buf)
// 		if !ok {
// 			break
// 		}
// 		for _, sample := range buf[:n] {
// 			pcmData = append(pcmData, int16(sample[0]*32767), int16(sample[1]*32767))
// 		}
// 	}

// 	return pcmData, nil
// }

// func main1() {
// 	// Path to the MP3 file
// 	filePath := "example.mp3"

// 	// Convert MP3 to PCM
// 	pcmData, err := Mp3ToPcm(filePath)
// 	if err != nil {
// 		log.Fatalf("Error converting MP3 to PCM: %v", err)
// 	}

// 	// Save PCM data to a file (for example purposes)
// 	os.WriteFile("output.pcm", int16ToBytes(pcmData), 0644)
// 	fmt.Println("PCM data saved to output.pcm")
// }

// // Helper function to convert []int16 to []byte
// func int16ToBytes(data []int16) []byte {
// 	bytes := make([]byte, len(data)*2)
// 	for i, v := range data {
// 		bytes[i*2] = byte(v)
// 		bytes[i*2+1] = byte(v >> 8)
// 	}
// 	return bytes
// }

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"

	"github.com/hajimehoshi/go-mp3"
)

func Mp3ToPcm(filename string) ([]int16, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	pcmData := bytesToInt16(file)
	// Output PCM sample count
	fmt.Println("PCM Data Length:", len(pcmData))
	return pcmData, nil
}

func Mp3ToPcm1(filename string) ([]int16, error) {
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	defer file.Close()

	// Decode the MP3 file
	decoder, err := mp3.NewDecoder(file)
	if err != nil {
		fmt.Println("Error decoding MP3:", err)
		return nil, err
	}

	// Read PCM data from decoder
	var pcmData []int16
	buf := make([]byte, 5_000_000) // Buffer for reading bytes

	for {
		n, err := decoder.Read(buf)
		if err != nil && err != io.EOF {
			fmt.Println("Error reading MP3:", err)
			return nil, err
		}
		if n == 0 {
			break
		}

		// Convert bytes to int16 PCM samples
		int16Samples := bytesToInt16(buf[:n])
		pcmData = append(pcmData, int16Samples...)
	}

	// Output PCM sample count
	fmt.Println("PCM Data Length:", len(pcmData))
	return pcmData, nil
}

// bytesToInt16 converts a byte slice into a slice of int16 samples
func bytesToInt16(data []byte) []int16 {
	int16Data := make([]int16, len(data)/2)
	err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &int16Data)
	if err != nil {
		fmt.Println("Error converting to int16:", err)
	}
	return int16Data
}
