package commands

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildHighlightChunks(t *testing.T) {
	tests := []struct {
		name       string
		nicks      []string
		exclude    string
		maxBytes   int
		wantChunks []string
	}{
		{
			name:       "empty input",
			nicks:      nil,
			exclude:    "aibird",
			maxBytes:   450,
			wantChunks: nil,
		},
		{
			name:       "single chunk",
			nicks:      []string{"alice", "bob", "carol"},
			exclude:    "aibird",
			maxBytes:   450,
			wantChunks: []string{"alice bob carol"},
		},
		{
			name:       "dedupe case-insensitive and exclude bot",
			nicks:      []string{"alice", "Alice", "ALICE", "aibird", "AiBird", "bob"},
			exclude:    "aibird",
			maxBytes:   450,
			wantChunks: []string{"alice bob"},
		},
		{
			name:       "splits exactly on byte budget",
			nicks:      []string{"aaaaaaaa", "bbbbbbbb", "cccccccc"},
			exclude:    "",
			maxBytes:   17, // 8 + 1 + 8 = 17 exactly; third nick must move on
			wantChunks: []string{"aaaaaaaa bbbbbbbb", "cccccccc"},
		},
		{
			name:       "overlong single nick skipped",
			nicks:      []string{"x", strings.Repeat("n", 20)},
			exclude:    "",
			maxBytes:   10,
			wantChunks: []string{"x"},
		},
		{
			name:       "overlong nick first emits no empty chunk",
			nicks:      []string{strings.Repeat("n", 20), "x"},
			exclude:    "",
			maxBytes:   10,
			wantChunks: []string{"x"},
		},
		{
			name:       "whitespace-only nicks dropped",
			nicks:      []string{"  ", "alice", ""},
			exclude:    "",
			maxBytes:   450,
			wantChunks: []string{"alice"},
		},
		{
			name:       "zero max returns nil",
			nicks:      []string{"alice"},
			exclude:    "",
			maxBytes:   0,
			wantChunks: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := BuildHighlightChunks(tt.nicks, tt.exclude, tt.maxBytes)
			assert.Equal(t, tt.wantChunks, got)
			for _, chunk := range got {
				assert.LessOrEqual(t, len(chunk), tt.maxBytes, "chunk exceeds byte budget")
			}
		})
	}
}

func TestClassifyLiveMessage(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    liveAction
	}{
		{"empty is status", "", liveActionStatus},
		{"whitespace is status", "   ", liveActionStatus},
		{"off", "off", liveActionOff},
		{"OFF case-insensitive", "OFF", liveActionOff},
		{"end synonym", "end", liveActionOff},
		{"end with padding", " end ", liveActionOff},
		{"watch url", "https://www.youtube.com/watch?v=dQw4w9WgXcQ", liveActionStart},
		{"share url", "https://youtu.be/dQw4w9WgXcQ", liveActionStart},
		{"live path url", "https://www.youtube.com/live/dQw4w9WgXcQ", liveActionStart},
		{"bare video ID invalid", "dQw4w9WgXcQ", liveActionInvalid},
		{"other site invalid", "https://twitch.tv/foo", liveActionInvalid},
		{"garbage invalid", "i am live now", liveActionInvalid},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, classifyLiveMessage(tt.message))
		})
	}
}

func TestLiveCommandRegistration(t *testing.T) {
	if !IsAdminCommand("live") {
		t.Error("IsAdminCommand(live) should be true")
	}
	if !IsValidCommand("live") {
		t.Error("IsValidCommand(live) should be true")
	}
	if IsStandardCommand("live") {
		t.Error("IsStandardCommand(live) should be false")
	}
	if IsOwnerCommand("live") {
		t.Error("IsOwnerCommand(live) should be false")
	}
	if IsQueueableFromHelp("live") {
		t.Error("!live must not be queueable — it is immediate")
	}
}
