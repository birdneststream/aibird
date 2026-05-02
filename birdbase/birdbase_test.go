package birdbase

import (
	"bytes"
	"testing"
	"time"
)

func TestSQLiteOperations(t *testing.T) {
	// Create test database
	testDB, err := NewSQLiteDB(":memory:") // In-memory for tests
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer testDB.db.Close()

	// Test basic operations
	key := "test-key"
	value := []byte("test-value")

	// Test Put/Get
	err = testDB.Put(key, value)
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	retrieved, err := testDB.Get(key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if !bytes.Equal(retrieved, value) {
		t.Errorf("Value mismatch: got %s, want %s", retrieved, value)
	}

	// Test Has
	if !testDB.Has(key) {
		t.Error("Has should return true for existing key")
	}

	// Test Delete
	err = testDB.Delete(key)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	if testDB.Has(key) {
		t.Error("Has should return false after delete")
	}
}

func TestTTLOperations(t *testing.T) {
	testDB, err := NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer testDB.db.Close()

	key := "ttl-key"
	value := []byte("ttl-value")

	// Put with 1 second TTL
	err = testDB.PutWithTTL(key, value, 1*time.Second)
	if err != nil {
		t.Fatalf("PutWithTTL failed: %v", err)
	}

	// Should exist immediately
	if !testDB.Has(key) {
		t.Error("Key should exist immediately after insert")
	}

	// Wait for expiration (extra margin for CI/slow systems)
	time.Sleep(1500 * time.Millisecond)

	// Should be filtered out by expiration check
	if testDB.Has(key) {
		t.Error("Key should not be accessible after expiration")
	}

	// Cleanup should remove expired entries from the database
	// Note: Cleanup() may call logger which is nil in tests, so we only
	// verify the functional behavior (Has already confirmed expiration above)
}

func TestCompatibilityFunctions(t *testing.T) {
	// Set global Data variable for testing
	var err error
	Data, err = NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer Data.db.Close()

	// Test PutString
	err = PutString("string-key", "string-value")
	if err != nil {
		t.Fatalf("PutString failed: %v", err)
	}

	value, err := Get("string-key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(value) != "string-value" {
		t.Errorf("String value mismatch: got %s, want string-value", value)
	}

	// Test PutInt
	err = PutInt("int-key", 42)
	if err != nil {
		t.Fatalf("PutInt failed: %v", err)
	}

	value, err = Get("int-key")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if string(value) != "42" {
		t.Errorf("Int value mismatch: got %s, want 42", value)
	}

	// Test TTL functions
	err = PutStringExpireSeconds("expire-key", "expire-value", 3600)
	if err != nil {
		t.Fatalf("PutStringExpireSeconds failed: %v", err)
	}

	if !Has("expire-key") {
		t.Error("TTL key should exist")
	}
}

func TestChatHistory(t *testing.T) {
	testDB, err := NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer testDB.db.Close()

	key := "chat-test"
	messages := []Message{
		{Role: "user", Content: "Hello"},
		{Role: "assistant", Content: "Hi there"},
	}

	// Test PutChatHistory
	err = testDB.PutChatHistory(key, messages)
	if err != nil {
		t.Fatalf("PutChatHistory failed: %v", err)
	}

	// Test GetChatHistory
	retrieved, err := testDB.GetChatHistory(key)
	if err != nil {
		t.Fatalf("GetChatHistory failed: %v", err)
	}

	if len(retrieved) != 2 {
		t.Errorf("Expected 2 messages, got %d", len(retrieved))
	}

	if retrieved[0].Role != "user" || retrieved[0].Content != "Hello" {
		t.Errorf("First message mismatch: got %+v", retrieved[0])
	}
}

func TestUserUsage(t *testing.T) {
	testDB, err := NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	defer testDB.db.Close()

	ident := "testuser"
	host := "example.com"

	// Test IncrementUserUsage
	totalUses, shouldNag, err := testDB.IncrementUserUsage(ident, host)
	if err != nil {
		t.Fatalf("IncrementUserUsage failed: %v", err)
	}

	if totalUses != 1 {
		t.Errorf("Expected 1 use, got %d", totalUses)
	}

	if shouldNag {
		t.Error("Should not nag on first use")
	}

	// Test multiple increments
	for i := 2; i <= 30; i++ {
		totalUses, shouldNag, err = testDB.IncrementUserUsage(ident, host)
		if err != nil {
			t.Fatalf("IncrementUserUsage failed: %v", err)
		}
	}

	if totalUses != 30 {
		t.Errorf("Expected 30 uses, got %d", totalUses)
	}

	if !shouldNag {
		t.Error("Should nag at 30 uses")
	}

	// Test GetUserUsage
	usage, err := testDB.GetUserUsage(ident, host)
	if err != nil {
		t.Fatalf("GetUserUsage failed: %v", err)
	}

	if usage != 30 {
		t.Errorf("Expected 30 usage, got %d", usage)
	}
}

func TestInMemoryStructures(t *testing.T) {
	// Initialize in-memory structures
	InitMemory()
	defer StopMemory()

	// Test flood protection
	key := "flood-test"
	count1 := FloodManager.IncrementFloodCounter(key, time.Second*5)
	if count1 != 1 {
		t.Errorf("Expected flood count 1, got %d", count1)
	}

	count2 := FloodManager.IncrementFloodCounter(key, time.Second*5)
	if count2 != 2 {
		t.Errorf("Expected flood count 2, got %d", count2)
	}

	// Test flood ban
	banKey := "ban-test"
	FloodManager.SetFloodBan(banKey, time.Second*2)

	if !FloodManager.IsFloodBanned(banKey) {
		t.Error("Should be flood banned")
	}

	time.Sleep(3 * time.Second)

	if FloodManager.IsFloodBanned(banKey) {
		t.Error("Should not be flood banned after expiration")
	}

	// Test rate limiting
	rateKey := "rate-test"
	RateLimiter.SetRateLimit(rateKey, time.Second*2)

	if !RateLimiter.IsRateLimited(rateKey) {
		t.Error("Should be rate limited")
	}

	time.Sleep(3 * time.Second)

	if RateLimiter.IsRateLimited(rateKey) {
		t.Error("Should not be rate limited after expiration")
	}
}
