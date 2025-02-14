package sip

import (
	"MRFGo/global"
	"MRFGo/rtp"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	UPart      string  = "999"
	sampleRate float64 = 16000

	ExtRaw string = "raw"
	ExtWav string = "wav"
	ExtMp3 string = "mp3"
)

var MRFRepos *MRFRepoCollection

type MRFRepoCollection struct {
	mu   sync.RWMutex
	maps map[string]map[string][]int16
}

func NewMRFRepoCollection() *MRFRepoCollection {
	var ivrs MRFRepoCollection
	ivrs.maps = loadMedia()
	return &ivrs
}

func dropExtension(fn string) string {
	idx := strings.LastIndex(fn, ".")
	if idx == -1 {
		return fn
	}
	return fn[:idx]
}

func getExtension(fn string) string {
	idx := strings.LastIndex(fn, ".")
	if idx == -1 {
		return "<no extension>"
	}
	return global.ASCIIToLower(fn[idx+1:])
}

func loadMedia() map[string]map[string][]int16 {
	mp := make(map[string]map[string][]int16)
	mp[UPart] = make(map[string][]int16)

	dentries, err := os.ReadDir(global.MediaPath)
	if err != nil {
		panic(err)
	}
	for _, dentry := range dentries {
		if dentry.IsDir() {
			continue
		}
		filename := dentry.Name()
		fullpath := filepath.Join(global.MediaPath, filename)

		var pcmBytes []int16
		var err error
		var rawpath string

		filenameonly := dropExtension(filename)

		switch ext := getExtension(filename); ext {
		case ExtRaw:
			pcmBytes, err = rtp.ReadPCMRaw(fullpath)
		case ExtWav, ExtMp3:
			rawpath, err = rtp.RunSox(global.MediaPath, filename, filenameonly)
			if err == nil {
				pcmBytes, err = rtp.ReadPCMRaw(rawpath)
			}
		default:
			fmt.Printf("Filename: %s - Unsupported Extension: %s - Skipped\n", filename, ext)
			continue
		}

		if err != nil {
			fmt.Println(err)
			continue
		}

		// Calculate duration -- TODO duration not accurate vs playback duration
		duration := float64(len(pcmBytes)) / sampleRate

		fmt.Printf("Filename: %s, Duration: %s\n", filename, formattedTime(duration))

		mp[UPart][filenameonly] = pcmBytes
	}

	return mp
}

func formattedTime(totsec float64) string {
	duration := time.Duration(totsec * float64(time.Second))

	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % 60
	milliseconds := int(duration.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, milliseconds)
}

func (mrfr *MRFRepoCollection) FilesCount() int {
	mrfr.mu.RLock()
	defer mrfr.mu.RUnlock()
	mp, ok := mrfr.maps[UPart]
	if ok {
		return len(mp)
	}
	return 0
}

func (mrfr *MRFRepoCollection) DoesMRFRepoExist(upart string) bool {
	mrfr.mu.RLock()
	defer mrfr.mu.RUnlock()
	_, ok := mrfr.maps[upart]
	return ok
}

func (mrfr *MRFRepoCollection) Get(upart, key string) ([]int16, bool) {
	mrfr.mu.RLock()
	defer mrfr.mu.RUnlock()
	if mp, ok := mrfr.maps[upart]; ok {
		ivr, ok := mp[key]
		if ivr == nil || len(ivr) == 0 {
			return nil, false
		}
		return ivr, ok
	}
	return nil, false
}

func (mrfr *MRFRepoCollection) AddOrUpdate(upart, key string, bytes []int16) {
	mrfr.mu.Lock()
	defer mrfr.mu.Unlock()
	mrfr.maps[upart][key] = bytes
}
