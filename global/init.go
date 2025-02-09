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
	"SRGo/rtp"
	"encoding/binary"
	"sync"
)

func InitializeEngine() {
	responsesHeadersInit()
	BufferPool = newSyncPool(BufferSize)
	MediaBufferPool = newSyncPool(MediaBufferSize)
	IsSystemBigEndian = checkSystemIndian()
	rtp.InitializeTX()
}

func newSyncPool(bsz int) *sync.Pool {
	return &sync.Pool{
		New: func() any {
			b := make([]byte, bsz)
			return &b
		},
	}
}

func checkSystemIndian() bool {
	var i int32 = 0x01020304
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(i))
	return i == int32(binary.BigEndian.Uint32(buf))
}
