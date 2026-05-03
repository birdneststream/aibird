package commands

import (
	"strings"
	"testing"
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
