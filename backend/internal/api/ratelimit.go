package api

import (
	"sync"
	"time"
)

// loginLimiter throttles failed sign-in attempts per key (email or IP).
// In-memory: acceptable for the single-instance v1 deployment.
type loginLimiter struct {
	mu       sync.Mutex
	failures map[string][]time.Time
	max      int
	window   time.Duration
}

func newLoginLimiter(maxFailures int, window time.Duration) *loginLimiter {
	return &loginLimiter{
		failures: map[string][]time.Time{},
		max:      maxFailures,
		window:   window,
	}
}

// blocked reports whether the key has exceeded the failure budget.
func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.prune(key)) >= l.max
}

func (l *loginLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.failures[key] = append(l.prune(key), time.Now())
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.failures, key)
}

// prune drops entries outside the window; caller must hold the lock.
func (l *loginLimiter) prune(key string) []time.Time {
	cutoff := time.Now().Add(-l.window)
	kept := l.failures[key][:0]
	for _, t := range l.failures[key] {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.failures, key)
		return nil
	}
	l.failures[key] = kept
	return kept
}
