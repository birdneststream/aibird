package comfyui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"

	"github.com/BurntSushi/toml"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	logger.Init(logger.Config{Level: "error", Format: "text"})
}

// =============================================================================
// workflowFile tests
// =============================================================================

func TestWorkflowFile(t *testing.T) {
	tests := []struct {
		model    string
		expected string
	}{
		{"sd", "comfyuijson/sd.json"},
		{"flux-img2img", "comfyuijson/flux-img2img.json"},
		{"", "comfyuijson/.json"},
	}
	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			assert.Equal(t, tt.expected, workflowFile(tt.model))
		})
	}
}

// =============================================================================
// GetAibirdMeta tests
// =============================================================================

// buildWorkflowJSON creates a valid ComfyUI workflow JSON with an aibird_meta node.
func buildWorkflowJSON(metaTOML string) []byte {
	// Build a minimal workflow with the aibird_meta node
	metaNode := map[string]interface{}{
		"title": "aibird_meta",
		"widgets_values": []interface{}{
			metaTOML,
		},
	}

	workflow := map[string]interface{}{
		"nodes": []interface{}{metaNode},
	}
	data, _ := json.Marshal(workflow)
	return data
}

func TestGetAibirdMeta_ValidWorkflow(t *testing.T) {
	metaTOML := `
name = "test-model"
command = "test"
description = "A test workflow"
type = "image"
accessLevel = 0

[promptTarget]
node = "sampler"
widget_index = 0
`
	data := buildWorkflowJSON(metaTOML)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	meta, err := GetAibirdMeta(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "test-model", meta.Name)
	assert.Equal(t, "test", meta.Command)
	assert.Equal(t, "A test workflow", meta.Description)
	assert.Equal(t, "image", meta.Type)
	assert.Equal(t, 0, meta.AccessLevel)
	assert.Equal(t, "sampler", meta.PromptTarget.Node)
	assert.Equal(t, 0, meta.PromptTarget.WidgetIndex)
}

func TestGetAibirdMeta_WithParameters(t *testing.T) {
	metaTOML := `
name = "test-params"
command = "tparams"
description = "Test params"
type = "image"
accessLevel = 1

[promptTarget]
node = "sampler"
widget_index = 0

[parameters.seed]
type = "int"
default = -1
description = "Random seed"

[[parameters.seed.targets]]
node = "sampler"
widget_index = 1
`
	data := buildWorkflowJSON(metaTOML)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "params.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	meta, err := GetAibirdMeta(tmpFile)
	require.NoError(t, err)
	assert.NotNil(t, meta.Parameters["seed"])
	assert.Equal(t, "int", meta.Parameters["seed"].Type)
	assert.Len(t, meta.Parameters["seed"].Targets, 1)
	assert.Equal(t, "sampler", meta.Parameters["seed"].Targets[0].Node)
}

func TestGetAibirdMeta_PathTraversal(t *testing.T) {
	_, err := GetAibirdMeta("../../etc/passwd")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "invalid")
}

func TestGetAibirdMeta_NonexistentFile(t *testing.T) {
	_, err := GetAibirdMeta("nonexistent_file.json")
	assert.Error(t, err)
}

