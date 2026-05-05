package commands

import (
	"strings"
	"testing"

	"aibird/birdbase"
	"aibird/irc/state"
	"aibird/irc/users"
)

func TestStripThinkingContent(t *testing.T) {
	tests := []struct {
		name            string
		input           string
		wantContains    []string // output must contain these
		wantNotContains []string // output must NOT contain these
	}{
		{
			name:            "removes think block",
			input:           "Hello <think let me think about this </think > world",
			wantContains:    []string{"Hello", "world"},
			wantNotContains: []string{"<think", "</think", "let me think"},
		},
		{
			name:            "removes multiline think block",
			input:           "Hello <think\nline1\nline2\n</think > world",
			wantContains:    []string{"Hello", "world"},
			wantNotContains: []string{"<think", "</think", "line1", "line2"},
		},
		{
			name:            "no think block unchanged",
			input:           "Just a regular message",
			wantContains:    []string{"Just a regular message"},
			wantNotContains: []string{"<think"},
		},
		{
			name:            "removes markdown bold and italic",
			input:           "Hello **bold** and *italic* text",
			wantContains:    []string{"Hello", "bold", "italic", "text"},
			wantNotContains: []string{"**", "*"},
		},
		{
			name:            "removes markdown code blocks",
			input:           "Here is code ```go\nfmt.Println(\"hi\")\n``` done",
			wantContains:    []string{"Here is code", "done"},
			wantNotContains: []string{"```", "fmt"},
		},
		{
			name:            "removes emoji characters",
			input:           "Hello 🧠 brain and 🎨 art here",
			wantContains:    []string{"Hello", "brain", "and", "art", "here"},
			wantNotContains: []string{"🧠", "🎨"},
		},
		{
			name:            "converts newlines to spaces",
			input:           "Line1\nLine2\nLine3",
			wantContains:    []string{"Line1", "Line2", "Line3"},
			wantNotContains: []string{"\n"},
		},
		{
			name:            "removes inline code",
			input:           "Use `code here` for that",
			wantContains:    []string{"Use", "code here", "for that"},
			wantNotContains: []string{"`"},
		},
		{
			name:            "removes markdown links",
			input:           "Check [this link](https://example.com) out",
			wantContains:    []string{"Check", "this link", "out"},
			wantNotContains: []string{"[", "](https"},
		},
		{
			name:            "removes headers",
			input:           "## Heading 2\nSome text",
			wantContains:    []string{"Heading 2", "Some text"},
			wantNotContains: []string{"##"},
		},
		{
			name:            "fixes escaped quotes",
			input:           `He said \"hello\" to me`,
			wantContains:    []string{`He said "hello" to me`},
			wantNotContains: []string{`\"`},
		},
		{
			name:            "empty string returns empty",
			input:           "",
			wantContains:    []string{},
			wantNotContains: []string{},
		},
		{
			name:            "whitespace only returns empty",
			input:           "   \n\t  ",
			wantContains:    []string{},
			wantNotContains: []string{},
		},
		{
			name:            "cleans multiple spaces",
			input:           "too   many    spaces",
			wantContains:    []string{"too many spaces"},
			wantNotContains: []string{"  "},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripThinkingContent(tt.input)

			for _, want := range tt.wantContains {
				if !strings.Contains(got, want) {
					t.Errorf("stripThinkingContent() = %q, want to contain %q", got, want)
				}
			}
			for _, notWant := range tt.wantNotContains {
				if strings.Contains(got, notWant) {
					t.Errorf("stripThinkingContent() = %q, want NOT to contain %q", got, notWant)
				}
			}
		})
	}
}

