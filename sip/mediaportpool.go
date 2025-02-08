package sip

import (
	"SRGo/global"
	"log"
	"sync"
)

var MediaPorts *MediaPool

type MediaSocket struct {
	IPv4 string
	Port int
}

type MediaPool struct {
	mu    sync.Mutex
	alloc map[string]map[int]bool
}

func NewMediaPortPool() *MediaPool {
	mpp := &MediaPool{alloc: make(map[string]map[int]bool, 1)}
	mpp.alloc[global.ServerIPv4] = make(map[int]bool, global.MediaEndPort-global.MediaStartPort+1)
	for port := global.MediaStartPort; port <= global.MediaEndPort; port++ {
		mpp.alloc[global.ServerIPv4][port] = false
	}
	return mpp
}

func (mpp *MediaPool) ReserveSocket(ipv4 string) *MediaSocket {
	mpp.mu.Lock()
	defer mpp.mu.Unlock()
	if ports, exists := mpp.alloc[ipv4]; exists {
		for port, inUse := range ports {
			if !inUse {
				mpp.alloc[ipv4][port] = true
				return &MediaSocket{IPv4: ipv4, Port: port}
			}
		}
	}
	log.Printf("No available ports for IPv4 %s\n", ipv4)
	return nil
}

func (mpp *MediaPool) ReleaseSocket(ms *MediaSocket) bool {
	if ms == nil {
		return true
	}
	mpp.mu.Lock()
	defer mpp.mu.Unlock()
	if _, exists := mpp.alloc[ms.IPv4]; exists {
		if mpp.alloc[ms.IPv4][ms.Port] {
			mpp.alloc[ms.IPv4][ms.Port] = false
			return true
		}
	}
	log.Printf("Port [%d] already released!\n", ms.Port)
	return false
}