func TestGetAibirdMeta_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "bad.json")
	require.NoError(t, os.WriteFile(tmpFile, []byte("not json"), 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unmarshal")
}

func TestGetAibirdMeta_NoNodes(t *testing.T) {
	workflow := map[string]interface{}{}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nonodes.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no nodes")
}

func TestGetAibirdMeta_NoMetaNode(t *testing.T) {
	workflow := map[string]interface{}{
		"nodes": []interface{}{
			map[string]interface{}{"title": "sampler", "widgets_values": []interface{}{"data"}},
		},
	}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nometa.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no aibird_meta node")
}

func TestGetAibirdMeta_NoWidgetValues(t *testing.T) {
	metaNode := map[string]interface{}{
		"title": "aibird_meta",
	}
	workflow := map[string]interface{}{
		"nodes": []interface{}{metaNode},
	}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nowidgets.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no widget_values")
}

func TestGetAibirdMeta_V1LegacyProperties(t *testing.T) {
	metaTOML := `
name = "v1-legacy"
command = "legacy"
description = "V1 legacy format"
type = "image"
accessLevel = 0

[promptTarget]
node = "sampler"
widget_index = 0
`
	// V1 format: meta title is in properties.title, not node.title
	metaNode := map[string]interface{}{
		"properties": map[string]interface{}{
			"title": "aibird_meta",
		},
		"widgets_values": []interface{}{metaTOML},
	}
	workflow := map[string]interface{}{
		"nodes": []interface{}{metaNode},
	}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "v1legacy.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	meta, err := GetAibirdMeta(tmpFile)
	require.NoError(t, err)
	assert.Equal(t, "v1-legacy", meta.Name)
}

func TestGetAibirdMeta_InvalidTOML(t *testing.T) {
	metaNode := map[string]interface{}{
		"title": "aibird_meta",
		"widgets_values": []interface{}{
			"this is not valid [[[[TOML",
		},
	}
	workflow := map[string]interface{}{
		"nodes": []interface{}{metaNode},
	}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "badtoml.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "TOML")
}

// =============================================================================
// parseUserInput tests
// =============================================================================

func newState(arguments []state.Argument, action, message string) *state.State {
	return &state.State{
		Command:   state.Command{Action: action, Message: message},
		Arguments: arguments,
		Config:    &settings.Config{},
	}
}

func TestParseUserInput_StringType(t *testing.T) {
	irc := newState(nil, "sd", "a woman walking")
	paramDef := ParameterDef{Type: "string"}
	result := parseUserInput(irc, "prompt", &paramDef, "hello world")
	assert.Equal(t, "hello world", result)
}

func TestParseUserInput_BoolType(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	result := parseUserInput(irc, "verbose", &paramDef, true)
	assert.Equal(t, true, result)
}

func TestParseUserInput_BoolTypeFalse(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	result := parseUserInput(irc, "verbose", &paramDef, false)
	assert.Equal(t, false, result)
}

func TestParseUserInput_BoolTypeFullblocks(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	// fullblocks inverts: --fullblocks=true → false (halfblock mode disabled)
	result := parseUserInput(irc, "fullblocks", &paramDef, true)
	assert.Equal(t, false, result)
}

func TestParseUserInput_BoolTypeFullblocksFalse(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	result := parseUserInput(irc, "fullblocks", &paramDef, false)
	assert.Equal(t, true, result)
}

func TestParseUserInput_IntType(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "int"}
	result := parseUserInput(irc, "steps", &paramDef, "30")
	assert.Equal(t, int64(30), result)
}

func TestParseUserInput_IntTypeInvalid(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "int"}
	result := parseUserInput(irc, "steps", &paramDef, "abc")
	errVal, ok := result.(error)
	assert.True(t, ok, "should return an error for invalid int")
	assert.Contains(t, errVal.Error(), "expected an int")
}

func TestParseUserInput_IntTypeMinBound(t *testing.T) {
	irc := newState(nil, "sd", "test")
	minVal := 5.0
	paramDef := ParameterDef{Type: "int", Min: &minVal}
	result := parseUserInput(irc, "steps", &paramDef, "3")
	errVal, ok := result.(error)
	assert.True(t, ok, "should return error for below min")
	assert.Contains(t, errVal.Error(), "too low")
}

func TestParseUserInput_IntTypeMaxBound(t *testing.T) {
	irc := newState(nil, "sd", "test")
	maxVal := 50.0
	paramDef := ParameterDef{Type: "int", Max: &maxVal}
	result := parseUserInput(irc, "steps", &paramDef, "100")
	errVal, ok := result.(error)
	assert.True(t, ok, "should return error for above max")
	assert.Contains(t, errVal.Error(), "too high")
}

