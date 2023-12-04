package main

import (
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/yunginnanet/girc-atomic"
)

// Basic markdown to IRC
func markdownToIrc(message string) string {
	markdownQuote := false

	markdownFormat := ""
	for _, line := range strings.Split(message, "\n") {
		if strings.Contains(line, "```") {
			markdownQuote = !markdownQuote
			line = strings.Replace(line, "```", "\n", -1)
		}

		if markdownQuote {
			// Make any quote text green
			line = "03" + line
		} else {
			// if message starts with "> "
			if strings.HasPrefix(line, "> ") {
				line = strings.Replace(line, "> ", "03> ", -1)
			}

			// add irc italics to replace markdown, but only replace "*" and not "* "
			if strings.HasPrefix(strings.TrimSpace(line), "* ") {
				line = strings.Replace(line, "* ", " - ", -1)
			}

			// add irc bold to replace markdown
			if strings.Count(line, "**")%2 == 0 {
				line = strings.Replace(line, "**", "\x02", -1)
			}

			// if line has more than one *
			if strings.Count(line, "*")%2 == 0 {
				line = strings.Replace(line, "*", "\x1D", -1)
			}

			// underline
			if strings.Count(line, "__")%2 == 0 {
				line = strings.Replace(line, "__", "\x1F", -1)
			}

			// strikethrough
			if strings.Count(line, "~~")%2 == 0 {
				line = strings.Replace(line, "~~", "\x1E", -1)
			}
		}

		markdownFormat = markdownFormat + "\n" + line
	}

	return markdownFormat
}

func sendToIrc(c *girc.Client, e girc.Event, message string) {
	// Convert any markdown to IRC friendly format
	message = markdownToIrc(message)

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(message, "\n") {
		sendString := ""
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
			if len(sendString) > 450 {

				_ = c.Cmd.Reply(e, sendString)
				sendString = ""
			}
		}

		_ = c.Cmd.Reply(e, sendString)
	}

}

func isAdmin(e girc.Event) bool {
	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
		if config.AiBird.ProtectedHosts[i].Host == e.Source.Host &&
			config.AiBird.ProtectedHosts[i].Ident == cleanFromModes(e.Source.Ident) &&
			config.AiBird.ProtectedHosts[i].Admin {
			return true
		}
	}

	return false
}

func shouldIgnore(nick string) bool {
	for i := 0; i < len(config.AiBird.IgnoreChatsFrom); i++ {
		if strings.EqualFold(strings.ToLower(cleanFromModes(nick)), strings.ToLower(config.AiBird.IgnoreChatsFrom[i])) {
			return true
		}
	}

	return false
}

func isUserMode(name string, channel string, user string, modes string) bool {
	key := []byte(name + "_" + channel + "_nicks")

	// Get the meta data from the database
	if birdBase.Has(key) {
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
	}

	return false
}

// Protects hosts in config.toml from deop
func protectHosts(c *girc.Client, e girc.Event, name string) {
	// We don't care if a protected user bans or -o people
	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
		if config.AiBird.ProtectedHosts[i].Host == e.Source.Host && config.AiBird.ProtectedHosts[i].Ident == e.Source.Ident {
			return
		}
	}

	// Deop on protected user protection
	if e.Command == "MODE" && e.Params[1] == "-o" {
		user := c.LookupUser(e.Params[2])

		if user != nil {
			for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
				if config.AiBird.ProtectedHosts[i].Host == user.Host {
					// set mode +o to protected user
					c.Cmd.Mode(e.Params[0], "+o", user.Nick.String())

					// Kick and ban the user
					c.Cmd.Ban(e.Params[0], e.Source.Host)
					c.Cmd.Kick(e.Params[0], e.Source.Name, "Birds fly above deop!")
				}
			}
		}
	}

	// Protect from ban, this needs more testing and should not be relied on
	if e.Command == "MODE" && e.Params[1] == "+b" {
		banHost := strings.Split(e.Params[2], "@")[1]

		// Get the nick and ident
		banIdentNick := strings.Split(e.Params[2], "@")[0]
		banNick := strings.Split(banIdentNick, "!")[0]
		banIdent := strings.Split(banIdentNick, "!")[1]

		if banHost == "*" && banIdent == "*" && banNick == "*" {
			c.Cmd.Mode(e.Params[0], "-b", e.Params[2])

			// Kick and ban the user
			c.Cmd.Ban(e.Params[0], e.Source.Host)
			c.Cmd.Kick(e.Params[0], e.Source.Name, "Birds fly above ban everything!")
			return
		}

		for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
			log.Println(banHost, banIdent, banNick)

			if strings.Contains(strings.ToLower(config.AiBird.ProtectedHosts[i].Host), strings.ToLower(banHost)) ||
				strings.Contains(strings.ToLower(config.AiBird.ProtectedHosts[i].Ident), strings.ToLower(banIdent)) {
				// set mode -b to protected user
				c.Cmd.Mode(e.Params[0], "-b", e.Params[2])

				// Kick and ban the user
				c.Cmd.Ban(e.Params[0], e.Source.Host)
				c.Cmd.Kick(e.Params[0], e.Source.Name, "Birds fly above ban!")
			}
		}
	}
}

