package birdbase

import (
	"strings"
	"testing"
	"time"
)

// seedChannelMessage writes one row directly so tests control the timestamp,
// bypassing the async write buffer used by StoreChannelMessage.
func seedChannelMessage(t *testing.T, networkID int64, channelName, nickname, eventType string, timestamp int64) {
	t.Helper()
	if err := SeedChannelMessageForTesting(networkID, channelName, nickname, eventType, "hello", timestamp); err != nil {
		t.Fatalf("failed to seed channel message: %v", err)
	}
}

func TestLastSeen_PrefersNewestRow(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	now := time.Now().Unix()
	seedChannelMessage(t, networkID, "#chan", "alice", "privmsg", now-3600)
	seedChannelMessage(t, networkID, "#chan", "alice", "privmsg", now-60)
	seedChannelMessage(t, networkID, "#chan", "alice", "privmsg", now-7200)

	got, err := LastSeen(networkID, "#chan", "alice")
	if err != nil {
		t.Fatalf("LastSeen returned error: %v", err)
	}
	if !got.Found {
		t.Fatal("LastSeen: expected Found=true")
	}
	if got.Timestamp != now-60 {
		t.Errorf("LastSeen timestamp = %d, want %d (newest row)", got.Timestamp, now-60)
	}
}

func TestLastSeen_IsCaseInsensitiveOnNick(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	now := time.Now().Unix()
	seedChannelMessage(t, networkID, "#chan", "Alice", "privmsg", now-42)

	// Stored as "Alice"; the query arrives as "alice" and "ALICE".
	for _, ask := range []string{"alice", "ALICE", "Alice"} {
		got, err := LastSeen(networkID, "#chan", ask)
		if err != nil {
			t.Fatalf("LastSeen(%q) returned error: %v", ask, err)
		}
		if !got.Found || got.Timestamp != now-42 {
			t.Errorf("LastSeen(%q) = %+v, want Found=true timestamp=%d", ask, got, now-42)
		}
	}
}

func TestLastSeen_UnknownNickNotFound(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	got, err := LastSeen(networkID, "#chan", "nobody")
	if err != nil {
		t.Fatalf("LastSeen returned error: %v", err)
	}
	if got.Found {
		t.Errorf("LastSeen = %+v, want Found=false for an unknown nick", got)
	}
	if got.Timestamp != 0 {
		t.Errorf("LastSeen timestamp = %d, want 0", got.Timestamp)
	}
}

func TestLastSeen_DoesNotCrossChannels(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	now := time.Now().Unix()
	seedChannelMessage(t, networkID, "#other", "alice", "privmsg", now-10)

	got, err := LastSeen(networkID, "#chan", "alice")
	if err != nil {
		t.Fatalf("LastSeen returned error: %v", err)
	}
	if got.Found {
		t.Errorf("LastSeen = %+v, want Found=false: activity happened in another channel", got)
	}
}

func TestLastSeen_ReportsJoinEventType(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	now := time.Now().Unix()
	seedChannelMessage(t, networkID, "#chan", "bob", "privmsg", now-600)
	seedChannelMessage(t, networkID, "#chan", "bob", "join", now-30)

	got, err := LastSeen(networkID, "#chan", "bob")
	if err != nil {
		t.Fatalf("LastSeen returned error: %v", err)
	}
	if got.EventType != "join" {
		t.Errorf("LastSeen EventType = %q, want %q", got.EventType, "join")
	}
}

// TestLastSeen_SeesBufferedMessageBeforeFlush is the regression test for the
// write-buffer race: StoreChannelMessage hands the row to a background worker,
// so a naive table-only lookup reports an older timestamp for a message sent a
// moment ago.
func TestLastSeen_SeesBufferedMessageBeforeFlush(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	now := time.Now().Unix()
	seedChannelMessage(t, networkID, "#chan", "carol", "privmsg", now-3600)

	StoreChannelMessage(networkID, "#chan", "carol", "privmsg", "just typed this")

	got, err := LastSeen(networkID, "#chan", "carol")
	if err != nil {
		t.Fatalf("LastSeen returned error: %v", err)
	}
	if got.Timestamp < now {
		t.Errorf("LastSeen timestamp = %d, want >= %d (buffered message must count)", got.Timestamp, now)
	}

	// After the flush the table agrees with the cache, so the answer holds.
	flushMessagesForTesting()

	got, err = LastSeen(networkID, "#chan", "carol")
	if err != nil {
		t.Fatalf("LastSeen after flush returned error: %v", err)
	}
	if got.Timestamp < now {
		t.Errorf("LastSeen after flush timestamp = %d, want >= %d", got.Timestamp, now)
	}
}

