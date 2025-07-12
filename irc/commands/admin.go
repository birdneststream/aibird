package commands

import (
	"aibird/helpers"
	"aibird/irc/channels"
	"aibird/irc/commands/help"
	"aibird/irc/state"
	"aibird/irc/users"
	"aibird/logger"
	"aibird/queue"
	"strings"

	meta "aibird/shared/meta"

	"github.com/lrstanley/girc"
)

func ParseAdmin(irc state.State) {
	// For now, call the original function without queue access
	// This will be updated when we have queue access
	parseAdminCommands(irc, nil)
}

func ParseAdminWithQueue(irc state.State, q *queue.DualQueue) {
	parseAdminCommands(irc, q)
}

func parseAdminCommands(irc state.State, q *queue.DualQueue) {
	if irc.User.IsAdmin {
		switch irc.Command.Action {

		case "debug":
			logger.Debug("IRC Client Debug Info",
				"channels", irc.Client.Channels(),
				"users", irc.Client.Users(),
				"network_name", irc.Client.NetworkName(),
				"client_string", irc.Client.String())

			for _, user := range irc.Client.Users() {
				logger.Debug("User info", "channels", strings.Join(user.ChannelList, "-"), "nick", user.Nick, "ident", user.Ident, "host", user.Host)
			}
			for _, channel := range irc.Client.Channels() {
				logger.Debug("Channel info", "channel", channel)
			}

			return
		case "admin":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.Format(help.AdminHelp())))
				return
			}
		case "user":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("user", irc.Config.AiBird)))
				return
			}

			var user *users.User

			// Check if we're dealing with ident@host format
			if strings.Contains(irc.Command.Message, "@") {
				// Parse ident@host format
				parts := strings.SplitN(irc.Command.Message, "@", 2)
				ident := parts[0]
				host := parts[1]

				// Try with original ident first
				user = irc.Network.GetUserWithIdentAndHost(ident, host)

				// If not found and ident doesn't already have ~, try with ~ prepended
				if user == nil && !strings.HasPrefix(ident, "~") {
					user = irc.Network.GetUserWithIdentAndHost("~"+ident, host)
				}
			} else {
				// Use the standard nick lookup if no @ is found
				user = irc.Network.GetUserWithNick(irc.Command.Message)
			}

			if user != nil {
				if irc.IsEmptyArguments() {
					irc.Send(girc.Fmt(user.String()))
					return
				}

				if user.IsOwnerUser() && !irc.User.IsOwnerUser() {
					irc.SendError("Cannot change owner user")
					return
				}

				irc.UpdateUserBasedOnArgs(user)
			}

			return
		case "channel":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("channel", irc.Config.AiBird)))
				return
			}

			var channel *channels.Channel
			if irc.IsEmptyMessage() {
				channel = irc.Network.GetNetworkChannel(helpers.FindChannelNameInEventParams(irc.Event))
			} else {
				channel = irc.Network.GetNetworkChannel(irc.Command.Message)
			}

			if channel != nil {
				if irc.IsEmptyArguments() {
					irc.Send(girc.Fmt(channel.String()))
					return
				}

				irc.UpdateChannelBasedOnArgs()
			}
		case "network":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("network", irc.Config.AiBird)))
				return
			}

			if irc.IsEmptyArguments() {
				irc.Send(girc.Fmt(irc.Network.String()))
				return
			}

			irc.UpdateNetworkBasedOnArgs()
			return
		case "sync":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("sync", irc.Config.AiBird)))
				return
			}

			irc.Send("Syncing network...")
			irc.Client.Cmd.SendRaw("WHO " + irc.Channel.Name)
			return
		case "op":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("op", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "+o", irc.Command.Message)
			return
		case "deop":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("deop", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "-o", irc.Command.Message)
			return
		case "voice":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("voice", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "+v", irc.Command.Message)
			return
		case "devoice":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("devoice", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "-v", irc.Command.Message)
			return
		case "kick":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("kick", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Kick(irc.Channel.Name, irc.Command.Message, "You have been kicked by "+irc.User.NickName)
			return
		case "ban":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("ban", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "+b", irc.Command.Message)
			return
		case "unban":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("unban", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Mode(irc.Channel.Name, "-b", irc.Command.Message)
			return
		case "topic":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("topic", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Topic(irc.Channel.Name, irc.Command.Message)
			return
		case "join":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("join", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Join(irc.Command.Message)
			return
		case "part":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("part", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Part(irc.Command.Message)
			return
		case "ignore":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("ignore", irc.Config.AiBird)))
				return
			}

			irc.Send("Ignoring " + irc.Command.Message)
			user, _ := irc.Channel.GetUserWithNick(irc.Command.Message)

			if user != nil {
				user.Ignore()
				irc.Network.Save()
			}

			return
		case "unignore":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("unignore", irc.Config.AiBird)))
				return
			}

			irc.Send("Unignoring " + irc.Command.Message)
			user, _ := irc.Channel.GetUserWithNick(irc.Message())

			if user != nil {
				user.UnIgnore()
				irc.Network.Save()
			}

			return
		case "nick":
			if irc.FindArgument("help", false).(bool) {
				irc.Send(girc.Fmt(help.FindHelp("nick", irc.Config.AiBird)))
				return
			}

			irc.Client.Cmd.Nick(irc.Command.Message)
			return
		case "clearqueue":
			if irc.FindArgument("help", false).(bool) {
				irc.Send("Usage: !admin clearqueue [4090|2070|all] - Clear specified queue(s)")
				return
			}

			if q == nil {
				irc.SendError("Queue system not available")
				return
			}

			target := irc.FindArgument("4090", "all").(string)
			switch target {
			case "4090":
				irc.Send("🔄 Clearing 4090 queue...")
				q.ClearQueue(meta.GPU4090)
				irc.Send("✅ 4090 queue cleared")
			case "2070":
				irc.Send("🔄 Clearing 2070 queue...")
				q.ClearQueue(meta.GPU2070)
				irc.Send("✅ 2070 queue cleared")
			case "all":
				irc.Send("🔄 Clearing all queues...")
				q.ClearAllQueues()
				irc.Send("✅ All queues cleared")
			default:
				irc.SendError("Invalid target. Use: 4090, 2070, or all")
			}
			return
		case "removecurrent":
			if irc.FindArgument("help", false).(bool) {
				irc.Send("Usage: !admin removecurrent - Remove the currently processing item from both queues")
				return
			}

			if q == nil {
				irc.SendError("Queue system not available")
				return
			}

			irc.Send("🔄 Removing current processing item...")
			if q.RemoveCurrentItem() {
				irc.Send("✅ Current processing item removed")
			} else {
				irc.Send("ℹ️ No items currently processing")
			}
			return

		}
	}
}
