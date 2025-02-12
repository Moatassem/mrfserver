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
	. "MRFGo/global"
)

type MessageBody struct {
	PartsContents map[BodyType]ContentPart //used to store incoming/outgoing body parts
	MessageBytes  []byte                   //used to store the generated body bytes for sending msgs
}

type ContentPart struct {
	Headers SipHeaders
	Bytes   []byte
}

func EmptyBody() MessageBody {
	var mb MessageBody
	return mb
}

func NewMessageBody(init bool) *MessageBody {
	if init {
		return &MessageBody{PartsContents: make(map[BodyType]ContentPart)}
	}
	return new(MessageBody)
}

func NewMessageSDPBody(sdpbytes []byte) MessageBody {
	mb := MessageBody{PartsContents: make(map[BodyType]ContentPart)}
	mb.PartsContents[SDP] = ContentPart{Bytes: sdpbytes}
	return mb
}

func NewContentPart(bt BodyType, bytes []byte) ContentPart {
	var ct ContentPart
	ct.Bytes = bytes
	ct.Headers = NewSipHeaders()
	ct.Headers.AddHeader(Content_Type, DicBodyContentType[bt])
	return ct
}

// ===============================================================

func NewMSCXML(xml string) MessageBody {
	hdrs := NewSipHeaders()
	hdrs.AddHeader(Content_Length, DicBodyContentType[MSCXML])
	return MessageBody{PartsContents: map[BodyType]ContentPart{MSCXML: {hdrs, []byte(xml)}}}
}

func NewJSON(jsonbytes []byte) MessageBody {
	hdrs := NewSipHeaders()
	hdrs.AddHeader(Content_Length, DicBodyContentType[AppJson])
	return MessageBody{PartsContents: map[BodyType]ContentPart{AppJson: {hdrs, jsonbytes}}}
}

func NewInData(binbytes []byte) MessageBody {
	hdrs := NewSipHeaders()
	hdrs.AddHeader(Content_Length, DicBodyContentType[VndOrangeInData])
	return MessageBody{PartsContents: map[BodyType]ContentPart{AppJson: {hdrs, binbytes}}}
}

// ===============================================================

func (messagebody *MessageBody) WithNoBody() bool {
	return messagebody.PartsContents == nil
}

func (messagebody *MessageBody) WithUnknownBodyPart() bool {
	if messagebody.WithNoBody() {
		return false
	}
	if len(messagebody.PartsContents) == 0 {
		return true
	}
	for k := range messagebody.PartsContents {
		if k == Unknown {
			return true
		}
	}
	return false
}

func (messagebody *MessageBody) IsMultiPartBody() bool {
	if messagebody.WithNoBody() {
		return false
	}
	return len(messagebody.PartsContents) >= 2
}

func (messagebody *MessageBody) ContainsSDP() bool {
	if messagebody.WithNoBody() {
		return false
	}
	_, ok := messagebody.PartsContents[SDP]
	return ok
}

func (messagebody *MessageBody) IsJSON() bool {
	if messagebody.WithNoBody() {
		return false
	}
	_, ok := messagebody.PartsContents[AppJson]
	return ok
}

func (messagebody *MessageBody) ContentLength() int {
	return len(messagebody.MessageBytes)
}
