package comfyui

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"aibird/logger"
	"aibird/settings"

	"github.com/BurntSushi/toml"
	"github.com/agnivade/levenshtein"
)

func WorkflowExists(workflow string) bool {
	_, err := os.Stat("comfyuijson/" + workflow + ".json")
	return !os.IsNotExist(err)
}

func GetWorkFlows(format bool) string {
	// return list of files in ComfyUi/*.json
	flows := ""
	files, err := filepath.Glob("comfyuijson/*.json")
	if err != nil {
		logger.Error("Failed to glob for workflow files", "error", err)
		return ""
	}
	for _, file := range files {
		file = strings.ReplaceAll(file, ".json", "")
		file = strings.ReplaceAll(file, "comfyuijson/", "")

		if format {
			flows = flows + "{b}" + file + "{b}, "
		} else {
			flows = flows + file + ", "
		}

	}

	return strings.TrimRight(flows, ", ")
}

func GetWorkFlowsSlice() []string {
	files, err := filepath.Glob("comfyuijson/*.json")
	if err != nil {
		logger.Error("Failed to glob for workflow files", "error", err)
		return nil
	}

	workflows := make([]string, 0, len(files))
	for _, file := range files {
		file = strings.ReplaceAll(file, ".json", "")
		file = strings.ReplaceAll(file, "comfyuijson/", "")
		workflows = append(workflows, file)
	}

	return workflows
}

// blocklist contains words to check with fuzzy matching (extracted from replacement patterns)
var blocklist = []string{
	// Female youth terms
	"girl", "girls", "grl", "grll", "grls", "grlls", "girly", "girlish",
	"maiden", "maidens", "schoolgirl", "loli", "lolli", "lolita",

	// Male youth terms
	"boy", "boys", "boi", "boii", "boyz", "boyish", "schoolboy",

	// Generic youth terms
	"child", "children", "kid", "kids", "kiddo", "kiddies", "kiddie",
	"youngster", "youngsters", "teen", "teens", "teenager", "teenagers",
	"teenage", "adolescent", "adolescents", "youth", "youths",
	"juvenile", "juveniles", "minor", "minors", "underage",
	"baby", "bby", "babies", "infant", "infants", "toddler", "toddlers",
	"preschooler", "preschoolers", "tween", "tweens", "preteen", "preteens",

	// School/education terms
	"elementary", "preschool", "kindergarten", "daycare", "playground",
	"playroom", "classroom", "schoolroom",

	// Scout terms
	"cubscout", "webelos", "brownie",

	// Angel-related terms
	"cherub", "cherubs", "cherubic",
}

// commonWords that should never be fuzzy matched
// Based on Google 10000 most common English words (no swears)
// Focusing on 4-letter words that could false-positive match blocklist terms
var commonWords = map[string]bool{
	// Common verbs/pronouns/conjunctions
	"then": true, "than": true, "them": true, "they": true, "when": true,
	"been": true, "seen": true, "keen": true, "open": true, "oven": true,
	"here": true, "were": true, "have": true, "more": true,
	"make": true, "made": true, "come": true, "came": true, "some": true,
	"time": true, "very": true, "just": true, "into": true, "even": true,
	"also": true, "must": true, "does": true, "each": true, "said": true,
	"over": true, "such": true, "well": true, "back": true, "only": true,
	"good": true, "know": true, "take": true, "used": true, "work": true,

	// Common nouns
	"home": true, "page": true, "news": true, "help": true, "site": true,
	"user": true, "data": true, "post": true, "city": true, "book": true,
	"read": true, "item": true, "real": true, "life": true, "week": true,
	"name": true, "year": true, "team": true, "area": true, "room": true,
	"food": true, "tree": true, "door": true, "fire": true, "wall": true,

	// Sound effects / casual words
	"boop": true, "beep": true, "poop": true, "zoom": true, "boom": true,
}

