package comfyui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdata fixtures are committed (not gitignored) so the API-format loader,
// sidecar meta, and input-application logic are exercised deterministically in
// CI without a live ComfyUI or the real (gitignored) workflow directory.

func TestLoadAPIGraph_Valid(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	require.NotNil(t, graph)
	// The fixture has nodes 4,5,6,7,3,8,9.
	assert.GreaterOrEqual(t, len(*graph), 5)
	// Node 3 is a KSampler with named inputs incl. "seed".
	n3, ok := (*graph)["3"]
	require.True(t, ok)
	assert.Equal(t, "KSampler", n3.ClassType)
	assert.Contains(t, n3.Inputs, "seed")
}

func TestLoadAPIGraph_PathTraversal(t *testing.T) {
	_, err := LoadAPIGraph("../etc/passwd")
	require.Error(t, err)
}

func TestLoadAPIGraph_Nonexistent(t *testing.T) {
	_, err := LoadAPIGraph("testdata/does_not_exist.json")
	require.Error(t, err)
}

func TestLoadAPIGraph_NotAPIFormat(t *testing.T) {
	// A UI-format file (nodes array) should fail to unmarshal into APIGraph
	// (which expects map[string]APINode). We write a stub to a temp file.
	dir := t.TempDir()
	p := filepath.Join(dir, "ui.json")
	require.NoError(t, os.WriteFile(p, []byte(`{"nodes": [], "links": [], "groups": []}`), 0644))
	_, err := LoadAPIGraph(p)
	require.Error(t, err)
}

func TestLoadSidecarMeta_FromFixture(t *testing.T) {
	// testdata/sd_api.json is the API-format graph; the sidecar
	// testdata/sd_api.meta.toml carries the metadata (derived path).
	meta, err := GetAibirdMeta("testdata/sd_api.json")
	require.NoError(t, err)
	assert.Equal(t, "SD Test", meta.Name)
	assert.Equal(t, "image", meta.Type)
	// PromptTarget uses node id "6" (unambiguous) + named input "text".
	assert.Equal(t, "6", meta.PromptTarget.Node)
	assert.Equal(t, "text", meta.PromptTarget.Input)
	// seed target uses the title "Sampler"; steps target uses class "KSampler".
	require.Contains(t, meta.Parameters, "seed")
	assert.Equal(t, "Sampler", meta.Parameters["seed"].Targets[0].Node)
	assert.Equal(t, "seed", meta.Parameters["seed"].Targets[0].Input)
	require.Contains(t, meta.Parameters, "steps")
	assert.Equal(t, "KSampler", meta.Parameters["steps"].Targets[0].Node)
	assert.Equal(t, "steps", meta.Parameters["steps"].Targets[0].Input)
}

func TestSidecarMetaPath(t *testing.T) {
	assert.Equal(t, "comfyuijson/sd.meta.toml", SidecarMetaPath("comfyuijson/sd.json"))
	assert.Equal(t, "foo/bar.meta.toml", SidecarMetaPath("foo/bar.json"))
}

func TestAPIGraph_Clone_IsolatesMutations(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	clone := graph.Clone()

	// Mutate the clone; the original must be unaffected.
	require.NoError(t, clone.ApplyInput("3", "seed", 999, false))
	assert.Equal(t, 999, (*clone)["3"].Inputs["seed"])
	origSeed, ok := (*graph)["3"].Inputs["seed"].(float64)
	require.True(t, ok, "original seed should still be the fixture value")
	assert.NotEqual(t, 999, int(origSeed))
}

func TestAPIGraph_ApplyInput_ByTitle(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	g := graph.Clone()
	// Node 6's _meta.title is "Positive" in the fixture.
	require.NoError(t, g.ApplyInput("Positive", "text", "a cat on the moon", false))
	assert.Equal(t, "a cat on the moon", (*g)["6"].Inputs["text"])
}

func TestAPIGraph_ApplyInput_AppendSemantics(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	g := graph.Clone()
	// Node 6 ("Positive") already has text "masterpiece best quality girl" in the fixture.
	require.NoError(t, g.ApplyInput("Positive", "text", "now a man", true))
	existing, ok := (*g)["6"].Inputs["text"].(string)
	require.True(t, ok)
	assert.Contains(t, existing, "masterpiece best quality girl")
	assert.Contains(t, existing, "now a man")
}

func TestAPIGraph_ApplyInput_NoMatchErrors(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	err = graph.ApplyInput("NonexistentNode", "text", "x", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no workflow node matches")
}

func TestAPIGraph_PromptJSON_RoundtripsShape(t *testing.T) {
	graph, err := LoadAPIGraph("testdata/sd_api.json")
	require.NoError(t, err)
	pj := graph.PromptJSON()
	require.Contains(t, pj, "3")
	node, ok := pj["3"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "KSampler", node["class_type"])
	require.Contains(t, node, "inputs")
	require.Contains(t, node, "_meta")
}
