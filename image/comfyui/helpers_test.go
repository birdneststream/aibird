package comfyui

import (
	"testing"
)

func TestCleanPrompt(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		// Basic filtering
		{
			name:     "normal prompt",
			input:    "a woman walking in the park",
			expected: "a woman walking in the park",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "only spaces",
			input:    "   ",
			expected: "",
		},

		// Exact blocklist matches (should be replaced by regex)
		{
			name:     "exact girl",
			input:    "a girl walking",
			expected: "a woman walking",
		},
		{
			name:     "exact boy",
			input:    "a boy playing",
			expected: "a man playing",
		},
		{
			name:     "exact toddler",
			input:    "a toddler laughing",
			expected: "a adult laughing",
		},
		{
			name:     "exact kid",
			input:    "a kid running",
			expected: "a adult running",
		},

		// Fuzzy matching - simple misspellings
		{
			name:     "toodler misspelling",
			input:    "a toodler playing",
			expected: "a adult playing",
		},
		{
			name:     "gril misspelling",
			input:    "a gril walking",
			expected: "a woman walking",
		},
		{
			name:     "boyy misspelling",
			input:    "a boyy running",
			expected: "a man running",
		},

		// Fuzzy matching - repeated characters
		{
			name:     "tooddleeer repeated chars",
			input:    "tooddleeer playing",
			expected: "adult playing",
		},
		{
			name:     "giiiirl repeated chars",
			input:    "giiiirl walking",
			expected: "woman walking",
		},
		{
			name:     "bbboooy repeated chars",
			input:    "bbboooy running",
			expected: "man running",
		},
		{
			name:     "kiiid repeated chars",
			input:    "kiiid laughing",
			expected: "adult laughing",
		},

		// Should NOT match - common words
		{
			name:     "the word should not match teen",
			input:    "while the holds",
			expected: "while the holds",
		},
		{
			name:     "who should not match",
			input:    "who is there",
			expected: "who is there",
		},
		{
			name:     "and should not match",
			input:    "and then",
			expected: "and then",
		},

		// Exceptions - should not be filtered
		{
			name:     "girlfriend exception",
			input:    "my girlfriend",
			expected: "my girlfriend",
		},
		{
			name:     "boyfriend exception",
			input:    "my boyfriend",
			expected: "my boyfriend",
		},
		{
			name:     "boy band exception",
			input:    "a boy band performing",
			expected: "a boy band performing",
		},
		{
			name:     "power girl exception",
			input:    "power girl costume",
			expected: "power girl costume",
		},
		{
			name:     "teenage mutant ninja turtles exception",
			input:    "teenage mutant ninja turtles",
			expected: "teenage mutant ninja turtles",
		},

		// Banned phrases - should return empty
		{
			name:     "jailbait banned",
			input:    "jailbait content",
			expected: "",
		},
		{
			name:     "barely legal banned",
			input:    "barely legal",
			expected: "",
		},
		{
			name:     "child model banned",
			input:    "child model posing",
			expected: "",
		},

		// Age filtering
		{
			name:     "age 5 filtered",
			input:    "5 years old",
			expected: "25 years old",
		},
		{
			name:     "age 15 filtered",
			input:    "15 year old person",
			expected: "25 years old person",
		},
		{
			name:     "under 18 filtered",
			input:    "under 18",
			expected: "over 25",
		},

		// Multiple terms
		{
			name:     "multiple terms",
			input:    "a gril and boyy playing",
			expected: "a woman and man playing",
		},

		// Size descriptor bypasses (full phrase replaced)
		{
			name:     "mini human bypass",
			input:    "mini human playing",
			expected: "adult playing",
		},
		{
			name:     "mini-human with hyphen",
			input:    "mini-human playing",
			expected: "minihuman playing", // hyphen removed by bracket filter, becomes one word, not caught by regex
		},
		{
			name:     "minihuman as one word",
			input:    "minihuman playing",
			expected: "minihuman playing", // not caught - would need specific pattern
		},
		{
			name:     "small human bypass",
			input:    "small human",
			expected: "adult",
		},
		{
			name:     "tiny human bypass",
			input:    "tiny human",
			expected: "adult",
		},
		{
			name:     "little person bypass",
			input:    "little person",
			expected: "adult",
		},

		// Legitimate uses of human/person should pass
		{
			name:     "human rights is okay",
			input:    "human rights advocacy",
			expected: "human rights advocacy",
		},
		{
			name:     "person of interest is okay",
			input:    "person of interest",
			expected: "person of interest",
		},

		// Edge cases
		{
			name:     "short words ignored",
			input:    "a to be or not",
			expected: "a to be or not",
		},
		{
			name:     "mixed case",
			input:    "A GIRL Walking",
			expected: "A woman Walking",
		},
		{
			name:     "boop should not match boy",
			input:    "boop beep",
			expected: "boop beep",
		},

		// Test all common words from whitelist
		{
			name:     "then is whitelisted",
			input:    "and then",
			expected: "and then",
		},
		{
			name:     "than is whitelisted",
			input:    "better than",
			expected: "better than",
		},
		{
			name:     "them is whitelisted",
			input:    "with them",
			expected: "with them",
		},
		{
			name:     "they is whitelisted",
			input:    "they walk",
			expected: "they walk",
		},
		{
			name:     "when is whitelisted",
			input:    "when ready",
			expected: "when ready",
		},
		{
			name:     "been is whitelisted",
			input:    "has been",
			expected: "has been",
		},
		{
			name:     "seen is whitelisted",
			input:    "have seen",
			expected: "have seen",
		},
		{
			name:     "keen is whitelisted",
			input:    "keen observer",
			expected: "keen observer",
		},
		{
			name:     "open is whitelisted",
			input:    "open door",
			expected: "open door",
		},
		{
			name:     "oven is whitelisted",
			input:    "oven baked",
			expected: "oven baked",
		},
		{
			name:     "beep is whitelisted",
			input:    "beep boop",
			expected: "beep boop",
		},
		{
			name:     "poop is whitelisted",
			input:    "poop emoji",
			expected: "poop emoji",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CleanPrompt(tt.input)
			if result != tt.expected {
				t.Errorf("CleanPrompt(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestNormalizeRepeatedChars(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "repeated double letters",
			input:    "tooddleeer",
			expected: "todler",
		},
		{
			name:     "repeated triple letters",
			input:    "giiiirl",
			expected: "girl",
		},
		{
			name:     "no repeated letters",
			input:    "normal",
			expected: "normal",
		},
		{
			name:     "single repeated letter",
			input:    "hello",
			expected: "helo",
		},
		{
			name:     "very short word",
			input:    "aa",
			expected: "aa",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeRepeatedChars(tt.input)
			if result != tt.expected {
				t.Errorf("normalizeRepeatedChars(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCorrectFuzzyMatches(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple typo",
			input:    "toodler playing",
			expected: "toddler playing",
		},
		{
			name:     "repeated chars",
			input:    "tooddleeer playing",
			expected: "toddler playing",
		},
		{
			name:     "gril typo",
			input:    "a gril walking",
			expected: "a girl walking",
		},
		{
			name:     "no fuzzy match needed",
			input:    "normal words",
			expected: "normal words",
		},
		{
			name:     "short words ignored",
			input:    "the cat",
			expected: "the cat",
		},
		{
			name:     "length difference too large",
			input:    "xyz walking",
			expected: "xyz walking",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := correctFuzzyMatches(tt.input, 2)
			if result != tt.expected {
				t.Errorf("correctFuzzyMatches(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}