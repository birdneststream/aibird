package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"git.mills.io/prologic/bitcask"
	"github.com/BurntSushi/toml"
	"github.com/dustin/go-humanize"
	gogpt "github.com/sashabaranov/go-openai"
	"github.com/yunginnanet/girc-atomic"
)

var config Config

func loadConfig() {
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		fmt.Println("Error in config.toml")
		fmt.Println(err)
		os.Exit(1)
	}
}

var birdBase *bitcask.Bitcask

func mergeKeys() {
	birdBase.Merge()
	time.Sleep(6 * time.Hour)
	go mergeKeys()
}

func main() {
	// Load json config
	loadConfig()

	var err error
	birdBase, err = bitcask.Open("bird.db")
	if err != nil {
		log.Fatal(err)
	}

	log.Println("AI bot connecting to IRC, please wait")

	var waitGroup sync.WaitGroup
	for name, network := range config.Networks {
		if len(network.Servers) == 0 {
			log.Printf("networks.%s has no servers defined, skipping", name)
		} else if network.Enabled {
			log.Printf("networks.%s connecting...", name)
			waitGroup.Add(1)
			go ircClient(network, name, &waitGroup)
		} else {
			log.Printf("networks.%s is disabled, skipping", name)
		}
	}

	go mergeKeys()

	waitGroup.Wait()

	log.Println("all done bye")

	//exit
	os.Exit(0)
}

var whatKey string             // keeps track of the current ai key to alert expired
var ctx = context.Background() // Initialise the openAI api client

