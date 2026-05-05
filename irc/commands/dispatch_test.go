package commands

import (
	"testing"

	"aibird/image/comfyui"
	"aibird/irc/state"

	"github.com/stretchr/testify/assert"
)

// findCommandByType queries the command registry for any command of the given type.
// Returns the command name or empty string if none found.
func findCommandByType(cmdType string) string {
	r := getRegistry()
	for _, entry := range r.byLower {
		if entry.Type == cmdType {
			return entry.Name
		}
	}
	return ""
}

// TestIsQueueableFromHelp_ComfyUIFallback verifies that unknown commands matching
// cached ComfyUI workflows are considered queueable.
func TestIsQueueableFromHelp_ComfyUIFallback(t *testing.T) {
	workflows := comfyui.GetCachedWorkflows()
	if len(workflows) == 0 {
		t.Skip("no ComfyUI workflows available in cache")
	}

	// The first cached workflow should be queueable even if not in the registry
	wf := workflows[0]
	assert.True(t, IsQueueableFromHelp(wf), "cached workflow %q should be queueable", wf)
}

// TestIsQueueableFromHelp_UnknownNotQueueable verifies that unknown commands
// with no workflow match are not queueable.
func TestIsQueueableFromHelp_UnknownNotQueueable(t *testing.T) {
	assert.False(t, IsQueueableFromHelp("totally-unknown-command-xyz123"))
}

// TestIsImageCommand_RegistryMatch verifies registry-based image command detection
// by querying the actual registry for a registered image command.
func TestIsImageCommand_RegistryMatch(t *testing.T) {
	cmd := findCommandByType(cmdTypeImage)
	if cmd == "" {
		t.Skip("no image commands in registry")
	}
	assert.True(t, isImageCommand(cmd), "registered image command %q should match", cmd)
}

// TestIsImageCommand_NoMatch verifies non-image commands return false.
func TestIsImageCommand_NoMatch(t *testing.T) {
	assert.False(t, isImageCommand("hello"))
	assert.False(t, isImageCommand("ai"))
	assert.False(t, isImageCommand("unknown-xyz"))
}

// TestIsImageCommand_ComfyUIMeta verifies ComfyUI workflow metadata matching.
func TestIsImageCommand_ComfyUIMeta(t *testing.T) {
	workflows := comfyui.GetCachedWorkflows()
	if len(workflows) == 0 {
		t.Skip("no ComfyUI workflows available in cache")
	}

	// Find a workflow with type "image" if any
	for _, wf := range workflows {
		meta := comfyui.GetCachedMeta(wf)
		if meta != nil && meta.Type == "image" {
			assert.True(t, isImageCommand(wf), "workflow %q with type image should match", wf)
			return
		}
	}
	t.Skip("no image workflows found in cache")
}

// TestIsVideoCommand_RegistryMatch verifies registry-based video command detection
// by querying the actual registry for a registered video command.
func TestIsVideoCommand_RegistryMatch(t *testing.T) {
	cmd := findCommandByType(cmdTypeVideo)
	if cmd == "" {
		t.Skip("no video commands in registry")
	}
	assert.True(t, isVideoCommand(cmd), "registered video command %q should match", cmd)
}

// TestIsVideoCommand_NoMatch verifies non-video commands return false.
func TestIsVideoCommand_NoMatch(t *testing.T) {
	assert.False(t, isVideoCommand("hello"))
	assert.False(t, isVideoCommand("ai"))
}

// TestIsVideoCommand_ComfyUIMeta verifies ComfyUI workflow metadata matching for video.
func TestIsVideoCommand_ComfyUIMeta(t *testing.T) {
	workflows := comfyui.GetCachedWorkflows()
	if len(workflows) == 0 {
		t.Skip("no ComfyUI workflows available in cache")
	}

	for _, wf := range workflows {
		meta := comfyui.GetCachedMeta(wf)
		if meta != nil && meta.Type == "video" {
			assert.True(t, isVideoCommand(wf), "workflow %q with type video should match", wf)
			return
		}
	}
	t.Skip("no video workflows found in cache")
}

// TestIsSoundCommand_RegistryMatch verifies registry-based sound command detection
// by querying the actual registry for a registered sound command.
func TestIsSoundCommand_RegistryMatch(t *testing.T) {
	cmd := findCommandByType(cmdTypeSound)
	if cmd == "" {
		t.Skip("no sound commands in registry")
	}
	assert.True(t, isSoundCommand(cmd), "registered sound command %q should match", cmd)
}

// TestIsSoundCommand_NoMatch verifies non-sound commands return false.
func TestIsSoundCommand_NoMatch(t *testing.T) {
	assert.False(t, isSoundCommand("hello"))
	assert.False(t, isSoundCommand("ai"))
}

// TestIsSoundCommand_ComfyUIMeta verifies ComfyUI workflow metadata matching for sound.
func TestIsSoundCommand_ComfyUIMeta(t *testing.T) {
	workflows := comfyui.GetCachedWorkflows()
	if len(workflows) == 0 {
		t.Skip("no ComfyUI workflows available in cache")
	}

	for _, wf := range workflows {
		meta := comfyui.GetCachedMeta(wf)
		if meta != nil && meta.Type == "sound" {
			assert.True(t, isSoundCommand(wf), "workflow %q with type sound should match", wf)
			return
		}
	}
	t.Skip("no sound workflows found in cache")
}

// TestIsQueueableCommand_EmptyAction verifies empty action returns false.
func TestIsQueueableCommand_EmptyAction(t *testing.T) {
	s := state.State{Command: state.Command{Action: ""}}
	assert.False(t, IsQueueableCommand(s))
}

// TestIsQueueableCommand_QueueableAction verifies a known queueable action returns true.
func TestIsQueueableCommand_QueueableAction(t *testing.T) {
	s := state.State{Command: state.Command{Action: "ai"}}
	assert.True(t, IsQueueableCommand(s))
}

// TestIsQueueableCommand_NonQueueableAction verifies a known non-queueable action returns false.
func TestIsQueueableCommand_NonQueueableAction(t *testing.T) {
	s := state.State{Command: state.Command{Action: "hello"}}
	assert.False(t, IsQueueableCommand(s))
}
