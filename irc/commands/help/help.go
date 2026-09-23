// Package help provides command help text metadata and lookup for all bot commands.
package help

import (
	"fmt"
	"strings"

	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/settings"
	"aibird/status"
)

// Command type constants used in help category definitions.
const (
	typeStandard = "standard"
	typeAdmin    = "admin"
	typeOwner    = "owner"
	typeText     = "text"
	typeVideo    = "video"
	typeHelp     = "help"
)

// Argument placeholder constants used across help definitions.
const (
	argNickname    = "<nickname>"
	argHours       = "<hours>"
	argChannelName = "<channel_name>"
	argMessage     = "<message>"
	valTrueFalse   = "true, false"
	valText        = "text"
	cmdHelp        = "help"
)

type (
	Help struct {
		Name      string
		Type      string
		Help      string
		Arguments []Arguments
		Queueable bool // Indicates if this command is queueable
		Example   string
	}

	Arguments struct {
		Argument string
		Help     string
		Values   string
	}
)

func StandardHelp() []Help {
	return []Help{
		{
			Name:      "hello",
			Type:      typeStandard,
			Help:      "Greets the user.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "status",
			Type:      typeStandard,
			Help:      "Displays the current status of the airig GPUs.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      cmdHelp,
			Type:      typeStandard,
			Help:      "Displays help information for available commands.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "headlies",
			Type:      typeStandard,
			Help:      "Summarizes the latest headlines from r/worldnews.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "ircnews",
			Type:      typeStandard,
			Help:      "Rewrites a random world news headline with an IRC theme.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "seen",
			Type: typeStandard,
			Help: "Reports when a user was last seen in this channel (chat, join, part, quit).",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to check.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name:      "support",
			Type:      typeStandard,
			Help:      "Provides information on how to support the project.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "models",
			Type:      typeStandard,
			Help:      "Lists all available image generation models/workflows.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "play",
			Type: typeStandard,
			Help: "Play IRC ASCII art from a birdhole derived text file URL.",
			Arguments: []Arguments{
				{Argument: "<url>", Help: "URL to the ASCII art text file (must be from hole.birdnest.live/derived)", Values: ""},
			},
			Queueable: false,
			Example:   "play https://hole.birdnest.live/derived/vf5t3imvd6jc.png/vf5t3imvd6jc.txt",
		},
		{
			Name:      "record",
			Type:      typeStandard,
			Help:      "Record the most recently generated ASCII art to the configured recording service.",
			Arguments: []Arguments{},
			Queueable: false,
			Example:   "record",
		},
		{
			Name: "leaderboard",
			Type: typeStandard,
			Help: "Display command usage leaderboard for the current network.",
			Arguments: []Arguments{
				{Argument: "global", Help: "Show global leaderboard across all networks (owner only)", Values: ""},
				{Argument: "<command>", Help: "Show leaderboard for specific command (owner only)", Values: "e.g., ai, hello"},
			},
			Queueable: false,
			Example:   "leaderboard",
		},
		{
			Name: "summary",
			Type: typeStandard,
			Help: "Summarizes recent channel activity using AI.",
			Arguments: []Arguments{
				{Argument: argHours, Help: "Number of hours to summarize (default: 24, max: 168).", Values: "e.g., 6, 12, 48"},
				{Argument: "--persona", Help: "Creative angle or persona for the summary.", Values: "e.g., \"a western cowboy\""},
			},
			Queueable: false,
			Example:   "summary 6h --persona \"from the view of a psychologist\"",
		},
		{
			Name: "recall",
			Type: typeStandard,
			Help: "Searches recent channel history for a keyword.",
			Arguments: []Arguments{
				{Argument: "<keyword>", Help: "Keyword or phrase to search for.", Values: ""},
			},
			Queueable: false,
			Example:   "recall bird",
		},
		{
			Name: "top",
			Type: typeStandard,
			Help: "Shows channel activity stats and top chatters.",
			Arguments: []Arguments{
				{Argument: argHours, Help: "Number of hours for stats (default: 24, max: 168).", Values: "e.g., 6, 24, 168"},
			},
			Queueable: false,
			Example:   "top 6h",
		},
		{
			Name: "mood",
			Type: typeStandard,
			Help: "Analyzes the current mood/vibe of the channel.",
			Arguments: []Arguments{
				{Argument: argHours, Help: "Number of hours to analyze (default: 24, max: 168).", Values: "e.g., 6, 24"},
			},
			Queueable: false,
			Example:   "mood",
		},
		{
			Name: "story",
			Type: typeStandard,
			Help: "Turns channel events into a short fictional story.",
			Arguments: []Arguments{
				{Argument: argHours, Help: "Number of hours of events (default: 24, max: 168).", Values: "e.g., 6, 24"},
				{Argument: "--persona", Help: "Genre or style for the story.", Values: "e.g., \"fantasy\", \"film noir\""},
			},
			Queueable: false,
			Example:   "story 6h --persona \"sci-fi\"",
		},
		{
			Name: "vibe",
			Type: typeStandard,
			Help: "Generates a visual artwork capturing the channel's vibe using AI.",
			Arguments: []Arguments{
				{Argument: argHours, Help: "Number of hours to analyze (default: 6, max: 48).", Values: "e.g., 3, 6, 24"},
			},
			Queueable: false,
			Example:   "vibe 6h",
		},
	}
}