func ircClient(network Network, name string, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()

	// Choose a random IRC server to connect to within the network
	var ircServer Server = network.returnRandomServer()

	ircConfig := girc.Config{
		Server:    ircServer.Host,
		Port:      ircServer.Port,
		SSL:       ircServer.Ssl,
		PingDelay: 60,
		Nick:      network.Nick,
		User:      network.Nick,
		Name:      network.Nick,
		Version:   "VERSION AiBird Bot - https://github.com/birdneststream/aibird",
	}

	if ircServer.Ssl {
		ircConfig.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if network.Pass != "" {
		ircConfig.ServerPass = network.Pass
	}

	if network.Throttle == 0 {
		ircConfig.AllowFlood = true
	}

	client := girc.New(ircConfig)

	if config.AiBird.Debug {
		client.Handlers.Add(girc.ALL_EVENTS, func(c *girc.Client, e girc.Event) {
			log.Println(e.String())
		})
	} else if config.AiBird.Showchat {
		client.Handlers.Add(girc.ALL_EVENTS, func(c *girc.Client, e girc.Event) {
			if msg, ok := e.Pretty(); ok {
				log.Println(msg)
			}
		})
	}
	client.Handlers.Add(girc.ERROR, func(c *girc.Client, e girc.Event) {
		log.Println("Server error: " + e.String())
	})
	client.Handlers.Add(girc.RPL_WELCOME, func(c *girc.Client, e girc.Event) {
		if network.NickServPass != "" {
			_ = c.Cmd.SendRaw("PRIVMSG NickServ :IDENTIFY " + network.Nick + " " + network.NickServPass)
		}

		for _, channel := range network.Channels {
			c.Cmd.Join(channel)
		}
	})
	client.Handlers.Add(girc.RPL_NAMREPLY, func(c *girc.Client, e girc.Event) {
		cacheNicks(e, name)
	})
	client.Handlers.Add(girc.RPL_ENDOFNAMES, func(c *girc.Client, e girc.Event) {
		saveNicks(e, name)
	})
	client.Handlers.Add(girc.RPL_WHOREPLY, func(c *girc.Client, e girc.Event) {
		cacheAutoLists(e, name)
	})
	client.Handlers.Add(girc.NICK, func(c *girc.Client, e girc.Event) {
		if e.Source.Name == network.Nick {
			network.Nick = e.Last()
			return
		}

		_ = c.Cmd.SendRaw("NAMES " + e.Params[0])
	})
	client.Handlers.Add(girc.JOIN, func(c *girc.Client, e girc.Event) {
		if isUserMode(name, e.Params[0], network.Nick, "@") {
			// Auto Op
			if isInList(name, e.Params[0], "o", e.Source.Ident, e.Source.Host) {
				c.Cmd.Mode(e.Params[0], "+o", e.Source.Name)
				return
			}

			// Auto Voice
			if isInList(name, e.Params[0], "v", e.Source.Ident, e.Source.Host) {
				c.Cmd.Mode(e.Params[0], "+v", e.Source.Name)
				return
			}

		}

		joinFloodCheck(c, e, name)
	})
	client.Handlers.Add(girc.MODE, func(c *girc.Client, e girc.Event) {
		// If there is a +b on a protected host, remove it.
		// This is not so secure at the moment.
		go protectHosts(c, e, name)

		// Cache the names list so only +v users at least can use the bot
		// _ = c.Cmd.SendRaw("NAMES " + e.Params[0])

		// Only want to cache this if we have ops
		if isUserMode(name, e.Params[0], network.Nick, "@") {
			_ = c.Cmd.SendRaw("WHO " + e.Params[0])
			return
		}

		// if the mode is +o and this bot
		if e.Params[1] == "+o" && !isUserMode(name, e.Params[0], network.Nick, "@") {
			// Cache the names list
			_ = c.Cmd.SendRaw("WHO " + e.Params[0])
		}

	})
	client.Handlers.Add(girc.KICK, func(c *girc.Client, e girc.Event) {
		if e.Params[1] == c.GetNick() {
			c.Cmd.Join(e.Params[0])
		}
	})
	client.Handlers.Add(girc.PRIVMSG, func(c *girc.Client, e girc.Event) {
		// Ignore own nick and other nicks defined in config.toml
		if shouldIgnore(e.Source.Name) || e.Source.Name == network.Nick {
			return
		}

		if !e.IsFromChannel() {
			// ChatGPT in PM
			if floodCheck(c, e, name) {
				return
			}

			cacheChatsForChatGtp(c, e, name)
			return
		}

		if !isUserMode(name, e.Params[0], e.Source.Name, "~&@%+") {
			return
		}

		if config.AiBird.ReplyToChats {
			if strings.HasPrefix(e.Last(), network.Nick) {
				cacheChatsForChatGtp(c, e, name)
				return
			} else if e.Params[0] == "#birdnest" && !strings.HasPrefix(e.Last(), "!") {
				go cacheChatsForReply(c, e, name, e.Last())
			}
		}

		msg := e.Last()

		// don't do anything if message isn't prefixed w/ an exclamation
		if !strings.HasPrefix(msg, "!") {
			return
		}

		// split command & prompt
		parts := strings.SplitN(msg, " ", 2)

		// require both command & prompt
		if len(parts) < 2 {
			return
		}

		if floodCheck(c, e, name) {
			return
		}

		cmd := parts[0]
		message := strings.TrimSpace(parts[1])

		// Model we will use
		var model string

		switch cmd {

		case "!admin":
			// if the host is not in config.AiBird.Admin.Host then +b the host
			if isAdmin(e) {
				log.Println("Admin command from " + e.Source.Name + " on " + e.Params[0] + ": " + message)
				parts := strings.SplitN(message, " ", 2)
				switch parts[0] {
				case "reload":
					_, err := toml.DecodeFile("config.toml", &config)
					if err != nil {
						sendToIrc(c, e, err.Error())
					}
					sendToIrc(c, e, "Reloaded config!")
					return

				case "raw":
					// remove raw from message and trim
					message = strings.TrimSpace(strings.TrimPrefix(message, "raw "))
					event := girc.ParseEvent(message)
					if event == nil {
						sendToIrc(c, e, "Raw string was not valid IRC")
						return
					}
					c.Send(event)
					return

				case "sd":
					sdAdmin(c, e, message)
					return

				case "personality":
					message = strings.TrimSpace(strings.TrimPrefix(message, "personality"))
					config.AiBird.ChatPersonality = message
					sendToIrc(c, e, "Set aibird personality to "+message)
					return

				case "birdbase":
					message = strings.TrimSpace(strings.TrimPrefix(message, "birdbase"))

					switch message {
					case "nicks":
						key := []byte(name + "_" + e.Params[0] + "_nicks")
						nickList, err := birdBase.Get(key)
						if err != nil {
							log.Println(err)
							return
						}

						sendToIrc(c, e, string(nickList))
						return

					case "merge":
						birdBase.Merge()

						sendToIrc(c, e, "We Mergin'")
						return

					case "stats":
						stats, err := birdBase.Stats()
						if err != nil {
							log.Println(err)
							sendToIrc(c, e, err.Error())
							return
						}

						sendToIrc(c, e, fmt.Sprintf("%+v", stats))
						return

					case "deleteall":
						err := birdBase.DeleteAll()
						if err != nil {
							log.Println(err)
							sendToIrc(c, e, err.Error())
							return
						}

						sendToIrc(c, e, "Removed all birdBase keys.")
						return
					}

				}

				return
			}

		case "!seen":
			if c.LookupUser(message) != nil {
				lastActive := c.LookupUser(message).LastActive
				currentTime := time.Now()

				// get difference in human readable format, e.g 1 Hour Ago
				difference := currentTime.Sub(lastActive)
				humanReadable := humanize.Time(time.Now().Add(-difference))

				sendToIrc(c, e, message+" was last active "+humanReadable+".")
			} else {
				sendToIrc(c, e, message+" has not been seen before.")
			}
			return

		case "!ping":
			sendToIrc(c, e, "Pong!")
			return

		// Dall-e 3 Commands
		case "!dale":
			quality := gogpt.CreateImageQualityStandard
			style := gogpt.CreateImageStyleNatural
			size := gogpt.CreateImageSize1024x1024
			model := gogpt.CreateImageModelDallE3

			// if string has -hd remove it
			if strings.Contains(message, "--hd") {
				message = strings.Replace(message, "--hd", "", -1)
				quality = gogpt.CreateImageQualityHD
			}

			// if string has -vivid remove it
			if strings.Contains(message, "--vivid") {
				message = strings.Replace(message, "--vivid", "", -1)
				style = gogpt.CreateImageStyleVivid
			}

			// if string has -1792x1024 remove it
			if strings.Contains(message, "--1792x1024") {
				message = strings.Replace(message, "--1792x1024", "", -1)
				size = gogpt.CreateImageSize1792x1024
			}

			// if string has 1024x1792 remove it
			if strings.Contains(message, "--1024x1792") {
				message = strings.Replace(message, "--1024x1792", "", -1)
				size = gogpt.CreateImageSize1024x1792
			}

			// if string has 1024x1024 remove it
			if strings.Contains(message, "--1024") {
				message = strings.Replace(message, "--1024", "", -1)
				size = gogpt.CreateImageSize1024x1024
			}

			// if string has 1024x1024 remove it
			if strings.Contains(message, "--512") {
				message = strings.Replace(message, "--512", "", -1)
				size = gogpt.CreateImageSize512x512
			}

			// if string has 1024x1024 remove it
			if strings.Contains(message, "--256") {
				message = strings.Replace(message, "--256", "", -1)
				size = gogpt.CreateImageSize256x256
			}

			// if string has -2 remove it
			if strings.Contains(message, "--2") {
				if size == gogpt.CreateImageSize1792x1024 || size == gogpt.CreateImageSize1024x1792 {
					sendToIrc(c, e, "You can't use Dall-e 2 with 1792x1024 or 1024x1792")
					return
				}

				message = strings.Replace(message, "--2", "", -1)
				model = gogpt.CreateImageModelDallE2
			}

			if model == gogpt.CreateImageModelDallE3 && (size == gogpt.CreateImageSize512x512 || size == gogpt.CreateImageSize256x256) {
				sendToIrc(c, e, "You can't use Dall-e 3 with 512x512 or 256x256")
				return
			}

			message = strings.Trim(message, " ")
			dalle(c, e, message, size, model, quality, style)
			return

		// Custom prompt to make better mirc art
		case "!aiscii":
			aiscii(c, e, message)
			return
		case "!birdmap":
			birdmap(c, e, message)
			return
		// Stable diffusion prompts
		case "!sd":
			// if !isUserMode(name, e.Params[0], e.Source.Name, "@~") {
			// 	sendToIrc(c, e, "Hey there chat pal "+e.Source.Name+", you have to be a birdnest patreon to use stable diffusion! Unless you want to donate your own GPU!")
			// 	return
			// }

			// if e.Params[0] != "#birdnest" {
			// 	sendToIrc(c, e, "Hey there chat pal "+e.Source.Name+", stable diffusion is only available in #birdnest!")
			// 	return
			// }

			sdRequest(c, e, message)
			return
		// The models for completion prompts
		case "!gpt3.5":
			chatGptContext := []gogpt.ChatCompletionMessage{}
			chatGptContext = append(chatGptContext, gogpt.ChatCompletionMessage{
				Role:    "user",
				Content: message,
			})
			conversation(c, e, gogpt.GPT3Dot5Turbo, chatGptContext)
			return
		case "!bard":
			bard(c, e, message)
			return
		case "!gpt4":
			chatGptContext := []gogpt.ChatCompletionMessage{}
			chatGptContext = append(chatGptContext, gogpt.ChatCompletionMessage{
				Role:    "user",
				Content: message,
			})
			conversation(c, e, gogpt.GPT4, chatGptContext)
			return
		case "!davinci":
			model = gogpt.GPT3Davinci
		case "!davinci3":
			model = gogpt.GPT3TextDavinci003
		case "!davinci2":
			model = gogpt.GPT3TextDavinci002
		case "!davinci1":
			model = gogpt.GPT3TextDavinci001
		case "!ada":
			model = gogpt.GPT3Ada
		case "!curie":
			model = gogpt.GPT3TextCurie001
		case "!babbage":
			model = gogpt.GPT3Babbage
		// Default model specified in the config.toml
		case "!ai":
			model = gogpt.GPT3Dot5TurboInstruct
		default:
			return
		}

		if model != "" {
			completion(c, e, message, model)
		}

	})

	client.Handlers.Add(girc.PRIVMSG, func(c *girc.Client, e girc.Event) {
		if shouldIgnore(e.Source.Name) || !e.IsFromChannel() {
			return
		}

		// Commands that do not need a following argument
		switch e.Last() {
		// Display help message
		case "!help":
			floodCheck(c, e, name)
			sendToIrc(c, e, "OpenAI Models: !gpt4, !gpt3.5, !davinci (newest), !davinci3, !davinci2, !davinci1, !ada, !babbage, !ai (as GPT3Dot5TurboInstruct), !bard (Google Bard), !sd (Stable diffusion)")
			sendToIrc(c, e, "Dall-E 3: !dale, --1024, --1792x1024, --1024x1792, --hd (high quality), --vivid (vivid style), --2 (Dall-E 2, can support --256 and --512)")
			sendToIrc(c, e, "Other: !aiscii (experimental ascii generation), !birdmap (run port scan on target), !sd (Stable diffusion request) - https://github.com/birdneststream/aibird")
			return
		}
	})

	for {
		if err := client.Connect(); err != nil {
			log.Printf("%s error: %s", name, err)
			log.Println("reconnecting in 30 seconds...")
			time.Sleep(30 * time.Second)
		} else {
			log.Println("Got to the end, quitting " + name)
			waitGroup.Add(1)
			go ircClient(network, name, waitGroup)
			return
		}
	}
}