func TestParseUserInput_IntTypeWithinBounds(t *testing.T) {
	irc := newState(nil, "sd", "test")
	minVal := 1.0
	maxVal := 100.0
	paramDef := ParameterDef{Type: "int", Min: &minVal, Max: &maxVal}
	result := parseUserInput(irc, "steps", &paramDef, "50")
	assert.Equal(t, int64(50), result)
}

func TestParseUserInput_FloatType(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "float"}
	result := parseUserInput(irc, "cfg", &paramDef, "7.5")
	assert.Equal(t, 7.5, result)
}

func TestParseUserInput_FloatTypeInvalid(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "float"}
	result := parseUserInput(irc, "cfg", &paramDef, "notafloat")
	errVal, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, errVal.Error(), "expected a float")
}

func TestParseUserInput_FloatTypeMinBound(t *testing.T) {
	irc := newState(nil, "sd", "test")
	minVal := 0.0
	paramDef := ParameterDef{Type: "float", Min: &minVal}
	result := parseUserInput(irc, "cfg", &paramDef, "-1.0")
	errVal, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, errVal.Error(), "too low")
}

func TestParseUserInput_FloatTypeMaxBound(t *testing.T) {
	irc := newState(nil, "sd", "test")
	maxVal := 20.0
	paramDef := ParameterDef{Type: "float", Max: &maxVal}
	result := parseUserInput(irc, "cfg", &paramDef, "25.0")
	errVal, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, errVal.Error(), "too high")
}

func TestParseUserInput_UnsupportedType(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "vector3d"}
	result := parseUserInput(irc, "pos", &paramDef, "data")
	errVal, ok := result.(error)
	assert.True(t, ok)
	assert.Contains(t, errVal.Error(), "unsupported parameter type")
}

func TestParseUserInput_NilRawInput(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "string"}
	result := parseUserInput(irc, "prompt", &paramDef, nil)
	assert.Nil(t, result)
}

// =============================================================================
// processParameter tests
// =============================================================================

func TestProcessParameter_RequiredMissing(t *testing.T) {
	irc := newState(nil, "sd", "test")
	paramDef := ParameterDef{Type: "string", Required: true, Description: "The prompt text"}
	result := processParameter(irc, "prompt", &paramDef)
	errVal, ok := result.(error)
	assert.True(t, ok, "should return error for missing required param")
	assert.Contains(t, errVal.Error(), "required")
	assert.Contains(t, errVal.Error(), "prompt text")
}

func TestProcessParameter_BoolDefaultFalse(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	result := processParameter(irc, "verbose", &paramDef)
	assert.Equal(t, false, result)
}

func TestProcessParameter_BoolFullblocksDefault(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	paramDef := ParameterDef{Type: "bool"}
	// fullblocks defaults to true (halfblock mode)
	result := processParameter(irc, "fullblocks", &paramDef)
	assert.Equal(t, true, result)
}

func TestProcessParameter_StringDefault(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	paramDef := ParameterDef{Type: "string", Default: "default_value"}
	result := processParameter(irc, "style", &paramDef)
	assert.Equal(t, "default_value", result)
}

func TestProcessParameter_SeedRandom(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	paramDef := ParameterDef{Type: "int", Default: -1}
	result := processParameter(irc, "seed", &paramDef)
	seedVal, ok := result.(int64)
	assert.True(t, ok, "seed should be int64")
	assert.True(t, seedVal >= 0, "seed should be non-negative")
}

func TestProcessParameter_VoiceSuffix(t *testing.T) {
	irc := newState([]state.Argument{
		{Key: "voice", Value: "alice"},
	}, "tts", "hello")
	paramDef := ParameterDef{Type: "string"}
	result := processParameter(irc, "voice", &paramDef)
	assert.Equal(t, "alice.wav", result)
}

func TestProcessParameter_VoiceAlreadyHasSuffix(t *testing.T) {
	irc := newState([]state.Argument{
		{Key: "voice", Value: "alice.wav"},
	}, "tts", "hello")
	paramDef := ParameterDef{Type: "string"}
	result := processParameter(irc, "voice", &paramDef)
	assert.Equal(t, "alice.wav", result)
}

