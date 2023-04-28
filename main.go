package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"git.mills.io/prologic/bitcask"
	"github.com/BurntSushi/toml"
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
			waitGroup.Add(1)
			go ircClient(network, name, &waitGroup)
		}
	}

	waitGroup.Wait()

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
		Server:     ircServer.Host,
		Port:       ircServer.Port,
		SSL:        ircServer.Ssl,
		ServerPass: ircServer.Pass,
		PingDelay:  60,
		Nick:       network.Nick,
		User:       network.Nick,
		Name:       network.Nick,
		Version:    "VERSION AiBird Bot - https://github.com/birdneststream/aibird",
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
		//On an error we should wait ~120seconds then reconnect
		//sometimes the errors are reconnecting too fast etc
		// Nah we will just wait 5 seconds instead hahaha
	})
	client.Handlers.Add(girc.RPL_WELCOME, func(c *girc.Client, e girc.Event) {
		for _, channel := range network.Channels {
			c.Cmd.Join(channel)
			time.Sleep(network.Throttle * time.Millisecond)
		}
	})
	client.Handlers.Add(girc.RPL_NAMREPLY, func(c *girc.Client, e girc.Event) {
		cacheNicks(name, e)
	})
	client.Handlers.Add(girc.RPL_ENDOFNAMES, func(c *girc.Client, e girc.Event) {
		saveNicks(name, e)
	})
	client.Handlers.Add(girc.RPL_WHOREPLY, func(c *girc.Client, e girc.Event) {
		cacheAutoLists(name, e)
	})
	client.Handlers.Add(girc.JOIN, func(c *girc.Client, e girc.Event) {
		// Only want to do these if we already have ops
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
	})
	client.Handlers.Add(girc.MODE, func(c *girc.Client, e girc.Event) {
		// If there is a +b on a protected host, remove it.
		// This is not so secure at the moment.
		// go protectHosts(c, e)

		// Cache the names list
		_ = c.Cmd.SendRaw("NAMES " + e.Params[0])
		time.Sleep(network.Throttle * time.Millisecond)

		// Only want to cache this if we have ops
		if isUserMode(name, e.Params[0], network.Nick, "@") {
			_ = c.Cmd.SendRaw("WHO " + e.Params[0])
			time.Sleep(network.Throttle * time.Millisecond)
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
			go cacheChatsForChatGtp(name, e, c)
			return
		}

		if !isUserMode(name, e.Params[0], e.Source.Name, "~&@%+") {
			return
		}

		if config.AiBird.ReplyToChats {
			if strings.HasPrefix(e.Last(), network.Nick) {
				// If the night is highlighted reply
				go replyToChats(e, e.Last(), c)
				return
			} else if e.Params[0] == "#birdnest" {
				// General chats
				go cacheChatsForReply(name, e.Last(), e, c)
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

		cmd := parts[0]
		message := strings.TrimSpace(parts[1])

		// Model we will use
		var model string
		var cost float64

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
						_ = c.Cmd.Reply(e, err.Error())
					}
					_ = c.Cmd.Reply(e, "Reloaded config!")
					return

				case "raw":
					// remove raw from message and trim
					message = strings.TrimSpace(strings.TrimPrefix(message, "raw "))
					event := girc.ParseEvent(message)
					if event == nil {
						_ = c.Cmd.Reply(e, "Raw string was not valid IRC")
						return
					}
					c.Send(event)
					return

				case "sd":
					sdAdmin(message, c, e)
					return

				case "aibird_personality":
					message = strings.TrimSpace(strings.TrimPrefix(message, "aibird_personality"))
					config.AiBird.ChatPersonality = message
					_ = c.Cmd.Reply(e, "Set aibird personality to "+message)
					return

				case "birdbase":
					message = strings.TrimSpace(strings.TrimPrefix(message, "birdbase"))

					if message == "nicks" {
						var key = []byte(name + "_" + e.Params[0] + "_nicks")
						nickList, err := birdBase.Get(key)
						if err != nil {
							log.Println(err)
							return
						}

						chunkToIrc(c, e, string(nickList))
					}

					return
				}
			}

			return
			// Dall-e Commands
		case "!dale":
			go dalle(e, message, c, gogpt.CreateImageSize512x512)
			return
		case "!dale256":
			go dalle(e, message, c, gogpt.CreateImageSize256x256)
			return
		case "!dale1024":
			go dalle(e, message, c, gogpt.CreateImageSize1024x1024)
			return
		// Custom prompt to make better mirc art
		case "!aiscii":
			go aiscii(e, message, c)
			return
		case "!aiscii4":
			if isAdmin(e) {
				go conversation(gogpt.GPT4, "Use the UTF-8 drawing characters and mIRC color codes (using ) to make a monospaced text art 80 characters wide and 30 characters height depicting '"+message+"'.", e, c)
			}
			return
		case "!birdmap":
			go birdmap(e, message, c)
			return
		// Stable diffusion prompts
		case "!sd":
			if !isUserMode(name, e.Params[0], e.Source.Name, "@~") {
				_ = c.Cmd.Reply(e, "Hey there chat pal "+e.Source.Name+", you have to be a birdnest patreon to use stable diffusion! Unless you want to donate your own GPU!")
				return
			}

			if e.Params[0] != "#birdnest" {
				_ = c.Cmd.Reply(e, "Hey there chat pal "+e.Source.Name+", stable diffusion is only available in #birdnest!")
				return
			}

			go sdRequest(message, c, e)
			return

		// The models for completion prompts
		case "!chatgpt":
			go conversation(gogpt.GPT3Dot5Turbo, message, e, c)
			return
		case "!gpt4":
			if isAdmin(e) {
				go conversation(gogpt.GPT4, message, e, c)
			}
			return
		case "!davinci":
			model = gogpt.GPT3TextDavinci003
			cost = 0.0200
		case "!davinci2":
			model = gogpt.GPT3TextDavinci002
			cost = 0.0200
		case "!davinci1":
			model = gogpt.GPT3TextDavinci001
			cost = 0.0200
		case "!codex":
			model = gogpt.CodexCodeDavinci002
			cost = 0.0200
		case "!ada":
			model = gogpt.GPT3Ada
			cost = 0.0004
		case "!babbage":
			model = gogpt.GPT3Babbage
			cost = 0.0005
		// Default model specified in the config.toml
		case "!ai":
			model = config.OpenAI.Model
			cost = 0.0200
		default:
			return
		}

		go completion(e, message, c, model, cost)
	})

	client.Handlers.Add(girc.PRIVMSG, func(c *girc.Client, e girc.Event) {
		if shouldIgnore(e.Source.Name) {
			return
		}

		if !e.IsFromChannel() {
			return
		}

		// Commands that do not need a following argument
		switch e.Last() {
		// Display help message
		case "!help":
			_ = c.Cmd.Reply(e, "OpenAI Models: !chatgpt - one shot gpt3.5 model (no context), !davinci (best), !davinci2, !davinci1, !codex (code generation), !ada, !babbage, !dale (512x512), !dale256 (256x256), !dale1024 (1024x1024 very slow), !ai (default model from config)")
			_ = c.Cmd.Reply(e, "Other: !aiscii (experimental ascii generation), !aiscii4 (gpt4 aiscii generation), !birdmap (run port scan on target), !sd (Stable diffusion request) - https://github.com/birdneststream/aibird")
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
