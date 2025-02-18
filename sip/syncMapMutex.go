/*
# Software Name : Media Resource Function Server (SR)
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
	"MRFGo/global"
	"sync"
)

type ConcurrentMapMutex struct {
	_map map[string]*SipSession
	mu   sync.RWMutex
}

func NewConcurrentMapMutex() ConcurrentMapMutex {
	return ConcurrentMapMutex{_map: make(map[string]*SipSession)}
}

func (c *ConcurrentMapMutex) Store(ky string, ss *SipSession) (ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	ok = true
	if ss.Direction == global.INBOUND && !global.CallLimiter.AcceptNewCall() {
		ok = false
	}
	c._map[ky] = ss
	global.Prometrics.ConSessions.Inc()
	return
}

func (c *ConcurrentMapMutex) Delete(ky string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c._map, ky)
	global.Prometrics.ConSessions.Dec()
}

func (c *ConcurrentMapMutex) Load(ky string) (*SipSession, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	s, ok := c._map[ky]
	return s, ok
}

func (c *ConcurrentMapMutex) Range() map[string]*SipSession {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c._map
}

func (c *ConcurrentMapMutex) IsEmpty() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c._map) == 0
}