// =============================================================================
// applyParameters tests
// =============================================================================

func TestApplyParameters_PromptTarget(t *testing.T) {
	irc := newState(nil, "sd", "a woman walking")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{Node: "sampler", WidgetIndex: 0},
		Parameters:   map[string]ParameterDef{},
		Hardcoded:    map[string]HardcodedValue{},
	}

	result, err := applyParameters(irc, meta, "a woman walking", "cuda:0")
	require.NoError(t, err)
	assert.Equal(t, "a woman walking", result["sampler"][0])
}

func TestApplyParameters_EmptyPromptTarget(t *testing.T) {
	irc := newState(nil, "sd", "test")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{}, // empty
		Parameters:   map[string]ParameterDef{},
		Hardcoded:    map[string]HardcodedValue{},
	}

	result, err := applyParameters(irc, meta, "test message", "cuda:0")
	require.NoError(t, err)
	assert.Empty(t, result, "empty prompt target should produce no widget updates")
}

func TestApplyParameters_HardcodedValues(t *testing.T) {
	irc := newState(nil, "sd", "test")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{},
		Parameters:   map[string]ParameterDef{},
		Hardcoded: map[string]HardcodedValue{
			"width": {
				Value:   1024,
				Targets: []Target{{Node: "loader", WidgetIndex: 1}},
			},
		},
	}

	result, err := applyParameters(irc, meta, "test", "cuda:0")
	require.NoError(t, err)
	assert.Equal(t, 1024, result["loader"][1])
}

func TestApplyParameters_GPUParameter(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{Node: "sampler", WidgetIndex: 0},
		Parameters: map[string]ParameterDef{
			"gpu_device": {
				Type: "string",
				Targets: []Target{
					{Node: "loader", WidgetIndex: 0},
				},
			},
		},
		Hardcoded: map[string]HardcodedValue{},
	}

	result, err := applyParameters(irc, meta, "test", "cuda:1")
	require.NoError(t, err)
	assert.Equal(t, "cuda:1", result["loader"][0])
}

func TestApplyParameters_ParameterError(t *testing.T) {
	irc := newState(nil, "sd", "test")
	minVal := 100.0
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{},
		Parameters: map[string]ParameterDef{
			"steps": {
				Type: "int",
				Min:  &minVal,
				Targets: []Target{
					{Node: "sampler", WidgetIndex: 1},
				},
			},
		},
		Hardcoded: map[string]HardcodedValue{},
	}
	// Provide a value below the minimum
	irc.Arguments = []state.Argument{{Key: "steps", Value: "5"}}

	_, err := applyParameters(irc, meta, "test", "cuda:0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "too low")
}

// =============================================================================
// preparePrompt tests
// =============================================================================

func TestPreparePrompt_Normal(t *testing.T) {
	irc := newState(nil, "sd", "a beautiful landscape")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "sd", config)
	assert.Equal(t, "a beautiful landscape", result)
}

func TestPreparePrompt_AIEnhanced(t *testing.T) {
	irc := newState(nil, "sd", "a cat")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "enhanced cat description", "sd", config)
	assert.Equal(t, "enhanced cat description", result)
}

func TestPreparePrompt_BadWords(t *testing.T) {
	irc := newState(nil, "sd", "greta thunberg")
	config := &settings.ComfyUiConfig{
		BadWords:       []string{"greta"},
		BadWordsPrompt: "FBI Agent making an arrest.",
	}
	result := preparePrompt(irc, "", "sd", config)
	assert.Equal(t, "FBI Agent making an arrest.", result)
}

func TestPreparePrompt_TTSNoClean(t *testing.T) {
	irc := newState(nil, "tts", "special chars <>&* here")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "tts", config)
	// TTS skips CleanPrompt — special chars should be preserved
	assert.Contains(t, result, "special chars")
}

