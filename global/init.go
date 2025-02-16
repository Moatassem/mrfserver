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

package global

import (
	"MRFGo/rtp"
	"MRFGo/rtp/dtmf"
	"encoding/binary"
	"sync"
)

func InitializeEngine() {
	responsesHeadersInit()

	BufferPool = newSyncPool(BufferSize, BufferSize)

	rtpsz := RTPHeaderSize + RTPPayloadSize
	RTPRXBufferPool = newSyncPool(rtpsz, rtpsz)
	RTPTXBufferPool = newSyncPool(0, rtpsz)

	IsSystemBigEndian = checkSystemIndian()
	rtp.InitializeTX()
	dtmf.Initialize(SamplingRate)
}

func newSyncPool(bsz, csz int) *sync.Pool {
	return &sync.Pool{
		New: func() any {
			return make([]byte, bsz, csz)
		},
	}
}

func checkSystemIndian() bool {
	var i int32 = 0x01020304
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, uint32(i))
	return i == int32(binary.BigEndian.Uint32(buf))
}
