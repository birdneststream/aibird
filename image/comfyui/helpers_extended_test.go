package comfyui

import (
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

// writeMetaSidecar writes a .meta.toml file next to a workflow path and returns
// the workflow JSON path (which GetAibirdMeta reads to derive the sidecar).
func writeMetaSidecar(t *testing.T, baseName, metaTOML string) string {
	t.Helper()
	dir := t.TempDir()
	workflowPath := filepath.Join(dir, baseName+".json")
	metaPath := filepath.Join(dir, baseName+".meta.toml")
	require.NoError(t, os.WriteFile(metaPath, []byte(metaTOML), 0644))
	return workflowPath
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
input = "text"
`
	workflowPath := writeMetaSidecar(t, "test", metaTOML)

	meta, err := GetAibirdMeta(workflowPath)
	require.NoError(t, err)
	assert.Equal(t, "test-model", meta.Name)
	assert.Equal(t, "test", meta.Command)
	assert.Equal(t, "A test workflow", meta.Description)
	assert.Equal(t, "image", meta.Type)
	assert.Equal(t, 0, meta.AccessLevel)
	assert.Equal(t, "sampler", meta.PromptTarget.Node)
	assert.Equal(t, "text", meta.PromptTarget.Input)
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
input = "text"

[parameters.seed]
type = "int"
default = -1
description = "Random seed"

[[parameters.seed.targets]]
node = "sampler"
input = "seed"
`
	workflowPath := writeMetaSidecar(t, "params", metaTOML)

	meta, err := GetAibirdMeta(workflowPath)
	require.NoError(t, err)
	assert.NotNil(t, meta.Parameters["seed"])
	assert.Equal(t, "int", meta.Parameters["seed"].Type)
	assert.Len(t, meta.Parameters["seed"].Targets, 1)
	assert.Equal(t, "sampler", meta.Parameters["seed"].Targets[0].Node)
	assert.Equal(t, "seed", meta.Parameters["seed"].Targets[0].Input)
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

func TestGetAibirdMeta_InvalidTOML(t *testing.T) {
	workflowPath := writeMetaSidecar(t, "badtoml", "this is not valid [[[[TOML")
	_, err := GetAibirdMeta(workflowPath)
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
		PromptTarget: PromptTarget{Node: "sampler", Input: "text"},
		Parameters:   map[string]ParameterDef{},
		Hardcoded:    map[string]HardcodedValue{},
	}

	result, err := applyParameters(irc, meta, "a woman walking", "cuda:0")
	require.NoError(t, err)
	assert.Equal(t, "a woman walking", result["sampler"]["text"])
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
	assert.Empty(t, result, "empty prompt target should produce no input updates")
}

func TestApplyParameters_HardcodedValues(t *testing.T) {
	irc := newState(nil, "sd", "test")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{},
		Parameters:   map[string]ParameterDef{},
		Hardcoded: map[string]HardcodedValue{
			"width": {
				Value:   1024,
				Targets: []Target{{Node: "emptylatentimage", Input: "width"}},
			},
		},
	}

	result, err := applyParameters(irc, meta, "test", "cuda:0")
	require.NoError(t, err)
	assert.Equal(t, 1024, result["emptylatentimage"]["width"])
}

func TestApplyParameters_GPUParameter(t *testing.T) {
	irc := newState([]state.Argument{}, "sd", "test")
	meta := &AibirdMeta{
		PromptTarget: PromptTarget{Node: "sampler", Input: "text"},
		Parameters: map[string]ParameterDef{
			"gpu_device": {
				Type: "string",
				Targets: []Target{
					{Node: "loader", Input: "device"},
				},
			},
		},
		Hardcoded: map[string]HardcodedValue{},
	}

	result, err := applyParameters(irc, meta, "test", "cuda:1")
	require.NoError(t, err)
	assert.Equal(t, "cuda:1", result["loader"]["device"])
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
					{Node: "sampler", Input: "steps"},
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
	result := preparePrompt(irc, "", "sd", config, false)
	assert.Equal(t, "a beautiful landscape", result)
}

func TestPreparePrompt_AIEnhanced(t *testing.T) {
	irc := newState(nil, "sd", "a cat")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "enhanced cat description", "sd", config, false)
	assert.Equal(t, "enhanced cat description", result)
}

func TestPreparePrompt_BadWords(t *testing.T) {
	irc := newState(nil, "sd", "greta thunberg")
	config := &settings.ComfyUiConfig{
		BadWords:       []string{"greta"},
		BadWordsPrompt: "FBI Agent making an arrest.",
	}
	result := preparePrompt(irc, "", "sd", config, false)
	assert.Equal(t, "FBI Agent making an arrest.", result)
}

