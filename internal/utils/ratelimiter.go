package utils

import (
	"sync"
	"time"
)

type RateLimiter struct {
	ratePerSecond int
	mu            sync.Mutex
	lastRequest   time.Time
}

func NewRateLimiter(ratePerSecond int) *RateLimiter {
	return &RateLimiter{
		ratePerSecond: ratePerSecond,
		lastRequest:   time.Now(),
	}
}

func (r *RateLimiter) Wait() {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Calculate minimum time between requests
	minGap := time.Second / time.Duration(r.ratePerSecond)

	elapsed := time.Since(r.lastRequest)
	if elapsed < minGap {
		time.Sleep(minGap - elapsed)
	}

	r.lastRequest = time.Now()
}
