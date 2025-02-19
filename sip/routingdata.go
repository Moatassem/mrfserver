package sip

import "net"

type RoutingData struct {
	NoAnswerTimeout uint16 //in seconds
	No18xTimeout    uint16 //in seconds
	MaxCallDuration uint16 //in seconds

	RURIUsername string
	RemoteUDP    *net.UDPAddr
	// Routes
}
