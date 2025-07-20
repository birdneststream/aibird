package commands

import (
	"aibird/image/comfyui"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/queue"
	"aibird/status"
	"strings"

	"github.com/lrstanley/girc"
)

func ParseStandard(irc state.State) {
	// For backward compatibility, call the version with queue
	ParseStandardWithQueue(irc, nil)
}

func formatHelp(prefix string, commands []help.Help) string {
	var names []string
	for _, cmd := range commands {
		names = append(names, "{b}"+cmd.Name+"{b}")
	}
	return prefix + strings.Join(names, ", ")
}

func filterHelp(commands []help.Help, irc state.State) []help.Help {
	var filtered []help.Help
	for _, cmd := range commands {
		if !help.IsCommandDenied(cmd.Name, irc) {
			filtered = append(filtered, cmd)
		}
	}
	return filtered
}

func ParseStandardWithQueue(irc state.State, q *queue.DualQueue) {
	switch irc.Command.Action {
	case "help":
		irc.Send("Type  <command> --help for more information on a command.")

		irc.Send(girc.Fmt(formatHelp("IRC: ", filterHelp(help.StandardHelp(), irc))))

		if irc.Channel.Sd {
			irc.Send(girc.Fmt(formatHelp("Images: ", filterHelp(help.ImageHelp(irc.Config.AiBird), irc))))
		}

		if irc.Channel.Sound {
			irc.Send(girc.Fmt(formatHelp("Audio: ", filterHelp(help.SoundHelp(irc.Config.AiBird), irc))))
		}

		if irc.Channel.Video {
			irc.Send(girc.Fmt(formatHelp("Video: ", filterHelp(help.VideoHelp(irc.Config.AiBird), irc))))
		}

		if irc.Channel.Ai {
			irc.Send(girc.Fmt(formatHelp("Text: ", filterHelp(help.TextHelp(), irc))))
		}

		// admin commands help
		if irc.User.IsAdmin {
			irc.Send(girc.Fmt(formatHelp("Admin: ", filterHelp(help.AdminHelp(), irc))))
		}

		// owner commands help
		if irc.User.IsOwner {
			irc.Send(girc.Fmt(formatHelp("Owner: ", filterHelp(help.OwnerHelp(), irc))))
		}

		return
	case "hello":
		irc.Send(girc.Fmt("{b}hello{b} {blue}" + irc.User.NickName + "{c}!"))
		return
	case "seen":
		user, _ := irc.Channel.GetUserWithNick(irc.Message())
		if user == nil {
			irc.Send("I have not seen this user")
			return
		}

		if irc.Event.Source.Name == user.NickName {
			irc.ReplyTo(girc.Fmt("{b}Hey pal you are seen!{b}"))
			return
		}

		irc.Send(user.Seen())
		return
	case "status":
		client := status.NewClient(irc.Config.AiBird)
		formattedStatus, err := client.GetFormattedStatus()
		if err != nil {
			irc.Send(girc.Fmt("❌ Error getting status: " + err.Error()))
			return
		}
		irc.Send(girc.Fmt(formattedStatus))
		if q != nil {
			irc.Send(ShowQueueStatus(irc, q))
		}
	
	case "support":
		for _, support := range irc.Config.AiBird.Support {
			irc.Send(girc.Fmt("💲 " + support.Name + ": " + support.Value))
		}
		irc.Send(girc.Fmt("After you have {b}supported{b} contact an admin to enable your support only features."))
		return

	case "models":
		// List all available image generation models/workflows
		if irc.Channel.Sd {
			irc.Send(girc.Fmt("📸 Available image generation models/workflows: " + comfyui.GetWorkFlows(true)))
		} else {
			irc.Send(girc.Fmt("❌ Image generation is disabled in this channel."))
		}
		return
	case "headlies":
		ParseHeadlines(irc)
	case "ircnews":
		ParseIrcNews(irc)
	}
}