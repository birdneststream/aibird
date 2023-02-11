package main

import "time"

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
	}
)
