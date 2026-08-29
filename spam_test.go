package main

import (
	"testing"
)

func TestNormalizeForSpam(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"plain", "Hello World 123", "hello world 123"},
		{"drops apostrophes", "It's Madeleine's", "its madeleines"},
		{"drops punctuation", "Hi!!! ...really?", "hi really"},
		{"drops unicode", "café naïve", "caf nave"},
		{"collapses whitespace", "a \t\n b", "a b"},
		{"empty", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeForSpam(tt.input); got != tt.want {
				t.Errorf("normalizeForSpam(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsSpamBotMessage(t *testing.T) {
	const exact = "Hi Guys! It's Madeleine Czura! Just thought I'd leave my number here in case you're lonely"

	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"exact message", exact, true},
		{"curly apostrophes", "Hi Guys! It’s Madeleine Czura! Just thought I’d leave my number here in case you’re lonely", true},
		{"all lowercase", "hi guys! it's madeleine czura! just thought i'd leave my number here in case you're lonely", true},
		{"trailing extra text", exact + " call me maybe", true},
		{"leading prefix text", "hey everyone: " + exact, true},
		{"double spaces inside", "Hi Guys!  It's Madeleine Czura!  Just thought I'd leave my number here in case you're lonely", true},
		{"innocent greeting", "hi guys whats up", false},
		{"mentions name only", "did you see madeleine czura yesterday", false},
		{"lonely only", "im so lonely tonight", false},
		{"empty", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSpamBotMessage(tt.input); got != tt.want {
				t.Errorf("isSpamBotMessage(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
