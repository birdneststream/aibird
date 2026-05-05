package commands

import (
	"strings"
	"testing"
)

func TestFormatLeaderboardTable_FourColumns(t *testing.T) {
	rows := [][]string{
		{" 1", "testuser        ", "ai              ", "   42"},
		{" 2", "otheruser       ", "glm             ", "   10"},
	}

	lines := formatLeaderboardTable(
		[]int{4, 17, 17, 7},
		[]string{"#", "Nickname", "Command", "Count"},
		rows,
	)

	expected := []string{
		"┌────┬─────────────────┬─────────────────┬───────┐",
		"│ #  │ Nickname        │ Command         │ Count │",
		"├────┼─────────────────┼─────────────────┼───────┤",
		"│  1 │ testuser        │ ai              │    42 │",
		"│  2 │ otheruser       │ glm             │    10 │",
		"└────┴─────────────────┴─────────────────┴───────┘",
	}

	if len(lines) != len(expected) {
		t.Fatalf("expected %d lines, got %d", len(expected), len(lines))
	}

	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d mismatch:\n  expected: %q\n  got:      %q", i, expected[i], line)
		}
	}
}

func TestFormatLeaderboardTable_FiveColumns(t *testing.T) {
	rows := [][]string{
		{" 1", "supernets       ", "testuser        ", "ai              ", "   42"},
	}

	lines := formatLeaderboardTable(
		[]int{4, 17, 17, 17, 7},
		[]string{"#", "Network", "Nickname", "Command", "Count"},
		rows,
	)

	expected := []string{
		"┌────┬─────────────────┬─────────────────┬─────────────────┬───────┐",
		"│ #  │ Network         │ Nickname        │ Command         │ Count │",
		"├────┼─────────────────┼─────────────────┼─────────────────┼───────┤",
		"│  1 │ supernets       │ testuser        │ ai              │    42 │",
		"└────┴─────────────────┴─────────────────┴─────────────────┴───────┘",
	}

	if len(lines) != len(expected) {
		t.Fatalf("expected %d lines, got %d", len(expected), len(lines))
	}

	for i, line := range lines {
		if line != expected[i] {
			t.Errorf("line %d mismatch:\n  expected: %q\n  got:      %q", i, expected[i], line)
		}
	}
}

func TestFormatLeaderboardTable_EmptyRows(t *testing.T) {
	lines := formatLeaderboardTable(
		[]int{4, 17, 7},
		[]string{"#", "Name", "Count"},
		nil,
	)

	if len(lines) != 4 {
		t.Fatalf("expected 4 lines (header+separator+footer), got %d", len(lines))
	}

	// Should have header border, header row, separator, footer border
	if !strings.HasPrefix(lines[0], "┌") {
		t.Error("first line should start with ┌")
	}
	if !strings.HasPrefix(lines[len(lines)-1], "└") {
		t.Error("last line should start with └")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		input  string
		maxLen int
		want   string
	}{
		{"short", 10, "short"},
		{"exactlen12345", 14, "exactlen12345"},
		{"this is a long string", 10, "this is..."},
		{"ab", 3, "ab"},
		{"abc", 2, "ab"},
	}

	for _, tt := range tests {
		result := truncateString(tt.input, tt.maxLen)
		if result != tt.want {
			t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.want)
		}
	}
}

func TestFormatLeaderboardTable_MismatchedHeaders(t *testing.T) {
	result := formatLeaderboardTable([]int{4, 7}, []string{"only_one"}, nil)
	if result != nil {
		t.Error("expected nil for mismatched headers/widths")
	}
}
