package utils

import (
	"sync"
	"time"
)

type RateLimiter struct {
	tokensPerSecond int
	tokenCh         chan struct{}
	stopCh          chan struct{}
	wg              sync.WaitGroup
	mu              sync.RWMutex
	stopped         bool
}

func NewRateLimiter(tokensPerSecond int) *RateLimiter {
	if tokensPerSecond <= 0 {
		tokensPerSecond = 1
	}

	rl := &RateLimiter{
		tokensPerSecond: tokensPerSecond,
		tokenCh:         make(chan struct{}, tokensPerSecond),
		stopCh:          make(chan struct{}),
	}

	rl.wg.Add(1)
	go rl.run()

	// Initial fill of tokens
	for i := 0; i < tokensPerSecond; i++ {
		rl.tokenCh <- struct{}{}
	}

	return rl
}

func (rl *RateLimiter) run() {
	defer rl.wg.Done()

	interval := time.Second / time.Duration(rl.tokensPerSecond)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial fill
	for i := 0; i < rl.tokensPerSecond; i++ {
		select {
		case rl.tokenCh <- struct{}{}:
		case <-rl.stopCh:
			return
		}
	}

	for {
		select {
		case <-ticker.C:
			select {
			case rl.tokenCh <- struct{}{}:
			default:
				// Channel is full, skip
			}
		case <-rl.stopCh:
			return
		}
	}
}

func (rl *RateLimiter) WaitC() <-chan struct{} {
	return rl.tokenCh
}

func (rl *RateLimiter) Stop() {
	rl.mu.Lock()
	if !rl.stopped {
		close(rl.stopCh)
		rl.stopped = true
	}
	rl.mu.Unlock()
	rl.wg.Wait()
}
