package main

import (
	"strings"

	"gopkg.in/irc.v3"
)

func chunkToIrc(c *irc.Client, m *irc.Message, responseString string) {
	var sendString string

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(responseString, "\n") {
		sendString = ""

		// Remove blank or one/two char lines
		if len(line) <= 2 {
			continue
		}

		// split line into chunks slice with space
		chunks := strings.Split(line, " ")

		// for each chunk
		for _, chunk := range chunks {
			// append chunk to sendString
			sendString += chunk + " "

			// Trim by words for a cleaner output
			if len(sendString) > 380 {
				// write message to channel
				c.WriteMessage(&irc.Message{
					Command: "PRIVMSG",
					Params: []string{
						m.Params[0],
						sendString,
					},
				})
				sendString = ""
			}
		}

		// Write the final message
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				sendString,
			},
		})
	}
}

func cleanFromModes(nick string) string {
	nick = strings.ReplaceAll(nick, "@", "")
	nick = strings.ReplaceAll(nick, "+", "")
	nick = strings.ReplaceAll(nick, "~", "")
	nick = strings.ReplaceAll(nick, "&", "")
	nick = strings.ReplaceAll(nick, "%", "")
	nick = strings.ReplaceAll(nick, "-", "")
	return nick
}

func isAdmin(m *irc.Message) bool {
	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
		if strings.Contains(m.Prefix.Host, config.AiBird.ProtectedHosts[i].Host) && config.AiBird.ProtectedHosts[i].Admin {
			return true
		}
	}

	return false
}

func isProtected(m *irc.Message) bool {
	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
		if strings.Contains(m.Prefix.Host, config.AiBird.ProtectedHosts[i].Host) {
			return true
		}
	}

	return false
}

func isUserMode(name string, channel string, user string, modes string) bool {
	var whatModes []string
	var checkNick string

	for i := 0; i < len(metaList.ircMeta); i++ {
		if metaList.ircMeta[i].Network != name {
			continue
		}

		if metaList.ircMeta[i].Channel == channel {
			tempNickList := strings.Split(metaList.ircMeta[i].Nicks, " ")
			whatModes = strings.Split(modes, "")
			for j := 0; j < len(tempNickList); j++ {
				checkNick = cleanFromModes(tempNickList[j])

				if checkNick == user {
					for k := 0; k < len(whatModes); k++ {
						if strings.Contains(tempNickList[j], whatModes[k]) {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

func protectHosts(c *irc.Client, m *irc.Message) {
	switch m.Params[1] {
	case "+b":
		if isProtected(m) {
			c.Write("MODE " + m.Params[0] + " -b " + m.Trailing())

			if !isAdmin(m) {
				c.Write("MODE " + m.Params[0] + " +b *!*@" + m.Prefix.Host)
				c.Write("KICK " + m.Params[0] + " " + m.Prefix.Name + " :Don't mess with the birds!")
			}

			break
		}

	case "-o":
		if isProtected(m) {
			c.Write("MODE " + m.Params[0] + " +o " + m.Params[2])

			if !isAdmin(m) {
				c.Write("MODE " + m.Params[0] + " +b *!*@" + m.Prefix.Host)
				c.Write("KICK " + m.Params[0] + " " + m.Prefix.Name + " :Don't mess with the birds!")
			}

			break
		}
	}

}
