package sip

import (
	"fmt"
	"mrfgo/global"
	"mrfgo/rtp"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	ExtRaw string = "raw"
	ExtWav string = "wav"
	ExtMp3 string = "mp3"
)

var MRFRepos *MRFRepoCollection

type MRFRepo struct {
	name string
	mu   sync.RWMutex
	data map[string][]int16
}

type MRFRepoCollection struct {
	mu    sync.RWMutex
	repos map[string]*MRFRepo
}

func NewMRFRepoCollection(rn string) *MRFRepoCollection {
	var ivrs MRFRepoCollection
	ivrs.repos = loadMedia(rn)
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

func loadMedia(rn string) map[string]*MRFRepo {
	mrfrepos := make(map[string]*MRFRepo)
	mrfrepo := MRFRepo{name: rn, data: make(map[string][]int16)}
	mrfrepos[rn] = &mrfrepo

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
		duration := float64(len(pcmBytes)) / global.PcmSamplingRate

		fmt.Printf("Filename: %s, Duration: %s\n", filename, formattedTime(duration))

		mrfrepo.data[filenameonly] = pcmBytes
	}

	return mrfrepos
}

func formattedTime(totsec float64) string {
	duration := time.Duration(totsec * float64(time.Second))

	minutes := int(duration.Minutes())
	seconds := int(duration.Seconds()) % 60
	milliseconds := int(duration.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d.%03d", minutes, seconds, milliseconds)
}

func (mrfr *MRFRepoCollection) FilesCount(up string) int {
	mrfr.mu.RLock()
	defer mrfr.mu.RUnlock()
	mp, ok := mrfr.repos[up]
	if ok {
		return mp.FilesCount()
	}
	return -1
}

func (mrfrps *MRFRepoCollection) GetMRFRepo(upart string) (*MRFRepo, bool) {
	mrfrps.mu.RLock()
	defer mrfrps.mu.RUnlock()
	mrfrp, ok := mrfrps.repos[upart]
	return mrfrp, ok
}

func (mrfrps *MRFRepoCollection) Get(upart, key string) ([]int16, bool) {
	mrfrps.mu.RLock()
	defer mrfrps.mu.RUnlock()
	if mp, ok := mrfrps.repos[upart]; ok {
		return mp.Get(key)
	}
	return nil, false
}

func (mrfrp *MRFRepo) Get(key string) ([]int16, bool) {
	mrfrp.mu.RLock()
	defer mrfrp.mu.RUnlock()
	if pcm, ok := mrfrp.data[key]; ok {
		if len(pcm) == 0 {
			return nil, false
		}
		return pcm, ok
	}
	return nil, false
}

func (mrfrp *MRFRepo) FilesCount() int {
	mrfrp.mu.RLock()
	defer mrfrp.mu.RUnlock()
	return len(mrfrp.data)
}

// func (mrfr *MRFRepoCollection) AddOrUpdate(upart, key string, bytes []int16) {
// 	mrfr.mu.Lock()
// 	defer mrfr.mu.Unlock()
// 	mrfr.repos[upart][key] = bytes
// }
