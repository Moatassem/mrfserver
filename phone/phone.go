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

package phone

import (
	"SRGo/global"
	"SRGo/sip/state"
	"fmt"
	"log"
	"net"
	"sync"
)

var Phones *IPPhoneRepo = NewIPPhoneRepo()

type IPPhoneRepo struct {
	mu     sync.RWMutex
	phones map[string]*IPPhone
}

func NewIPPhoneRepo() *IPPhoneRepo {
	return &IPPhoneRepo{phones: make(map[string]*IPPhone)}
}

func (r *IPPhoneRepo) IsPhoneExt(ext string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.phones[ext]
	return ok
}

func (r *IPPhoneRepo) Get(ext string) (*IPPhone, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	phone, ok := r.phones[ext]
	return phone, ok
}

func (r *IPPhoneRepo) AddOrUpdate(ext, ruri, ipport string, expires int) state.SessionState {
	r.mu.Lock()
	defer r.mu.Unlock()
	phone, ok := r.phones[ext]
	if !ok {
		phone = &IPPhone{Extension: ext, RURI: ruri}
		r.phones[ext] = phone
	}
	if phone.UA != nil && phone.UA.UDPAddr.String() == ipport {
		goto finish
	}
	{
		phone.IsReachable = true
		udpaddr, err := net.ResolveUDPAddr("udp", ipport)
		if err != nil {
			log.Printf("Error resolving UDP address: %v\n", err)
			phone.IsReachable = false
			goto finish
		}
		phone.UA = global.NewSipUdpUserAgent(udpaddr)
	}
finish:
	phone.IsRegistered = expires > 0
	log.Printf("IPPhone: [%s]\n", phone)
	if phone.IsRegistered {
		return state.Registered
	}
	return state.Unregistered
}

func (r *IPPhoneRepo) Remove(ext string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.phones, ext)
}

func (r *IPPhoneRepo) All() map[string]*IPPhone {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.phones
}

// =================================================================================================

type IPPhone struct {
	Extension    string
	RURI         string
	UA           *global.SipUdpUserAgent
	IsReachable  bool
	IsRegistered bool
}

func (p *IPPhone) String() string {
	return fmt.Sprintf(`Extension: %s, RURI: %s, IsRegistered: %t`, p.Extension, p.RURI, p.IsRegistered)
}