// This builds a temporary list of nicks in a channel
func cacheNicks(e girc.Event, name string) {
	var key = []byte(name + "_" + e.Params[2] + "_temp_nick")
	nicks := strings.Split(e.Last(), " ")
	tempNickList := ""

	for i := 0; i < len(nicks); i++ {
		tempNickList = strings.Trim(tempNickList+" "+strings.Split(nicks[i], "!")[0], " ")
	}

	// if birdbase key exists create new tempMeta with name and channel then store it
	if !birdBase.Has(key) {
		birdBase.Put(key, []byte(tempNickList))
		return
	} else {
		nickList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		// store tempMeta in the database
		birdBase.Put(key, []byte(string(nickList)+" "+tempNickList))
		return
	}

}

// When the end of the nick list is returned we cache the final list and remove the temp
func saveNicks(e girc.Event, name string) {
	channel := e.Params[1]
	var key = []byte(name + "_" + channel + "_temp_nick")

	nickList, err := birdBase.Get(key)
	if err != nil {
		log.Println(err)
		return
	}

	// remove key from database if it exists
	if birdBase.Has(key) {
		birdBase.Delete(key)
	}

	key = []byte(name + "_" + channel + "_nicks")
	birdBase.PutWithTTL(key, nickList, time.Hour*24*180)
}

// Remember +o and +v for when users join next time
func cacheAutoLists(e girc.Event, name string) {
	channel := e.Params[1]
	user := e.Params[2]
	host := e.Params[3]
	status := e.Params[6]

	// If the user is not in the list and has +v
	autoVoiceKey := cacheKey(name+channel+user+host, "v")
	if strings.Contains(status, "+") && !birdBase.Has(autoVoiceKey) {
		birdBase.PutWithTTL(autoVoiceKey, []byte(""), time.Hour*24*180)
	} else if !strings.Contains(status, "+") {
		birdBase.Delete(autoVoiceKey)
	}

	// If the user is not in the list and has +o
	autoOpKey := cacheKey(name+channel+user+host, "o")
	if strings.Contains(status, "@") && !birdBase.Has(autoOpKey) {
		birdBase.PutWithTTL(autoOpKey, []byte(""), time.Hour*24*180)
	} else if !strings.Contains(status, "@") {
		birdBase.Delete(autoOpKey)
	}
}

// Spam protection
func floodCheck(c *girc.Client, e girc.Event, name string) bool {
	ban := cacheKey(name+e.Params[0]+e.Source.Host+e.Source.Ident, "b")

	if birdBase.Has(ban) {
		return true
	}

	key := cacheKey(name+e.Params[0]+e.Source.Host+e.Source.Ident, "f")

	waitTime := 3 * time.Second

	if !birdBase.Has(key) {
		birdBase.PutWithTTL(key, []byte("1"), waitTime)
	} else {
		count, _ := birdBase.Get(key)
		countInt, _ := strconv.Atoi(string(count))
		countInt++
		birdBase.PutWithTTL(key, []byte(strconv.Itoa(countInt)), waitTime)

		if countInt > config.AiBird.FloodThresholdMessages {
			birdBase.PutWithTTL(ban, []byte("1"), config.AiBird.FloodIgnoreTime*time.Minute)
			c.Cmd.Kick(e.Params[0], e.Source.Name, "Birds fly above floods!")
		}

		return true
	}

	return false
}

// Join flood check, if there are a lot of clients that join then we auto +i the channel
func joinFloodCheck(c *girc.Client, e girc.Event, name string) {
	key := cacheKey(e.Params[0]+name, "i")
	waitTime := 3 * time.Second

	// If we have a lot of people rejoin on a netsplit we don't want to trigger this
	// we can see if the bot already reconises them
	if c.LookupUser(e.Source.Name) != nil {
		return
	}

	if !birdBase.Has(key) {
		birdBase.PutWithTTL(key, []byte("1"), waitTime)
	} else {
		count, _ := birdBase.Get(key)
		countInt, _ := strconv.Atoi(string(count))
		countInt++
		birdBase.PutWithTTL(key, []byte(strconv.Itoa(countInt)), waitTime)

		if countInt > 4 {
			go removeFloodCheck(c, e, name)
			// +i the channel
			c.Cmd.Mode(e.Params[0], "+i")
			c.Cmd.Mode(e.Params[0], "+m")
		}

	}

}

// After two minutes remove +i and +m
func removeFloodCheck(c *girc.Client, e girc.Event, name string) {
	time.Sleep(2 * time.Minute)

	key := cacheKey(e.Params[0]+name, "i")
	birdBase.Delete(key)

	c.Cmd.Mode(e.Params[0], "-i")
	c.Cmd.Mode(e.Params[0], "-m")
}
