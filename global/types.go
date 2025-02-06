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

package global

import (
	"fmt"
	"net"
)

type SystemError struct {
	Code    int
	Details string
}

func NewError(code int, details string) error {
	return &SystemError{Code: code, Details: details}
}

func (se *SystemError) Error() string {
	return fmt.Sprintf("Code: %d - Details: %s", se.Code, se.Details)
}

type SipUdpUserAgent struct {
	UDPAddr *net.UDPAddr
	IsAlive bool
}

func NewSipUdpUserAgent(udpAddr *net.UDPAddr) *SipUdpUserAgent {
	if udpAddr == nil {
		return nil
	}
	return &SipUdpUserAgent{UDPAddr: udpAddr}
}
