package main

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"

	"github.com/richinsley/comfy2go/client"
	"github.com/schollz/progressbar/v3"
	"github.com/yunginnanet/girc-atomic"
)

func listJSONFiles() ([]string, error) {
	files, err := filepath.Glob("comfyui/*.json")
	if err != nil {
		return nil, err
	}
	return files, nil
}

func getFlows() string {
	// return list of files in comfyui/*.json
	flows := ""
	files, err := listJSONFiles()
	if err != nil {
		fmt.Println("Error:", err)
		return ""
	}
	for _, file := range files {
		fmt.Println(file)

		file = strings.ReplaceAll(file, ".json", "")
		file = strings.ReplaceAll(file, "comfyui/", "")

		flows = flows + "!" + file + ", "
	}

	return strings.TrimRight(flows, ", ")
}

func parseComfyUi(c *girc.Client, e girc.Event, workflow string, message string) bool {
	// return list of files in comfyui/*.json
	files, err := listJSONFiles()
	if err != nil {
		fmt.Println("Error:", err)
		return false
	}
	for _, file := range files {
		fmt.Println(file)

		// clean strings
		workflow = strings.ReplaceAll(workflow, "!", "")
		file = strings.ReplaceAll(file, ".json", "")
		file = strings.ReplaceAll(file, "comfyui/", "")

		// if workflow matches file
		if workflow == file {
			// queue the prompt and get the resulting image
			go comfyui(c, e, workflow, message)
			return true
		}

	}

	return false
}

func comfyui(irc *girc.Client, e girc.Event, workflow string, message string) {

	clientaddr := "10.17.0.3"
	clientport := 8188
	workflow = "comfyui/" + workflow + ".json"

	sendToIrc(irc, e, fmt.Sprintf("%s: Queuing '%s'...", e.Source.Name, message))

	// callbacks can be used respond to QueuedItem updates, or client status changes
	callbacks := &client.ComfyClientCallbacks{
		ClientQueueCountChanged: func(c *client.ComfyClient, queuecount int) {
			log.Printf("Client %s at %s Queue size: %d", c.ClientID(), clientaddr, queuecount)
		},
		QueuedItemStarted: func(c *client.ComfyClient, qi *client.QueueItem) {
			log.Printf("Queued item %s started\n", qi.PromptID)
			sendToIrc(irc, e, fmt.Sprintf("%s: Queued item '%s' has started processing... please wait.", e.Source.Name, message))
		},
		QueuedItemStopped: func(cc *client.ComfyClient, qi *client.QueueItem, reason client.QueuedItemStoppedReason) {
			log.Printf("Queued item %s stopped\n", qi.PromptID)
		},
		QueuedItemDataAvailable: func(cc *client.ComfyClient, qi *client.QueueItem, pmd *client.PromptMessageData) {
			log.Printf("image data available:\n")
			for _, v := range pmd.Images {
				log.Printf("\tFilename: %s Subfolder: %s Type: %s\n", v.Filename, v.Subfolder, v.Type)

			}
		},
	}

	// create a client
	c := client.NewComfyClient(clientaddr, clientport, callbacks)

	// the client needs to be in an initialized state before usage
	if !c.IsInitialized() {
		log.Printf("Initialize Client with ID: %s\n", c.ClientID())
		err := c.Init()
		if err != nil {
			log.Println("Error initializing client:", err)
			os.Exit(1)
		}
	}

	// load the workflow
	graph, _, err := c.NewGraphFromJsonFile(workflow)
	if err != nil {
		log.Println("Error loading graph JSON:", err)
		os.Exit(1)
	}

	simple_api := graph.GetNodesInGroup(graph.GetGroupWithTitle("API"))

	// foreach node in the API group, print
	for _, v := range simple_api {
		log.Printf("Node: %d Title: %s\n", v.ID, v.Title)

		switch v.Title {

		case "Positive":
			v.WidgetValues[0] = message

		case "Sampler":
			v.WidgetValues[0] = rand.Intn(999999999999999999)
		}

	}

	// queue the prompt and get the resulting image
	item, err := c.QueuePrompt(graph)
	if err != nil {
		log.Println("Failed to queue prompt:", err)
		os.Exit(1)
	}

	// we'll provide a progress bar
	var bar *progressbar.ProgressBar = nil

	// continuously read messages from the QueuedItem until we get the "stopped" message type
	var currentNodeTitle string
	for continueLoop := true; continueLoop; {
		msg := <-item.Messages
		switch msg.Type {
		case "started":
			qm := msg.ToPromptMessageStarted()
			log.Printf("Start executing prompt ID %s\n", qm.PromptID)
		case "executing":
			bar = nil
			qm := msg.ToPromptMessageExecuting()
			// store the node's title so we can use it in the progress bar
			currentNodeTitle = qm.Title
			log.Printf("Executing Node: %d\n", qm.NodeID)
		case "progress":
			// update our progress bar
			qm := msg.ToPromptMessageProgress()
			if bar == nil {
				bar = progressbar.Default(int64(qm.Max), currentNodeTitle)
			}
			bar.Set(qm.Value)
		case "stopped":
			// if we were stopped for an exception, display the exception message
			qm := msg.ToPromptMessageStopped()
			if qm.Exception != nil {
				log.Println(qm.Exception)
				os.Exit(1)
			}
			continueLoop = false
		case "data":
			qm := msg.ToPromptMessageData()
			for _, v := range qm.Images {
				img_data, err := c.GetImage(v)
				if err != nil {
					log.Println("Failed to get image:", err)
					os.Exit(1)
				}
				f, err := os.Create(v.Filename)
				if err != nil {
					log.Println("Failed to write image:", err)
					os.Exit(1)
				}
				f.Write(*img_data)
				f.Close()
				log.Println("Got image: ", v.Filename)

				content := fileHole("https://filehole.org/", v.Filename)

				content = e.Source.Name + ": " + message + " - " + content

				sendToIrc(irc, e, content)

				err = os.Remove(v.Filename)
				if err != nil {
					log.Println("Failed to remove image:", err)
					os.Exit(1)
				}

			}
		}
	}
}
