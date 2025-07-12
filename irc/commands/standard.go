package commands

import (
	"aibird/image/comfyui"
	"aibird/irc/state"
	"aibird/queue"
	"aibird/status"

	"github.com/lrstanley/girc"
)

func ParseStandard(irc state.State) {
	// For backward compatibility, call the version with queue
	ParseStandardWithQueue(irc, nil)
}

func ParseStandardWithQueue(irc state.State, q *queue.DualQueue) {
	switch irc.Command.Action {
	case "help":

		irc.Send("Type  <command> --help for more information on a command.")

		irc.Send(girc.Fmt("Commands: {b}help{b}, {b}hello{b}, {b}seen{b}, {b}support{b}, {b}queue{b}, {b}models{b}, {b}headlies{b}, {b}ircnews{b}"))

		if irc.Channel.Sd {
			irc.Send(girc.Fmt("Sd commands: {b}sd{b}, " + comfyui.GetWorkFlows(true)))
		}

		if irc.Channel.Sound {
			irc.Send(girc.Fmt("Sound commands: {b}tts{b}"))
		}

		if irc.Channel.Ai {
			irc.Send(girc.Fmt("Ai commands: {b}ai{b}, {b}openrouter{b}, {b}bard{b}, {b}gemini{b} (same as bard)"))
		}

		// admin commands help
		if irc.User.IsAdmin {
			irc.Send(girc.Fmt("Admin commands: {b}user{b}, {b}op{b}, {b}deop{b}, {b}voice{b}, {b}devoice{b}, {b}kick{b}, {b}ban{b}, {b}unban{b}, {b}topic{b}, {b}join{b}, {b}part{b}, {b}ignore{b}, {b}unignore{b}"))
		}

		// owner commands help
		if irc.User.IsOwner {
			irc.Send(girc.Fmt("Owner commands: {b}debug{b}, {b}save{b}, {b}ip{b}"))
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

	case "support":
		for _, support := range irc.Config.AiBird.Support {
			irc.Send(girc.Fmt("💲 " + support.Name + ": " + support.Value))
		}
		irc.Send(girc.Fmt("After you have {b}supported{b} contact an admin to enable your support only features."))
		return

	case "queue":
		if q != nil {
			ShowQueueStatus(irc, q)
		} else {
			irc.Send("❌ Queue status unavailable")
		}
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
