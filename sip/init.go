package sip

import (
	"fmt"
	"log"
	"mrfgo/cl"
	"mrfgo/global"
	"net"
	"os"
	"runtime"
	"strings"
)

var (
	Sessions ConcurrentMapMutex
)

func StartServer(ipv4 string, sup, htp int) *net.UDPConn {
	fmt.Print("Initializing Global Parameters...")
	Sessions = NewConcurrentMapMutex()

	global.SipUdpPort = sup
	global.HttpTcpPort = htp

	global.InitializeEngine()
	fmt.Println("Ready!")

	fmt.Printf("Loading files in directory: %s\n", global.MediaPath)
	MRFRepos = NewMRFRepoCollection(global.MRFRepoName)
	fmt.Printf("Audio files count: %d \n", MRFRepos.FilesCount(global.MRFRepoName))

	triedAlready := false
tryAgain:
	fmt.Print("Attempting to listen on SIP...")
	serverUDPListener, err := global.StartListening(global.ServerIPv4, global.SipUdpPort)
	if err != nil {
		if triedAlready {
			fmt.Println(err)
			os.Exit(2)
		}
		fmt.Printf("Error: %s\n", err)
		if opErr, ok := err.(*net.OpError); ok && strings.Contains(opErr.Error(), "bind") {
			global.ServerIPv4 = getlocalIPv4()
			triedAlready = true
			goto tryAgain
		}
	}
	MediaPorts = NewMediaPortPool()

	startWorkers(serverUDPListener)
	udpLoopWorkers(serverUDPListener)
	fmt.Println("Success: UDP", serverUDPListener.LocalAddr().String())

	fmt.Print("Setting Rate Limiter...")
	global.CallLimiter = cl.NewCallLimiter(global.RateLimit, global.Prometrics, &global.WtGrp)
	fmt.Printf("OK (%d)\n", global.RateLimit)

	return serverUDPListener
}

func getlocalIPv4() net.IP {
	fmt.Print("Checking Interfaces...")
	serverIPs, err := global.GetLocalIPs()
	if err != nil {
		fmt.Println("Failed to find an IPv4 interface:", err)
		os.Exit(1)
	}
	var serverIP net.IP
	if len(serverIPs) == 1 {
		serverIP = serverIPs[0]
		fmt.Println("Found:", serverIP)
	} else {
		var idx int
		for {
			fmt.Printf("Found (%d) interfaces:\n", len(serverIPs))
			for i, s := range serverIPs {
				fmt.Printf("%d- %s\n", i+1, s.String())
			}
			fmt.Print("Your choice:? ")
			n, err := fmt.Scanln(&idx)
			if n == 0 {
				log.Panic("no proper interface selected")
			}
			if idx <= 0 || idx > len(serverIPs) {
				fmt.Println("Invalid interface selected")
				continue
			}
			if err == nil {
				break
			}
			fmt.Println(err)
		}
		serverIP = serverIPs[idx-1]
		fmt.Println("Selected:", serverIP)
	}
	return serverIP
}

// =================================================================================================
// Worker Pattern

var (
	WorkerCount = runtime.NumCPU()
	QueueSize   = 500
	packetQueue = make(chan Packet, QueueSize)
)

type Packet struct {
	sourceAddr *net.UDPAddr
	buffer     *[]byte
	bytesCount int
}

func startWorkers(conn *net.UDPConn) {
	// Start worker pool
	global.WtGrp.Add(WorkerCount)
	for i := 0; i < WorkerCount; i++ {
		go worker(i, conn, packetQueue)
	}
}

func udpLoopWorkers(conn *net.UDPConn) {
	global.WtGrp.Add(1)
	defer func() {
		global.WtGrp.Done()
		if r := recover(); r != nil {
			global.LogCallStack(r)
			udpLoopWorkers(conn)
		}
	}()
	go func() {
		for {
			buf := global.BufferPool.Get().(*[]byte)
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
	defer global.WtGrp.Done()
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
			ss.SIPUDPListenser = conn
		}
		sipStack(msg, ss, newSesType)
		pdu = pdutmp
	}
	global.BufferPool.Put(packet.buffer)
}
