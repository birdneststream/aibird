package comfyui

type (
	Config struct {
		Enabled        bool
		Url            string
		Port           string
		Port1          string
		Port2          string
		BadWords       []string
		BadWordsPrompt string
		MaxQueueSize   int
		RewritePrompt  bool
	}
)

// AibirdMeta defines the structure for the aibird_meta TOML file.
type AibirdMeta struct {
	Name         string                    `toml:"name"`
	Command      string                    `toml:"command"`
	Description  string                    `toml:"description"`
	URL          string                    `toml:"url"`
	Example      string                    `toml:"example"`
	AccessLevel  int                       `toml:"accessLevel"`
	Type         string                    `toml:"type"`
	BigModel     bool                      `toml:"bigModel"`
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
