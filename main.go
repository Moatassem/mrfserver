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

package main

import (
	"SRGo/global"
	"SRGo/prometheus"
	"SRGo/sip"
	"SRGo/webserver"
	"fmt"
	"net"
	"os"
)

// environment variables
const (
	Own_IP_IPv4     string = "server_ipv4"
	Own_SIP_UdpPort string = "sip_udp_port"
	Own_Http_Port   string = "http_port"
	Media_Path      string = "media_path"
)

// use this for initializations
func init() {
}

func main() {
	greeting()
	global.Prometrics = prometheus.NewMetrics(global.B2BUAName)
	conn := sip.StartServer(checkArgs())
	defer conn.Close() //close SIP server connection
	webserver.StartWS(global.ServerIPv4)
	global.WtGrp.Wait()
}

func greeting() {
	fmt.Printf("Welcome to %s - Product of %s 2025\n", global.B2BUAName, global.ASCIIPascal(global.EntityName))
}

func checkArgs() (ipv4 string, sipuport, httpport int) {
	ipv4, ok := os.LookupEnv(Own_IP_IPv4)
	if !ok {
		fmt.Println("No self IPv4 address provided!")
		os.Exit(1)
	}
	global.ServerIPv4 = net.ParseIP(ipv4)

	sup := os.Getenv(Own_SIP_UdpPort)
	sipuport, _ = global.Str2IntDefaultMinMax(sup, 5060, 4999, 6000)

	hp := os.Getenv(Own_Http_Port)
	httpport, _ = global.Str2IntDefaultMinMax(hp, 8080, 79, 9999)

	mp, ok := os.LookupEnv(Media_Path)
	if ok {
		global.MediaPath = mp
	} else {
		fmt.Println("No media directory provided!")
		os.Exit(2)
	}

	return
}