func TestPreparePrompt_TTSNoClean(t *testing.T) {
	irc := newState(nil, "tts", "special chars <>&* here")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "tts", config, false)
	// TTS skips CleanPrompt — special chars should be preserved
	assert.Contains(t, result, "special chars")
}

func TestPreparePrompt_ErnieModel(t *testing.T) {
	irc := newState(nil, "ernie", "a sunset")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "ernie", config, false)
	assert.Contains(t, result, "a sunset")
	assert.Contains(t, result, "SYSTEM_PROMPT")
}

func TestPreparePrompt_EmptyMessage(t *testing.T) {
	irc := newState(nil, "sd", "")
	config := &settings.ComfyUiConfig{}
	result := preparePrompt(irc, "", "sd", config, false)
	assert.Equal(t, "", result)
}

// =============================================================================
// preparePrompt rawPrompt tests (raw_prompt meta flag for trusted LLM output)
// =============================================================================

func TestPreparePrompt_RawPrompt_PreservesStructure(t *testing.T) {
	// rawPrompt=true must NOT run CleanPrompt on the enhanced prompt, so newlines,
	// brackets, and timecodes survive verbatim.
	irc := newState(nil, "sd", "a cat")
	config := &settings.ComfyUiConfig{}
	enhanced := "VISUAL STYLE\n[0s-5s] Shot 1: low angle\nAUDIO: cello note"
	result := preparePrompt(irc, enhanced, "m3", config, true)
	assert.Equal(t, enhanced, result)
}

func TestPreparePrompt_RawPrompt_TrimmedOnly(t *testing.T) {
	// Surrounding whitespace is trimmed but inner structure preserved.
	irc := newState(nil, "sd", "a cat")
	config := &settings.ComfyUiConfig{}
	enhanced := "\n\n  VISUAL STYLE\n[0s-5s] Shot 1  \n"
	result := preparePrompt(irc, enhanced, "m3", config, true)
	assert.Equal(t, "VISUAL STYLE\n[0s-5s] Shot 1", result)
}

func TestPreparePrompt_RawPrompt_BadWordsStillApply(t *testing.T) {
	// The safety filter still runs even with rawPrompt=true.
	irc := newState(nil, "sd", "greta thunberg")
	config := &settings.ComfyUiConfig{
		BadWords:       []string{"greta"},
		BadWordsPrompt: "FBI Agent making an arrest.",
	}
	result := preparePrompt(irc, "enhanced greta content", "m3", config, true)
	assert.Equal(t, "FBI Agent making an arrest.", result)
}

func TestPreparePrompt_RawPromptFalse_StillCleans(t *testing.T) {
	// rawPrompt=false (default) must still CleanPrompt the enhanced prompt,
	// stripping characters outside the allowed set (e.g. en-dash, newlines).
	irc := newState(nil, "sd", "a cat")
	config := &settings.ComfyUiConfig{}
	enhanced := "VISUAL STYLE\n[0s-5s] Shot 1"
	result := preparePrompt(irc, enhanced, "m3", config, false)
	// Newlines collapse to spaces and the en-dash is removed by CleanPrompt.
	assert.NotContains(t, result, "\n")
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
input = "positive"

[parameters.seed]
type = "int"
default = -1
description = "Seed"
required = false

[[parameters.seed.targets]]
node = "ksampler"
input = "seed"

[hardcoded.width]
value = 1024

[[hardcoded.width.targets]]
node = "emptylatentimage"
input = "width"
`
	var meta AibirdMeta
	_, err := toml.Decode(tomlStr, &meta)
	require.NoError(t, err)

	assert.Equal(t, "test-workflow", meta.Name)
	assert.Equal(t, 5, meta.AccessLevel)
	assert.True(t, meta.BigModel)
	assert.Equal(t, "ksampler", meta.PromptTarget.Node)
	assert.Equal(t, "positive", meta.PromptTarget.Input)

	seedParam, ok := meta.Parameters["seed"]
	assert.True(t, ok)
	assert.Equal(t, "int", seedParam.Type)
	assert.Len(t, seedParam.Targets, 1)
	assert.Equal(t, "ksampler", seedParam.Targets[0].Node)
	assert.Equal(t, "seed", seedParam.Targets[0].Input)

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
// Ensure coverage for fmt import used in error paths
// =============================================================================

func TestWorkflowFile_WithSubdir(t *testing.T) {
	assert.Equal(t, "comfyuijson/sub/model.json", workflowFile("sub/model"))
}