func ImageHelp(config settings.AiBird) []Help {
	return getHelpForWorkflowType("image", config)
}

func VideoHelp(config settings.AiBird) []Help {
	// The h3 video command runs the MiniMax H3 workflow. Patreon-gated (free in
	// #h4x).
	videoHelp := getHelpForWorkflowType(typeVideo, config)
	return append(videoHelp, enhancedVideoCommands()...)
}

// enhancedVideoCommands returns the help entry for the patreon-gated h3 video
// command, which runs the MiniMax H3 workflow. Patreon-gated (free in #h4x).
func enhancedVideoCommands() []Help {
	return []Help{
		{
			Name:      "h3",
			Type:      typeVideo,
			Help:      "Generate a MiniMax H3 video from your idea using the h3 workflow. Patreon-only (free in #h4x).",
			Arguments: []Arguments{{Argument: argMessage, Help: "Your short video idea or rough description.", Values: ""}},
			Queueable: true,
			Example:   "!h3 a woman walking through snow at night, lonely",
		},
	}
}

func TextHelp() []Help {
	return []Help{
		{
			Name: "ai",
			Type: typeText,
			Help: "Interact with the AI services for various tasks. When MCP is enabled, the AI can search the web and scrape URLs for up-to-date information.",
			Arguments: []Arguments{
				{Argument: argMessage, Help: "Your question or request for the AI.", Values: ""},
				{Argument: cmdHelp, Help: "Show help information for AI commands.", Values: ""},
				{Argument: "info", Help: "Display current AI service and model information.", Values: ""},
				{Argument: "--tts", Help: "Convert the AI response to speech.", Values: ""},
				{Argument: "--voice", Help: "Specify the voice for TTS.", Values: "e.g., woman"},
				{Argument: "setPersonality", Help: "Set the AI personality.", Values: ""},
				{Argument: "clearPersonality", Help: "Clear the AI personality.", Values: ""},
				{Argument: "setBasePrompt", Help: "Set the base prompt for AI interactions.", Values: ""},
				{Argument: "clearBasePrompt", Help: "Clear the base prompt for AI interactions.", Values: ""},
				{Argument: "setAiModel", Help: "Set the AI model.", Values: ""},
				{Argument: "clearAiModel", Help: "Clear the AI model.", Values: ""},
				{Argument: "setAiService", Help: "Set the AI service. Options: llamacpp, glm", Values: "llamacpp, glm"},
				{Argument: "clearAiService", Help: "Reset the AI service to default (llamacpp).", Values: ""},
			},
			Queueable: true,
			Example:   "!ai What are the latest developments in Go 1.25?",
		},
	}
}

func SoundHelp(config settings.AiBird) []Help {
	// Dynamically get help for sound workflows from ComfyUI metadata
	soundHelp := getHelpForWorkflowType("sound", config)

	// Manually add commands that are not ComfyUI workflows
	soundHelp = append(soundHelp, Help{
		Name: "tts-add",
		Type: "sound",
		Help: "Add a new TTS voice from a URL. (Admin only)",
		Arguments: []Arguments{
			{Argument: "--url", Help: "URL of the audio file.", Values: "url"},
			{Argument: "--name", Help: "Name for the new voice.", Values: "string"},
			{Argument: "--start", Help: "Start time of the clip.", Values: "e.g., 00:01:23"},
			{Argument: "--duration", Help: "Duration of the clip in seconds.", Values: "e.g., 10"},
		},
		Queueable: false, // Admin command, not queueable
	})

	return soundHelp
}

func OwnerHelp() []Help {
	return []Help{
		{
			Name:      "debug",
			Type:      typeOwner,
			Help:      "Toggle debug mode or display debugging information.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "save",
			Type:      typeOwner,
			Help:      "Save current state to persistent storage.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "ip",
			Type:      typeOwner,
			Help:      "Display IP information.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "dbstats",
			Type:      typeOwner,
			Help:      "Display SQLite database statistics including keys and disk usage.",
			Arguments: []Arguments{},
			Queueable: false,
		},
	}
}

