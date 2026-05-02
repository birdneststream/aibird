package networks

import (
	"testing"

	"aibird/irc/channels"
	"aibird/irc/users"
	"aibird/logger"
)

func init() {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
}

func newTestNetwork() *Network {
	return &Network{
		NetworkName: "testnet",
		Nick:        "testbot",
		Users: []users.User{
			{NickName: "alice", Ident: "~alice", Host: "alice.host", LatestActivity: 1000},
			{NickName: "alice_old", Ident: "~alice", Host: "alice.host", LatestActivity: 500},
			{NickName: "bob", Ident: "~bob", Host: "bob.host", LatestActivity: 2000},
			{NickName: "charlie", Ident: "~charlie", Host: "charlie.host", LatestActivity: 3000},
		},
		AdminHosts: []AdminHost{
			{Host: "admin.host", Ident: "admin", Owner: true},
			{Host: "op.host", Ident: "op", Owner: false},
		},
		IgnoredNicks: []string{"spambot"},
	}
}

func TestGetUserWithIdentAndHost_SingleMatch(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithIdentAndHost("~bob", "bob.host")
	if u == nil {
		t.Fatal("expected user, got nil")
	}
	if u.NickName != "bob" {
		t.Errorf("got nick %q, want %q", u.NickName, "bob")
	}
}

func TestGetUserWithIdentAndHost_MultipleMatch_ReturnsLatest(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithIdentAndHost("~alice", "alice.host")
	if u == nil {
		t.Fatal("expected user, got nil")
	}
	if u.NickName != "alice" {
		t.Errorf("got nick %q, want %q (latest activity)", u.NickName, "alice")
	}
}

func TestGetUserWithIdentAndHost_NotFound(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithIdentAndHost("~nobody", "nowhere.host")
	if u != nil {
		t.Errorf("expected nil, got %v", u)
	}
}

func TestGetUserWithIdentAndHost_BlankIdentHost(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithIdentAndHost("", "")
	if u != nil {
		t.Errorf("expected nil for blank ident/host, got %v", u)
	}
}

func TestGetUserWithNick_SingleMatch(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithNick("bob")
	if u == nil {
		t.Fatal("expected user, got nil")
	}
	if u.NickName != "bob" {
		t.Errorf("got nick %q, want %q", u.NickName, "bob")
	}
}

func TestGetUserWithNick_NotFound(t *testing.T) {
	n := newTestNetwork()
	u := n.GetUserWithNick("nobody")
	if u != nil {
		t.Errorf("expected nil, got %v", u)
	}
}

func TestGetNetworkChannel_Found(t *testing.T) {
	n := newTestNetwork()
	n.Channels = []channels.Channel{
		{Name: "#test"},
		{Name: "#dev"},
	}
	ch := n.GetNetworkChannel("#test")
	if ch == nil {
		t.Fatal("expected channel, got nil")
	}
	if ch.Name != "#test" {
		t.Errorf("got %q, want %q", ch.Name, "#test")
	}
}

func TestGetNetworkChannel_NotFound(t *testing.T) {
	n := newTestNetwork()
	ch := n.GetNetworkChannel("#nonexistent")
	if ch != nil {
		t.Errorf("expected nil, got %v", ch)
	}
}

func TestIsNickIgnored(t *testing.T) {
	n := newTestNetwork()
	if !n.IsNickIgnored("spambot") {
		t.Error("spambot should be ignored")
	}
	if n.IsNickIgnored("alice") {
		t.Error("alice should not be ignored")
	}
}

func TestIsIdentHostAdmin(t *testing.T) {
	n := newTestNetwork()
	if !n.IsIdentHostAdmin("admin", "admin.host") {
		t.Error("admin should be recognized as admin")
	}
	if n.IsIdentHostAdmin("admin", "wrong.host") {
		t.Error("wrong host should not be admin")
	}
}

func TestIsIdentHostOwner(t *testing.T) {
	n := newTestNetwork()
	if !n.IsIdentHostOwner("admin", "admin.host") {
		t.Error("admin should be recognized as owner")
	}
	if n.IsIdentHostOwner("op", "op.host") {
		t.Error("op should not be recognized as owner")
	}
}

func TestGetModesAtOnce_Default(t *testing.T) {
	n := &Network{}
	if n.GetModesAtOnce() != 4 {
		t.Errorf("expected default 4, got %d", n.GetModesAtOnce())
	}
}

func TestGetModesAtOnce_Custom(t *testing.T) {
	n := &Network{ModesAtOnce: 6}
	if n.GetModesAtOnce() != 6 {
		t.Errorf("expected 6, got %d", n.GetModesAtOnce())
	}
}
