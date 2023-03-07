package main

import (
	"context"
	"log"
	"math/rand"
	"strings"

	gogpt "github.com/sashabaranov/go-gpt3"
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

// Needs to be rewritten
// func isProtected(m *irc.Message) bool {
// 	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
// 		if strings.Contains(m.Prefix.Host, config.AiBird.ProtectedHosts[i].Host) {
// 			return true
// 		}
// 	}

// 	return false
// }

func shouldIgnore(nick string) bool {
	for i := 0; i < len(config.AiBird.IgnoreChatsFrom); i++ {
		if strings.ToLower(cleanFromModes(nick)) == strings.ToLower(config.AiBird.IgnoreChatsFrom[i]) {
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

// Needs to be rewritten
// func protectHosts(c *irc.Client, m *irc.Message) {
// 	switch m.Params[1] {
// 	case "+b":
// 		if isProtected(m) {
// 			c.Write("MODE " + m.Params[0] + " -b " + m.Trailing())

// 			if !isAdmin(m) {
// 				c.Write("MODE " + m.Params[0] + " +b *!*@" + m.Prefix.Host)
// 				c.Write("KICK " + m.Params[0] + " " + m.Prefix.Name + " :Don't mess with the birds!")
// 			}

// 			break
// 		}

// 	case "-o":
// 		if isProtected(m) {
// 			c.Write("MODE " + m.Params[0] + " +o " + m.Params[2])

// 			if !isAdmin(m) {
// 				c.Write("MODE " + m.Params[0] + " +b *!*@" + m.Prefix.Host)
// 				c.Write("KICK " + m.Params[0] + " " + m.Prefix.Name + " :Don't mess with the birds!")
// 			}

// 			break
// 		}
// 	}

// }

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
	channel := m.Params[1]
	user := m.Params[2]
	host := m.Params[3]
	nick := m.Params[5]
	status := m.Params[6]

	// If the user is not in the list and has +v
	if strings.Contains(status, "+") && !isInList(name, channel, "voice", user, host, nick) {
		log.Println("Adding " + nick + " to the auto voice list.")

		autoVoiceKey := []byte(name + "_" + m.Params[1] + "_autovoice")
		if birdBase.Has(autoVoiceKey) {
			autoVoiceList, err := birdBase.Get(autoVoiceKey)
			if err != nil {
				log.Println(err)
				return
			}

			birdBase.Put(autoVoiceKey, []byte(user+" "+host+" "+nick+"#"+string(autoVoiceList)))
			return
		}

		birdBase.Put(autoVoiceKey, []byte(user+" "+host+" "+nick))
		return
	}

	// If the user is not in the list and has +o
	if strings.Contains(status, "@") && !isInList(name, channel, "op", user, host, nick) {
		log.Println("Adding " + nick + " to the auto op list.")

		autoOpKey := []byte(name + "_" + m.Params[1] + "_autoop")
		if birdBase.Has(autoOpKey) {
			autoOpList, err := birdBase.Get(autoOpKey)
			if err != nil {
				log.Println(err)
				return
			}

			birdBase.Put(autoOpKey, []byte(user+" "+host+" "+nick+"#"+string(autoOpList)))
			return
		}

		birdBase.Put(autoOpKey, []byte(user+" "+host+" "+nick))
		return
	}

	// If the user is in the list and has -v
	if !strings.Contains(status, "+") && !strings.Contains(status, "@") && isInList(name, channel, "voice", user, host, nick) {
		log.Println("Removing " + nick + " from the auto voice list.")

		autoVoiceKey := []byte(name + "_" + m.Params[1] + "_autovoice")
		if birdBase.Has(autoVoiceKey) {
			autoVoiceList, err := birdBase.Get(autoVoiceKey)
			if err != nil {
				log.Println(err)
				return
			}

			sliceAutoVoiceList := strings.Split(string(autoVoiceList), "#")

			for j := 0; j < len(sliceAutoVoiceList); j++ {
				nickDetails := strings.Split(sliceAutoVoiceList[j], " ")
				if (nickDetails[0] == user) && (nickDetails[1] == host) && (nickDetails[2] == nick) {
					sliceAutoVoiceList = append(sliceAutoVoiceList[:j], sliceAutoVoiceList[j+1:]...)
					break
				}
			}

			birdBase.Put(autoVoiceKey, []byte(strings.Join(sliceAutoVoiceList, "#")))
			return
		}
	}

	// If the user is in the list and has -o
	if !strings.Contains(status, "+") && !strings.Contains(status, "@") && isInList(name, channel, "op", user, host, nick) {
		log.Println("Removing " + nick + " from the auto op list.")

		autoOpKey := []byte(name + "_" + m.Params[1] + "_autoop")
		if birdBase.Has(autoOpKey) {
			autoOpList, err := birdBase.Get(autoOpKey)
			if err != nil {
				log.Println(err)
				return
			}

			sliceAutoOpList := strings.Split(string(autoOpList), "#")

			for j := 0; j < len(sliceAutoOpList); j++ {
				nickDetails := strings.Split(sliceAutoOpList[j], " ")
				if (nickDetails[0] == user) && (nickDetails[1] == host) && (nickDetails[2] == nick) {
					sliceAutoOpList = append(sliceAutoOpList[:j], sliceAutoOpList[j+1:]...)
					break
				}
			}

			birdBase.Put(autoOpKey, []byte(strings.Join(sliceAutoOpList, "#")))
			return
		}
	}
}

// This one doesn't rely on m.Params which can change depending what event has occurred.
func isInList(name string, channel string, what string, user string, host string, nick string) bool {
	key := []byte(name + "_" + channel + "_auto" + what)

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
			if (nickDetails[0] == user) && (nickDetails[1] == host) && (nickDetails[2] == nick) {
				return true
			}
		}
	}

	return false
}

func cacheChatsForReply(name string, message string, m *irc.Message, c *irc.Client, aiClient *gogpt.Client, ctx context.Context) {
	// Get the meta data from the database

	// if the string contains \x03 then return
	if strings.Contains(message, "\x03") {
		return
	}

	key := []byte(name + "_" + m.Params[0] + "_chats_cache")
	message = m.Prefix.Name + ": " + message

	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		birdBase.Put(key, []byte(message+"."+"\n"+string(chatList)))

		sliceChatList := strings.Split(message+"\n"+string(chatList), "\n")
		if len(sliceChatList) >= config.AiBird.ReplyTotalMessages {
			birdBase.Delete(key)

			// Send the message to the AI, with a 1 in 3 chance
			if rand.Intn(config.AiBird.ReplyChance) == 0 {
				replyToChats(m, message+"\n"+string(chatList), c, aiClient, ctx)
			}
		}

		return
	}

	birdBase.Put(key, []byte(message+"."))
}