func TestTouchUserActivity_MovesForwardOnly(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	const ident, host = "carol", "example.org"
	now := time.Now().Unix()

	// Create the row the normal way first.
	if err := SaveSingleUser("seen-net", ident, host, &UserData{
		NickName: "carol", Ident: ident, Host: host,
		FirstSeen: now - 1000, LatestActivity: now - 1000,
	}); err != nil {
		t.Fatalf("SaveSingleUser failed: %v", err)
	}

	if err := TouchUserActivity("seen-net", ident, host, now); err != nil {
		t.Fatalf("TouchUserActivity failed: %v", err)
	}

	// An older timestamp must not regress the stored value.
	if err := TouchUserActivity("seen-net", ident, host, now-500); err != nil {
		t.Fatalf("TouchUserActivity (older) failed: %v", err)
	}

	got, err := ReadUserLatestActivityForTesting(networkID, ident, host)
	if err != nil {
		t.Fatalf("read back latest_activity: %v", err)
	}
	if got != now {
		t.Errorf("latest_activity = %d, want %d", got, now)
	}
}

func TestTouchUserActivity_UnknownUserIsNotAnError(t *testing.T) {
	setupMessagesTest(t)
	seedNetworkForMessages(t, "seen-net")

	if err := TouchUserActivity("seen-net", "ghost", "nowhere.invalid", time.Now().Unix()); err != nil {
		t.Errorf("TouchUserActivity for an unknown user should not error, got: %v", err)
	}
}

func TestTouchUserActivity_ZeroTimestampIgnored(t *testing.T) {
	setupMessagesTest(t)
	networkID := seedNetworkForMessages(t, "seen-net")

	const ident, host = "dave", "example.org"
	now := time.Now().Unix()

	if err := SaveSingleUser("seen-net", ident, host, &UserData{
		NickName: "dave", Ident: ident, Host: host,
		FirstSeen: now, LatestActivity: now,
	}); err != nil {
		t.Fatalf("SaveSingleUser failed: %v", err)
	}

	if err := TouchUserActivity("seen-net", ident, host, 0); err != nil {
		t.Fatalf("TouchUserActivity(0) failed: %v", err)
	}

	got, err := ReadUserLatestActivityForTesting(networkID, ident, host)
	if err != nil {
		t.Fatalf("read back latest_activity: %v", err)
	}
	if got != now {
		t.Errorf("latest_activity = %d, want %d unchanged", got, now)
	}
}

// TestNickIndexExists guards the index that backs LastSeen's nick lookup. The
// DDL is only valid SQLite if lower(nickname) is accepted as an expression
// index, and a silently-missing index would degrade every !seen into a scan.
func TestNickIndexExists(t *testing.T) {
	setupMessagesTest(t)

	Data.mu.RLock()
	defer Data.mu.RUnlock()

	// The index must exist: initSchema's DDL is the only place it is created,
	// and a typo there would silently degrade every !seen into a table scan.
	var name string
	err := Data.db.QueryRow(`
		SELECT name FROM sqlite_master
		WHERE type = 'index' AND tbl_name = 'channel_messages'
		  AND name = 'idx_channel_messages_nick'
	`).Scan(&name)
	if err != nil {
		t.Fatalf("idx_channel_messages_nick missing from schema: %v", err)
	}

	// Confirm the planner selects it. This build's EXPLAIN QUERY PLAN
	// returns (id, parent, notused, detail); detail carries the index name.
	var id, parent, notused int
	var detail string
	err = Data.db.QueryRow(`
		EXPLAIN QUERY PLAN SELECT timestamp FROM channel_messages
		WHERE network_id = 1 AND channel_name = '#c' AND lower(nickname) = 'alice'
		ORDER BY timestamp DESC LIMIT 1
	`).Scan(&id, &parent, &notused, &detail)
	if err != nil {
		t.Fatalf("EXPLAIN QUERY PLAN failed: %v", err)
	}
	if !containsIndex(detail) {
		t.Errorf("planner did not use the nick index; detail = %q", detail)
	}
}

// containsIndex keeps the assertion tolerant to SQLite phrasing changes while
// still failing if the index is bypassed in favor of a scan.
func containsIndex(detail string) bool {
	return strings.Contains(detail, "idx_channel_messages_nick")
}
