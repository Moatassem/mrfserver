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
	global.LogInfo(global.LTSystem, fmt.Sprintf("Welcome to %s - Product of %s 2025", global.B2BUAName, global.ASCIIPascal(global.EntityName)))
}

func checkArgs() (string, int, int) {
	ipv4, ok := os.LookupEnv(OwnIPv4)
	if !ok {
		global.LogWarning(global.LTConfiguration, "No self IPv4 address provided - First available shall be used")
	}

	global.ServerIPv4 = net.ParseIP(ipv4)

	sup, ok := os.LookupEnv(OwnSIPUdpPort)
	minS := 4999
	maxS := 6000

	var sipuport, httpport int

	if !ok {
		global.LogWarning(global.LTConfiguration, fmt.Sprintf("No self SIP UDP port provided - %d shall be used", global.DefaultSipPort))
		sipuport = global.DefaultSipPort
	} else {
		sipuport, ok = global.Str2IntDefaultMinMax(sup, global.DefaultSipPort, minS, maxS)
		if !ok {
			global.LogWarning(global.LTConfiguration, "Invalid SIP UDP port: "+sup)
		}
	}

	hp, ok := os.LookupEnv(OwnHttpPort)

	if !ok {
		global.LogWarning(global.LTConfiguration, fmt.Sprintf("No self HTTP port provided - %d shall be used", global.DefaultHttpPort))
		httpport = global.DefaultHttpPort
	} else {
		minH := 79
		maxH := 9999
		httpport, ok = global.Str2IntDefaultMinMax(hp, global.DefaultHttpPort, minH, maxH)

		if !ok {
			global.LogWarning(global.LTConfiguration, "Invalid HTTP port: "+hp)
		}
	}

	mp, ok := os.LookupEnv(MediaDirectory)
	if ok {
		global.MediaPath = mp
	} else {
		global.LogError(global.LTConfiguration, "No media directory provided!")
		os.Exit(1)
	}

	return ipv4, sipuport, httpport
}