// normalizeRepeatedChars reduces repeated characters to handle obfuscation like "tooddleeer" -> "todler"
func normalizeRepeatedChars(word string) string {
	if len(word) < 3 {
		return word
	}

	var result strings.Builder
	result.WriteByte(word[0])

	for i := 1; i < len(word); i++ {
		// Only add character if it's different from the previous one
		if word[i] != word[i-1] {
			result.WriteByte(word[i])
		}
	}

	return result.String()
}

// correctFuzzyMatches replaces misspelled words with their correct blocklist term
func correctFuzzyMatches(text string, maxDistance int) string {
	words := strings.Fields(text)
	corrected := false

	for i, word := range words {
		wordLower := strings.ToLower(word)
		// Skip very short words to avoid false positives
		if len(wordLower) < 4 {
			continue
		}

		// Skip common words that should never be matched
		if commonWords[wordLower] {
			continue
		}

		// First try direct match
		for _, blocked := range blocklist {
			// Only match if lengths are reasonably similar (within 3 chars difference)
			lengthDiff := len(wordLower) - len(blocked)
			if lengthDiff < -3 || lengthDiff > 3 {
				continue
			}

			distance := levenshtein.ComputeDistance(wordLower, blocked)
			if distance <= maxDistance && distance > 0 {
				words[i] = blocked
				corrected = true
				break
			}
		}

		// If no match found, try with normalized repeated characters
		if !corrected || words[i] == word {
			normalized := normalizeRepeatedChars(wordLower)
			if normalized != wordLower {
				for _, blocked := range blocklist {
					// Only match if lengths are reasonably similar
					lengthDiff := len(normalized) - len(blocked)
					if lengthDiff < -3 || lengthDiff > 3 {
						continue
					}

					distance := levenshtein.ComputeDistance(normalized, blocked)
					if distance <= maxDistance {
						words[i] = blocked
						corrected = true
						break
					}
				}
			}
		}
	}

	if corrected {
		return strings.Join(words, " ")
	}
	return text
}

