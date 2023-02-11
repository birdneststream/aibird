package main

import (
	"math/rand"
	"time"
)

type (
	Config struct {
		OpenAI   OpenAI
		Networks map[string]Network
	}

	Network struct {
		Nick     string
		Servers  []Server
		Channels []string
		Enabled  bool
		Throttle time.Duration
		Burst    int
	}

	Server struct {
		Host string
		Port int
		Pass string
		Ssl  bool
	}

	OpenAI struct {
		Keys        []string
		Tokens      int
		Model       string
		Temperature float32
		currentKey  int
	}
)

func (network *Network) returnRandomServer() Server {
	return network.Servers[rand.Intn(len(network.Servers))]
}

func (openAI *OpenAI) nextApiKey() string {
	// Rotate API key
	openAI.currentKey = (openAI.currentKey + 1) % len(openAI.Keys)
	return openAI.Keys[openAI.currentKey]
}
