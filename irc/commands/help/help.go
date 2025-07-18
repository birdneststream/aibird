package help

import (
	"aibird/image/comfyui"
	"aibird/logger"
	"aibird/settings"
	"aibird/status"
	"fmt"
	"strings"
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
			Type:      "standard",
			Help:      "Greets the user.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "status",
			Type:      "standard",
			Help:      "Displays the current status of the airig GPUs.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "help",
			Type:      "standard",
			Help:      "Displays help information for available commands.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "headlies",
			Type:      "standard",
			Help:      "Summarizes the latest headlines from r/worldnews.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "ircnews",
			Type:      "standard",
			Help:      "Rewrites a random world news headline with an IRC theme.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "seen",
			Type: "standard",
			Help: "Checks if a user has been active recently.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to check.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name:      "support",
			Type:      "standard",
			Help:      "Provides information on how to support the project.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "models",
			Type:      "standard",
			Help:      "Lists all available image generation models/workflows.",
			Arguments: []Arguments{},
			Queueable: false,
		},
	}
}

func ImageHelp(config settings.AiBird) []Help {
	return getHelpForWorkflowType("image", config)
}

func VideoHelp(config settings.AiBird) []Help {
	return getHelpForWorkflowType("video", config)
}

func TextHelp() []Help {
	return []Help{
		{
			Name: "ai",
			Type: "text",
			Help: "Interact with the AI services for various tasks.",
			Arguments: []Arguments{
				{Argument: "help", Help: "Show help information for AI commands.", Values: ""},
				{Argument: "info", Help: "Display current AI service and model information.", Values: ""},
				{Argument: "--voice", Help: "Specify the voice for TTS.", Values: "e.g., woman"},
				{Argument: "setPersonality", Help: "Set the AI personality.", Values: ""},
				{Argument: "clearPersonality", Help: "Clear the AI personality.", Values: ""},
				{Argument: "setBasePrompt", Help: "Set the base prompt for AI interactions.", Values: ""},
				{Argument: "clearBasePrompt", Help: "Clear the base prompt for AI interactions.", Values: ""},
				{Argument: "setAiModel", Help: "Set the AI model.", Values: ""},
				{Argument: "clearAiModel", Help: "Clear the AI model.", Values: ""},
				{Argument: "setAiService", Help: "Set the AI service. Options: ollama, openrouter", Values: "ollama, openrouter"},
				{Argument: "clearAiService", Help: "Reset the AI service to default (ollama).", Values: ""},
			},
			Queueable: true,
		},
		{
			Name: "bard",
			Type: "text",
			Help: "Process a Google Gemini request.",
			Arguments: []Arguments{
				{Argument: "<message>", Help: "Specify the message to process.", Values: ""},
				{Argument: "--help", Help: "Show this help message.", Values: ""},
				{Argument: "--voice", Help: "Specify the voice for TTS.", Values: "e.g., woman"},
			},
			Queueable: false,
		},
		{
			Name: "gemini",
			Type: "text",
			Help: "Process a Google Gemini request.",
			Arguments: []Arguments{
				{Argument: "<message>", Help: "Specify the message to process.", Values: ""},
				{Argument: "--help", Help: "Show this help message.", Values: ""},
				{Argument: "--voice", Help: "Specify the voice for TTS.", Values: "e.g., woman"},
			},
			Queueable: false,
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
			Type:      "owner",
			Help:      "Toggle debug mode or display debugging information.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "save",
			Type:      "owner",
			Help:      "Save current state to persistent storage.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "ip",
			Type:      "owner",
			Help:      "Display IP information.",
			Arguments: []Arguments{},
			Queueable: false,
		},
	}
}

