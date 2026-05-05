package birdbase

import (
	"fmt"
	"testing"
	"time"
)

// flushMessagesForTesting forces a synchronous flush of the message buffer
// for deterministic testing. Must be called after StoreChannelMessage and
// before any read operations.
func flushMessagesForTesting() {
	if messageChan == nil {
		return
	}
	// Close the channel to trigger the worker to flush and exit.
	close(messageChan)
	// Wait for the worker to drain and exit deterministically.
	<-messageWorkerDone
	// Re-initialize for subsequent operations in the same test.
	messageChan = make(chan storedMessage, messageBufferSize)
	messageWorkerDone = make(chan struct{})
	go messageWriteWorker()
}

// waitForMessageWorker waits for the current message worker to finish.
func waitForMessageWorker() {
	if messageWorkerDone != nil {
		<-messageWorkerDone
	}
}

// setupMessagesTest creates an isolated test environment for message storage tests.
// Returns a cleanup function that must be deferred.
func setupMessagesTest(t *testing.T) {
	t.Helper()

	// Create in-memory DB with single connection to ensure shared state
	db, err := NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}
	db.db.SetMaxOpenConns(1)

	origData := Data
	Data = db

	// Reset message worker: close existing and wait for completion
	if messageChan != nil {
		close(messageChan)
		waitForMessageWorker()
	}
	messageChan = make(chan storedMessage, messageBufferSize)
	messageWorkerDone = make(chan struct{})
	go messageWriteWorker()

	// Clear network ID cache
	networkIDCache.Range(func(key, _ any) bool {
		networkIDCache.Delete(key)
		return true
	})

	t.Cleanup(func() {
		if messageChan != nil {
			close(messageChan)
			waitForMessageWorker()
			messageChan = nil
			messageWorkerDone = nil
		}
		Data = origData
		db.db.Close()
	})
}

// seedNetworkForMessages creates a network in the DB and returns its ID.
func seedNetworkForMessages(t *testing.T, networkName string) int64 {
	t.Helper()
	Data.mu.Lock()
	defer Data.mu.Unlock()

	var id int64
	err := Data.db.QueryRow(
		"INSERT INTO networks (network_name, enabled, nick) VALUES (?, 1, 'testbot') RETURNING id",
		networkName,
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to seed network: %v", err)
	}
	return id
}

// mustInsertMessage inserts a message with a specific timestamp, failing the test on error.
func mustInsertMessage(t *testing.T, networkID int64, channel, nickname, eventType, message string, timestamp int64) {
	t.Helper()
	Data.mu.Lock()
	defer Data.mu.Unlock()
	_, err := Data.db.Exec(
		"INSERT INTO channel_messages (network_id, channel_name, nickname, event_type, message, timestamp) VALUES (?, ?, ?, ?, ?, ?)",
		networkID, channel, nickname, eventType, message, timestamp,
	)
	if err != nil {
		t.Fatalf("Failed to insert message: %v", err)
	}
}

func TestStoreAndGetChannelMessages(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	// Store messages
	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "hello world")
	StoreChannelMessage(networkID, "#test", "bob", "privmsg", "hi there")
	StoreChannelMessage(networkID, "#test", "alice", "action", "waves")

	// Flush before reading
	flushMessagesForTesting()

	// Get messages for last 1 hour
	messages, err := GetChannelMessages(networkID, "#test", 1, 100)
	if err != nil {
		t.Fatalf("GetChannelMessages failed: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d", len(messages))
	}

	// Verify chronological ordering (oldest first)
	if messages[0].Nickname != "alice" || messages[0].EventType != "privmsg" {
		t.Errorf("First message: got %s/%s, want alice/privmsg", messages[0].Nickname, messages[0].EventType)
	}
	if messages[1].Nickname != "bob" || messages[1].EventType != "privmsg" {
		t.Errorf("Second message: got %s/%s, want bob/privmsg", messages[1].Nickname, messages[1].EventType)
	}
	if messages[2].Nickname != "alice" || messages[2].EventType != "action" {
		t.Errorf("Third message: got %s/%s, want alice/action", messages[2].Nickname, messages[2].EventType)
	}
}

