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
	. "SRGo/global"
)

// -------------------------------------------

type SipStartLine struct {
	Method
	UriScheme      string
	UserPart       string
	HostPart       string
	UserParameters *map[string]string

	StatusCode   int
	ReasonPhrase string

	Ruri      string
	StartLine string //only set for incoming messages - to be removed!!!

	UriParameters *map[string]string
}

type RequestPack struct {
	Method
	RUriUP        string
	FromUP        string
	Max70         bool
	CustomHeaders SipHeaders
	IsProbing     bool
}

type ResponsePack struct {
	StatusCode    int
	ReasonPhrase  string
	ContactHeader string

	CustomHeaders SipHeaders

	LinkedPRACKST  *Transaction
	PRACKRequested bool
}

func NewResponsePackRFWarning(stc int, rsnphrs, warning string) ResponsePack {
	return ResponsePack{
		StatusCode:    stc,
		ReasonPhrase:  rsnphrs,
		CustomHeaders: NewSHQ850OrSIP(0, warning, ""),
	}
}

// reason != "" ==> Warning & Reason headers are always created.
//
// reason == "" ==>
//
// stc == 0 ==> only Warning header
//
// stc != 0 ==> only Reason header
func NewResponsePackSRW(stc int, warning string, reason string) ResponsePack {
	var hdrs SipHeaders
	if reason == "" {
		hdrs = NewSHQ850OrSIP(stc, warning, "")
	} else {
		hdrs = NewSHQ850OrSIP(0, warning, "")
		hdrs.SetHeader(Reason, reason)
	}
	return ResponsePack{
		StatusCode:    stc,
		CustomHeaders: hdrs,
	}
}

func NewResponsePackSIPQ850Details(sipc, q850c int, details string) ResponsePack {
	hdrs := NewSHQ850OrSIP(q850c, details, "")
	return ResponsePack{
		StatusCode:    sipc,
		CustomHeaders: hdrs,
	}
}
