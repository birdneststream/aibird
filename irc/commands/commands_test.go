package commands

import (
	"testing"

	"aibird/settings"
)

// minimalTestConfig returns a minimal config for command validation tests.
// It doesn't have ComfyUI workflows, so image/sound/video commands come from
// the hardcoded help lists only.
func minimalTestConfig() settings.AiBird {
	return settings.AiBird{
		ActionTrigger: "!",
	}
}

// TestIsValidCommand_KnownStandardCommands verifies standard commands are recognized.
func TestIsValidCommand_KnownStandardCommands(t *testing.T) {
	config := minimalTestConfig()

	standardCmds := []string{"hello", "status", "help", "seen", "support", "models", "leaderboard"}
	for _, cmd := range standardCmds {
		if !IsValidCommand(cmd, config) {
			t.Errorf("IsValidCommand(%q) should be true for standard command", cmd)
		}
	}
}

// TestIsValidCommand_CaseSensitive verifies command matching is case-sensitive.
func TestIsValidCommand_CaseSensitive(t *testing.T) {
	config := minimalTestConfig()

	if IsValidCommand("Hello", config) {
		t.Error("IsValidCommand should be case-sensitive: 'Hello' should not match 'hello'")
	}
	if IsValidCommand("STATUS", config) {
		t.Error("IsValidCommand should be case-sensitive: 'STATUS' should not match 'status'")
	}
}

// TestIsValidCommand_UnknownCommand verifies unknown commands return false.
func TestIsValidCommand_UnknownCommand(t *testing.T) {
	config := minimalTestConfig()

	unknownCmds := []string{"foo", "bar", "nonexistent", ""}
	for _, cmd := range unknownCmds {
		if IsValidCommand(cmd, config) {
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
	config := minimalTestConfig()

	// AI commands should not be valid when AI is disabled
	if IsValidCommandForChannel("ai", config, false, true, true, true, false, false) {
		t.Error("'ai' command should not be valid when AI is disabled")
	}

	// AI commands should be valid when AI is enabled
	if !IsValidCommandForChannel("ai", config, true, true, true, true, false, false) {
		t.Error("'ai' command should be valid when AI is enabled")
	}
}

// TestIsValidCommandForChannel_StandardAlwaysAvailable verifies standard commands are always available.
func TestIsValidCommandForChannel_StandardAlwaysAvailable(t *testing.T) {
	config := minimalTestConfig()

	// Standard commands should work regardless of feature flags
	if !IsValidCommandForChannel("hello", config, false, false, false, false, false, false) {
		t.Error("'hello' should always be valid")
	}
	if !IsValidCommandForChannel("status", config, false, false, false, false, false, false) {
		t.Error("'status' should always be valid")
	}
}

// TestIsValidCommandForChannel_AdminCommands verifies admin commands require admin flag.
func TestIsValidCommandForChannel_AdminCommands(t *testing.T) {
	config := minimalTestConfig()

	// Admin commands should not be valid for non-admin
	if IsValidCommandForChannel("op", config, true, true, true, true, false, false) {
		t.Error("'op' should not be valid for non-admin user")
	}

	// Admin commands should be valid for admin
	if !IsValidCommandForChannel("op", config, true, true, true, true, true, false) {
		t.Error("'op' should be valid for admin user")
	}
}

// TestGetAllCommands_ReturnsKnownCommands verifies GetAllCommands returns at least standard commands.
func TestGetAllCommands_ReturnsKnownCommands(t *testing.T) {
	config := minimalTestConfig()

	commands := GetAllCommands(config, true, true, true, true, false, false)

	// Should contain at least the standard commands
	cmdMap := make(map[string]bool)
	for _, cmd := range commands {
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
	config := minimalTestConfig()

	commands := GetAllCommandsUnfiltered(config)

	// Should contain commands from all categories
	cmdMap := make(map[string]bool)
	for _, cmd := range commands {
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