func TestGetChannelMessages_MaxMessages(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	// Insert 5 messages with explicitly distinct timestamps (1-second resolution)
	baseTime := time.Now().Add(-10 * time.Second)
	for i := 0; i < 5; i++ {
		mustInsertMessage(t, networkID, "#test", "user", "privmsg",
			fmt.Sprintf("msg%d", i), baseTime.Add(time.Duration(i)*time.Second).Unix())
	}

	// Cap at 3 — should get the 3 most recent (msg2, msg3, msg4) in ASC order
	messages, err := GetChannelMessages(networkID, "#test", 1, 3)
	if err != nil {
		t.Fatalf("GetChannelMessages with maxMessages failed: %v", err)
	}

	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages with cap, got %d", len(messages))
	}

	// Verify the returned messages are the most recent 3 in chronological order
	if messages[0].Message != "msg2" {
		t.Errorf("Oldest of cap: got %q, want \"msg2\"", messages[0].Message)
	}
	if messages[1].Message != "msg3" {
		t.Errorf("Middle of cap: got %q, want \"msg3\"", messages[1].Message)
	}
	if messages[2].Message != "msg4" {
		t.Errorf("Newest of cap: got %q, want \"msg4\"", messages[2].Message)
	}
}

func TestGetChannelMessages_EmptyResults(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	// GetChannelMessages returns nil (not empty slice) for no results
	messages, err := GetChannelMessages(networkID, "#test", 1, 100)
	if err != nil {
		t.Fatalf("GetChannelMessages on empty channel failed: %v", err)
	}
	if messages != nil {
		t.Errorf("Expected nil for empty channel, got %d entries", len(messages))
	}
}

func TestGetChannelMessages_HoursFilter(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	// Store a message now
	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "recent message")
	flushMessagesForTesting()

	// Should find it with 1 hour window
	messages, err := GetChannelMessages(networkID, "#test", 1, 100)
	if err != nil {
		t.Fatalf("GetChannelMessages failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 recent message, got %d", len(messages))
	}

	// Insert an old message directly (timestamp 2 days ago)
	mustInsertMessage(t, networkID, "#test", "bob", "privmsg", "old",
		time.Now().Add(-48*time.Hour).Unix())

	// 1 hour window should still return only 1 (the recent one)
	messages2, err := GetChannelMessages(networkID, "#test", 1, 100)
	if err != nil {
		t.Fatalf("GetChannelMessages with hours filter failed: %v", err)
	}
	if len(messages2) != 1 {
		t.Errorf("Expected 1 message in 1h window (old should be filtered), got %d", len(messages2))
	}

	// 72 hour window should return both
	messages3, err := GetChannelMessages(networkID, "#test", 72, 100)
	if err != nil {
		t.Fatalf("GetChannelMessages with 72h window failed: %v", err)
	}
	if len(messages3) != 2 {
		t.Errorf("Expected 2 messages in 72h window, got %d", len(messages3))
	}
}

