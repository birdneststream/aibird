package main

import (
	"context"
	"strings"

	"aibird/irc/commands"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/logger"
	"aibird/queue"
	"aibird/shared/meta"

	"github.com/lrstanley/girc"
)

func dispatchCommand(irc state.State, q *queue.ProcessingQueue) {
	// Check if the command is denied at any level
	action := irc.Action()
	if irc.Channel != nil {
		for _, deniedCmd := range irc.Channel.DenyCommands {
			if strings.EqualFold(action, deniedCmd) {
				return
			}
		}
	}
	if irc.Network != nil {
		for _, deniedCmd := range irc.Network.DenyCommands {
			if strings.EqualFold(action, deniedCmd) {
				return
			}
		}
	}
	for _, deniedCmd := range irc.Config.AiBird.DenyCommands {
		if strings.EqualFold(action, deniedCmd) {
			return
		}
	}

	if helpArg, ok := irc.FindArgument("help", false).(bool); ok && helpArg {
		helpMsg := help.FindHelp(irc)
		irc.Send(girc.Fmt(helpMsg))
		return
	}

	// Special handling for !ai with llama.cpp - needs queue and can_use check
	if commands.ShouldQueueLlamaCppAi(irc) {
		// Check can_use flag before queueing
		if err := commands.CheckAiCanUse(irc); err != nil {
			irc.SendError(err.Error())
			return
		}

		queueItem := queue.QueueItem{
			Item: queue.Item{
				State: irc,
				Function: func(ctx context.Context, s state.State, gpu meta.GPUType) {
					commands.ProcessLlamaCppAiRequest(ctx, s, gpu)
				},
			},
			Model: "llamacpp-ai", // Special identifier for llama.cpp requests
			User:  irc.User,
			GPU:   meta.GPU4090,
		}

		msg, err := q.Enqueue(queueItem)
		if err != nil {
			irc.SendError(err.Error())
		} else if msg != "" {
			irc.Send(msg)
		}
		return
	}

	if commands.IsQueueableCommand(irc) {
		// Create QueueItem with model information
		queueItem := queue.QueueItem{
			Item: queue.Item{
				State: irc,
				Function: func(ctx context.Context, s state.State, gpu meta.GPUType) {
					commands.RunQueueableCommand(ctx, s, gpu)
				},
			},
			Model: irc.Action(), // Use the command as the model identifier
			User:  irc.User,     // User implements UserAccess interface
		}

		msg, err := q.Enqueue(queueItem)
		if err != nil {
			irc.SendError(err.Error())
		} else if msg != "" {
			irc.Send(msg)
		}
	} else {
		// Not a queueable command, so we find the correct parser
		if commands.IsTextCommand(irc.Action()) {
			commands.ParseAiText(irc)
			return
		}
		switch {
		case commands.IsStandardCommand(irc.Action()):
			go commands.ParseStandardWithQueue(irc, q)
		case commands.IsAdminCommand(irc.Action()):
			go commands.ParseAdminWithQueue(irc, q)
		case commands.IsOwnerCommand(irc.Action()):
			go commands.ParseOwner(irc)
		case commands.IsSoundCommand(irc.Action(), irc.Config.AiBird):
			go commands.ParseAiSound(irc)
		case commands.IsVideoCommand(irc.Action(), irc.Config.AiBird):
			go commands.ParseAiVideo(irc)
		default:
			logger.Warn("Command was valid but no parser was found", "command", irc.Action())
		}
	}
}