func TestStripEmojis(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"no emoji", "Hello world", "Hello world"},
		{"removes emoticons", "smile 😀", "smile "},
		{"removes transport", "car 🚗", "car "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripEmojis(tt.input)
			if got != tt.want {
				t.Errorf("stripEmojis() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestStripMarkdown(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"bold", "**bold text**", "bold text"},
		{"italic", "*italic text*", "italic text"},
		{"strikethrough", "~~deleted~~", "deleted"},
		{"horizontal rule", "before---after", "before---after"}, // only matches ^[-*]{3,}$
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripMarkdown(tt.input)
			if got != tt.want {
				t.Errorf("stripMarkdown() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSanitizePersona(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty", "", ""},
		{"simple text", "hello", "hello"},
		{"strips newlines", "hello\nworld", "hello world"},
		{"strips carriage returns", "hello\r\nworld", "hello  world"},
		{"trims whitespace", "  hello  ", "hello"},
		{"truncates at 200", strings.Repeat("a", 250), strings.Repeat("a", 200)},
		{"exactly 200 chars", strings.Repeat("a", 200), strings.Repeat("a", 200)},
		{"under 200 unchanged", "short text", "short text"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizePersona(tt.input)
			if got != tt.want {
				t.Errorf("sanitizePersona() = %q (len=%d), want %q (len=%d)", got, len(got), tt.want, len(tt.want))
			}
		})
	}
}

func TestFormatEventLog(t *testing.T) {
	messages := []birdbase.ChannelMessage{
		{Nickname: "alice", EventType: "privmsg", Message: "hello", Timestamp: 1000},
		{Nickname: "bob", EventType: "action", Message: "waves", Timestamp: 2000},
		{Nickname: "carol", EventType: "join", Message: "", Timestamp: 3000},
		{Nickname: "dave", EventType: "part", Message: "", Timestamp: 4000},
		{Nickname: "eve", EventType: "quit", Message: "", Timestamp: 5000},
		{Nickname: "frank", EventType: "kick", Message: "alice", Timestamp: 6000},
	}

	result := formatEventLog("#test", 6, messages)

	if !strings.Contains(result, "Channel: #test") {
		t.Error("should contain channel name")
	}
	if !strings.Contains(result, "last 6 hours") {
		t.Error("should contain hours")
	}
	if !strings.Contains(result, "<alice> hello") {
		t.Error("should format privmsg")
	}
	if !strings.Contains(result, "* bob waves") {
		t.Error("should format action")
	}
	if !strings.Contains(result, "--> carol joined") {
		t.Error("should format join")
	}
	if !strings.Contains(result, "<-- dave left") {
		t.Error("should format part")
	}
	if !strings.Contains(result, "<-- eve quit") {
		t.Error("should format quit")
	}
	if !strings.Contains(result, "<-- frank was kicked by alice") {
		t.Error("should format kick")
	}

	// Empty messages
	emptyResult := formatEventLog("#empty", 1, nil)
	if !strings.Contains(emptyResult, "Channel: #empty") {
		t.Error("should contain header even with no messages")
	}
}

func TestParseHoursFromMessage(t *testing.T) {
	tests := []struct {
		name         string
		msg          string
		defaultHours int
		maxHours     int
		want         int
	}{
		{"empty string returns default", "", 24, 168, 24},
		{"6h", "6h", 24, 168, 6},
		{"plain number", "12", 24, 168, 12},
		{"clamped to max", "200h", 24, 168, 168},
		{"with trailing text", "6h --persona cowboy", 24, 168, 6},
		{"zero returns default", "0", 24, 168, 24},
		{"negative returns default", "-5", 24, 168, 24},
		{"non-numeric returns default", "hello", 24, 168, 24},
		{"exact max", "168h", 24, 168, 168},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseHoursFromMessage(tt.msg, tt.defaultHours, tt.maxHours)
			if got != tt.want {
				t.Errorf("parseHoursFromMessage(%q, %d, %d) = %d, want %d", tt.msg, tt.defaultHours, tt.maxHours, got, tt.want)
			}
		})
	}
}