func CleanPrompt(message string) string {
	// Early return for empty messages
	if message = strings.TrimSpace(message); message == "" {
		return ""
	}

	// Filter to only allow alphanumeric, spaces, and common punctuation characters
	allowedChars := regexp.MustCompile(`[^a-zA-Z0-9 ?!\-\(\)\[\]\.,;:'"]+`)
	message = allowedChars.ReplaceAllString(message, "")

	// Remove brackets/parentheses/punctuation when directly adjacent to letters to prevent bypass
	// This handles cases like (girl, boy), [kid], ((girl, ,girl, etc.
	// Exception: Allow comma followed by space after words (e.g., "bird, flying")
	// Match pattern: special char followed by letter, or letter followed by special char (but not ", ")
	// Loop until no more replacements are made to catch nested/multiple special chars
	bracketBypass := regexp.MustCompile(`[\(\)\[\]\.,;:'"!?\-][a-zA-Z]|[a-zA-Z][\(\)\[\];:'"!?\-]`)
	for {
		replaced := bracketBypass.ReplaceAllStringFunc(message, func(match string) string {
			// Keep only the letter from the match
			if match[0] >= 'a' && match[0] <= 'z' || match[0] >= 'A' && match[0] <= 'Z' {
				return string(match[0])
			}
			return string(match[1])
		})
		if replaced == message {
			break
		}
		message = replaced
	}

	// Trim again after filtering
	if message = strings.TrimSpace(message); message == "" {
		return ""
	}

	// Expanded list of banned phrases
	bannedPhrases := []string{
		"jailbait",
		"barely legal",
		"not legal",
		"child model",
		"teen model",
		"young model",
		"underage model",
		"juvenile model",
		"minor model",
		"age restricted",
		"age verification",
		"age check",
		"too young",
	}

	// Check for banned phrases
	messageLower := strings.ToLower(message)
	for _, phrase := range bannedPhrases {
		if strings.Contains(messageLower, phrase) {
			return "" // Return empty string for banned content
		}
	}

	// Exceptions - words we don't want to replace
	exceptions := []string{
		"power girl",
		"girl power",
		"boy band",
		"girlfriend",
		"boyfriend",
		"boyband",
		"girlband",
		"girls generation",
		"spice girls",
		"teenage mutant ninja turtles",
		"hells angels",  // Added legitimate exception
		"angel food",    // Added food-related exception
		"angel hair",    // Added food-related exception
		"angel numbers", // Added spiritual/numerology exception
	}

	// Check if message contains any exceptions
	for _, exc := range exceptions {
		if strings.Contains(strings.ToLower(message), exc) {
			return message
		}
	}

	// Correct fuzzy matches (misspellings) to proper blocklist terms
	// AFTER exception check so exceptions aren't fuzzy-matched
	// Use distance of 1 to be more conservative and avoid false positives
	message = correctFuzzyMatches(message, 1)

	// Expanded replacement patterns
	replacements := map[string]string{
		// Female youth terms
		`(?i)\b(girl|girls|grl|grll|grls|grlls)\b`:                                                "woman",
		`(?i)\b(girly|girlish|maiden|maidens)\b`:                                                  "woman",
		`(?i)\b(little girl|young girl|small girl|tiny girl|young lady|little lady|small lady)\b`: "woman",
		`(?i)\b(schoolgirl|college girl|highschool girl|middle school girl|elementary girl)\b`:    "woman",
		`(?i)\b(loli|lolli|lolita|gothic lolita|sweet lolita)\b`:                                  "woman",
		`(?i)\b(brownie scout|girl scout|guides|junior guides)\b`:                                 "adult group",

		// Male youth terms
		`(?i)\b(boy|boys|boi|boii|boyz)\b`: "man",
		`(?i)\b(boyish)\b`:                 "mature",
		`(?i)\b(little boy|young boy|small boy|tiny boy|young man|little man|small man)\b`: "man",
		`(?i)\b(schoolboy|college boy|highschool boy|middle school boy|elementary boy)\b`:  "man",
		`(?i)\b(cubscout|cub scout|boy scout|eagle scout|webelos)\b`:                       "adult group",

		// Family terms with age context
		`(?i)\b(young|little|small|tiny)\s+(daughter|son|niece|nephew)\b`: "adult relative",
		`(?i)\b(granddaughter|grandson)\b`:                                "relative",

		// Generic youth terms
		`(?i)\b(pre-k|pre k|child|children|kid|kids|kiddo|kiddies|kiddie|youngster|youngsters)\b`: "adult",
		`(?i)\b(teen|teens|teenager|teenagers|teenage|adolescent|adolescents)\b`:                  "adult",
		`(?i)\b(youth|youths|juvenile|juveniles|minor|minors|underage)\b`:                         "adult",
		`(?i)\b(baby|bby|babies|infant|infants|toddler|toddlers|preschooler|preschoolers)\b`:      "adult",
		`(?i)\b(tween|tweens|preteen|preteens|pre-teen|pre-teens)\b`:                              "adult",
		`(?i)\b(young adult|young person|young people|young ones|young individual)\b`:             "adult",

		// Size descriptor bypasses (mini/small/tiny + human/person)
		`(?i)\b(mini|small|tiny|little|petite)\s+(human|humans|person|persons|people)\b`: "adult",

		// School and education terms
		`(?i)\b(elementary school|grade school|primary school)\b`:               "workplace",
		`(?i)\b(middle school|junior high|intermediate school)\b`:               "workplace",
		`(?i)\b(high school|secondary school|prep school|preparatory school)\b`: "workplace",
		`(?i)\b(daycare|day care|nursery|preschool|kindergarten)\b`:             "workplace",
		`(?i)\b(playground|playroom|play area|jungle gym|swing set)\b`:          "recreation area",
		`(?i)\b(classroom|schoolroom|homeroom|study hall)\b`:                    "meeting room",

		// Physical description terms
		`(?i)\b(young body|young figure|youthful figure|youthful appearance)\b`: "mature appearance",
		`(?i)\b(underdeveloped|developing body|growing body|maturing)\b`:        "mature appearance",
		`(?i)\b(innocent look|innocent appearance|pure|pure looking)\b`:         "mature appearance",
		`(?i)\b(developing figure|budding|blossoming)\b`:                        "mature appearance",

		// Age patterns with extended range (1-20) and comprehensive variations
		// Basic age patterns
		`(?i)\b(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years|y\.o\.|yo|yos|year-old|years-old)\b`: "25 years",
		`(?i)\bage[d]?\s+(?:[1-9]|1\d|20)\b`:                                                "age 25",
		`(?i)\b(?:[1-9]|1\d|20)\s*years?\s*old\b`:                                           "25 years old",

		// Variations with "aged"
		`(?i)\baged?\s*(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\b`: "age 25",
		`(?i)\b(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)-?aged\b`:    "25-year-aged",

		// Age of/at age variations
		`(?i)\b(?:age|aged)\s+of\s+(?:[1-9]|1\d|20)\b`:           "age of 25",
		`(?i)\bat\s+(?:age|the\s+age\s+of)\s+(?:[1-9]|1\d|20)\b`: "at age 25",
		`(?i)\b(?:[1-9]|1\d|20)\s*y(?:ea)?rs?\s+of\s+age\b`:      "25 years of age",

		// Descriptive age patterns
		`(?i)\bis\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*old\b`:                                                "is 25 years old",
		`(?i)\b(?:just|only|about|around|approximately|near(?:ly)?)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|years?)?\s*old?\b`: "25 years old",
		`(?i)\baround\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*old\b`:                                            "around 25 years old",

		// "Under" age patterns
		`(?i)\b(?:under|below|beneath|less\s+than)\s*(?:[1-9]|1\d|20|twenty)\b`:                  "over 25",
		`(?i)\b(?:under|below|beneath|less\s+than)\s*the\s*age\s*of\s*(?:[1-9]|1\d|20|twenty)\b`: "over age 25",

		// Specific age descriptors
		`(?i)\b(?:turned|turning)\s+(?:[1-9]|1\d|20)\b`:                       "turned 25",
		`(?i)\b(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*young\b`:           "25 years old",
		`(?i)\b(?:a|an)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years?)?\s*old\b`: "a 25 year old",

		// Age with adjectives
		`(?i)\b(?:young|mere(?:ly)?|bare(?:ly)?|only)\s+(?:[1-9]|1\d|20)\b`:                                "25",
		`(?i)\b(?:young|mere(?:ly)?|bare(?:ly)?|only)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*old\b`: "25 years old",

		// Age ranges
		`(?i)\b(?:[1-9]|1\d|20)\s*(?:to|-)\s*(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*old\b`:        "25 years old",
		`(?i)\bbetween\s+(?:[1-9]|1\d|20)\s*(?:and|&|-)\s*(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\b`: "25 years",

		// Specific contexts
		`(?i)\b(?:appears|looks|seems)\s+(?:[1-9]|1\d|20)\b`:                              "appears 25",
		`(?i)\b(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)?\s*of\s*age\b`:                    "25 years of age",
		`(?i)\bage[d]?\s*(?:approximately|about|around|near(?:ly)?)\s*(?:[1-9]|1\d|20)\b`: "age 25",

		// Written number variations (can add more as needed)
		`(?i)\b(?:one|two|three|four|five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|fifteen|sixteen|seventeen|eighteen|nineteen|twenty)\s*(?:yr|yrs|year|years)?\s*old\b`: "25 years old",

		// Age with decimals
		`(?i)\b(?:[1-9]|1\d)\.5\s*(?:yr|yrs|year|years)?\s*old\b`: "25 years old",

		// Possessive forms
		`(?i)\b(?:his|her|their)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)\b`: "their 25 years",

		// Time-related contexts
		`(?i)\b(?:after|before)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)\b`: "after 25 years",
		`(?i)\b(?:since|for)\s+(?:[1-9]|1\d|20)\s*(?:yr|yrs|year|years)\b`:    "for 25 years",

		// New angel-related patterns
		`(?i)\b(young|little|small|tiny|pure|innocent)\s*(angel|angels)\b`:                             "person",
		`(?i)\b(angel|angels)\s*(model|models)\b`:                                                      "person",
		`(?i)\b(angelic)\s*(youth|child|children|girl|girls|boy|boys|teen|teens|teenager|teenagers)\b`: "person",
		`(?i)\b(cherub|cherubs|cherubic)\b`:                                                            "person",
	}

	// Apply all replacements
	for pattern, replacement := range replacements {
		re := regexp.MustCompile(pattern)
		message = re.ReplaceAllString(message, replacement)
	}

	// Normalize spaces
	message = strings.Join(strings.Fields(message), " ")

	return message
}