func TestSearchChannelMessages(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "hello world")
	StoreChannelMessage(networkID, "#test", "bob", "privmsg", "goodbye world")
	StoreChannelMessage(networkID, "#test", "carol", "action", "joins the world")
	StoreChannelMessage(networkID, "#test", "dave", "join", "")
	flushMessagesForTesting()

	// Search for "world" — finds 3 messages that contain the keyword.
	// The join event has empty message so it wouldn't match regardless,
	// but the query also filters to privmsg/action event types only.
	results, err := SearchChannelMessages(networkID, "#test", "world", 10)
	if err != nil {
		t.Fatalf("SearchChannelMessages failed: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}

	// Verify ordering and content: alice → bob → carol (chronological)
	if results[0].Nickname != "alice" || results[0].Message != "hello world" {
		t.Errorf("First result: got %s/%q, want alice/\"hello world\"", results[0].Nickname, results[0].Message)
	}
	if results[1].Nickname != "bob" || results[1].Message != "goodbye world" {
		t.Errorf("Second result: got %s/%q, want bob/\"goodbye world\"", results[1].Nickname, results[1].Message)
	}
	if results[2].Nickname != "carol" || results[2].Message != "joins the world" {
		t.Errorf("Third result: got %s/%q, want carol/\"joins the world\"", results[2].Nickname, results[2].Message)
	}

	// All results must be privmsg or action (not join/part/etc.)
	for _, msg := range results {
		if msg.EventType != "privmsg" && msg.EventType != "action" {
			t.Errorf("Expected only privmsg/action events, got %s", msg.EventType)
		}
	}
}

func TestSearchChannelMessages_SingleResult(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "unique keyword xyz")
	flushMessagesForTesting()

	results, err := SearchChannelMessages(networkID, "#test", "xyz", 10)
	if err != nil {
		t.Fatalf("SearchChannelMessages single result failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results))
	}
	if results[0].Nickname != "alice" {
		t.Errorf("Expected alice, got %s", results[0].Nickname)
	}
}

func TestSearchChannelMessages_NoResults(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "hello world")
	flushMessagesForTesting()

	results, err := SearchChannelMessages(networkID, "#test", "nonexistent", 10)
	if err != nil {
		t.Fatalf("SearchChannelMessages no results failed: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("Expected 0 results for missing keyword, got %d", len(results))
	}
}

func TestSearchChannelMessages_LikeInjection(t *testing.T) {
	tests := []struct {
		name          string
		message       string
		keyword       string
		expectMatches int
	}{
		{
			name:          "percent wildcard is escaped",
			message:       "hello world",
			keyword:       "%",
			expectMatches: 0,
		},
		{
			name:          "underscore wildcard is escaped",
			message:       "hello world",
			keyword:       "_",
			expectMatches: 0,
		},
		{
			name:          "backslash is escaped",
			message:       "normal text",
			keyword:       `\`,
			expectMatches: 0,
		},
		{
			name:          "percent does not match everything",
			message:       "specific content only",
			keyword:       "%",
			expectMatches: 0,
		},
		{
			name:          "normal keyword still works after escaping",
			message:       "hello world",
			keyword:       "hello",
			expectMatches: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setupMessagesTest(t)
			networkID := seedNetworkForMessages(t, "testnet")

			StoreChannelMessage(networkID, "#test", "alice", "privmsg", tt.message)
			flushMessagesForTesting()

			results, err := SearchChannelMessages(networkID, "#test", tt.keyword, 10)
			if err != nil {
				t.Fatalf("SearchChannelMessages failed: %v", err)
			}
			if len(results) != tt.expectMatches {
				t.Errorf("Expected %d results for keyword %q, got %d", tt.expectMatches, tt.keyword, len(results))
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	tests := []struct {
		input  string
		output string
	}{
		{"hello", "hello"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`path\to`, `path\\to`},
		{`100%_done`, `100\%\_done`},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := escapeLike(tt.input)
			if got != tt.output {
				t.Errorf("escapeLike(%q) = %q, want %q", tt.input, got, tt.output)
			}
		})
	}
}

func TestGetChannelStats(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "msg1")
	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "msg2")
	StoreChannelMessage(networkID, "#test", "alice", "privmsg", "msg3")
	StoreChannelMessage(networkID, "#test", "bob", "privmsg", "msg4")
	StoreChannelMessage(networkID, "#test", "bob", "action", "does something")
	StoreChannelMessage(networkID, "#test", "carol", "join", "")
	flushMessagesForTesting()

	stats, err := GetChannelStats(networkID, "#test", 1)
	if err != nil {
		t.Fatalf("GetChannelStats failed: %v", err)
	}

	// Total events = 6
	if stats.TotalMessages != 6 {
		t.Errorf("Expected 6 total messages, got %d", stats.TotalMessages)
	}

	// Top chatters: alice (3 privmsg) > bob (1 privmsg + 1 action)
	if len(stats.TopChatters) < 2 {
		t.Fatalf("Expected at least 2 top chatters, got %d", len(stats.TopChatters))
	}
	if stats.TopChatters[0].Nickname != "alice" {
		t.Errorf("Expected alice as top chatter, got %s", stats.TopChatters[0].Nickname)
	}

	// Event breakdown
	if stats.EventCounts["privmsg"] != 4 {
		t.Errorf("Expected 4 privmsg events, got %d", stats.EventCounts["privmsg"])
	}
	if stats.EventCounts["action"] != 1 {
		t.Errorf("Expected 1 action event, got %d", stats.EventCounts["action"])
	}
	if stats.EventCounts["join"] != 1 {
		t.Errorf("Expected 1 join event, got %d", stats.EventCounts["join"])
	}
}

func TestGetChannelStats_Empty(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	stats, err := GetChannelStats(networkID, "#test", 1)
	if err != nil {
		t.Fatalf("GetChannelStats on empty channel failed: %v", err)
	}

	if stats.TotalMessages != 0 {
		t.Errorf("Expected 0 total messages, got %d", stats.TotalMessages)
	}
	if len(stats.TopChatters) != 0 {
		t.Errorf("Expected empty TopChatters, got %d", len(stats.TopChatters))
	}
	if stats.EventCounts == nil {
		t.Error("Expected initialized EventCounts map, got nil")
	}
}

func TestCleanupOldMessages(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "testnet")

	// Insert an old message directly
	oldTimestamp := time.Now().Add(-8 * 24 * time.Hour).Unix() // 8 days ago
	recentTimestamp := time.Now().Unix()

	mustInsertMessage(t, networkID, "#test", "alice", "privmsg", "old message", oldTimestamp)
	mustInsertMessage(t, networkID, "#test", "bob", "privmsg", "recent message", recentTimestamp)

	// Cleanup messages older than 7 days
	deleted, err := CleanupOldMessages(7)
	if err != nil {
		t.Fatalf("CleanupOldMessages failed: %v", err)
	}
	if deleted != 1 {
		t.Errorf("Expected 1 deleted message, got %d", deleted)
	}

	// Verify only recent message remains
	messages, err := GetChannelMessages(networkID, "#test", 168, 100) // 7 days
	if err != nil {
		t.Fatalf("GetChannelMessages after cleanup failed: %v", err)
	}
	if len(messages) != 1 {
		t.Errorf("Expected 1 message after cleanup, got %d", len(messages))
	}
	if messages[0].Message != "recent message" {
		t.Errorf("Expected 'recent message', got %s", messages[0].Message)
	}
}

func TestResolveNetworkID_Cache(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "cachenet")

	// First call — cache miss, DB lookup
	id := ResolveNetworkID("cachenet")
	if id != networkID {
		t.Errorf("First call: expected %d, got %d", networkID, id)
	}

	// Verify cache was populated
	if cached, ok := networkIDCache.Load("cachenet"); !ok {
		t.Error("Expected networkIDCache to be populated after first call")
	} else if cached != networkID {
		t.Errorf("Cache value: expected %d, got %d", networkID, cached)
	}

	// Second call — should use cached value
	id2 := ResolveNetworkID("cachenet")
	if id2 != networkID {
		t.Errorf("Cache hit: expected %d, got %d", networkID, id2)
	}

	// Non-existent network returns 0
	id3 := ResolveNetworkID("nonexistent")
	if id3 != 0 {
		t.Errorf("Non-existent network: expected 0, got %d", id3)
	}

	// Verify non-existent network did NOT pollute cache with 0
	if _, ok := networkIDCache.Load("nonexistent"); ok {
		t.Error("Non-existent network should not be cached")
	}
}
