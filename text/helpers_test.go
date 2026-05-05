package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendFullStop(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    string
	}{
		{"no punctuation", "hello", "hello."},
		{"has period", "hello.", "hello."},
		{"has exclamation", "hello!", "hello!"},
		{"has question", "hello?", "hello?"},
		{"empty", "", "."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AppendFullStop(tt.message))
		})
	}
}

func TestGetPersonalityFile_InvalidInput(t *testing.T) {
	// Path traversal should be rejected
	_, err := GetPersonalityFile("../../etc/passwd")
	assert.Error(t, err)

	_, err = GetPersonalityFile("../secret")
	assert.Error(t, err)

	_, err = GetPersonalityFile(".")
	assert.Error(t, err)

	_, err = GetPersonalityFile("..")
	assert.Error(t, err)

	// Non-existent personality
	_, err = GetPersonalityFile("nonexistent_personality_xyz")
	assert.Error(t, err)
}

func TestGetPrompt_InvalidInput(t *testing.T) {
	// Path traversal should be rejected
	_, err := GetPrompt("../../etc/passwd")
	assert.Error(t, err)

	_, err = GetPrompt("../secret")
	assert.Error(t, err)

	// Non-existent prompt
	_, err = GetPrompt("nonexistent_prompt_xyz.txt")
	assert.Error(t, err)
}

func TestReadSanitizedFile_PathTraversal(t *testing.T) {
	tests := []struct {
		name string
		input string
	}{
		{"absolute unix path", "/etc/passwd"},
		{"parent traversal", "../../etc/passwd"},
		{"single parent", ".."},
		{"current dir", "."},
		{"backslash path", "..\\..\\windows\\system32"},
		{"embedded traversal", "foo/../../bar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := readSanitizedFile("text/prompts", tt.input)
			assert.Error(t, err, "expected error for input: %s", tt.input)
		})
	}
}

func TestReadSanitizedFile_NonExistent(t *testing.T) {
	_, err := readSanitizedFile("text/prompts", "does_not_exist.md")
	assert.Error(t, err)
}
