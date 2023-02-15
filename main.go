package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/BurntSushi/toml"
	gogpt "github.com/sashabaranov/go-gpt3"
	"gopkg.in/irc.v3"
)

var config Config
var nickList []string

func loadConfig() {
	_, err := toml.DecodeFile("config.toml", &config)
	if err != nil {
		fmt.Println("Error in config.toml")
		fmt.Println(err)
		os.Exit(1)
	}
}

type DalEUrl struct {
	Url string `json:"url"`
}
type DalE struct {
	Created int64     `json:"created"`
	Data    []DalEUrl `json:"data"`
}

type content struct {
	fname string
	ftype string
	fdata []byte
}

func main() {
	// Load json config
	loadConfig()

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

// Used to send responseString to IRC
var sendString string

// Response we get back from API
var responseString string

// Model we will use
var model string

// What is passed to the API
// var message string

var cost float64

// Size of the Dall-E request image
var size string

// Used for !aiscii
var asciiName string // ai generated name

var prompt string // prompt

func ircClient(network Network, name string, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
	sslConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Choose a random IRC server to connect to within the network
	var ircServer Server = network.returnRandomServer()

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", fmt.Sprint(ircServer.Host), ircServer.Port))
	if err != nil {
		log.Fatal(err)
	}

	if ircServer.Ssl {
		conn = tls.Client(conn, sslConfig)
	}

	var chansList = network.Channels

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
			// if m contains string banned
			if strings.Contains(m.Command, "ERROR") {
				// Reconnect if we get an error
				log.Println("Error: " + m.Command)
			}

			switch m.Command {
			case "001":
				// On successful connect, attempt to join channels iterate over chansList
				for j := 0; j < len(chansList); j++ {
					c.Write("JOIN " + chansList[j])
				}
			case "PRIVMSG":
				if !c.FromChannel(m) {
					return
				}

				msg := m.Trailing()

				// don't do anything if message isn't prefixed w/ an exclamation
				if !strings.HasPrefix(msg, "!") {
					return
				}

				// Display help message
				if msg == "!help" {
					c.WriteMessage(&irc.Message{
						Command: "PRIVMSG",
						Params: []string{
							m.Params[0],
							"Models: !davinci (best), !davinci2, !davinci1, !codex (code generation), !ada, !babbage, !dale (512x512), !dale256 (256x256), !dale1024 (1024x1024 very slow), !ai (default model) - https://github.com/birdneststream/aibird",
						},
					})
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
				aiClient := gogpt.NewClient(config.OpenAI.nextApiKey())

				switch cmd {
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
				case "!dale":
					size = gogpt.CreateImageSize512x512
					cost = 0.018
					go dalle(m, message, c, aiClient, ctx, size)

					return
				case "!dale256":
					model = "dall-e"
					size = gogpt.CreateImageSize256x256
					cost = 0.016
				case "!dale1024":
					model = "dall-e"
					size = gogpt.CreateImageSize1024x1024
					cost = 0.020
				case "!ai":
					model = config.OpenAI.Model
					cost = 0.0200
				case "!aiscii":
					// Custom prompt to make better mirc art
					go aiscii(m, message, c, aiClient, ctx)

					return
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

func completion(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, model string, cost float64) {
	req := gogpt.CompletionRequest{
		Model:       model,
		MaxTokens:   config.OpenAI.Tokens,
		Prompt:      message,
		Temperature: config.OpenAI.Temperature,
	}

	if model == gogpt.CodexCodeDavinci002 {
		req = gogpt.CompletionRequest{
			Model:            model,
			MaxTokens:        config.OpenAI.Tokens,
			Prompt:           message,
			Temperature:      0,
			TopP:             1,
			FrequencyPenalty: 0,
			PresencePenalty:  0,
		}
	}

	// Process a completion request
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing: " + message,
		},
	})

	// Perform the actual API request to openAI
	resp, err := aiClient.CreateCompletion(ctx, req)
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

	// resp.Usage.TotalTokens / 1000 * cost
	total := strconv.FormatFloat((float64(resp.Usage.TotalTokens)/1000)*cost, 'f', 5, 64)

	responseString = strings.TrimSpace(resp.Choices[0].Text) + " ($" + total + ")"

	chunkToIrc(c, m, responseString)
}

func dalle(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context, size string) {
	req := gogpt.ImageRequest{
		Prompt: message,
		Size:   size,
		N:      1,
	}

	// Alert the irc chan that the bot is processing
	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing Dall-E: " + message,
		},
	})

	resp, err := aiClient.CreateImage(ctx, req)
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

	daleResponse := saveDalleRequest(message, resp.Data[0].URL)

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			m.Prefix.Name + ": " + daleResponse,
		},
	})
}

// aiscii function, hopefully will prevent ping timeouts
func aiscii(m *irc.Message, message string, c *irc.Client, aiClient *gogpt.Client, ctx context.Context) {

	parts := strings.SplitN(message, " ", 2)

	if parts[0] == "--save" {
		message = parts[1]
	}

	prompt = "'{0-16},{0-16}#' use this to create an embedded mirc text art.\n\nReplace the # from the following '▀▁▂▃▄▅▆▇█▉▊▋▌▍▎▏▐░▒▓▔▕▖▗▘▙▚▛▜▝▞'.\n\nThe art must be at least 20 lines and 80 column width of mirc embedded color codes and ascii text art. Ascii text art of " + message + "."

	req := gogpt.CompletionRequest{
		Model:            gogpt.GPT3TextDavinci003,
		MaxTokens:        config.OpenAI.Tokens,
		Prompt:           prompt,
		Temperature:      0,
		TopP:             1,
		FrequencyPenalty: 0,
		PresencePenalty:  0,
	}

	c.WriteMessage(&irc.Message{
		Command: "PRIVMSG",
		Params: []string{
			m.Params[0],
			"Processing mIRC aiscii art (it can take a while): " + message,
		},
	})

	resp, err := aiClient.CreateCompletion(ctx, req)

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

	responseString = strings.TrimSpace(resp.Choices[0].Text)

	if parts[0] == "--save" {
		message = parts[1]
		// Generate a title for the art
		req = gogpt.CompletionRequest{
			Model:            gogpt.GPT3TextDavinci002,
			MaxTokens:        128,
			Prompt:           "Write a short three word title for your mirc ascii art based on '" + message + "'. Use only alphabetical characters and spaces only.",
			Temperature:      0.8,
			TopP:             1,
			FrequencyPenalty: 0.6,
			PresencePenalty:  0.3,
		}

		resp, err := aiClient.CreateCompletion(ctx, req)
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
		asciiName = strings.TrimSpace(resp.Choices[0].Text)

		// get alphabet letters from asciiName only
		asciiName := cleanFileName(asciiName)

		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				"@record " + asciiName,
			},
		})
	}

	// for each new line break in response choices write to channel
	for _, line := range strings.Split(responseString, "\n") {
		sendString = ""

		// Write the final message
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				line,
			},
		})
	}

	message = "As a snobby reddit intellectual artist, shortly explain your new artistic masterpiece '" + message + "'" + " to the masses."

	req = gogpt.CompletionRequest{
		Model:       gogpt.GPT3TextDavinci002,
		MaxTokens:   256,
		Prompt:      message,
		Temperature: 1.1,
	}

	resp, err = aiClient.CreateCompletion(ctx, req)
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

	responseString = strings.TrimSpace(resp.Choices[0].Text)

	chunkToIrc(c, m, responseString)

	if parts[0] == "--save" {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				"@end",
			},
		})
	}
}
