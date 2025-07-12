package commands

import (
	"aibird/helpers"
	"aibird/irc/state"
)

func ParseOwner(irc state.State) {
	if irc.User.IsOwner {
		switch irc.Command.Action {
		case "save":
			irc.Network.Save()
			irc.ReplyTo("Saved databases")
		case "ip":
			ip, _ := helpers.GetIp()
			irc.ReplyTo(ip)
		case "raw":
			_ = irc.Client.Cmd.SendRaw(irc.Command.Message)
		}
	}
}
