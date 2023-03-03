package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"git.mills.io/prologic/bitcask"
	"github.com/BurntSushi/toml"
	gogpt "github.com/sashabaranov/go-gpt3"
	"gopkg.in/irc.v3"
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

	db, _ := bitcask.Open("bird.db")
	birdBase = db

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

var whatKey string // keeps track of the current ai key to alert expired

func ircClient(network Network, name string, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	sslConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Choose a random IRC server to connect to within the network
	var ircServer Server = network.returnRandomServer()

	protocol := "tcp"
	if ircServer.Ipv6 {
		protocol = "tcp6"
	}

	conn, err := net.Dial(protocol, fmt.Sprintf("%s:%d", fmt.Sprint(ircServer.Host), ircServer.Port))
	if err != nil {
		log.Fatal(err)
	}

	if ircServer.Ssl {
		conn = tls.Client(conn, sslConfig)
	}

	// Model we will use
	var model string
	var cost float64

	// Initialise the openAI api client
	ctx := context.Background()

	ircConfig := irc.ClientConfig{
		Nick:      network.Nick,
		Pass:      ircServer.Pass,
		User:      network.Nick,
		Name:      network.Nick,
		SendLimit: network.Throttle,
		SendBurst: network.Burst,
		Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {
			if config.AiBird.Debug {
				log.Println(m)
			}

			if strings.Contains(m.Command, "ERROR") {
				// Reconnect if we get an error
				log.Println("Error: " + m.Command)
			}

			switch m.Command {

			case "VERSION":
				c.Write("NOTICE " + m.Prefix.Name + " :VERSION AiBird Bot - https://github.com/birdneststream/aibird")
				return

			case "001":
				// On successful connect, attempt to join channels iterate over chansList
				for j := 0; j < len(network.Channels); j++ {
					c.Write("JOIN " + network.Channels[j])
					time.Sleep(network.Throttle * time.Millisecond)
					c.Write("WHO " + network.Channels[j])
				}
				return

			// Build the names list
			case "353":
				cacheNicks(name, m)

				return

			case "366":
				saveNicks(name, m)

				return

			// Who the channel
			case "352":
				cacheAutoLists(name, m)

				return

			case "JOIN":
				// Cycle over Admins then Auto Ops
				// for i := 0; i < len(config.AiBird.ProtectedHosts); i++ {
				// 	if config.AiBird.ProtectedHosts[i].Host == m.Prefix.Host {
				// 		c.Write("MODE " + m.Params[0] + " +o " + m.Prefix.Name)
				// 		return
				// 	}
				// }

				// Only want to do these if we already have ops
				if isUserMode(name, m.Params[0], network.Nick, "@") {
					// Auto Voice
					if isInList(name, m.Params[0], "voice", m.Prefix.User, m.Prefix.Host, m.Prefix.Name) {
						c.Write("MODE " + m.Params[0] + " +v " + m.Prefix.Name)
						return
					}

					// Auto Op
					if isInList(name, m.Params[0], "op", m.Prefix.User, m.Prefix.Host, m.Prefix.Name) {
						c.Write("MODE " + m.Params[0] + " +o " + m.Prefix.Name)
						return
					}
				}

				return

			case "MODE":
				// If there is a +b on a protected host, remove it.
				// This is not so secure at the moment.
				// go protectHosts(c, m)

				// Cache the names list
				c.Write("NAMES " + m.Params[0])
				time.Sleep(network.Throttle * time.Millisecond)

				// Only want to cache this if we have ops
				if isUserMode(name, m.Params[0], network.Nick, "@") {
					c.Write("WHO " + m.Params[0])
					time.Sleep(network.Throttle * time.Millisecond)
				}
				return

			case "KICK":
				// if the bot is kicked, rejoin
				if m.Params[1] == network.Nick {
					c.Write("JOIN " + m.Params[0])
				}
				return

			case "PRIVMSG":
				if !c.FromChannel(m) {
					return
				}

				key := config.OpenAI.nextApiKey()
				whatKey = key
				aiClient := gogpt.NewClient(key)

				if config.AiBird.ReplyToChats && m.Params[0] == "#birdnest" && !shouldIgnore(m.Prefix.Name) {
					go cacheChatsForReply(name, m.Trailing(), m, c, aiClient, ctx)
				}

				if !isUserMode(name, m.Params[0], m.Prefix.Name, "~&@%+") {
					return
				}

				msg := m.Trailing()

				// don't do anything if message isn't prefixed w/ an exclamation
				if !strings.HasPrefix(msg, "!") {
					return
				}

				// Commands that do not need a following argument
				switch msg {
				// Display help message
				case "!help":
					c.WriteMessage(&irc.Message{
						Command: "PRIVMSG",
						Params: []string{
							m.Params[0],
							"OpenAI Models: !davinci (best), !davinci2, !davinci1, !codex (code generation), !ada, !babbage, !dale (512x512), !dale256 (256x256), !dale1024 (1024x1024 very slow), !ai (default model from config)",
						},
					})
					c.WriteMessage(&irc.Message{
						Command: "PRIVMSG",
						Params: []string{
							m.Params[0],
							"Other: !aiscii (experimental ascii generation), !birdmap (run port scan on target), !sd (Stable diffusion request) - https://github.com/birdneststream/aibird",
						},
					})
					return

				case "!modes":
					// cycle irc channels and update modes
					for i := 0; i < len(network.Channels); i++ {
						c.Write("WHO " + network.Channels[i])
					}

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

				switch cmd {

				case "!admin":
					// if the host is not in config.AiBird.Admin.Host then +b the host
					if isAdmin(m) {
						log.Println("Admin command from " + m.Prefix.Name + " on " + m.Params[0] + ": " + message)
						parts := strings.SplitN(message, " ", 2)
						switch parts[0] {
						case "reload":
							_, err := toml.DecodeFile("config.toml", &config)
							if err != nil {
								c.WriteMessage(&irc.Message{
									Command: "PRIVMSG",
									Params: []string{
										m.Params[0],
										err.Error(),
									},
								})
								return
							}

							c.WriteMessage(&irc.Message{
								Command: "PRIVMSG",
								Params: []string{
									m.Params[0],
									"Reloaded config!",
								},
							})
							return

						case "raw":
							// remove raw from message and trim
							message = strings.TrimSpace(strings.TrimPrefix(message, "raw"))
							c.Write(message)
							return

						case "sd":
							sdAdmin(message, c, m)
							return

						case "birdbase":
							message = strings.TrimSpace(strings.TrimPrefix(message, "birdbase"))

							// get from database efnet_#birdnest_meta as string
							// nickList, err := birdBase.Get([]byte(string(message + "_nicks")))
							// if err != nil {
							// 	log.Println(err)
							// 	return
							// }

							// log.Println("NICKS: " + string(nickList))

							voiceList, err := birdBase.Get([]byte(string(message + "_autovoice")))
							if err != nil {
								log.Println(err)
								return
							}

							log.Println("AUTO VOICE: " + string(voiceList))

							autoOps, err := birdBase.Get([]byte(string(message + "_autoop")))
							if err != nil {
								log.Println(err)
								return
							}

							log.Println("AUTO OPS: " + string(autoOps))

							// chunkToIrc(c, m, message)
							return
						}
					}

					return
					// Dall-e Commands
				case "!dale":
					go dalle(m, message, c, aiClient, ctx, gogpt.CreateImageSize512x512)
					return
				case "!dale256":
					go dalle(m, message, c, aiClient, ctx, gogpt.CreateImageSize256x256)
					return
				case "!dale1024":
					go dalle(m, message, c, aiClient, ctx, gogpt.CreateImageSize1024x1024)
					return
				// Custom prompt to make better mirc art
				case "!aiscii":
					go aiscii(m, message, c, aiClient, ctx)
					return
				case "!birdmap":
					go birdmap(m, message, c, aiClient, ctx)
					return
				// Stable diffusion prompts
				case "!sd":
					if !isUserMode(name, m.Params[0], m.Prefix.Name, "@~") {
						c.WriteMessage(&irc.Message{
							Command: "PRIVMSG",
							Params: []string{
								m.Params[0],
								"Hey there chat pal " + m.Prefix.Name + ", you have to be a birdnest patreon to use stable diffusion! Unless you want to donate your own GPU!",
							},
						})

						return
					}

					if m.Params[0] != "#birdnest" {
						c.WriteMessage(&irc.Message{
							Command: "PRIVMSG",
							Params: []string{
								m.Params[0],
								"Hey there chat pal " + m.Prefix.Name + ", stable diffusion is only available in #birdnest!",
							},
						})

						return
					}

					go sdRequest(message, c, m)
					return
				// The models for completion prompts
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

				go completion(m, message, c, aiClient, ctx, model, cost)

				return
			}

		}),
	}

	// Create the client
	client := irc.NewClient(conn, ircConfig)
	err = client.Run()

	// log client to console
	if err != nil {
		log.Println(err)
	}

	log.Println("Got to the end, quitting " + name)
	waitGroup.Add(1)
	go ircClient(network, name, waitGroup)
}
