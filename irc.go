package main

import (
	"encoding/hex"
	"log"
	"math/rand"
	"strings"
	"time"

	gogpt "github.com/sashabaranov/go-openai"
	"github.com/yunginnanet/girc-atomic"
	"golang.org/x/crypto/sha3"
)

func chunkToIrc(c *girc.Client, e girc.Event, message string) {
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
			if len(sendString) > 450 {
				// write message to channel
				time.Sleep(550 * time.Millisecond)
				_ = c.Cmd.Reply(e, sendString)
				sendString = ""
			}
		}

		// Write the final message
		time.Sleep(550 * time.Millisecond)
		_ = c.Cmd.Reply(e, sendString)
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

// Needs to be rewritten
// func isProtected(e *irc.Message) bool {
// 	for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
// 		if strings.Contains(e.Prefix.Host, config.AiBird.ProtectedHosts[i].Host) {
// 			return true
// 		}
// 	}

// 	return false
// }

func shouldIgnore(nick string) bool {
	for i := 0; i < len(config.AiBird.IgnoreChatsFrom); i++ {
		// strings equalFOld
		if strings.EqualFold(strings.ToLower(cleanFromModes(nick)), strings.ToLower(config.AiBird.IgnoreChatsFrom[i])) {
			// if strings.ToLower(cleanFromModes(nick)) == strings.ToLower(config.AiBird.IgnoreChatsFrom[i]) {
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

// Needs to be rewritten
// func protectHosts(c *irc.Client, e *irc.Message) {
// 	switch e.Params[1] {
// 	case "+b":
// 		if isProtected(e) {
// 			c.Write("MODE " + e.Params[0] + " -b " + e.Trailing())

// 			if !isAdmin(e) {
// 				c.Write("MODE " + e.Params[0] + " +b *!*@" + e.Prefix.Host)
// 				c.Write("KICK " + e.Params[0] + " " + e.Prefix.Name + " :Don't mess with the birds!")
// 			}

// 			break
// 		}

// 	case "-o":
// 		if isProtected(e) {
// 			c.Write("MODE " + e.Params[0] + " +o " + e.Params[2])

// 			if !isAdmin(e) {
// 				c.Write("MODE " + e.Params[0] + " +b *!*@" + e.Prefix.Host)
// 				c.Write("KICK " + e.Params[0] + " " + e.Prefix.Name + " :Don't mess with the birds!")
// 			}

// 			break
// 		}
// 	}

// }

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
	birdBase.Put(key, nickList)
}

func cacheAutoLists(e girc.Event, name string) {
	channel := e.Params[1]
	user := e.Params[2]
	host := e.Params[3]
	status := e.Params[6]

	hash := sha3.Sum224([]byte(name + channel + user + host))
	hashString := hex.EncodeToString(hash[:])

	// If the user is not in the list and has +v
	autoVoiceKey := []byte("v" + hashString)
	if strings.Contains(status, "+") && !birdBase.Has(autoVoiceKey) {
		birdBase.Put(autoVoiceKey, []byte(""))
	} else if !strings.Contains(status, "+") {
		birdBase.Delete(autoVoiceKey)
	}

	// If the user is not in the list and has +o
	autoOpKey := []byte("o" + hashString)
	if strings.Contains(status, "@") && !birdBase.Has(autoOpKey) {
		birdBase.Put(autoOpKey, []byte(""))
	} else if !strings.Contains(status, "@") {
		birdBase.Delete(autoOpKey)
	}

}

// This one doesn't rely on e.Params which can change depending on what event has occurred.
func isInList(name string, channel string, what string, user string, host string) bool {
	hash := sha3.Sum224([]byte(name + channel + user + host))
	hashString := hex.EncodeToString(hash[:])

	key := []byte(what + hashString)

	return birdBase.Has(key)
}

// Maybe can move this into openai.go
func cacheChatsForReply(c *girc.Client, e girc.Event, name string, message string) {
	// Get the meta data from the database

	// check if message contains unicode
	if !strings.ContainsAny(message, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789") {
		return
	}

	key := []byte(name + "_" + e.Params[0] + "_chats_cache")
	message = e.Source.Name + ": " + message

	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		birdBase.Put(key, []byte(message+"."+"\n"+string(chatList)))

		sliceChatList := strings.Split(message+"\n"+string(chatList), "\n")
		if len(sliceChatList) > 5 {
			birdBase.Delete(key)

			// Send the message to the AI, with a 1 in 3 chance
			if rand.Intn(3) == 0 {
				replyToChats(c, e, message+"\n"+string(chatList))
			}
		}

		return
	}

	birdBase.Put(key, []byte(message+"."))
}

func cacheChatsForChatGtp(c *girc.Client, e girc.Event, name string) {
	// Ignore ASCII color codes
	if strings.Contains(e.Last(), "\x03") {
		return
	}

	key := []byte(name + "_" + e.Params[0] + "_chats_cache_gpt_" + e.Source.Name)

	if e.Last() == "!forget" {
		birdBase.Delete(key)
		chunkToIrc(c, e, "Okay starting fresh.")
		return
	}

	if e.Last() == "!context" {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		chunkToIrc(c, e, string(chatList))
		return
	}

	if !birdBase.Has(key) {
		chunkToIrc(c, e, "Type !forget to start fresh.")

		// make new empty key
		birdBase.Put(key, []byte(""))
	}

	if birdBase.Has(key) {
		chatList, err := birdBase.Get(key)
		if err != nil {
			log.Println(err)
			return
		}

		latestChat := string(chatList) + "\n" + e.Last()
		sliceChatList := strings.Split(latestChat, "\n")

		if len(sliceChatList)-1 >= config.AiBird.ChatGptTotalMessages {
			sliceChatList = sliceChatList[1:]
		}

		gpt3Chat := []gogpt.ChatCompletionMessage{}

		gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
			Role:    "system",
			Content: "You are an " + config.AiBird.ChatPersonality + " and must reply to the following chats:",
		})

		for i := 0; i < len(sliceChatList); i++ {
			// if chat is empty, skip
			if sliceChatList[i] == "" {
				continue
			}

			// if sliceChatList starts with "AIBIRD :" then
			if strings.HasPrefix(sliceChatList[i], "AI: ") {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "assistant",
					Content: strings.Split(sliceChatList[i], "AI: ")[1],
				})
			} else {
				gpt3Chat = append(gpt3Chat, gogpt.ChatCompletionMessage{
					Role:    "user",
					Content: sliceChatList[i],
				})
			}

		}

		birdBase.Put(key, []byte(strings.Join(sliceChatList, "\n")))

		chatGptContext(c, e, name, gpt3Chat)

		return
	}
}
