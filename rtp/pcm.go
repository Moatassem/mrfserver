package rtp

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

var SoxPath string = `C:\Program Files (x86)\sox-14-4-2`

// var SoxPath string = `C:\Users\Tassem PC\Documents\VB Projects\SIP Engine Service\bin\Debug\sox`

func RunSox(parentDir, filenamewithExt, filenameonly string) (string, error) {
	var filename, rawfilename string
	var err error
	filename, err = filepath.Abs(filepath.Join(parentDir, filenamewithExt))
	rawfilename, err = filepath.Abs(filepath.Join(parentDir, fmt.Sprintf("%s.raw", filenameonly)))

	soxcmd := filepath.Join(SoxPath, "sox")
	cmd := exec.Command(soxcmd, "--clobber", "--no-glob",
		filename,
		"-e", "signed-integer",
		"-b", "16",
		"-c", "1",
		"-r", "16000",
		rawfilename,
		"speed", "2")

	// Redirect output to console
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Run the command
	err = cmd.Run()
	if err == nil {
		_ = os.Remove(filename)
	}
	return rawfilename, err
}

func ReadPCMRaw(filename string) ([]int16, error) {
	file, err := os.ReadFile(filename)
	if err != nil {
		fmt.Println("Error opening file:", err)
		return nil, err
	}
	pcmData := bytesToInt16s(file)
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
