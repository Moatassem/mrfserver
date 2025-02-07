package ivr

import (
	"SRGo/global"
	"bytes"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/hajimehoshi/go-mp3"
)

type IVRs struct {
	mu  sync.RWMutex
	lst map[string]*[]byte
}

var IVRsRepo *IVRs

func NewIVRs() *IVRs {
	var ivrs IVRs
	ivrs.lst = *loadMedia()
	return &ivrs
}

func loadMedia() *map[string]*[]byte {
	lst := make(map[string]*[]byte)

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
		audioBytes, err := os.ReadFile(fullpath)
		if err != nil {
			fmt.Printf("Error reading file %s: %v", fullpath, err)
			continue
		}
		// Decode MP3 to PCM
		decoder, err := mp3.NewDecoder(bytes.NewReader(audioBytes))
		if err != nil {
			fmt.Println("Error decoding mp3 file", err)
			continue
		}

		pcmBytes := make([]byte, 6217800)
		_, err = decoder.Read(pcmBytes)
		if err != nil {
			panic(err)
		}
		lst[dentry.Name()] = &pcmBytes
	}

	return &lst
}

func (r *IVRs) Get(key string) (*[]byte, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	ivr, ok := r.lst[key]
	return ivr, ok
}

func (r *IVRs) AddOrUpdate(key string, ivr *[]byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lst[key] = ivr
}
