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

	// IRC
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

	// OpenAI
	OpenAI struct {
		Keys        []string
		Tokens      int
		Model       string
		Temperature float32
		currentKey  int
	}

	// AiBird specific configurations
	AiBird struct {
		Admin   []Admin
		AutoOps []AutoOps
		Debug   bool
	}

	// Auto +o on join
	AutoOps struct {
		Host  string
		Ident string
	}

	// Auto +o on join and admin features
	Admin struct {
		Host  string
		Ident string
	}

	// Caching of each channel and their user modes
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
