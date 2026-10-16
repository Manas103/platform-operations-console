package collector

import (
	"sync"
	"time"
)

// TokenBucket is a per-source rate limiter: capacity tokens, refilled at refillRate tokens per
// second, never exceeding capacity. One bucket per source means a storm on one upstream cannot
// starve the other three.
type TokenBucket struct {
	mu          sync.Mutex
	capacity    float64
	tokens      float64
	refillRate  float64 // tokens per second
	lastRefill  time.Time
	nowFunc     func() time.Time
}

func NewTokenBucket(capacity float64, refillRate float64) *TokenBucket {
	return &TokenBucket{
		capacity:   capacity,
		tokens:     capacity,
		refillRate: refillRate,
		lastRefill: time.Now(),
		nowFunc:    time.Now,
	}
}

func (b *TokenBucket) refillLocked() {
	now := b.nowFunc()
	elapsed := now.Sub(b.lastRefill).Seconds()
	if elapsed <= 0 {
		return
	}
	b.tokens += elapsed * b.refillRate
	if b.tokens > b.capacity {
		b.tokens = b.capacity
	}
	b.lastRefill = now
}

// Allow consumes one token if available and reports whether the call may proceed.
func (b *TokenBucket) Allow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.refillLocked()
	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

// Wait blocks until a token is available or the deadline passes, returning false on timeout.
func (b *TokenBucket) Wait(timeout time.Duration) bool {
	deadline := b.nowFunc().Add(timeout)
	for {
		if b.Allow() {
			return true
		}
		if b.nowFunc().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
}
