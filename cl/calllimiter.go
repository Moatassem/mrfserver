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

package cl

import (
	"SRGo/prometheus"
	"sync"
	"time"
)

type CallLimiter struct {
	rate      int          // rate limiter
	ticker    *time.Ticker // ticker for timing
	callCount int          // current call count
	mu        sync.Mutex   // mutex for thread safety
}

func NewCallLimiter(rate int, pm *prometheus.Metrics, wg *sync.WaitGroup) *CallLimiter {
	cl := &CallLimiter{
		rate:   rate,
		ticker: time.NewTicker(time.Second),
	}
	wg.Add(1)
	go cl.resetCount(pm, wg)
	return cl
}

func (clmtr *CallLimiter) resetCount(pm *prometheus.Metrics, wg *sync.WaitGroup) {
	defer wg.Done()
	for range clmtr.ticker.C {
		clmtr.mu.Lock()
		pm.Caps.Set(float64(clmtr.callCount))
		clmtr.callCount = 0
		clmtr.mu.Unlock()
	}
}

func (clmtr *CallLimiter) AcceptNewCall() bool {
	clmtr.mu.Lock()
	defer clmtr.mu.Unlock()
	if clmtr.rate == -1 || clmtr.callCount < clmtr.rate {
		clmtr.callCount++
		return true // Call can be attempted
	}
	return false // Rate limit exceeded
}
