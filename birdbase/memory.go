package birdbase

import (
	"context"
	"sync"
	"time"
)

// cleanupIntervals for in-memory data structures
const (
	floodCleanupInterval     = 30 * time.Second
	rateLimitCleanupInterval = 5 * time.Minute
)

// In-memory data structures for hot, short-lived data
var (
	FloodManager *FloodProtection
	RateLimiter  *RateLimitManager
	memCtx       context.Context
	memCancel    context.CancelFunc
)

// FloodProtection manages flood counters in memory
type FloodProtection struct {
	mu       sync.RWMutex
	counters map[string]*FloodCounter
	bans     map[string]time.Time
}

type FloodCounter struct {
	Count     int
	ExpiresAt time.Time
}

// NewFloodProtection creates a new flood protection manager
func NewFloodProtection(ctx context.Context) *FloodProtection {
	fp := &FloodProtection{
		counters: make(map[string]*FloodCounter),
		bans:     make(map[string]time.Time),
	}

	// Start cleanup goroutine with context cancellation
	go fp.cleanupLoop(ctx)

	return fp
}

// IncrementFloodCounter increments flood counter for a key
func (fp *FloodProtection) IncrementFloodCounter(key string, ttl time.Duration) int {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	now := time.Now()

	counter, exists := fp.counters[key]
	if !exists || counter.ExpiresAt.Before(now) {
		// Create new counter
		fp.counters[key] = &FloodCounter{
			Count:     1,
			ExpiresAt: now.Add(ttl),
		}
		return 1
	}

	// Increment existing counter
	counter.Count++
	counter.ExpiresAt = now.Add(ttl) // Reset expiration
	return counter.Count
}

// GetFloodCount returns current flood count for a key
func (fp *FloodProtection) GetFloodCount(key string) int {
	fp.mu.RLock()
	defer fp.mu.RUnlock()

	counter, exists := fp.counters[key]
	if !exists || counter.ExpiresAt.Before(time.Now()) {
		return 0
	}

	return counter.Count
}

// SetFloodBan sets a flood ban with expiration
func (fp *FloodProtection) SetFloodBan(key string, duration time.Duration) {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	fp.bans[key] = time.Now().Add(duration)
}

// IsFloodBanned checks if a key is currently banned
func (fp *FloodProtection) IsFloodBanned(key string) bool {
	fp.mu.RLock()
	defer fp.mu.RUnlock()

	banExpires, exists := fp.bans[key]
	if !exists {
		return false
	}

	return banExpires.After(time.Now())
}

// cleanupLoop removes expired counters and bans
func (fp *FloodProtection) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(floodCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			fp.cleanup()
		}
	}
}

func (fp *FloodProtection) cleanup() {
	fp.mu.Lock()
	defer fp.mu.Unlock()

	now := time.Now()

	// Clean expired counters
	for key, counter := range fp.counters {
		if counter.ExpiresAt.Before(now) {
			delete(fp.counters, key)
		}
	}

	// Clean expired bans
	for key, banExpires := range fp.bans {
		if banExpires.Before(now) {
			delete(fp.bans, key)
		}
	}
}

// RateLimitManager manages temporary rate limits in memory
type RateLimitManager struct {
	mu     sync.RWMutex
	limits map[string]time.Time
}

func NewRateLimitManager(ctx context.Context) *RateLimitManager {
	rlm := &RateLimitManager{
		limits: make(map[string]time.Time),
	}

	// Start cleanup goroutine with context cancellation
	go rlm.cleanupLoop(ctx)

	return rlm
}

// SetRateLimit sets a rate limit with expiration
func (rlm *RateLimitManager) SetRateLimit(key string, duration time.Duration) {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	rlm.limits[key] = time.Now().Add(duration)
}

// IsRateLimited checks if a key is currently rate limited
func (rlm *RateLimitManager) IsRateLimited(key string) bool {
	rlm.mu.RLock()
	defer rlm.mu.RUnlock()

	limitExpires, exists := rlm.limits[key]
	if !exists {
		return false
	}

	return limitExpires.After(time.Now())
}

func (rlm *RateLimitManager) cleanupLoop(ctx context.Context) {
	ticker := time.NewTicker(rateLimitCleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rlm.cleanup()
		}
	}
}

func (rlm *RateLimitManager) cleanup() {
	rlm.mu.Lock()
	defer rlm.mu.Unlock()

	now := time.Now()
	for key, limitExpires := range rlm.limits {
		if limitExpires.Before(now) {
			delete(rlm.limits, key)
		}
	}
}

// Initialize in-memory structures with context for graceful shutdown
func InitMemory() {
	// Cancel any existing context to prevent orphaned goroutines
	if memCancel != nil {
		memCancel()
	}
	memCtx, memCancel = context.WithCancel(context.Background())
	FloodManager = NewFloodProtection(memCtx)
	RateLimiter = NewRateLimitManager(memCtx)
}

// StopMemory cancels the context for cleanup goroutines
func StopMemory() {
	if memCancel != nil {
		memCancel()
	}
}