func AdminHelp() []Help {
	return []Help{
		{
			Name: "user",
			Type: typeAdmin,
			Help: "Manage user settings and permissions.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user.", Values: ""},
				{Argument: "--latestActivity", Help: "Set the latest activity timestamp.", Values: "unix timestamp"},
				{Argument: "--firstSeen", Help: "Set the first seen timestamp.", Values: "unix timestamp"},
				{Argument: "--latestChat", Help: "Set the latest chat message.", Values: valText},
				{Argument: "--isAdmin", Help: "Set the admin status.", Values: valTrueFalse},
				{Argument: "--isOwner", Help: "Set the owner status.", Values: valTrueFalse},
				{Argument: "--ignored", Help: "Set the ignored status.", Values: valTrueFalse},
				{Argument: "--accessLevel", Help: "Set the access level.", Values: "integer"},
				{Argument: "--aiService", Help: "Set the AI service.", Values: "llamacpp, glm"},
				{Argument: "--aiModel", Help: "Set the AI model.", Values: "model name"},
				{Argument: "--aiBasePrompt", Help: "Set the AI base prompt.", Values: valText},
				{Argument: "--aiPersonality", Help: "Set the AI personality.", Values: valText},
			},
			Queueable: false,
		},
		{
			Name: "channel",
			Type: typeAdmin,
			Help: "Manage channel settings and permissions. No <channel_name> argument use the current channel.",
			Arguments: []Arguments{
				{Argument: argChannelName, Help: "Specify the name of the channel.", Values: ""},
				{Argument: "--ai", Help: "Enable or disable AI features.", Values: valTrueFalse},
				{Argument: "--sd", Help: "Enable or disable Stable Diffusion image generation.", Values: valTrueFalse},
				{Argument: "--imageDescribe", Help: "Enable or disable image description features.", Values: valTrueFalse},
				{Argument: "--sound", Help: "Enable or disable sound features.", Values: valTrueFalse},
				{Argument: "--video", Help: "Enable or disable video features.", Values: valTrueFalse},
				{Argument: "--actionTrigger", Help: "Set the trigger for actions.", Values: valText},
				{Argument: "--trimOutput", Help: "Enable or disable trimming of output for responses.", Values: valTrueFalse},
			},
			Queueable: false,
		},
		{
			Name:      "network",
			Type:      typeAdmin,
			Help:      "Display network information or modify network settings.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "sync",
			Type:      typeOwner,
			Help:      "Synchronize channel state and restore missing user modes.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "op",
			Type: typeAdmin,
			Help: "Grant operator privileges to a user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to op.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "deop",
			Type: typeAdmin,
			Help: "Remove operator privileges from a user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to deop.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "voice",
			Type: typeAdmin,
			Help: "Grant voice privileges to a user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to voice.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "devoice",
			Type: typeAdmin,
			Help: "Remove voice privileges from a user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to devoice.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "kick",
			Type: typeAdmin,
			Help: "Kick a user from a channel.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to kick.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "ban",
			Type: typeAdmin,
			Help: "Ban a user from a channel.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to ban.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "unban",
			Type: typeAdmin,
			Help: "Remove a ban from a user in a channel.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to unban.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "topic",
			Type: typeAdmin,
			Help: "Set or view the topic of the current channel.",
			Arguments: []Arguments{
				{Argument: "<topic>", Help: "Specify the new topic for the channel.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "join",
			Type: typeAdmin,
			Help: "Join a channel.",
			Arguments: []Arguments{
				{Argument: argChannelName, Help: "Specify the name of the channel to join.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "part",
			Type: typeAdmin,
			Help: "Leave a channel.",
			Arguments: []Arguments{
				{Argument: argChannelName, Help: "Specify the name of the channel to leave.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "ignore",
			Type: typeAdmin,
			Help: "Ignore messages from a specified user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to ignore.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "unignore",
			Type: typeAdmin,
			Help: "Stop ignoring messages from a specified user.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the nickname of the user to unignore.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "nick",
			Type: typeAdmin,
			Help: "Change the bots nickname.",
			Arguments: []Arguments{
				{Argument: argNickname, Help: "Specify the new nickname.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name:      "clearqueue",
			Type:      typeAdmin,
			Help:      "Clear the processing queue.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "removecurrent",
			Type:      typeAdmin,
			Help:      "Remove the currently processing item from both queues.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "live",
			Type: typeAdmin,
			Help: "Announce a YouTube live stream: highlights the channel, sets an ON AIR topic, and auto-updates the topic when the stream ends (auto-detect needs [youtube] apiKey). No arguments shows the current status.",
			Arguments: []Arguments{
				{Argument: "<youtube_url>", Help: "YouTube watch or share URL of the stream.", Values: ""},
				{Argument: "off", Help: "End the on-air state and set the WAS LIVE topic.", Values: ""},
			},
			Queueable: false,
			Example:   "!live https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		},
	}
}

func FindHelp(irc *state.State) string {
	config := irc.Config.AiBird
	name := irc.Action()

	helpItems := append(AdminHelp(), SoundHelp(config)...)
	if irc.User != nil && irc.User.IsOwnerUser() {
		helpItems = append(helpItems, OwnerHelp()...)
	}
	helpItems = append(helpItems, ImageHelp(config)...)
	helpItems = append(helpItems, VideoHelp(config)...)
	helpItems = append(helpItems, StandardHelp()...)
	helpItems = append(helpItems, TextHelp()...)

	var foundItems []Help
	processed := make(map[string]bool)

	if name == typeHelp {
		for _, help := range helpItems {
			if !irc.IsCommandDenied(help.Name) && !processed[help.Name] {
				foundItems = append(foundItems, help)
				processed[help.Name] = true
			}
		}
	} else {
		for _, help := range helpItems {
			if irc.IsCommandDenied(help.Name) {
				continue
			}
			if help.Name == name && !processed[name] {
				foundItems = append(foundItems, help)
				processed[name] = true
			}
		}
	}

	return Format(foundItems)
}

func Format(helpItems []Help) string {
	var result strings.Builder

	for _, help := range helpItems {
		// Add queueable indicator to the help text
		queueableIndicator := ""
		if help.Queueable {
			queueableIndicator = " [Queueable]"
		}

		fmt.Fprintf(&result, "%s - %s%s\n", help.Name, help.Help, queueableIndicator)
		if len(help.Arguments) > 0 {
			for i, arg := range help.Arguments {
				// Determine the prefix based on whether the argument is the last in the slice
				prefix := " ├  "
				if i == len(help.Arguments)-1 {
					prefix = " └  "
				}

				argInfo := fmt.Sprintf("%s%s: %s", prefix, arg.Argument, arg.Help)
				if arg.Values != "" {
					argInfo += fmt.Sprintf(" (Values: %s)", arg.Values)
				}
				result.WriteString(argInfo + "\n")
			}
		}
		if help.Example != "" {
			fmt.Fprintf(&result, " Example: %s\n", help.Example)
		}
		result.WriteString("\n") // Add an extra newline for better separation
	}

	return result.String()
}

func getHelpForWorkflowType(workflowType string, config settings.AiBird) []Help {
	var helpItems []Help

	workflows := comfyui.GetWorkFlowsSlice()
	for _, workflowName := range workflows {
		workflowFile := "comfyuijson/" + workflowName + ".json"
		meta, err := comfyui.GetAibirdMeta(workflowFile)
		if err != nil {
			logger.Warn("Skipping workflow for help generation due to error", "workflow", workflowName, "error", err)
			continue // Skip workflows that cause errors
		}

		if meta == nil {
			continue // Skip workflows without metadata
		}

		if meta.Type != workflowType {
			continue
		}

		arguments := []Arguments{}
		if meta.PromptTarget.Node != "" {
			arguments = append(arguments, Arguments{
				Argument: argMessage,
				Help:     "The main prompt for the generation.",
				Values:   "",
			})
		}

		for paramName, paramDef := range meta.Parameters {
			var valuesString string

			// Special handling for dynamic voice list
			if paramName == "voice" {
				statusClient := status.NewClient(&config)
				voices, err := statusClient.GetWavs()
				if err != nil {
					logger.Error("Could not get voices for help message", "error", err)
					valuesString = "dynamically loaded"
				} else {
					valuesString = strings.Join(voices, ", ")
				}
			} else {
				var valueParts []string
				if paramDef.Type != "" {
					valueParts = append(valueParts, paramDef.Type)
				}
				if paramDef.Default != nil {
					valueParts = append(valueParts, fmt.Sprintf("default: %v", paramDef.Default))
				}
				if paramDef.Min != nil && paramDef.Max != nil {
					valueParts = append(valueParts, fmt.Sprintf("range: %g-%g", *paramDef.Min, *paramDef.Max))
				} else if paramDef.Min != nil {
					valueParts = append(valueParts, fmt.Sprintf("min: %g", *paramDef.Min))
				} else if paramDef.Max != nil {
					valueParts = append(valueParts, fmt.Sprintf("max: %g", *paramDef.Max))
				}
				valuesString = strings.Join(valueParts, ", ")
			}

			arguments = append(arguments, Arguments{
				Argument: "--" + paramName,
				Help:     paramDef.Description,
				Values:   valuesString,
			})
		}

		description := meta.Description
		if meta.URL != "" {
			description = fmt.Sprintf("%s (More Info: %s)", description, meta.URL)
		}

		helpItem := Help{
			Name:      workflowName,
			Type:      workflowType,
			Help:      description,
			Arguments: arguments,
			Queueable: true, // All ComfyUI workflows are queueable
			Example:   meta.Example,
		}
		helpItems = append(helpItems, helpItem)
	}

	return helpItems
}
