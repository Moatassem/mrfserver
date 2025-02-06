/*
# Software Name : Session Router (SR)
# SPDX-FileCopyrightText: Copyright (c) Orange Business - OINIS/Services/NSF
# SPDX-License-Identifier: Apache-2.0
#
# This software is distributed under the Apache-2.0
# See the "LICENSES" directory for more details.
#
# Authors:
# - Moatassem Talaat <moatassem.talaat@orange.com>

---
*/

package sip

import (
	"SRGo/cl"
	. "SRGo/global"
	"SRGo/phone"
	"fmt"
	"net"
	"os"
	"runtime"
	"time"
)

var (
	Sessions    ConcurrentMapMutex
	ASUserAgent *SipUdpUserAgent
	SkipAS      bool
)

func StartServer(asUdpskt *net.UDPAddr, ipv4 string, sup, htp int) (*net.UDPConn, net.IP) {
	fmt.Print("Initializing Global Parameters...")
	Sessions = NewConcurrentMapMutex()

	SkipAS = asUdpskt == nil
	ASUserAgent = NewSipUdpUserAgent(asUdpskt)

	SipUdpPort = sup
	HttpTcpPort = htp

	InitializeEngine()
	fmt.Println("Ready!")

	// fmt.Print("Checking Interfaces...")
	// serverIPs, err := GetLocalIPs()
	// if err != nil {
	// 	fmt.Println("Failed to find an IPv4 interface:", err)
	// 	os.Exit(1)
	// }
	// var serverIP net.IP
	// if len(serverIPs) == 1 {
	// 	serverIP = serverIPs[0]
	// 	fmt.Println("Found:", serverIP)
	// } else {
	// 	var idx int
	// 	for {
	// 		fmt.Printf("Found (%d) interfaces:\n", len(serverIPs))
	// 		for i, s := range serverIPs {
	// 			fmt.Printf("%d- %s\n", i+1, s.String())
	// 		}
	// 		fmt.Print("Your choice:? ")
	// 		n, err := fmt.Scanln(&idx)
	// 		if n == 0 {
	// 			log.Panic("no proper interface selected")
	// 		}
	// 		if idx <= 0 || idx > len(serverIPs) {
	// 			fmt.Println("Invalid interface selected")
	// 			continue
	// 		}
	// 		if err == nil {
	// 			break
	// 		}
	// 		fmt.Println(err)
	// 	}
	// 	serverIP = serverIPs[idx-1]
	// 	fmt.Println("Selected:", serverIP)
	// }

	serverIP := net.ParseIP(ipv4)

	fmt.Print("Attempting to listen on SIP...")
	serverUDPListener, err := StartListening(serverIP, SipUdpPort)
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
	startWorkers(serverUDPListener)
	udpLoopWorkers(serverUDPListener)
	fmt.Println("Success: UDP", serverUDPListener.LocalAddr().String())

	// starting probing loop
	go periodicUAProbing(serverUDPListener)

	fmt.Print("Setting Rate Limiter...")
	CallLimiter = cl.NewCallLimiter(RateLimit, Prometrics, &WtGrp)
	fmt.Printf("OK (%d)\n", RateLimit)

	return serverUDPListener, serverIP
}

// =================================================================================================
// Worker Pattern

var (
	WorkerCount = runtime.NumCPU()
	QueueSize   = 1000
	packetQueue = make(chan Packet, QueueSize)
)

type Packet struct {
	sourceAddr *net.UDPAddr
	buffer     *[]byte
	bytesCount int
}

func startWorkers(conn *net.UDPConn) {
	// Start worker pool
	WtGrp.Add(WorkerCount)
	for i := 0; i < WorkerCount; i++ {
		go worker(i, conn, packetQueue)
	}
}

func udpLoopWorkers(conn *net.UDPConn) {
	WtGrp.Add(1)
	defer func() {
		WtGrp.Done()
		if r := recover(); r != nil {
			LogCallStack(r)
			udpLoopWorkers(conn)
		}
	}()
	go func() {
		for {
			buf := BufferPool.Get().(*[]byte)
			n, addr, err := conn.ReadFromUDP(*buf)
			if err != nil {
				fmt.Println(err)
				continue
			}
			// Enqueue the packet
			packetQueue <- Packet{sourceAddr: addr, buffer: buf, bytesCount: n}
		}
	}()
}

func worker(id int, conn *net.UDPConn, queue <-chan Packet) {
	defer WtGrp.Done()
	for packet := range queue {
		// TODO use the id to log the worker id
		_ = id
		// fmt.Printf("Worker %d processing packet from %s\n", id, packet.SourceAddr)
		processPacket(packet, conn)
	}
}

func processPacket(packet Packet, conn *net.UDPConn) {
	pdu := (*packet.buffer)[:packet.bytesCount]
	for {
		if len(pdu) == 0 {
			break
		}
		msg, pdutmp, err := processPDU(pdu)
		if err != nil {
			fmt.Println("Bad PDU -", err)
			fmt.Println(string(pdu))
			break
		} else if msg == nil {
			break
		}
		ss, newSesType := sessionGetter(msg)
		if ss != nil {
			ss.RemoteUDP = packet.sourceAddr
			ss.UDPListenser = conn
		}
		sipStack(msg, ss, newSesType)
		pdu = pdutmp
	}
	BufferPool.Put(packet.buffer)
}

func periodicUAProbing(conn *net.UDPConn) {
	WtGrp.Add(1)
	defer WtGrp.Done()
	ticker := time.NewTicker(5 * time.Second)
	for range ticker.C {
		ProbeUA(conn, ASUserAgent)
		for _, phne := range phone.Phones.All() {
			if phne.IsReachable && phne.IsRegistered {
				ProbeUA(conn, phne.UA)
			}
		}
	}
}
