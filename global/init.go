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
	"sync"
)

func InitializeEngine() {
	responsesHeadersInit()
	BufferPool = newSyncPool()

}

func newSyncPool() *sync.Pool {
	return &sync.Pool{
		New: func() any {
			b := make([]byte, BufferSize)
			return &b
		},
	}
}
