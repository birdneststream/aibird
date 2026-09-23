package channels

import (
	"testing"

	"aibird/irc/users"
)

func TestGetUserWithNick_MatchesCaseInsensitively(t *testing.T) {
	// IRC treats nicks case-insensitively: a user present as "Bob" answers to
	// "!seen bob". Exact matching used to miss and surface a stale duplicate.
	channel := &Channel{
		Name: "#test",
		Users: []*users.User{
			{NickName: "Bob", Ident: "bob", Host: "example.org", LatestActivity: 100},
		},
	}

	for _, ask := range []string{"Bob", "bob", "BOB", "bOb"} {
		got, err := channel.GetUserWithNick(ask)
		if err != nil {
			t.Fatalf("GetUserWithNick(%q) error: %v", ask, err)
		}
		if got == nil {
			t.Fatalf("GetUserWithNick(%q) = nil, want the Bob entry", ask)
		}
	}
}

func TestGetUserWithNick_PicksMostRecentDuplicate(t *testing.T) {
	// Same nick seen under two hosts: the one with the newest activity wins.
	channel := &Channel{
		Name: "#test",
		Users: []*users.User{
			{NickName: "alice", Ident: "alice", Host: "old.example.org", LatestActivity: 100},
			{NickName: "alice", Ident: "alice", Host: "new.example.org", LatestActivity: 900},
		},
	}

	got, err := channel.GetUserWithNick("Alice")
	if err != nil {
		t.Fatalf("GetUserWithNick error: %v", err)
	}
	if got == nil {
		t.Fatal("GetUserWithNick = nil")
	}
	if got.Host != "new.example.org" {
		t.Errorf("GetUserWithNick picked host %q, want the one with newest activity", got.Host)
	}
}

func TestGetUserWithNick_UnknownNickReturnsNil(t *testing.T) {
	channel := &Channel{
		Name:  "#test",
		Users: []*users.User{{NickName: "alice", Ident: "alice", Host: "example.org"}},
	}

	got, _ := channel.GetUserWithNick("carol")
	if got != nil {
		t.Errorf("GetUserWithNick = %+v, want nil for an unrelated nick", got)
	}
}
