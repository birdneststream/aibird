package helpers

import (
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/hako/durafmt"
	"github.com/lrstanley/girc"
)

var (
	modeMapData = map[int32]string{
		'o': "@",
		'v': "+",
		'h': "%",
		'a': "&",
		'q': "~",
	}
	reverseModeMapData = make(map[string]string)
)

var markdownReplacements = [][2]string{
	// Must be processed first
	{"(?m)^(#+)\\s+(.*)$", "{b}$2{b}"}, // Headers: # text
	{"(?m)^>\\s+(.*)$", "{green}> $1"}, // Blockquotes: > text

	// Paired formatting
	{"\\x60(.*?)\\x60", "{cyan}$1{c}"}, // Inline code: `code`
	{"\\*\\*(.*?)\\*\\*", "{b}$1{b}"},  // Bold: **text**
	{"__(.*?)__", "{b}$1{b}"},          // Bold: __text__
	{"\\*(.*?)\\*", "{i}$1{i}"},        // Italics: *text*
	{"_(.*?)_", "{i}$1{i}"},            // Italics: _text_

	// Lists - must be processed after paired formatting
	{"(?m)^\\s*[\\*\\-]\\s+(.*)$", "- $1"}, // List items: * item or - item
}

var compiledMarkdownReplacements []*regexp.Regexp

func init() {
	for k, v := range modeMapData {
		reverseModeMapData[v] = string(k)
	}

	for _, replacement := range markdownReplacements {
		compiledMarkdownReplacements = append(compiledMarkdownReplacements, regexp.MustCompile(replacement[0]))
	}
}

func GetIp() (string, error) {
	resp, err := http.Get("https://ifconfig.io")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

func AppendSlashUrl(url string) string {
	if url == "" {
		return "/"
	}
	if len(url) > 0 && url[len(url)-1:] != "/" {
		return url + "/"
	}
	return url
}

func MakeUrlWithPort(url string, port string) string {
	return AppendSlashUrl(url + ":" + port)
}

func WrapText(input string, limit int) string {
	var result strings.Builder
	lines := strings.Split(input, "\n")

	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			result.WriteString(line)
		} else {
			words := strings.Fields(line)
			var currentLine strings.Builder
			for _, word := range words {
				// If the current line is empty, just add the word.
				if currentLine.Len() == 0 {
					currentLine.WriteString(word)
				} else if currentLine.Len()+len(word)+1 <= limit {
					// If the word fits, add a space and the word.
					currentLine.WriteString(" ")
					currentLine.WriteString(word)
				} else {
					// If the word does not fit, finalize the current line and start a new one.
					result.WriteString(currentLine.String())
					result.WriteString("\n")
					currentLine.Reset()
					currentLine.WriteString(word)
				}
			}
			result.WriteString(currentLine.String())
		}

		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return result.String()
}

func UnixTimeToHumanReadable(timestamp int64) string {
	if timestamp == 0 {
		return "never"
	}

	return durafmt.Parse(time.Second * time.Duration(time.Now().Unix()-timestamp)).String()
}

// StringToStatusIndicator converts a string to a status indicator string.
func StringToStatusIndicator(s string) string {
	if s == "" {
		return "[N/A]" // ASCII for empty or not available
	}
	// Assuming you want to keep the boolean to emoji functionality as well
	if s == "true" {
		return "[YES]"
	} else if s == "false" {
		return "[NO]"
	}
	// Return a default emoji if the string is not empty, true, or false
	return "[?]"
}

func GetModes(modes string) []string {
	var foundModes []string
	for _, char := range modes {
		switch char {
		case '@', '+', '~', '&', '%':
			foundModes = append(foundModes, string(char))
		}
	}
	return foundModes
}

func ModeHas(modes []string, checkMode string) bool {
	for _, mode := range modes {
		if mode == checkMode {
			return true
		}
	}
	return false
}

func ModeMap(mode int32) string {
	if val, ok := modeMapData[mode]; ok {
		return val
	}
	return ""
}

func ReverseModeMap(mode string) string {
	if val, ok := reverseModeMapData[mode]; ok {
		return val
	}
	return ""
}

func FindChannelNameInEventParams(event girc.Event) string {
	for _, param := range event.Params {
		if strings.HasPrefix(param, "#") {
			return param
		}
	}
	return ""
}

// MarkdownToIrc converts markdown to irc formatting
func MarkdownToIrc(message string) string {
	inCodeBlock := false
	var result strings.Builder
	lines := strings.Split(message, "\n")

	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			inCodeBlock = !inCodeBlock
			if inCodeBlock {
				result.WriteString("{cyan}[code]{clear}")
			} else {
				result.WriteString("{cyan}[/code]{clear}")
			}
		} else if inCodeBlock {
			result.WriteString("{green}")
			result.WriteString(line)
		} else {
			processed := line
			for i, re := range compiledMarkdownReplacements {
				processed = re.ReplaceAllString(processed, markdownReplacements[i][1])
			}
			result.WriteString(processed)
		}

		if i < len(lines)-1 {
			result.WriteString("\n")
		}
	}

	return girc.Fmt(result.String())
}

func CapitaliseFirst(s string) string {
	if s == "" {
		return ""
	}
	r := []rune(s)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}
