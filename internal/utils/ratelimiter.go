package utils

import (
	"time"
)

type RateLimiter struct {
	tick    *time.Ticker
	stopCh  chan struct{}
	tokenCh chan struct{}
}

func NewRateLimiter(ratePerSecond int) *RateLimiter {
	rl := &RateLimiter{
		tick:    time.NewTicker(time.Second / time.Duration(ratePerSecond)),
		stopCh:  make(chan struct{}),
		tokenCh: make(chan struct{}),
	}
	go rl.run()
	return rl
}

func (rl *RateLimiter) run() {
	defer close(rl.tokenCh)
	for {
		select {
		case <-rl.stopCh:
			return
		case <-rl.tick.C:
			rl.tokenCh <- struct{}{}
		}
	}
}

func (rl *RateLimiter) Stop() {
	close(rl.stopCh)
}

func (rl *RateLimiter) Wait() {
	<-rl.tokenCh
}
