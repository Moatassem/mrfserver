package sip

import (
	"MRFGo/global"
	"fmt"
	"log"
	"net"
	"sync"
	"time"
)

var MediaPorts *MediaPool

type MediaPool struct {
	mu    sync.Mutex
	alloc map[int]bool
	used  []int
}

const checkUsedUDPPortIntervalSec int = 10

func NewMediaPortPool() *MediaPool {
	mpp := &MediaPool{alloc: make(map[int]bool, global.MediaEndPort-global.MediaStartPort+1)}
	for port := global.MediaStartPort; port <= global.MediaEndPort; port++ {
		mpp.alloc[port] = false
	}
	go mpp.checkUsedPorts()
	return mpp
}

func (mpp *MediaPool) checkUsedPorts() {
	tmr := time.NewTicker(time.Duration(checkUsedUDPPortIntervalSec) * time.Second)
	defer tmr.Stop()
	for {
		<-tmr.C
		i := 0
		mpp.mu.Lock()
		for {
			if i >= len(mpp.used) {
				break
			}
			port := mpp.used[i]
			addr := fmt.Sprintf("%s:%d", global.ServerIPv4, port)
			conn, err := net.ListenPacket("udp", addr)
			if err != nil {
				i++
				continue
			}
			conn.Close()
			mpp.alloc[port] = false
			mpp.used = removeAt(mpp.used, i)
		}
		mpp.mu.Unlock()
	}
}

func removeAt(slice []int, i int) []int {
	slice[i] = slice[len(slice)-1] // Move last element to index i
	return slice[:len(slice)-1]    // Trim the last element
}

func (mpp *MediaPool) ReserveSocket() *net.UDPConn {
	mpp.mu.Lock()
	defer mpp.mu.Unlock()
	for port, inUse := range mpp.alloc {
		if !inUse {
			socket, err := global.StartListening(global.ServerIPv4, port)
			mpp.alloc[port] = true
			if err != nil {
				mpp.used = append(mpp.used, port)
				continue
			}
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
	log.Printf("Port [%d] already released!\n", port)
	return false
}
