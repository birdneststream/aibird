package main

import (
	"math/rand"
	"time"
)

type (
	Config struct {
		OpenAI   OpenAI
		Networks map[string]Network
		AiBird   AiBird
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

	AiBird struct {
		Admin []Admin
	}

	Admin struct {
		Host  string
		Ident string
	}

	ircMeta struct {
		Network string
		Channel string
		Nicks   string
	}
	ircMetaList struct {
		ircMeta []ircMeta
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

// Dall-E responses
type DalEUrl struct {
	Url string `json:"url"`
}
type DalE struct {
	Created int64     `json:"created"`
	Data    []DalEUrl `json:"data"`
}
