package comfyui

// AibirdMeta defines the structure for the aibird_meta TOML file.
type AibirdMeta struct {
	Name        string `toml:"name"`
	Command     string `toml:"command"`
	Description string `toml:"description"`
	URL         string `toml:"url"`
	Example     string `toml:"example"`
	AccessLevel int    `toml:"accessLevel"`
	Type        string `toml:"type"`
	BigModel    bool   `toml:"bigModel"`
	// RawPrompt, when true, skips the aggressive CleanPrompt sanitization for the
	// enhanced/positive prompt and only trims surrounding whitespace. Use ONLY for
	// workflows whose prompt is produced by a trusted source (e.g. an LLM template
	// like the MiniMax H3 prompt engineer) and requires structured formatting
	// (newlines, brackets, timecodes, section headers). The BadWordsCheck safety
	// filter still runs regardless of this flag. Defaults to false (unchanged
	// behavior). Do NOT enable for workflows that accept raw, untrusted user input
	// as the prompt.
	RawPrompt    bool                      `toml:"raw_prompt"`
	PromptTarget PromptTarget              `toml:"promptTarget"`
	Parameters   map[string]ParameterDef   `toml:"parameters"`
	Hardcoded    map[string]HardcodedValue `toml:"hardcoded"`
}

// PromptTarget defines where the main prompt text should go.
type PromptTarget struct {
	Node        string `toml:"node"`
	WidgetIndex int    `toml:"widget_index"`
}

// ParameterDef defines the structure for a user-configurable parameter.
type ParameterDef struct {
	Type        string      `toml:"type"`
	Default     interface{} `toml:"default"`
	Description string      `toml:"description"`
	Targets     []Target    `toml:"targets"`
	Min         *float64    `toml:"min"`
	Max         *float64    `toml:"max"`
	Required    bool        `toml:"required"`
}

// HardcodedValue defines a value to be set directly in the workflow.
type HardcodedValue struct {
	Value   interface{} `toml:"value"`
	Targets []Target    `toml:"targets"`
}

// Target defines a specific widget in a ComfyUI workflow to update.
type Target struct {
	Node        string `toml:"node"`
	WidgetIndex int    `toml:"widget_index"`
}
