package sip

import "MRFGo/global"

// -------------------------------------------

type SipStartLine struct {
	global.Method
	UriScheme      string
	UserPart       string
	HostPart       string
	UserParameters *map[string]string
	Password       string

	StatusCode   int
	ReasonPhrase string

	Ruri      string
	StartLine string //only set for incoming messages - to be removed!!!

	UriParameters *map[string]string
}

type RequestPack struct {
	global.Method
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
func NewResponsePackSRW(sipc int, warning string, reason string) ResponsePack {
	var hdrs SipHeaders
	if reason == "" {
		hdrs = NewSHQ850OrSIP(sipc, warning, "")
	} else {
		hdrs = NewSHQ850OrSIP(0, warning, "")
		hdrs.SetHeader(global.Reason, reason)
	}
	return ResponsePack{
		StatusCode:    sipc,
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

func NewResponsePackWarning(sipc int, warning string) ResponsePack {
	hdrs := NewSHQ850OrSIP(0, warning, "")
	return ResponsePack{
		StatusCode:    sipc,
		CustomHeaders: hdrs,
	}
}
