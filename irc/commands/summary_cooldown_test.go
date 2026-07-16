package commands

import (
	"testing"

	"aibird/birdbase"
)

// TestSummaryCooldownKey verifies the key is network-namespaced and the
// channel is lowercased so "#soy" and "#SOY" share one limit.
func TestSummaryCooldownKey(t *testing.T) {
	got := summaryCooldownKey("Libera", "#SoY")
	want := "summary-cooldown:Libera:#soy"
	if got != want {
		t.Errorf("summaryCooldownKey() = %q, want %q", got, want)
	}
}

// TestEnforceSummaryCooldown covers the gating, first-allowed-then-blocked
// behavior, case-insensitivity, and per-network isolation.
func TestEnforceSummaryCooldown(t *testing.T) {
	birdbase.InitMemory()
	t.Cleanup(birdbase.StopMemory)

	// Non-cooldown channels are never limited, no matter how many times called.
	for i := 0; i < 5; i++ {
		if !enforceSummaryCooldown("testnet", "#elsewhere") {
			t.Fatal("non-cooldown channel should always be allowed")
		}
	}

	// #soy: first call allowed, subsequent calls within the window blocked.
	if !enforceSummaryCooldown("testnet-allowed", "#soy") {
		t.Fatal("first #soy summary should be allowed")
	}
	for i := 0; i < 4; i++ {
		if enforceSummaryCooldown("testnet-allowed", "#soy") {
			t.Fatalf("attempt %d inside cooldown window should be blocked", i+2)
		}
	}

	// Case-insensitive channel match: "#SOY" shares the limit with "#soy".
	if enforceSummaryCooldown("testnet-allowed", "#SOY") {
		t.Fatal("#SOY should be blocked (same channel, different case)")
	}

	// Cooldown is namespaced per network: a different network's #soy is independent.
	if !enforceSummaryCooldown("testnet-other", "#soy") {
		t.Fatal("different network #soy should be independent and allowed")
	}
}
