package main

import (
	"log"
	"strings"

	"gopkg.in/irc.v3"
)

func chunkToIrc(c *irc.Client, m *irc.Message, message string) {
	var sendString string

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(message, "\n") {
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
	key := []byte(name + "_" + channel + "_nicks")

	// Get the meta data from the database
	nickList, err := birdBase.Get(key)
	if err != nil {
		log.Println(err)
		return false
	}

	sliceNickList := strings.Split(string(nickList), " ")
	whatModes := strings.Split(modes, "")

	for j := 0; j < len(sliceNickList); j++ {
		checkNick := cleanFromModes(sliceNickList[j])

		if checkNick == user {
			for k := 0; k < len(whatModes); k++ {
				if strings.Contains(sliceNickList[j], whatModes[k]) {
					return true
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

// This builds a temporary list of nicks in a channel
func cacheNicks(name string, m *irc.Message) {
	var key = []byte(name + "_" + m.Params[2] + "_temp_nick")

	// if birdbase key exists create new tempMeta with name and channel then store it
	if !birdBase.Has(key) {
		// store tempMeta in the database
		birdBase.Put(key, []byte(m.Trailing()))
		return
	}

	// if birdbase key exists get the meta data and append the new meta data to it
	if birdBase.Has(key) {
		nickList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		// store tempMeta in the database
		birdBase.Put(key, []byte(string(nickList)+" "+m.Trailing()))
		return
	}

}

// When the end of the nick list is returned we cache the final list and remove the temp
func saveNicks(name string, m *irc.Message) {
	var key = []byte(name + "_" + m.Params[1] + "_temp_nick")
	nickList, err := birdBase.Get(key)
	if err != nil {
		log.Println(err)
		return
	}

	// remove key from database if it exists
	if birdBase.Has(key) {
		birdBase.Delete(key)
	}

	key = []byte(name + "_" + m.Params[1] + "_nicks")
	birdBase.Put(key, nickList)
}

func cacheAutoLists(name string, m *irc.Message) {
	ident := m.Params[2]
	host := m.Params[3]
	nick := m.Params[5]
	status := m.Params[6]

	// if status contains +
	if strings.Contains(status, "+") {
		autoVoiceKey := []byte(name + "_" + m.Params[1] + "_autovoice")
		if birdBase.Has(autoVoiceKey) {
			autoVoiceList, err := birdBase.Get(autoVoiceKey)
			if err != nil {
				log.Println(err)
				return
			}

			birdBase.Put(autoVoiceKey, []byte(ident+" "+host+" "+nick+"#"+string(autoVoiceList)))
			return
		}

		birdBase.Put(autoVoiceKey, []byte(ident+" "+host+" "+nick))
		return
	}

	// if status contains @
	if strings.Contains(status, "@") {
		autoOpKey := []byte(name + "_" + m.Params[1] + "_autoop")
		if birdBase.Has(autoOpKey) {
			autoOpList, err := birdBase.Get(autoOpKey)
			if err != nil {
				log.Println(err)
				return
			}

			birdBase.Put(autoOpKey, []byte(ident+" "+host+" "+nick+"#"+string(autoOpList)))
			return
		}

		birdBase.Put(autoOpKey, []byte(ident+" "+host+" "+nick))
		return
	}
}

func canAuto(name string, m *irc.Message, what string) bool {
	key := []byte(name + "_" + m.Params[0] + "_auto" + what)

	if birdBase.Has(key) {
		// Get the meta data from the database
		nickList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return false
		}

		sliceNickList := strings.Split(string(nickList), "#")

		for j := 0; j < len(sliceNickList); j++ {
			nickDetails := strings.Split(sliceNickList[j], " ")
			if (nickDetails[0] == m.Prefix.User) && (nickDetails[1] == m.Prefix.Host) && (nickDetails[2] == m.Prefix.Name) {
				return true
			}
		}
	}

	return false
}
