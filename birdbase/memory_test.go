package birdbase

import (
	"testing"
	"time"
)

// TestFloodProtection_BasicIncrement verifies that flood counters increment correctly.
func TestFloodProtection_BasicIncrement(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:increment"
	count := FloodManager.IncrementFloodCounter(key, 5*time.Second)
	if count != 1 {
		t.Errorf("Expected first increment to return 1, got %d", count)
	}

	count = FloodManager.IncrementFloodCounter(key, 5*time.Second)
	if count != 2 {
		t.Errorf("Expected second increment to return 2, got %d", count)
	}

	count = FloodManager.IncrementFloodCounter(key, 5*time.Second)
	if count != 3 {
		t.Errorf("Expected third increment to return 3, got %d", count)
	}
}

// TestFloodProtection_CounterExpiry verifies that counters reset after TTL expires.
func TestFloodProtection_CounterExpiry(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:expiry"
	count := FloodManager.IncrementFloodCounter(key, 100*time.Millisecond)
	if count != 1 {
		t.Errorf("Expected 1, got %d", count)
	}

	time.Sleep(150 * time.Millisecond)

	// Counter should have expired, so a new increment starts at 1
	count = FloodManager.IncrementFloodCounter(key, 100*time.Millisecond)
	if count != 1 {
		t.Errorf("Expected 1 after expiry, got %d", count)
	}
}

// TestFloodProtection_BanSetAndCheck verifies flood ban lifecycle.
func TestFloodProtection_BanSetAndCheck(t *testing.T) {
	InitMemory()
	defer StopMemory()

	banKey := "test:ban"
	if FloodManager.IsFloodBanned(banKey) {
		t.Error("Should not be banned initially")
	}

	FloodManager.SetFloodBan(banKey, 200*time.Millisecond)
	if !FloodManager.IsFloodBanned(banKey) {
		t.Error("Should be banned after SetFloodBan")
	}

	time.Sleep(250 * time.Millisecond)
	if FloodManager.IsFloodBanned(banKey) {
		t.Error("Should not be banned after expiration")
	}
}

// TestFloodProtection_DifferentKeysAreIndependent verifies that different keys don't interfere.
func TestFloodProtection_DifferentKeysAreIndependent(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key1 := "test:independent:1"
	key2 := "test:independent:2"

	FloodManager.IncrementFloodCounter(key1, 5*time.Second)
	FloodManager.IncrementFloodCounter(key1, 5*time.Second)

	count := FloodManager.IncrementFloodCounter(key2, 5*time.Second)
	if count != 1 {
		t.Errorf("Key2 should be independent from key1, got count %d", count)
	}
}

// TestFloodProtection_GetFloodCount verifies read-only count access.
func TestFloodProtection_GetFloodCount(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:getcount"
	if count := FloodManager.GetFloodCount(key); count != 0 {
		t.Errorf("Expected 0 for non-existent key, got %d", count)
	}

	FloodManager.IncrementFloodCounter(key, 5*time.Second)
	FloodManager.IncrementFloodCounter(key, 5*time.Second)

	if count := FloodManager.GetFloodCount(key); count != 2 {
		t.Errorf("Expected 2, got %d", count)
	}
}

// TestFloodProtection_GetFloodCountExpiry verifies GetFloodCount returns 0 for expired counters.
func TestFloodProtection_GetFloodCountExpiry(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:getcountexpiry"
	FloodManager.IncrementFloodCounter(key, 100*time.Millisecond)
	FloodManager.IncrementFloodCounter(key, 100*time.Millisecond)

	time.Sleep(150 * time.Millisecond)

	if count := FloodManager.GetFloodCount(key); count != 0 {
		t.Errorf("Expected 0 after expiry, got %d", count)
	}
}

// TestFloodProtection_ConcurrentAccess verifies thread safety under concurrent load.
func TestFloodProtection_ConcurrentAccess(t *testing.T) {
	InitMemory()
	defer StopMemory()

	const goroutines = 50
	const incrementsPerGoroutine = 10
	key := "test:concurrent"

	var done = make(chan bool, goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			for i := 0; i < incrementsPerGoroutine; i++ {
				FloodManager.IncrementFloodCounter(key, 10*time.Second)
			}
			done <- true
		}()
	}

	for g := 0; g < goroutines; g++ {
		<-done
	}

	// All increments should be counted
	expected := goroutines * incrementsPerGoroutine
	if count := FloodManager.GetFloodCount(key); count != expected {
		t.Errorf("Expected %d concurrent increments, got %d", expected, count)
	}
}

// TestFloodProtection_BanDoesNotAffectOtherKeys verifies ban isolation.
func TestFloodProtection_BanDoesNotAffectOtherKeys(t *testing.T) {
	InitMemory()
	defer StopMemory()

	banKey1 := "test:ban:1"
	banKey2 := "test:ban:2"

	FloodManager.SetFloodBan(banKey1, 5*time.Second)

	if !FloodManager.IsFloodBanned(banKey1) {
		t.Error("banKey1 should be banned")
	}
	if FloodManager.IsFloodBanned(banKey2) {
		t.Error("banKey2 should not be banned")
	}
}

// TestRateLimitManager_Basic tests basic rate limit set/check lifecycle.
func TestRateLimitManager_Basic(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:ratelimit"
	if RateLimiter.IsRateLimited(key) {
		t.Error("Should not be rate limited initially")
	}

	RateLimiter.SetRateLimit(key, 200*time.Millisecond)
	if !RateLimiter.IsRateLimited(key) {
		t.Error("Should be rate limited after SetRateLimit")
	}

	time.Sleep(250 * time.Millisecond)
	if RateLimiter.IsRateLimited(key) {
		t.Error("Should not be rate limited after expiration")
	}
}

// TestFloodProtection_SlidingTTL verifies that incrementing resets the expiry window.
// This is a critical behavioral detail: a user who keeps sending will never expire.
func TestFloodProtection_SlidingTTL(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key := "test:sliding"

	// Increment with a short TTL
	count := FloodManager.IncrementFloodCounter(key, 200*time.Millisecond)
	if count != 1 {
		t.Errorf("Expected 1, got %d", count)
	}

	// Wait 100ms (half the TTL), then increment again
	time.Sleep(100 * time.Millisecond)
	count = FloodManager.IncrementFloodCounter(key, 200*time.Millisecond)
	if count != 2 {
		t.Errorf("Expected 2, got %d", count)
	}

	// Wait another 100ms. The original TTL would have expired by now (200ms total),
	// but the second increment reset it. Counter should still be alive.
	time.Sleep(110 * time.Millisecond)
	currentCount := FloodManager.GetFloodCount(key)
	if currentCount != 2 {
		t.Errorf("Expected counter to survive via sliding TTL (count=2), got %d", currentCount)
	}

	// Now wait for the full TTL to expire
	time.Sleep(210 * time.Millisecond)
	if count := FloodManager.GetFloodCount(key); count != 0 {
		t.Errorf("Expected 0 after full TTL expiry, got %d", count)
	}
}

// TestRateLimitManager_DifferentKeysAreIndependent verifies rate limit key isolation.
func TestRateLimitManager_DifferentKeysAreIndependent(t *testing.T) {
	InitMemory()
	defer StopMemory()

	key1 := "test:rate:1"
	key2 := "test:rate:2"

	RateLimiter.SetRateLimit(key1, 5*time.Second)

	if !RateLimiter.IsRateLimited(key1) {
		t.Error("key1 should be rate limited")
	}
	if RateLimiter.IsRateLimited(key2) {
		t.Error("key2 should not be rate limited")
	}
}
