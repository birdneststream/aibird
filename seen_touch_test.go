package main

import (
	"testing"
	"time"

	"aibird/birdbase"
	"aibird/irc/networks"
	"aibird/irc/users"
	"aibird/settings"

	"github.com/lrstanley/girc"
)

// setupTouchDB installs an in-memory database and seeds one network. Returns
// the network id.
func setupTouchDB(t *testing.T, networkName string) int64 {
	t.Helper()

	db, err := birdbase.NewSQLiteDB(":memory:")
	if err != nil {
		t.Fatalf("failed to create test database: %v", err)
	}
	db.LimitToSingleConn()
	birdbase.ClearNetworkIDCache()

	oldData := birdbase.Data
	birdbase.Data = db

	networkID, err := birdbase.SeedNetworkForTesting(networkName)
	if err != nil {
		birdbase.Data = oldData
		db.CloseTestDB()
		t.Fatalf("failed to seed network: %v", err)
	}

	t.Cleanup(func() {
		birdbase.Data = oldData
		birdbase.ClearNetworkIDCache()
		db.CloseTestDB()
	})

	return networkID
}

// TestHandlePrivMsg_PlainChatRefreshesActivity is the end-to-end regression
// test for the "!seen says weeks ago" bug. A non-command message used to stop
// at storeChannelEvent and never reach State.Verify(), the only place that
// called User.Touch() — so the sender's activity stayed frozen at whatever the
// last bot command was, however many weeks ago that happened.
//
// This drives handlePrivMsg itself rather than the helper, so reverting the
// wiring in the non-command branch makes it fail.
func TestHandlePrivMsg_PlainChatRefreshesActivity(t *testing.T) {
	const ident, host = "wiring", "wiring.example.org"
	networkID := setupTouchDB(t, "wiring-net")

	weeksAgo := time.Now().Unix() - 21*24*3600
	if _, err := birdbase.SeedIRCUserForTesting(networkID, "wiring", ident, host, weeksAgo, weeksAgo); err != nil {
		t.Fatalf("failed to seed user row: %v", err)
	}

	net := &networks.Network{
		NetworkName: "wiring-net",
		Nick:        "aibird",
		Users: []users.User{
			{NickName: "wiring", Ident: ident, Host: host, FirstSeen: weeksAgo, LatestActivity: weeksAgo},
		},
	}

	config := &settings.Config{}
	config.AiBird.ActionTrigger = "!"

	e := girc.Event{
		Source: &girc.Source{Name: "wiring", Ident: ident, Host: host},
		Params: []string{"#chan", "no trigger here, just chat"},
	}

	handlePrivMsg(nil, e, net, config, nil)

	user := net.GetUserWithNick("wiring")
	if user == nil {
		t.Fatal("user vanished from the network")
	}
	if user.LatestActivity <= weeksAgo {
		t.Errorf("plain chat did not refresh LatestActivity: got %d, want > %d", user.LatestActivity, weeksAgo)
	}
}

// TestTouchChatSender_RefreshesActivityForPlainChat covers the helper in isolation.
func TestTouchChatSender_RefreshesActivityForPlainChat(t *testing.T) {
	const ident, host = "alice", "alice.example.org"
	networkID := setupTouchDB(t, "touch-net")

	weeksAgo := time.Now().Unix() - 21*24*3600

	if _, err := birdbase.SeedIRCUserForTesting(networkID, "alice", ident, host, weeksAgo, weeksAgo); err != nil {
		t.Fatalf("failed to seed user row: %v", err)
	}

	net := &networks.Network{
		NetworkName: "touch-net",
		Nick:        "aibird",
		Users: []users.User{
			{NickName: "alice", Ident: ident, Host: host, FirstSeen: weeksAgo, LatestActivity: weeksAgo},
		},
	}

	e := girc.Event{
		Source: &girc.Source{
			Name:  "alice",
			Ident: ident,
			Host:  host,
		},
		Params: []string{"#chan", "just chatting, no bang in sight"},
	}

	touchChatSender(net, e)

	// In-memory state must move to now, immediately.
	user := net.GetUserWithNick("alice")
	if user == nil {
		t.Fatal("user vanished from the network after touch")
	}
	if user.LatestActivity <= weeksAgo {
		t.Errorf("in-memory LatestActivity = %d, want it advanced past %d", user.LatestActivity, weeksAgo)
	}

	// And the stored column must follow, otherwise the value is lost on restart.
	got, err := birdbase.ReadUserLatestActivityForTesting(networkID, ident, host)
	if err != nil {
		t.Fatalf("failed to read stored latest_activity: %v", err)
	}
	if got <= weeksAgo {
		t.Errorf("stored latest_activity = %d, want it advanced past %d", got, weeksAgo)
	}
}

func TestTouchChatSender_IgnoresTheBotsOwnOutput(t *testing.T) {
	const ident, host = "aibird", "bot.host"
	networkID := setupTouchDB(t, "touch-net")

	stored := int64(1000)
	if _, err := birdbase.SeedIRCUserForTesting(networkID, "aibird", ident, host, stored, stored); err != nil {
		t.Fatalf("failed to seed user row: %v", err)
	}

	net := &networks.Network{
		NetworkName: "touch-net",
		Nick:        "aibird",
		Users: []users.User{
			{NickName: "aibird", Ident: ident, Host: host, LatestActivity: stored},
		},
	}

	// Cased differently on purpose: self-recognition must be case-insensitive.
	e := girc.Event{
		Source: &girc.Source{Name: "AIBIRD", Ident: ident, Host: host},
		Params: []string{"#chan", "hello from myself"},
	}

	touchChatSender(net, e)

	if got := net.GetUserWithNick("aibird").LatestActivity; got != stored {
		t.Errorf("bot's own message changed LatestActivity to %d, want it unchanged at %d", got, stored)
	}
}

func TestTouchChatSender_UnknownSenderIsNotAWrite(t *testing.T) {
	networkID := setupTouchDB(t, "touch-net")

	net := &networks.Network{
		NetworkName: "touch-net",
		Nick:        "aibird",
		Users:       []users.User{},
	}

	e := girc.Event{
		Source: &girc.Source{Name: "ghost", Ident: "ghost", Host: "nowhere.invalid"},
		Params: []string{"#chan", "hello?"},
	}

	// Must not panic and must not invent a row for an untracked sender.
	touchChatSender(net, e)

	if _, err := birdbase.ReadUserLatestActivityForTesting(networkID, "ghost", "nowhere.invalid"); err == nil {
		t.Error("expected no irc_users row for an untracked sender")
	}
}

func TestTouchChatSender_NilSourceIsSafe(t *testing.T) {
	setupTouchDB(t, "touch-net")

	net := &networks.Network{NetworkName: "touch-net", Nick: "aibird"}
	touchChatSender(net, girc.Event{Params: []string{"#chan", "malformed"}})
}
