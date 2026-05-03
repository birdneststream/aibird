package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestAppendSlashUrl(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{"empty", "", "/"},
		{"no slash", "http://example.com", "http://example.com/"},
		{"has slash", "http://example.com/", "http://example.com/"},
		{"path no slash", "http://example.com/path", "http://example.com/path/"},
		{"path with slash", "http://example.com/path/", "http://example.com/path/"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, AppendSlashUrl(tt.url))
		})
	}
}

func TestMakeUrlWithPort(t *testing.T) {
	result := MakeUrlWithPort("http://example.com", "8080")
	assert.Equal(t, "http://example.com:8080/", result)
}

func TestStringToStatusIndicator(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "[N/A]"},
		{"true", "[YES]"},
		{"false", "[NO]"},
		{"other", "[?]"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, StringToStatusIndicator(tt.input))
		})
	}
}

func TestGetModes(t *testing.T) {
	assert.Equal(t, []string{"@", "+"}, GetModes("@+"))
	assert.Equal(t, []string{"~"}, GetModes("~"))
	assert.Equal(t, []string{"&", "%"}, GetModes("&%"))
	assert.Empty(t, GetModes("abc"))
	assert.Empty(t, GetModes(""))
}

func TestModeMap(t *testing.T) {
	assert.Equal(t, "@", ModeMap('o'))
	assert.Equal(t, "+", ModeMap('v'))
	assert.Equal(t, "%", ModeMap('h'))
	assert.Equal(t, "&", ModeMap('a'))
	assert.Equal(t, "~", ModeMap('q'))
	assert.Equal(t, "", ModeMap('x'))
}

func TestReverseModeMap(t *testing.T) {
	assert.Equal(t, "o", ReverseModeMap("@"))
	assert.Equal(t, "v", ReverseModeMap("+"))
	assert.Equal(t, "h", ReverseModeMap("%"))
	assert.Equal(t, "a", ReverseModeMap("&"))
	assert.Equal(t, "q", ReverseModeMap("~"))
	assert.Equal(t, "", ReverseModeMap("x"))
}

func TestCapitaliseFirst(t *testing.T) {
	assert.Equal(t, "Hello", CapitaliseFirst("hello"))
	assert.Equal(t, "Hello world", CapitaliseFirst("hello world"))
	assert.Equal(t, "", CapitaliseFirst(""))
	assert.Equal(t, "A", CapitaliseFirst("a"))
}

func TestMarkdownToIrc(t *testing.T) {
	// Bold
	result := MarkdownToIrc("**bold**")
	assert.Contains(t, result, "bold")

	// Italic
	result = MarkdownToIrc("*italic*")
	assert.Contains(t, result, "italic")

	// Empty
	result = MarkdownToIrc("")
	assert.Empty(t, result)
}

func TestUnixTimeToHumanReadable(t *testing.T) {
	// Zero should return "never"
	assert.Equal(t, "never", UnixTimeToHumanReadable(0))

	// Recent timestamp should produce a non-empty result
	result := UnixTimeToHumanReadable(1700000000)
	assert.NotEmpty(t, result)
	assert.NotEqual(t, "never", result)
}
