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
	"os"

	"github.com/go-audio/wav"
)

func ReadPCMRaw(filename string) ([]int16, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	pcmData := bytesToInt16s(file)
	return pcmData, nil
}

func ReadPCMWav(filename string) ([]int16, error) {
	// Open the WAV file
	file, err := os.Open(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	defer file.Close()

	// Create a new WAV decoder
	decoder := wav.NewDecoder(file)

	// Check if the file is valid and not corrupt
	if !decoder.IsValidFile() {
		fmt.Println("Invalid WAV file")
		return nil, err
	}

	// Decode the WAV file
	buffer, err := decoder.FullPCMBuffer()
	if err != nil {
		fmt.Println("Error decoding file:", err)
		return nil, err
	}

	// Convert PCM data to int16 slice
	// pcmData := intsToInt16s(buffer.Data)
	pcmData := intsToInt16sSpeed(buffer.Data, 2)

	// pcmData := bytesToInt16(file)
	// Output PCM sample count
	fmt.Println("PCM Data Length:", len(pcmData))
	return pcmData, nil
}

// bytesToInt16s converts a byte slice into a slice of int16 samples
func bytesToInt16s(data []byte) []int16 {
	int16Data := make([]int16, len(data)/2)
	err := binary.Read(bytes.NewReader(data), binary.LittleEndian, &int16Data)
	if err != nil {
		fmt.Println("Error converting to int16:", err)
	}
	return int16Data
}

func intsToInt16s(data []int) []int16 {
	int16Data := make([]int16, len(data))
	for i, v := range data {
		int16Data[i] = int16(v) // Direct conversion
	}
	return int16Data
}

func intsToInt16sSpeed(data []int, speedFactor int) []int16 {
	if speedFactor < 1 {
		speedFactor = 1 // Prevent invalid values
	}

	newLength := len(data) / speedFactor
	int16Data := make([]int16, newLength)

	for i := 0; i < newLength; i++ {
		int16Data[i] = int16(data[i*speedFactor])
	}

	return int16Data
}