func AdminHelp() []Help {
	return []Help{
		{
			Name: "user",
			Type: "admin",
			Help: "Manage user settings and permissions.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user.", Values: ""},
				{Argument: "--latestActivity", Help: "Set the latest activity timestamp.", Values: "unix timestamp"},
				{Argument: "--firstSeen", Help: "Set the first seen timestamp.", Values: "unix timestamp"},
				{Argument: "--latestChat", Help: "Set the latest chat message.", Values: "text"},
				{Argument: "--isAdmin", Help: "Set the admin status.", Values: "true, false"},
				{Argument: "--isOwner", Help: "Set the owner status.", Values: "true, false"},
				{Argument: "--ignored", Help: "Set the ignored status.", Values: "true, false"},
				{Argument: "--accessLevel", Help: "Set the access level.", Values: "integer"},
				{Argument: "--aiService", Help: "Set the AI service.", Values: "ollama, openrouter"},
				{Argument: "--aiModel", Help: "Set the AI model.", Values: "model name"},
				{Argument: "--aiBasePrompt", Help: "Set the AI base prompt.", Values: "text"},
				{Argument: "--aiPersonality", Help: "Set the AI personality.", Values: "text"},
			},
			Queueable: false,
		},
		{
			Name: "channel",
			Type: "admin",
			Help: "Manage channel settings and permissions. No <channel_name> argument use the current channel.",
			Arguments: []Arguments{
				{Argument: "<channel_name>", Help: "Specify the name of the channel.", Values: ""},
				{Argument: "--ai", Help: "Enable or disable AI features.", Values: "true, false"},
				{Argument: "--sd", Help: "Enable or disable Stable Diffusion image generation.", Values: "true, false"},
				{Argument: "--imageDescribe", Help: "Enable or disable image description features.", Values: "true, false"},
				{Argument: "--sound", Help: "Enable or disable sound features.", Values: "true, false"},
				{Argument: "--video", Help: "Enable or disable video features.", Values: "true, false"},
				{Argument: "--actionTrigger", Help: "Set the trigger for actions.", Values: "text"},
				{Argument: "--trimOutput", Help: "Enable or disable trimming of output for responses.", Values: "true, false"},
			},
			Queueable: false,
		},
		{
			Name:      "network",
			Type:      "admin",
			Help:      "Display network information or modify network settings.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name:      "sync",
			Type:      "admin",
			Help:      "Synchronize the current state with the network.",
			Arguments: []Arguments{},
			Queueable: false,
		},
		{
			Name: "op",
			Type: "admin",
			Help: "Grant operator privileges to a user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to op.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "deop",
			Type: "admin",
			Help: "Remove operator privileges from a user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to deop.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "voice",
			Type: "admin",
			Help: "Grant voice privileges to a user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to voice.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "devoice",
			Type: "admin",
			Help: "Remove voice privileges from a user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to devoice.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "kick",
			Type: "admin",
			Help: "Kick a user from a channel.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to kick.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "ban",
			Type: "admin",
			Help: "Ban a user from a channel.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to ban.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "unban",
			Type: "admin",
			Help: "Remove a ban from a user in a channel.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to unban.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "topic",
			Type: "admin",
			Help: "Set or view the topic of the current channel.",
			Arguments: []Arguments{
				{Argument: "<topic>", Help: "Specify the new topic for the channel.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "join",
			Type: "admin",
			Help: "Join a channel.",
			Arguments: []Arguments{
				{Argument: "<channel_name>", Help: "Specify the name of the channel to join.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "part",
			Type: "admin",
			Help: "Leave a channel.",
			Arguments: []Arguments{
				{Argument: "<channel_name>", Help: "Specify the name of the channel to leave.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "ignore",
			Type: "admin",
			Help: "Ignore messages from a specified user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to ignore.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "unignore",
			Type: "admin",
			Help: "Stop ignoring messages from a specified user.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the nickname of the user to unignore.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "nick",
			Type: "admin",
			Help: "Change the bots nickname.",
			Arguments: []Arguments{
				{Argument: "<nickname>", Help: "Specify the new nickname.", Values: ""},
			},
			Queueable: false,
		},
		{
			Name: "clearqueue",
			Type: "admin",
			Help: "Clear specified queue(s) or all queues.",
			Arguments: []Arguments{
				{Argument: "[4090|2070|all]", Help: "Specify which queue(s) to clear. Use 'all' to clear both queues.", Values: "4090, 2070, all"},
			},
			Queueable: false,
		},
		{
			Name:      "removecurrent",
			Type:      "admin",
			Help:      "Remove the currently processing item from both queues.",
			Arguments: []Arguments{},
			Queueable: false,
		},
	}
}

func FindHelp(name string, config settings.AiBird) string {
	helpItems := append(AdminHelp(), SoundHelp(config)...)
	helpItems = append(helpItems, ImageHelp(config)...)
	helpItems = append(helpItems, VideoHelp(config)...)
	helpItems = append(helpItems, StandardHelp()...)
	helpItems = append(helpItems, TextHelp()...)

	var foundItems []Help
	processed := make(map[string]bool)
	for _, help := range helpItems {
		if help.Name == name && !processed[name] {
			foundItems = append(foundItems, help)
			processed[name] = true
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

		result.WriteString(fmt.Sprintf("%s - %s%s\n", help.Name, help.Help, queueableIndicator))
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
			result.WriteString(fmt.Sprintf(" Example: %s\n", help.Example))
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
				Argument: "<message>",
				Help:     "The main prompt for the generation.",
				Values:   "",
			})
		}

		for paramName, paramDef := range meta.Parameters {
			var valuesString string

			// Special handling for dynamic voice list
			if paramName == "voice" {
				statusClient := status.NewClient(config)
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