// Maybe can move this into openai.go
func cacheChatsForChatGpt(name string, message string, m *irc.Message, c *irc.Client, aiClient *gogpt.Client, ctx context.Context) {
	if strings.Contains(message, "\x03") {
		return
	}

	key := []byte(name + "_" + m.Params[0] + "_chats_cache_gpt")

	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		birdBase.Put(key, []byte(message+"."+"\n"+string(chatList)))

		sliceChatList := strings.Split(message+"\n"+string(chatList), "\n")

		// reverse sliceChatList, seriously golang?
		for i := len(sliceChatList)/2 - 1; i >= 0; i-- {
			opp := len(sliceChatList) - 1 - i
			sliceChatList[i], sliceChatList[opp] = sliceChatList[opp], sliceChatList[i]
		}

		if len(sliceChatList) >= config.AiBird.ChatGptTotalMessages {
			sliceChatList = sliceChatList[:config.AiBird.ChatGptTotalMessages]
		}

		gpt3Chat := []gogpt.ChatCompletionMessage{}

		// Send the message to the AI, with a 1 in 3 chance
		gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
			Role:    "assistant",
			Content: "You are an " + config.AiBird.ChatPersonality + ". And must reply to the following messages:",
		})

		for i := 0; i < len(sliceChatList); i++ {
			// if sliceChatList[i] starts with ASSISTANT:
			if strings.HasPrefix(sliceChatList[i], "ASSISTANT: ") {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "assistant",
					Content: strings.TrimPrefix(sliceChatList[i], "ASSISTANT: "),
				})
			} else {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "user",
					Content: sliceChatList[i],
				})
			}
		}

		// reverse sliceChatList, seriously golang?
		for i := len(sliceChatList)/2 - 1; i >= 0; i-- {
			opp := len(sliceChatList) - 1 - i
			sliceChatList[i], sliceChatList[opp] = sliceChatList[opp], sliceChatList[i]
		}

		birdBase.Put(key, []byte(strings.Join(sliceChatList, "\n")))

		chatGpt(name, m, gpt3Chat, c, aiClient, ctx)
		return
	}

	birdBase.Put(key, []byte(message+"."))
}
