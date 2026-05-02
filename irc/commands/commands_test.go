package commands

import (
	"testing"

	"aibird/logger"
)

func TestMain(m *testing.M) {
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
	m.Run()
}

// TestIsValidCommand_KnownStandardCommands verifies standard commands are recognized.
func TestIsValidCommand_KnownStandardCommands(t *testing.T) {
	standardCmds := []string{"hello", "status", "help", "seen", "support", "models", "leaderboard"}
	for _, cmd := range standardCmds {
		if !IsValidCommand(cmd) {
			t.Errorf("IsValidCommand(%q) should be true for standard command", cmd)
		}
	}
}

// TestIsValidCommand_CaseSensitive verifies command matching is case-sensitive.
func TestIsValidCommand_CaseSensitive(t *testing.T) {
	if IsValidCommand("Hello") {
		t.Error("IsValidCommand should be case-sensitive: 'Hello' should not match 'hello'")
	}
	if IsValidCommand("STATUS") {
		t.Error("IsValidCommand should be case-sensitive: 'STATUS' should not match 'status'")
	}
}

// TestIsValidCommand_UnknownCommand verifies unknown commands return false.
func TestIsValidCommand_UnknownCommand(t *testing.T) {
	unknownCmds := []string{"foo", "bar", "nonexistent", ""}
	for _, cmd := range unknownCmds {
		if IsValidCommand(cmd) {
			t.Errorf("IsValidCommand(%q) should be false for unknown command", cmd)
		}
	}
}

// TestIsStandardCommand verifies standard command classification.
func TestIsStandardCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"hello", true},
		{"status", true},
		{"help", true},
		{"seen", true},
		{"support", true},
		{"models", true},
		{"leaderboard", true},
		{"headlies", true},
		{"ircnews", true},
		{"play", true},
		{"record", true},
		{"debug", false}, // owner command
		{"user", false},  // admin command
		{"ai", false},    // text command
		{"", false},
		{"unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := IsStandardCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsStandardCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}

// TestIsAdminCommand verifies admin command classification.
func TestIsAdminCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"user", true},
		{"channel", true},
		{"network", true},
		{"sync", true},
		{"op", true},
		{"deop", true},
		{"voice", true},
		{"devoice", true},
		{"kick", true},
		{"ban", true},
		{"unban", true},
		{"topic", true},
		{"join", true},
		{"part", true},
		{"ignore", true},
		{"unignore", true},
		{"nick", true},
		{"clearqueue", true},
		{"removecurrent", true},
		{"hello", false}, // standard command
		{"debug", false}, // owner command
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := IsAdminCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsAdminCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}

// TestIsOwnerCommand verifies owner command classification.
func TestIsOwnerCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"debug", true},
		{"save", true},
		{"ip", true},
		{"dbstats", true},
		{"hello", false},
		{"user", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := IsOwnerCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsOwnerCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}

// TestIsTextCommand verifies text command classification.
func TestIsTextCommand(t *testing.T) {
	tests := []struct {
		cmd      string
		expected bool
	}{
		{"ai", true},
		{"glm", true},
		{"glm-img", true},
		{"hello", false},
		{"debug", false},
	}

	for _, tt := range tests {
		t.Run(tt.cmd, func(t *testing.T) {
			result := IsTextCommand(tt.cmd)
			if result != tt.expected {
				t.Errorf("IsTextCommand(%q) = %v, want %v", tt.cmd, result, tt.expected)
			}
		})
	}
}

// TestIsTextCommand_CaseInsensitive verifies text command matching is case-insensitive.
func TestIsTextCommand_CaseInsensitive(t *testing.T) {
	if !IsTextCommand("AI") {
		t.Error("IsTextCommand should be case-insensitive: 'AI' should match")
	}
	if !IsTextCommand("Glm") {
		t.Error("IsTextCommand should be case-insensitive: 'Glm' should match")
	}
}

