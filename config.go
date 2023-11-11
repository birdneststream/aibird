package main

import (
	"math/rand"
	"time"
)

type (
	Config struct {
		OpenAI          OpenAI
		Networks        map[string]Network
		AiBird          AiBird
		StableDiffusion StableDiffusion
		Proxy           Proxy
		RecordingUrl    string
		LocalAi         LocalAi
		Bard            Bard
	}

	// Proxy
	Proxy struct {
		Enabled  bool
		Type     string
		Username string
		Password string
		Host     string
		Port     int
	}

	// IRC
	Network struct {
		Nick         string
		Servers      []Server
		Channels     []string
		Enabled      bool
		Throttle     time.Duration
		Burst        int
		NickServPass string
		Pass         string
	}

	Server struct {
		Host string
		Port int
		Pass string
		Ssl  bool
		Ipv6 bool
	}

	// OpenAI
	OpenAI struct {
		Keys        []string
		Tokens      int
		Model       string
		Temperature float32
		currentKey  int
		Gpt4Key     string
	}

	// AiBird specific configurations
	AiBird struct {
		ProtectedHosts         []ProtectedHosts
		Debug                  bool
		Showchat               bool
		ChatPersonality        string
		ReplyToChats           bool
		ReplyChance            int
		ReplyTotalMessages     int
		IgnoreChatsFrom        []string
		ChatGptTotalMessages   int
		FloodThresholdMessages int
		FloodThresholdSeconds  time.Duration
		FloodIgnoreTime        time.Duration
	}

	// Auto +o on join and admin features
	ProtectedHosts struct {
		Host  string
		Ident string
		Admin bool
	}

	// Stable Diffusion Configuration
	StableDiffusion struct {
		NegativePrompt string
		Steps          int
		Sampler        string
		RestoreFace    bool
		CfgScale       float32
		Host           string
		Width          int
		Height         int
		BadWords       []string
		BadWordsPrompt string
	}

	// LocalAI
	LocalAi struct {
		Enabled     bool
		Temperature float32
		Model       string
		Host        string
		MaxTokens   int
	}

	// Bard
	Bard struct {
		Enabled   bool
		Host      string
		SessionId string
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
