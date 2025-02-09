package sip

import (
	"SRGo/global"
	"SRGo/rtp"
	"fmt"
	"os"
	"path"
	"sync"
)

const UPart string = "999"

var MRFRepos *MRFRepoCollection

type MRFRepoCollection struct {
	mu   sync.RWMutex
	maps *map[string]map[string]*[]byte
}

func NewMRFRepoCollection() *MRFRepoCollection {
	var ivrs MRFRepoCollection
	ivrs.maps = loadMedia()
	return &ivrs
}

func loadMedia() *map[string]map[string]*[]byte {
	mp := make(map[string]map[string]*[]byte)
	mp[UPart] = make(map[string]*[]byte)

	dentries, err := os.ReadDir(global.MediaPath)
	if err != nil {
		panic(err)
	}
	for _, dentry := range dentries {
		if dentry.IsDir() {
			continue
		}
		fullpath := path.Join(global.MediaPath, dentry.Name())
		fmt.Println(fullpath)
		// audioBytes, err := os.ReadFile(fullpath)
		// if err != nil {
		// 	fmt.Printf("Error reading file %s: %v", fullpath, err)
		// 	continue
		// }

		pcmBytes, err := rtp.Mp3ToPcm(fullpath)
		if err != nil {
			continue
		}

		// codec := "G722"

		// output := make([]byte, len(pcmBytes))
		// for i, sample := range pcmBytes {
		// 	switch codec {
		// 	case "PCMA":
		// 		output[i] = rtp.PCMToALaw(sample)
		// 	case "PCMU":
		// 		output[i] = rtp.PCMToMuLaw(sample)
		// 	}
		// }
		output := rtp.PCM2G722(pcmBytes)

		// // Decode MP3 to PCM
		// decoder, err := mp3.NewDecoder(bytes.NewReader(audioBytes))
		// if err != nil {
		// 	fmt.Println("Error decoding mp3 file", err)
		// 	continue
		// }

		// pcmBytes := make([]byte, 6217800)
		// _, err = decoder.Read(pcmBytes)
		// if err != nil {
		// 	panic(err)
		// }
		mp[UPart][dentry.Name()] = &output
	}

	return &mp
}

func (mrfr *MRFRepoCollection) DoesMRFRepoExist(upart string) bool {
	_, ok := (*mrfr.maps)[upart]
	return ok
}

func (mrfr *MRFRepoCollection) Get(upart, key string) (*[]byte, bool) {
	mrfr.mu.RLock()
	defer mrfr.mu.RUnlock()
	if mp, ok := (*mrfr.maps)[upart]; ok {
		ivr, ok := mp[key]
		return ivr, ok
	}
	return nil, false
}

func (mrfr *MRFRepoCollection) AddOrUpdate(upart, key string, bytes *[]byte) {
	mrfr.mu.Lock()
	defer mrfr.mu.Unlock()
	(*mrfr.maps)[upart][key] = bytes
}