func TestParseIntSafe(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    int
		wantErr bool
	}{
		{"simple number", "123", 123, false},
		{"zero", "0", 0, true},
		{"non-numeric", "abc", 0, true},
		{"stops at non-digit", "12abc", 12, false},
		{"empty string", "", 0, true},
		{"with whitespace", " 42 ", 42, false},
		{"single digit", "7", 7, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseIntSafe(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseIntSafe(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseIntSafe(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestIsAiSettingsCommand(t *testing.T) {
	tests := []struct {
		name      string
		arguments []state.Argument
		want      bool
	}{
		{
			name:      "no flags returns false",
			arguments: nil,
			want:      false,
		},
		{
			name:      "info flag returns true",
			arguments: []state.Argument{{Key: "info", Value: true}},
			want:      true,
		},
		{
			name:      "setPersonality returns true",
			arguments: []state.Argument{{Key: "setPersonality", Value: "friendly"}},
			want:      true,
		},
		{
			name:      "empty setPersonality returns false",
			arguments: []state.Argument{{Key: "setPersonality", Value: ""}},
			want:      false,
		},
		{
			name:      "clearPersonality returns true",
			arguments: []state.Argument{{Key: "clearPersonality", Value: true}},
			want:      true,
		},
		{
			name:      "setBasePrompt returns true",
			arguments: []state.Argument{{Key: "setBasePrompt", Value: "You are helpful"}},
			want:      true,
		},
		{
			name:      "clearBasePrompt returns true",
			arguments: []state.Argument{{Key: "clearBasePrompt", Value: true}},
			want:      true,
		},
		{
			name:      "setAiModel returns true",
			arguments: []state.Argument{{Key: "setAiModel", Value: "gpt-4"}},
			want:      true,
		},
		{
			name:      "clearAiModel returns true",
			arguments: []state.Argument{{Key: "clearAiModel", Value: true}},
			want:      true,
		},
		{
			name:      "setAiService returns true",
			arguments: []state.Argument{{Key: "setAiService", Value: "glm"}},
			want:      true,
		},
		{
			name:      "clearAiService returns true",
			arguments: []state.Argument{{Key: "clearAiService", Value: true}},
			want:      true,
		},
		{
			name: "unrelated flag returns false",
			arguments: []state.Argument{
				{Key: "verbose", Value: true},
				{Key: "model", Value: "test"},
			},
			want: false,
		},
		{
			name: "mixed settings and unrelated flags returns true",
			arguments: []state.Argument{
				{Key: "verbose", Value: true},
				{Key: "info", Value: true},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			irc := &state.State{
				Arguments: tt.arguments,
			}
			got := isAiSettingsCommand(irc)
			if got != tt.want {
				t.Errorf("isAiSettingsCommand() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldQueueLlamaCppAi(t *testing.T) {
	tests := []struct {
		name      string
		action    string
		message   string
		arguments []state.Argument
		aiService string
		want      bool
	}{
		{
			name:      "non-ai action returns false",
			action:    "sd",
			message:   "a cat",
			aiService: "llamacpp",
			want:      false,
		},
		{
			name:      "info flag returns false",
			action:    "ai",
			message:   "hello",
			arguments: []state.Argument{{Key: "info", Value: true}},
			aiService: "llamacpp",
			want:      false,
		},
		{
			name:      "empty message returns false",
			action:    "ai",
			message:   "",
			aiService: "llamacpp",
			want:      false,
		},
		{
			name:      "glm service returns false",
			action:    "ai",
			message:   "hello",
			aiService: "glm",
			want:      false,
		},
		{
			name:      "llamacpp with message returns true",
			action:    "ai",
			message:   "tell me a joke",
			aiService: "llamacpp",
			want:      true,
		},
		{
			name:      "default service with message returns true",
			action:    "ai",
			message:   "hello",
			aiService: "",
			want:      true,
		},
		{
			name:      "settings command returns false",
			action:    "ai",
			message:   "hello",
			arguments: []state.Argument{{Key: "setPersonality", Value: "funny"}},
			aiService: "llamacpp",
			want:      false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			irc := &state.State{
				Command: state.Command{
					Action:  tt.action,
					Message: tt.message,
				},
				Arguments: tt.arguments,
			}
			if irc.User == nil {
				irc.User = &users.User{}
			}
			// Set AiService through the available setter if needed
			if tt.aiService != "" {
				irc.User.SetAiService(tt.aiService)
			}

			got := ShouldQueueLlamaCppAi(irc)
			if got != tt.want {
				t.Errorf("ShouldQueueLlamaCppAi() = %v, want %v", got, tt.want)
			}
		})
	}
}
