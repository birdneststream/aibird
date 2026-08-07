package comfyui

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRealSidecars_Load validates the generated comfyuijson/*.meta.toml files
// parse via BurntSushi/toml (the production loader) and that every active
// parameter/hardcoded target has a non-empty input name. Skips if the
// gitignored comfyuijson/ dir is absent (CI). Run locally where the real
// workflows live.
func TestRealSidecars_Load(t *testing.T) {
	dir, err := filepath.Abs("../../comfyuijson")
	require.NoError(t, err)
	matches, err := filepath.Glob(filepath.Join(dir, "*.meta.toml"))
	if err != nil || len(matches) == 0 {
		t.Skip("comfyuijson/*.meta.toml not present (gitignored); run locally")
	}
	for _, m := range matches {
		workflowPath := m[:len(m)-len(".meta.toml")] + ".json"
		meta, err := GetAibirdMeta(workflowPath)
		require.NoError(t, err, m)
		assert.NotEmpty(t, meta.Command, m)
		if meta.PromptTarget.Node != "" {
			assert.NotEmpty(t, meta.PromptTarget.Input, "%s: promptTarget must have an input name", m)
		}
		for pname, p := range meta.Parameters {
			for i, tgt := range p.Targets {
				assert.NotEmpty(t, tgt.Input, "%s: parameters.%s.targets[%d] must have input", m, pname, i)
				assert.NotEmpty(t, tgt.Node, "%s: parameters.%s.targets[%d] must have node", m, pname, i)
			}
		}
		for hname, h := range meta.Hardcoded {
			for i, tgt := range h.Targets {
				assert.NotEmpty(t, tgt.Input, "%s: hardcoded.%s.targets[%d] must have input", m, hname, i)
			}
		}
	}
}
