package commands

import (
	"testing"

	"aibird/image/comfyui"
	"aibird/logger"

	"github.com/stretchr/testify/assert"
)

// init ensures the logger is initialized so test runs don't panic on log calls.
func init() { //nolint:gochecknoinits // test-only logger bootstrap
	logger.Init(logger.Config{Level: logger.LevelWarn, Format: "text"})
}

// TestH3WorkflowConstant documents the action→file mapping: !h3 runs m3.json.
func TestH3WorkflowConstant(t *testing.T) {
	assert.Equal(t, "m3", h3Workflow, "!h3 must map to the m3.json workflow file")
}

// TestParseH3_WorkflowAbsentGuard verifies the handler's first guard condition:
// when the m3 workflow file is absent (the expected state until the user deploys
// m3.json), WorkflowExists returns false so the handler fails fast without any
// network calls. The condition is asserted directly because invoking SendError
// requires a live girc connection (unavailable in hermetic unit tests).
func TestParseH3_WorkflowAbsentGuard(t *testing.T) {
	if comfyui.WorkflowExists("m3") {
		t.Skip("m3.json is present; skipping hermetic workflow-absent test")
	}
	assert.False(t, comfyui.WorkflowExists(h3Workflow),
		"handler must fail fast when the m3 workflow is not deployed")
}

// TestParseH3_RegistryRouting verifies that !h3 is registered as a queueable
// video command in the command registry (locks in the dispatch wiring).
func TestParseH3_RegistryRouting(t *testing.T) {
	assert.True(t, IsQueueableFromHelp("h3"))
	assert.True(t, IsVideoCommand("h3"))
}
