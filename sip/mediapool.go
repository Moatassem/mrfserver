package sip

import (
	"fmt"
	"log"
	"mrfgo/global"
	"net"
	"sync"
)

var MediaPorts *MediaPool

type MediaPool struct {
	mu    sync.Mutex
	alloc map[int]bool
}

func NewMediaPortPool() *MediaPool {
	mpp := &MediaPool{alloc: make(map[int]bool, global.MediaEndPort-global.MediaStartPort+1)}
	for port := global.MediaStartPort; port <= global.MediaEndPort; port++ {
		mpp.alloc[port] = false
	}
	return mpp
}

func (mpp *MediaPool) ReserveSocket() *net.UDPConn {
	mpp.mu.Lock()
	defer mpp.mu.Unlock()
	for port, inUse := range mpp.alloc {
		if !inUse {
			socket, err := global.StartListening(global.ServerIPv4, port)
			if err != nil {
				continue
			}
			mpp.alloc[port] = true
			return socket
		}
	}
	log.Printf("No available ports for IPv4 %s\n", global.ServerIPv4)
	return nil
}

func (mpp *MediaPool) ReleaseSocket(conn *net.UDPConn) bool {
	if conn == nil {
		return true
	}
	port := global.GetUDPortFromConn(conn)
	conn.Close()
	mpp.mu.Lock()
	defer mpp.mu.Unlock()
	if mpp.alloc[port] {
		mpp.alloc[port] = false
		return true
	}
	global.LogWarning(global.LTMediaStack, fmt.Sprintf("Port [%d] already released!\n", port))
	return false
}
