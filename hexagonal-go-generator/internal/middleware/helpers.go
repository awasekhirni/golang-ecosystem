// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// helpers.go - small private helpers used by the middleware stack.
package middleware

import (
	"sync"

	"golang.org/x/time/rate"
)

// ipBuckets is a thread-safe map of per-IP rate limiters. Production should
// swap this for a Redis-backed limiter; the interface stays identical.
type ipBuckets struct {
	mu sync.Mutex
	m  map[string]*rate.Limiter
}

func (b *ipBuckets) get(ip string, rps float64, burst int) *rate.Limiter {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.m == nil {
		b.m = map[string]*rate.Limiter{}
	}
	lim, ok := b.m[ip]
	if !ok {
		lim = rate.NewLimiter(rate.Limit(rps), burst)
		b.m[ip] = lim
	}
	return lim
}
