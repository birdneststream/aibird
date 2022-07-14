package main

import (
	"context"
	crypto_rand "crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"os"
	"strings"
	"time"

	gogpt "github.com/sashabaranov/go-gpt3"

	"gopkg.in/irc.v3"
)

type Config struct {
	Network []Network `json:"networks"`
	Openai  Openai    `json:"openai"`
}

type Network struct {
	Name     string   `json:"name"`
	Nick     string   `json:"nick"`
	Servers  []Server `json:"servers"`
	Channels []string `json:"channels"`
	Enabled  bool     `json:"enabled"`
}

type Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Ssl  bool   `json:"ssl"`
}

type Openai struct {
	Key    string `json:"key"`
	Tokens int    `json:"tokens"`
	Model  string `json:"model"`
}

var config Config
var b [8]byte
var randValue int64
var nickList []string

func loadConfig() {
	// Load config.json
	jsonFile, err := os.Open("config.json")

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Successfully Opened config.json")
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	json.Unmarshal(byteValue, &config)
}

func returnRandomServer(network Network) Server {
	// Better non tine based random number generator
	_, err := crypto_rand.Read(b[:])
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}

	rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))

	// return random server
	return network.Servers[rand.Intn(len(network.Servers))]
}

func main() {
	// Load json config
	loadConfig()

	log.Println("AI bot connecting to IRC, please wait")

	for _, network := range config.Network {
		if network.Enabled {
			ircClient(network)
		}
	}

	//exit
	os.Exit(0)
}

func wait() {
	// Efnet safe delay
	time.Sleep(time.Millisecond * 510)
}

var processing = false
var stringLength int
var sendString string
var responseString string

func ircClient(network Network) {
	sslConfig := &tls.Config{
		InsecureSkipVerify: true,
	}

	// Choose a random IRC server to connect to within the network
	var ircServer Server
	ircServer = returnRandomServer(network)

	conn, err := net.Dial("tcp", fmt.Sprintf("%s:%d", fmt.Sprint(ircServer.Host), ircServer.Port))
	if err != nil {
		log.Fatal(err)
	}

	if ircServer.Ssl {
		conn = tls.Client(conn, sslConfig)
	}

	var chansList = network.Channels

	// Initialise the openAI api client
	aiClient := gogpt.NewClient(config.Openai.Key)
	ctx := context.Background()

	ircConfig := irc.ClientConfig{
		Nick: network.Nick,
		Pass: "",
		User: network.Nick,
		Name: network.Nick,
		Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {

			// if m contains string banned
			if strings.Contains(m.Command, "ERROR") {
				// Reconnect if we get an error
				log.Println("Error: " + m.Command)
			}

			switch m.Command {
			// On successful join attempt to join channels
			case "001":
				wait()
				c.Write("JOIN " + chansList[0])
				break

			case "PRIVMSG":
				if c.FromChannel(m) {
					log.Println(m)

					// if m.Trailing() starts with !ai
					if strings.HasPrefix(m.Trailing(), "!ai") && !processing {
						// Attempt to prevent overlapping api requests
						processing = true

						// Get the message after !ai
						message := strings.TrimPrefix(m.Trailing(), "!ai")

						// trim white space from message
						message = strings.TrimSpace(message)

						// Can expand this part out with more custom json config stuff
						req := gogpt.CompletionRequest{
							MaxTokens:   config.Openai.Tokens,
							Prompt:      message,
							Temperature: 0.8,
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
						resp, err := aiClient.CreateCompletion(ctx, config.Openai.Model, req)
						if err != nil {
							return
						}

						log.Println(resp.Choices[0].Text)

						// trim resp.Choices[0].Text
						responseString = strings.TrimSpace(resp.Choices[0].Text)

						// for each new line break in response choices write to channel
						for _, line := range strings.Split(responseString, "\n") {
							stringLength = 0
							sendString = ""

							// if line length is one or less then continue
							if len(line) <= 2 {
								continue
							}

							// split line into chunks slice with space
							chunks := strings.Split(line, " ")

							// for each chunk
							for _, chunk := range chunks {
								// add length of chunk to stringLength
								stringLength += len(chunk)

								// append chunk to sendString
								sendString += chunk + " "

								// if stringLength is greater than 300
								if stringLength > 300 {
									// write message to channel
									c.WriteMessage(&irc.Message{
										Command: "PRIVMSG",
										Params: []string{
											m.Params[0],
											sendString,
										},
									})
									wait()
									stringLength = 0
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
							wait()

							processing = false
						}
					}
				}

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
}
