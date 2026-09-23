package commands

import (
	"strings"
	"testing"
	"time"

	"aibird/birdbase"
)

// setupSeenDB installs an in-memory database with one network and returns its
// ID. Returns a cleanup function to defer.
func setupSeenDB(t *testing.T) int64 {
	t.Helper()

	db, err := birdbase.NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	db.LimitToSingleConn()

	// Each in-memory database has its own autoincrement ids, so drop any cached
	// name→id mapping left behind by an earlier test.
	birdbase.ClearNetworkIDCache()

	oldData := birdbase.Data
	birdbase.Data = db

	networkID, err := birdbase.SeedNetworkForTesting(seenTestNetwork)
	if err != nil {
		birdbase.Data = oldData
		db.CloseTestDB()
		t.Fatalf("failed to seed network: %v", err)
	}
	if networkID == 0 {
		birdbase.Data = oldData
		db.CloseTestDB()
		t.Fatal("seeded network resolved to id 0")
	}

	t.Cleanup(func() {
		birdbase.Data = oldData
		birdbase.ClearNetworkIDCache()
		db.CloseTestDB()
	})

	return networkID
}

// seenTestNetwork is the network name seeded by setupSeenDB.
const seenTestNetwork = "seen-cmd-net"

// seedSeen inserts one channel_messages row with an explicit timestamp, so the
// test controls "how long ago" rather than racing the async writer.
func seedSeen(t *testing.T, networkID int64, channel, nick, eventType string, timestamp int64) {
	t.Helper()
	if err := birdbase.SeedChannelMessageForTesting(networkID, channel, nick, eventType, "x", timestamp); err != nil {
		t.Fatalf("failed to seed channel message: %v", err)
	}
}

func TestLastSeenMessage_UsesChannelHistoryNotStaleMemory(t *testing.T) {
	// This is the reported bug: the user chatted a moment ago, but the
	// in-memory LatestActivity is from a command they ran three weeks back.
	networkID := setupSeenDB(t)
	const channel = "#seen"

	now := time.Now().Unix()
	weeksAgo := now - 21*24*3600

	seedSeen(t, networkID, channel, "alice", "privmsg", now-5)

	got := lastSeenMessage(networkID, channel, "alice", weeksAgo)
	if got == "" {
		t.Fatal("lastSeenMessage returned empty string")
	}
	if !strings.Contains(got, "seconds ago") {
		t.Errorf("lastSeenMessage = %q, want recency from channel history, not weeks", got)
	}
	if strings.Contains(got, "weeks") {
		t.Errorf("lastSeenMessage = %q, must not report a stale in-memory timestamp", got)
	}
}

func TestLastSeenMessage_FreshMemoryBeatsNoHistory(t *testing.T) {
	networkID := setupSeenDB(t)

	// No channel_messages rows, but the process saw a command 10s ago.
	got := lastSeenMessage(networkID, "#seen", "bob", time.Now().Unix()-10)
	if got == "" {
		t.Fatal("lastSeenMessage returned empty string for fresh in-memory activity")
	}
	if !strings.Contains(got, "seconds ago") {
		t.Errorf("lastSeenMessage = %q, want 'seconds ago'", got)
	}
}

func TestLastSeenMessage_DropsStaleMemoryWhenNoHistory(t *testing.T) {
	networkID := setupSeenDB(t)

	// Nothing in the database and the memory value is 30 days old: refuse to
	// assert instead of confidently saying "4 weeks ago".
	got := lastSeenMessage(networkID, "#seen", "carol", time.Now().Unix()-30*24*3600)
	if got != "" {
		t.Errorf("lastSeenMessage = %q, want empty string for stale memory with no history", got)
	}
}

func TestLastSeenMessage_ZeroMemoryNoHistory(t *testing.T) {
	networkID := setupSeenDB(t)

	if got := lastSeenMessage(networkID, "#seen", "dave", 0); got != "" {
		t.Errorf("lastSeenMessage = %q, want empty string for a never-seen nick", got)
	}
}

func TestLastSeenMessage_NonChatEventMentionsTheEvent(t *testing.T) {
	networkID := setupSeenDB(t)
	const channel = "#seen"

	now := time.Now().Unix()
	seedSeen(t, networkID, channel, "erin", "privmsg", now-600)
	seedSeen(t, networkID, channel, "erin", "join", now-60)

	got := lastSeenMessage(networkID, channel, "erin", 0)
	if !strings.Contains(got, "join") {
		t.Errorf("lastSeenMessage = %q, want it to name the join event", got)
	}
}

func TestLastSeenMessage_ChatEventSaysChatted(t *testing.T) {
	networkID := setupSeenDB(t)
	const channel = "#seen"

	seedSeen(t, networkID, channel, "frank", "privmsg", time.Now().Unix()-60)

	got := lastSeenMessage(networkID, channel, "frank", 0)
	if !strings.Contains(got, "chatted") {
		t.Errorf("lastSeenMessage = %q, want 'chatted' for a plain message", got)
	}
}
