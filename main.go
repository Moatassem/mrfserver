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
	AS_SIP_UdpIpPort string = "as_sip_udp"
	Own_IP_IPv4      string = "server_ipv4"
	Own_SIP_UdpPort  string = "sip_udp_port"
	Own_Http_Port    string = "http_port"
)

// use this for initializations
func init() {
}

func main() {
	greeting()
	global.Prometrics = prometheus.NewMetrics(global.B2BUAName)
	conn, ip := sip.StartServer(checkArgs())
	defer conn.Close() //close SIP server connection
	webserver.StartWS(ip)
	global.WtGrp.Wait()
}

func greeting() {
	fmt.Printf("Welcome to %s - Product of %s 2025\n", global.B2BUAName, global.ASCIIPascal(global.EntityName))
}

func checkArgs() (udpskt *net.UDPAddr, ipv4 string, sipuport, httpport int) {
	siplyr, ok := os.LookupEnv(AS_SIP_UdpIpPort)
	if !ok {
		fmt.Println("No AS address provided! - switching to built-in AS")
		goto skipAS
	}

	{
		var err error
		udpskt, err = global.GetUDPSocket(siplyr)
		if err != nil {
			os.Exit(1)
		}
		fmt.Printf("AS Routing: [%s]\n", siplyr)
	}

skipAS:
	ipv4, ok = os.LookupEnv(Own_IP_IPv4)
	if !ok {
		fmt.Println("No self IPv4 address provided!")
		os.Exit(1)
	}

	sup, ok := os.LookupEnv(Own_SIP_UdpPort)
	if ok {
		sipuport = global.Str2int[int](sup)
	} else {
		sipuport = 5060
	}

	hp, ok := os.LookupEnv(Own_Http_Port)
	if ok {
		httpport = global.Str2int[int](hp)
	} else {
		httpport = 8080
	}

	return
}
