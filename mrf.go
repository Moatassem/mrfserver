package main

import (
	"fmt"
	"mrfgo/global"
	"mrfgo/prometheus"
	"mrfgo/sip"
	"mrfgo/webserver"
	"net"
	"os"
)

// environment variables
//
//nolint:revive
const (
	OwnIPv4       string = "server_ipv4"
	OwnSIPUdpPort string = "sip_udp_port"
	//nolint:stylecheck
	OwnHttpPort    string = "http_port"
	MediaDirectory string = "media_dir"
)

func main() {
	greeting()

	global.Prometrics = prometheus.NewMetrics(global.B2BUAName)
	conn := sip.StartServer(checkArgs())

	defer conn.Close() // close SIP server connection

	webserver.StartWS(global.ServerIPv4)
	global.WtGrp.Wait()
}

func greeting() {
	global.LogInfo(global.LTSystem, fmt.Sprintf("Welcome to %s - Product of %s 2025\n", global.B2BUAName, global.ASCIIPascal(global.EntityName)))
}

func checkArgs() (string, int, int) {
	ipv4, ok := os.LookupEnv(OwnIPv4)
	if !ok {
		global.LogError(global.LTConfiguration, "No self IPv4 address provided!")
		os.Exit(1)
	}

	global.ServerIPv4 = net.ParseIP(ipv4)

	sup := os.Getenv(OwnSIPUdpPort)
	minS := 4999
	maxS := 6000
	sipuport, ok := global.Str2IntDefaultMinMax(sup, global.DefaultSipPort, minS, maxS)

	if !ok {
		global.LogError(global.LTConfiguration, "Invalid SIP UDP port: "+sup)
		os.Exit(1)
	}

	hp := os.Getenv(OwnHttpPort)
	minH := 79
	maxH := 9999
	httpport, ok := global.Str2IntDefaultMinMax(hp, global.DefaultHttpPort, minH, maxH)

	if !ok {
		global.LogError(global.LTConfiguration, "Invalid HTTP port: "+hp)
		os.Exit(1)
	}

	mp, ok := os.LookupEnv(MediaDirectory)
	if ok {
		global.MediaPath = mp
	} else {
		global.LogWarning(global.LTConfiguration, "No media directory provided!")
		os.Exit(1)
	}

	return ipv4, sipuport, httpport
}