func TestPreparePrompt_ErnieModel(t *testing.T) {
	irc := newState(nil, "ernie", "a sunset")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "ernie", config)
	assert.Contains(t, result, "a sunset")
	assert.Contains(t, result, "SYSTEM_PROMPT")
}

func TestPreparePrompt_EmptyMessage(t *testing.T) {
	irc := newState(nil, "sd", "")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "sd", config)
	assert.Equal(t, "", result)
}

// =============================================================================
// WorkflowExists tests
// =============================================================================

func TestWorkflowExists_Nonexistent(t *testing.T) {
	assert.False(t, WorkflowExists("nonexistent_workflow_that_does_not_exist"))
}

// =============================================================================
// Cache tests
// =============================================================================

func TestGetCachedMeta_NotFound(t *testing.T) {
	// Force cache initialization with empty directory
	origOk := workflowCacheOk
	origCache := workflowCache
	workflowCache = make(map[string]*AibirdMeta)
	workflowCacheOk = true
	defer func() {
		workflowCache = origCache
		workflowCacheOk = origOk
	}()

	result := GetCachedMeta("nonexistent")
	assert.Nil(t, result)
}

func TestGetCachedMeta_CaseInsensitive(t *testing.T) {
	origOk := workflowCacheOk
	origCache := workflowCache
	workflowCache = map[string]*AibirdMeta{
		"flux": {Name: "Flux"},
	}
	workflowCacheOk = true
	defer func() {
		workflowCache = origCache
		workflowCacheOk = origOk
	}()

	assert.NotNil(t, GetCachedMeta("flux"))
	assert.NotNil(t, GetCachedMeta("FLUX"))
	assert.NotNil(t, GetCachedMeta("Flux"))
}

func TestGetCachedWorkflows(t *testing.T) {
	origOk := workflowCacheOk
	origCache := workflowCache
	workflowCache = map[string]*AibirdMeta{
		"flux": {Name: "Flux"},
		"sd":   {Name: "SD"},
	}
	workflowCacheOk = true
	defer func() {
		workflowCache = origCache
		workflowCacheOk = origOk
	}()

	names := GetCachedWorkflows()
	assert.Len(t, names, 2)
	assert.Contains(t, names, "flux")
	assert.Contains(t, names, "sd")
}

// =============================================================================
// Type tests
// =============================================================================

func TestAibirdMeta_TOMLDecode(t *testing.T) {
	tomlStr := `
name = "test-workflow"
command = "tw"
description = "Test"
url = "http://example.com"
example = "!tw a cat"
accessLevel = 5
type = "image"
bigModel = true

[promptTarget]
node = "ksampler"
widget_index = 0

[parameters.seed]
type = "int"
default = -1
description = "Seed"
required = false

[[parameters.seed.targets]]
node = "ksampler"
widget_index = 3

[hardcoded.width]
value = 1024

[[hardcoded.width.targets]]
node = "emptylatentimage"
widget_index = 1
`
	var meta AibirdMeta
	_, err := toml.Decode(tomlStr, &meta)
	require.NoError(t, err)

	assert.Equal(t, "test-workflow", meta.Name)
	assert.Equal(t, 5, meta.AccessLevel)
	assert.True(t, meta.BigModel)
	assert.Equal(t, "ksampler", meta.PromptTarget.Node)
	assert.Equal(t, 0, meta.PromptTarget.WidgetIndex)

	seedParam, ok := meta.Parameters["seed"]
	assert.True(t, ok)
	assert.Equal(t, "int", seedParam.Type)
	assert.Len(t, seedParam.Targets, 1)
	assert.Equal(t, "ksampler", seedParam.Targets[0].Node)
	assert.Equal(t, 3, seedParam.Targets[0].WidgetIndex)

	widthHardcoded, ok := meta.Hardcoded["width"]
	assert.True(t, ok)
	assert.Equal(t, int64(1024), widthHardcoded.Value)
	assert.Len(t, widthHardcoded.Targets, 1)
}

