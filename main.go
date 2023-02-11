package main

import (
	"bytes"
	"context"
	crypto_rand "crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
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

	gogpt "github.com/sashabaranov/go-gpt3"

	"gopkg.in/irc.v3"
)

type Config struct {
	Network []Network `json:"networks"`
	Openai  Openai    `json:"openai"`
}

type Network struct {
	Name     string        `json:"name"`
	Nick     string        `json:"nick"`
	Servers  []Server      `json:"servers"`
	Channels []string      `json:"channels"`
	Enabled  bool          `json:"enabled"`
	Throttle time.Duration `json:"throttle"`
}

type Server struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Pass string `json:"pass"`
	Ssl  bool   `json:"ssl"`
}

type Openai struct {
	Key    []string `json:"key"`
	Tokens int      `json:"tokens"`
	Model  string   `json:"model"`
}

var config Config
var b [8]byte
var randValue int64
var nickList []string
var roundRobinKey = 0

func loadConfig() {
	// Load config.json
	jsonFile, err := os.Open("config.json")

	if err != nil {
		fmt.Println(err)
	}
	fmt.Println("Successfully Opened config.json")
	defer jsonFile.Close()

	byteValue, _ := ioutil.ReadAll(jsonFile)

	err = json.Unmarshal(byteValue, &config)
	if err != nil {
		fmt.Println("Error in config.json")
		fmt.Println(err)
		os.Exit(1)
	}
}

func rotateApiKey() string {
	// Rotate API key
	roundRobinKey++
	if roundRobinKey >= len(config.Openai.Key) {
		roundRobinKey = 0
	}

	return config.Openai.Key[roundRobinKey]
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

func daleRequest(prompt string) string {

	// do a POST request to https://api.openai.com/v1/images/generations

	posturl := "https://api.openai.com/v1/images/generations"

	// Try to remove problem chars
	prompt = strings.ReplaceAll(prompt, ":", "")
	prompt = strings.ReplaceAll(prompt, "/", "")
	prompt = strings.ReplaceAll(prompt, "\\", "")
	prompt = strings.ReplaceAll(prompt, ".", "")
	prompt = strings.ReplaceAll(prompt, "\"", "")
	prompt = strings.ReplaceAll(prompt, "'", "")
	prompt = strings.ReplaceAll(prompt, ",", "")

	// new variable that converts prompt to slug
	slug := strings.ReplaceAll(prompt, " ", "-")

	// lowercase slug
	slug = strings.ToLower(slug)

	if len(slug) > 225 {
		slug = slug[:225]
	}

	// JSON body
	body := []byte(`{
		"prompt": "` + prompt + `",
		"n": 1,
		"size": "512x512"
	}`)

	// log body
	log.Println(string(body))

	// Create a HTTP post request
	r, err := http.NewRequest("POST", posturl, bytes.NewBuffer(body))
	if err != nil {
		panic(err)
	}

	r.Header.Add("Content-Type", "application/json")
	r.Header.Add("Authorization", "Bearer "+rotateApiKey())
	client := &http.Client{}
	res, err := client.Do(r)
	if err != nil {
		panic(err)
	}

	defer res.Body.Close()

	post := &DalE{}
	derr := json.NewDecoder(res.Body).Decode(post)
	if derr != nil {
		panic(derr)
	}

	if res.StatusCode != http.StatusOK {
		if res.StatusCode == 400 {
			return "Bad request, maybe violated Dal-E ToS (No violence, nudity, anything fun, etc). Please keep it G rated!"
		}

		return (res.Status)
	}

	// generate random string
	_, err = crypto_rand.Read(b[:])
	if err != nil {
		panic("cannot seed math/rand package with cryptographically secure random number generator")
	}

	rand.Seed(int64(binary.LittleEndian.Uint64(b[:])))
	randValue = rand.Int63()
	// generate a random value with length of 4
	randValue = randValue % 10000
	fileName := slug + "_" + strconv.FormatInt(randValue, 4) + ".png"

	downloadFile(post.Data[0].Url, fileName)

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
	for _, network := range config.Network {
		waitGroup.Add(1)
		if network.Enabled {
			go ircClient(network, &waitGroup)
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

func ircClient(network Network, waitGroup *sync.WaitGroup) {
	defer waitGroup.Done()
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
	ctx := context.Background()

	processing = false

	ircConfig := irc.ClientConfig{
		Nick: network.Nick,
		Pass: ircServer.Pass,
		User: network.Nick,
		Name: network.Nick,
		Handler: irc.HandlerFunc(func(c *irc.Client, m *irc.Message) {
			// log.Println(m)

			// if m contains string banned
			if strings.Contains(m.Command, "ERROR") {
				// Reconnect if we get an error
				log.Println("Error: " + m.Command)
			}

			if m.Command == "001" {
				// On successful connect, attempt to join channels
				wait(network.Throttle)
				// iterate over chansList
				for j := 0; j < len(chansList); j++ {
					c.Write("JOIN " + chansList[j])
					wait(network.Throttle)
				}
			} else if m.Command == "PRIVMSG" {
				if c.FromChannel(m) == false {
					return
				}

				msg := m.Trailing()

				// don't do anything if message isn't prefixed w/ an exclamation
				if strings.HasPrefix(msg, "!") == false {
					return
				}

				// split command & prompt
				parts := strings.SplitN(msg, " ", 2)

				// require both command & prompt
				if len(parts) < 2 {
					return
				}

				cmd := parts[0]
				prompt := parts[1]

				switch cmd {
				case "!davinci":
					model = "text-davinci-002"
					cost = 0.0200
					break
				case "!ada":
					model = "text-ada-001"
					cost = 0.0004
					break
				case "!babbage":
					model = "text-babbage-001"
					cost = 0.0005
					break
				case "!dale":
					model = "dale"
					cost = 1
					break
				case "!ai":
					model = config.Openai.Model
					cost = 0.0020
					break
				default:
					return
				}

				message := strings.TrimSpace(prompt)

				// if the model is dale
				if model == "dale" {
					// Alert the irc chan that the bot is processing
					c.WriteMessage(&irc.Message{
						Command: "PRIVMSG",
						Params: []string{
							m.Params[0],
							"Processing Dal-E: " + message,
						},
					})

					daleResponse := daleRequest(message) + " - " + message

					c.WriteMessage(&irc.Message{
						Command: "PRIVMSG",
						Params: []string{
							m.Params[0],
							daleResponse,
						},
					})

					return
				}

				// non-dale models
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

				aiClient := gogpt.NewClient(rotateApiKey())
				// Perform the actual API request to openAI
				resp, err := aiClient.CreateCompletion(ctx, model, req)
				if err != nil {
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
						if len(sendString) > 300 {
							// write message to channel
							c.WriteMessage(&irc.Message{
								Command: "PRIVMSG",
								Params: []string{
									m.Params[0],
									sendString,
								},
							})
							wait(network.Throttle)
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
					wait(network.Throttle)
				}

				processing = false
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
	go ircClient(network, waitGroup)
}
