package networks

import "testing"

// IRC nicks are case-insensitive on the wire, so a lookup for "Bob" must find
// a user tracked as "bob". The seen command relied on exact matching and would
// report "have not seen" (or a stale duplicate) for correctly-cased requests.
func TestGetUserWithNick_CaseInsensitive(t *testing.T) {
	n := newTestNetwork()

	for _, ask := range []string{"bob", "Bob", "BOB"} {
		u := n.GetUserWithNick(ask)
		if u == nil {
			t.Fatalf("GetUserWithNick(%q) = nil, want the bob entry", ask)
		}
		if u.Ident != "~bob" {
			t.Errorf("GetUserWithNick(%q) matched %q, want ident %q", ask, u.NickName, "~bob")
		}
	}
}

func TestGetUserWithNick_CaseInsensitiveDoesNotOverMatch(t *testing.T) {
	n := newTestNetwork()

	// Case folding must not turn into prefix or fuzzy matching.
	for _, ask := range []string{"b", "bo", "bob.host", "robert"} {
		if u := n.GetUserWithNick(ask); u != nil {
			t.Errorf("GetUserWithNick(%q) matched %q, want nil", ask, u.NickName)
		}
	}
}