// TestIsValidCommandForChannel_AiDisabled verifies AI commands filtered when AI is off.
func TestIsValidCommandForChannel_AiDisabled(t *testing.T) {
	// AI commands should not be valid when AI is disabled
	if IsValidCommandForChannel("ai", false, true, true, true, false, false) {
		t.Error("'ai' command should not be valid when AI is disabled")
	}

	// AI commands should be valid when AI is enabled
	if !IsValidCommandForChannel("ai", true, true, true, true, false, false) {
		t.Error("'ai' command should be valid when AI is enabled")
	}
}

// TestIsValidCommandForChannel_StandardAlwaysAvailable verifies standard commands are always available.
func TestIsValidCommandForChannel_StandardAlwaysAvailable(t *testing.T) {
	// Standard commands should work regardless of feature flags
	if !IsValidCommandForChannel("hello", false, false, false, false, false, false) {
		t.Error("'hello' should always be valid")
	}
	if !IsValidCommandForChannel("status", false, false, false, false, false, false) {
		t.Error("'status' should always be valid")
	}
}

// TestIsValidCommandForChannel_AdminCommands verifies admin commands require admin flag.
func TestIsValidCommandForChannel_AdminCommands(t *testing.T) {
	// Admin commands should not be valid for non-admin
	if IsValidCommandForChannel("op", true, true, true, true, false, false) {
		t.Error("'op' should not be valid for non-admin user")
	}

	// Admin commands should be valid for admin
	if !IsValidCommandForChannel("op", true, true, true, true, true, false) {
		t.Error("'op' should be valid for admin user")
	}
}

// TestGetAllCommands_ReturnsKnownCommands verifies GetAllCommands returns at least standard commands.
func TestGetAllCommands_ReturnsKnownCommands(t *testing.T) {
	cmds := GetAllCommands(true, true, true, true, false, false)

	cmdMap := make(map[string]bool)
	for _, cmd := range cmds {
		cmdMap[cmd] = true
	}

	for _, expected := range []string{"hello", "status", "help", "seen"} {
		if !cmdMap[expected] {
			t.Errorf("Expected %q in GetAllCommands result", expected)
		}
	}
}

// TestGetAllCommandsUnfiltered_ReturnsAll verifies unfiltered returns everything.
func TestGetAllCommandsUnfiltered_ReturnsAll(t *testing.T) {
	cmds := GetAllCommandsUnfiltered()

	cmdMap := make(map[string]bool)
	for _, cmd := range cmds {
		cmdMap[cmd] = true
	}

	// Standard
	if !cmdMap["hello"] {
		t.Error("Should contain standard command 'hello'")
	}
	// Admin (unfiltered includes admin)
	if !cmdMap["user"] {
		t.Error("Should contain admin command 'user'")
	}
	// Owner (unfiltered includes owner)
	if !cmdMap["debug"] {
		t.Error("Should contain owner command 'debug'")
	}
	// Text (unfiltered includes AI)
	if !cmdMap["ai"] {
		t.Error("Should contain text command 'ai'")
	}
}

// TestIsQueueableFromHelp verifies queueable flag from registry.
func TestIsQueueableFromHelp(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"ai", true},       // text command, queueable
		{"glm", false},     // text command, not queueable
		{"hello", false},   // standard command, not queueable
		{"user", false},    // admin command, not queueable
		{"debug", false},   // owner command, not queueable
		{"unknown", false}, // not in registry
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			if got := IsQueueableFromHelp(tt.action); got != tt.want {
				t.Errorf("IsQueueableFromHelp(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

// TestGetAllCommands_Sorted verifies GetAllCommands returns sorted output.
func TestGetAllCommands_Sorted(t *testing.T) {
	cmds := GetAllCommands(true, true, true, true, false, false)
	for i := 1; i < len(cmds); i++ {
		if cmds[i] < cmds[i-1] {
			t.Errorf("GetAllCommands not sorted: %q > %q at index %d", cmds[i-1], cmds[i], i)
		}
	}
}
