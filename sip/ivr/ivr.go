package ivr

import "sync"

type IVRs struct {
	mu  sync.RWMutex
	lst map[string]*[]byte
}

var IVRsRepo *IVRs = NewIVRs()

func NewIVRs() *IVRs {
	return &IVRs{lst: make(map[string]*[]byte)}
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
