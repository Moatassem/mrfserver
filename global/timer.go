package global

import (
	"time"
)

type SipTimer struct {
	DoneCh chan bool
	Tmr    *time.Timer
}