func TestParameterDef_MinMax(t *testing.T) {
	minVal := 1.0
	maxVal := 100.0
	param := ParameterDef{
		Type:    "int",
		Min:     &minVal,
		Max:     &maxVal,
		Default: 50,
	}
	assert.Equal(t, "int", param.Type)
	assert.Equal(t, 1.0, *param.Min)
	assert.Equal(t, 100.0, *param.Max)
	assert.Equal(t, 50, param.Default)
}

// =============================================================================
// Build workflow JSON helper — verify it produces valid output
// =============================================================================

func TestBuildWorkflowJSON_ProducesValidJSON(t *testing.T) {
	data := buildWorkflowJSON(`name = "test"`)
	var parsed map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &parsed))
	nodes, ok := parsed["nodes"].([]interface{})
	require.True(t, ok)
	assert.Len(t, nodes, 1)
}

// =============================================================================
// Additional CleanPrompt edge cases
// =============================================================================

func TestCleanPrompt_OnlyBannedPhrase(t *testing.T) {
	assert.Equal(t, "", CleanPrompt("jailbait"))
	assert.Equal(t, "", CleanPrompt("barely legal"))
}

func TestCleanPrompt_MultipleFilteredTerms(t *testing.T) {
	result := CleanPrompt("a girl and a boy playing")
	assert.NotContains(t, result, "girl")
	assert.NotContains(t, result, "boy")
}

func TestCleanPrompt_NormalTextUnchanged(t *testing.T) {
	input := "a woman walking her dog in the park"
	assert.Equal(t, input, CleanPrompt(input))
}

func TestCleanPrompt_SpecialCharacters(t *testing.T) {
	result := CleanPrompt("hello@world#test$money%percent^caret&amp*star")
	assert.NotContains(t, result, "@")
	assert.NotContains(t, result, "#")
}

// =============================================================================
// Additional BadWordsCheck edge cases
// =============================================================================

func TestBadWordsCheck_NilConfig(t *testing.T) {
	// Nil config should not panic — empty BadWords slice
	result := BadWordsCheck("test", &settings.ComfyUiConfig{})
	assert.False(t, result)
}

func TestBadWordsCheck_MultipleBadWords(t *testing.T) {
	config := &settings.ComfyUiConfig{
		BadWords: []string{"bad", "evil", "wrong"},
	}
	assert.True(t, BadWordsCheck("something bad happened", config))
	assert.True(t, BadWordsCheck("evil intent", config))
	assert.False(t, BadWordsCheck("all good here", config))
}

// =============================================================================
// GetWorkFlowsSlice with temp dir
// =============================================================================

func TestGetWorkFlowsSlice_NoFiles(t *testing.T) {
	// This test is environment-dependent; it reads from comfyuijson/
	// Just verify it doesn't panic and returns a slice (possibly empty)
	result := GetWorkFlowsSlice()
	assert.NotNil(t, result) // should be []string, not nil (unless glob fails)
}

func TestGetWorkFlows_NoFiles(t *testing.T) {
	result := GetWorkFlows(false)
	// May be empty if no comfyuijson/ directory
	assert.NotNil(t, result)
}

// =============================================================================
// Meta node with no TOML string
// =============================================================================

func TestGetAibirdMeta_WidgetValueNotString(t *testing.T) {
	metaNode := map[string]interface{}{
		"title": "aibird_meta",
		"widgets_values": []interface{}{
			42, // not a string
		},
	}
	workflow := map[string]interface{}{
		"nodes": []interface{}{metaNode},
	}
	data, _ := json.Marshal(workflow)

	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "nonstring.json")
	require.NoError(t, os.WriteFile(tmpFile, data, 0644))

	_, err := GetAibirdMeta(tmpFile)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not a string")
}

// =============================================================================
// Ensure coverage for fmt import used in error paths
// =============================================================================

func TestWorkflowFile_WithSubdir(t *testing.T) {
	assert.Equal(t, "comfyuijson/sub/model.json", workflowFile("sub/model"))
}
