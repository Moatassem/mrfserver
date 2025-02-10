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

package numtype

type NumberType int

const (
	CalledRURI NumberType = iota
	CalledTo
	CalledBoth
	CallingFrom
	CallingPAI
	CallingBoth
)
