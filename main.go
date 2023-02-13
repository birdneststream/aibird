package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"math/rand"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

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

func saveDalleRequest(prompt string, url string) string {
	// Clean the filename, there has to be a better way to do this
	slug := strings.ReplaceAll(prompt, " ", "-")
	slug = strings.ReplaceAll(slug, "/", "-")
	slug = strings.ReplaceAll(slug, "\\", "-")
	slug = strings.ReplaceAll(slug, ":", "-")
	slug = strings.ReplaceAll(slug, "*", "-")
	slug = strings.ReplaceAll(slug, "?", "-")
	slug = strings.ReplaceAll(slug, "\"", "-")
	slug = strings.ReplaceAll(slug, "<", "-")
	slug = strings.ReplaceAll(slug, ">", "-")
	slug = strings.ReplaceAll(slug, "|", "-")
	slug = strings.ReplaceAll(slug, ".", "-")
	slug = strings.ReplaceAll(slug, ",", "-")
	slug = strings.ReplaceAll(slug, ";", "-")
	slug = strings.ReplaceAll(slug, "'", "-")
	slug = strings.ReplaceAll(slug, "!", "-")
	slug = strings.ReplaceAll(slug, "@", "-")
	slug = strings.ReplaceAll(slug, "#", "-")

	if len(slug) > 220 {
		slug = slug[:220]
	}

	randValue := rand.Int63n(10000)
	// Place a random number on the end to (maybe almost) avoid overwriting duplicate requests
	fileName := slug + "_" + strconv.FormatInt(randValue, 4) + ".png"

	downloadFile(url, fileName)

	// append the current pwd to fileName
	fileName = filepath.Base(fileName)

	// download image
	content := fileHole("https://filehole.org/", fileName)

	return string(content)
}

func fileHole(url string, fileName string) string {
	method := "POST"

	payload := &bytes.Buffer{}
	writer := multipart.NewWriter(payload)
	file, errFile1 := os.Open(fileName)
	defer file.Close()
	part1,
		errFile1 := writer.CreateFormFile("file", filepath.Base(fileName))
	_, errFile1 = io.Copy(part1, file)
	if errFile1 != nil {
		fmt.Println(errFile1)

	}
	_ = writer.WriteField("expiry", "432000")
	_ = writer.WriteField("url_len", "5")
	err := writer.Close()
	if err != nil {
		fmt.Println(err)

	}

	client := &http.Client{}
	req, err := http.NewRequest(method, url, payload)

	if err != nil {
		fmt.Println(err)

	}

	req.Header.Set("Content-Type", writer.FormDataContentType())
	res, err := client.Do(req)
	if err != nil {
		fmt.Println(err)

	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		fmt.Println(err)

	}
	fmt.Println(string(body))

	return string(body)
}

func downloadFile(URL, fileName string) error {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("Received non 200 response code")
	}
	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
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

func wait(delay time.Duration) {
	// Flood safe delay
	time.Sleep(time.Millisecond * delay)
}

// Used to send responseString to IRC
var sendString string

// Response we get back from API
var responseString string

// Model we will use
var model string

// What is passed to the API
var message string

var processing bool

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
	var ircServer Server
	ircServer = network.returnRandomServer()

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

	processing = false

	ircConfig := irc.ClientConfig{
		Nick:          network.Nick,
		Pass:          ircServer.Pass,
		User:          network.Nick,
		Name:          network.Nick,
		SendLimit:     network.Throttle,
		SendBurst:     network.Burst,
		PingFrequency: time.Second * 8,
		PingTimeout:   time.Second * 120,
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
				break
			case "PRIVMSG":
				if c.FromChannel(m) == false {
					return
				}

				msg := m.Trailing()

				// don't do anything if message isn't prefixed w/ an exclamation
				if strings.HasPrefix(msg, "!") == false {
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
					processing = false
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
				case "!davinci":
					model = gogpt.GPT3TextDavinci003
					cost = 0.0200
					break
				case "!davinci2":
					model = gogpt.GPT3TextDavinci002
					cost = 0.0200
					break
				case "!davinci1":
					model = gogpt.GPT3TextDavinci001
					cost = 0.0200
					break
				case "!codex":
					model = gogpt.CodexCodeDavinci002
					cost = 0.0200
					break
				case "!ada":
					model = gogpt.GPT3Ada
					cost = 0.0004
					break
				case "!babbage":
					model = gogpt.GPT3Babbage
					cost = 0.0005
					break
				case "!dale":
					model = "dall-e"
					size = gogpt.CreateImageSize512x512
					cost = 0.018
					break
				case "!dale256":
					model = "dall-e"
					size = gogpt.CreateImageSize256x256
					cost = 0.016
					break
				case "!dale1024":
					model = "dall-e"
					size = gogpt.CreateImageSize1024x1024
					cost = 0.020
					break
				case "!ai":
					model = config.OpenAI.Model
					cost = 0.0020
					break
				case "!aiscii":
					// Custom prompt to make better mirc art
					model = "aiascii"
					break
				default:
					return
				}

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

				processing = true
				aiClient := gogpt.NewClient(config.OpenAI.nextApiKey())

				// if the model is dale
				if model == "dall-e" {
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
						processing = false
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

					return
				}

				// mIRC ascii art generation, shows without the price and has a pre-loaded prompt for better generation.
				if model == "aiascii" {
					// if message = --save
					go aiscii(m, message, c, aiClient, ctx)

					return
				}

				// Alert the irc chan that the bot is processing
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
					processing = false
					return
				}

				// resp.Usage.TotalTokens / 1000 * cost
				total := strconv.FormatFloat((float64(resp.Usage.TotalTokens)/1000)*cost, 'f', 5, 64)

				responseString = strings.TrimSpace(resp.Choices[0].Text) + " ($" + total + ")"

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
						if len(sendString) > 350 {
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

				processing = false
				break
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

	log.Println("Got to the end, quitting " + network.Nick)
	processing = false
	waitGroup.Add(1)
	go ircClient(network, name, waitGroup)
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
		processing = false
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
			processing = false
			return
		}
		asciiName = strings.TrimSpace(resp.Choices[0].Text)

		// get alphabet letters from asciiName only
		asciiName = strings.ReplaceAll(asciiName, " ", "-")
		asciiName = strings.ReplaceAll(asciiName, ".", "")
		// make lowercase asciiName
		asciiName = strings.ToLower(asciiName)

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
		processing = false
		return
	}

	responseString = strings.TrimSpace(resp.Choices[0].Text)

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
			if len(sendString) > 350 {
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

	if parts[0] == "--save" {
		c.WriteMessage(&irc.Message{
			Command: "PRIVMSG",
			Params: []string{
				m.Params[0],
				"@end",
			},
		})
	}
	processing = false
	return

}