func BadWordsCheck(message string, config settings.ComfyUiConfig) bool {
	for _, word := range config.BadWords {
		if strings.Contains(strings.ToLower(message), strings.ToLower(word)) {
			return true
		}
	}

	return false
}

func GetAibirdMeta(workflowFile string) (*AibirdMeta, error) { //nolint:gocyclo
	// aibird_meta node title
	const metaNodeTitle = "aibird_meta"

	// Validate file path to prevent path traversal
	if strings.Contains(workflowFile, "..") {
		return nil, fmt.Errorf("invalid workflow file path: %s", workflowFile)
	}

	// Read the workflow JSON file
	data, err := os.ReadFile(workflowFile) // #nosec G304 - Path traversal already validated above
	if err != nil {
		logger.Error("Failed to read workflow file", "file", workflowFile, "error", err)
		return nil, fmt.Errorf("failed to read workflow file %s: %w", workflowFile, err)
	}

	// Unmarshal JSON into a generic map
	var workflowData map[string]interface{}
	if err := json.Unmarshal(data, &workflowData); err != nil {
		logger.Error("Failed to unmarshal workflow json", "file", workflowFile, "error", err)
		return nil, fmt.Errorf("failed to unmarshal workflow json: %w", err)
	}

	// Find the aibird_meta node
	nodes, ok := workflowData["nodes"].([]interface{})
	if !ok {
		return nil, errors.New("workflow has no nodes")
	}

	var metaNode map[string]interface{}
	for _, n := range nodes {
		node, ok := n.(map[string]interface{})
		if !ok {
			continue
		}

		// v1 legacy comfyui support
		properties, ok := node["properties"].(map[string]interface{})
		if ok {
			if title, ok := properties["title"].(string); ok && title == metaNodeTitle {
				metaNode = node
				break
			}
		}

		// v2 current comfyui support
		if title, ok := node["title"].(string); ok && title == metaNodeTitle {
			metaNode = node
			break
		}

	}

	if metaNode == nil {
		return nil, fmt.Errorf("workflow %s has no %s node", workflowFile, metaNodeTitle)
	}

	// Extract the TOML string from widget_values
	widgetValues, ok := metaNode["widgets_values"].([]interface{})
	if !ok || len(widgetValues) == 0 {
		return nil, fmt.Errorf("node %s has no widget_values", metaNodeTitle)
	}

	tomlString, ok := widgetValues[0].(string)
	if !ok {
		return nil, fmt.Errorf("first widget value in node %s is not a string", metaNodeTitle)
	}

	// Decode the TOML string into the AibirdMeta struct
	var meta AibirdMeta
	if _, err := toml.Decode(tomlString, &meta); err != nil {
		logger.Error("Failed to decode aibird_meta TOML", "error", err)
		return nil, fmt.Errorf("failed to decode aibird_meta TOML: %w", err)
	}

	return &meta, nil
}
