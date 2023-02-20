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
		ProtectedHosts []ProtectedHosts
		Debug          bool
		UseIpv6        bool
	}

	// Auto +o on join and admin features
	ProtectedHosts struct {
		Host  string
		Ident string
		Admin bool
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

	// Stable Diffusion
	StableDiffusion struct {
		NegativePrompt string
		Steps          int
		Sampler        string
		RestoreFace    bool
		CfgScale       float32
		Host           string
		Width          int
		Height         int
	}

	// Txt2Img API Request
	StableDiffusionRequest struct {
		EnableHr          bool                   `json:"enable_hr"`
		DenoisingStrength int                    `json:"denoising_strength"`
		FirstphaseWidth   int                    `json:"firstphase_width"`
		FirstphaseHeight  int                    `json:"firstphase_height"`
		Prompt            string                 `json:"prompt"`
		Styles            []string               `json:"styles"`
		Seed              int                    `json:"seed"`
		Subseed           int                    `json:"subseed"`
		SubseedStrength   int                    `json:"subseed_strength"`
		SeedResizeFromH   int                    `json:"seed_resize_from_h"`
		SeedResizeFromW   int                    `json:"seed_resize_from_w"`
		BatchSize         int                    `json:"batch_size"`
		NIter             int                    `json:"n_iter"`
		Steps             int                    `json:"steps"`
		CfgScale          float32                `json:"cfg_scale"`
		Width             int                    `json:"width"`
		Height            int                    `json:"height"`
		RestoreFaces      bool                   `json:"restore_faces"`
		Tiling            bool                   `json:"tiling"`
		NegativePrompt    string                 `json:"negative_prompt"`
		Eta               int                    `json:"eta"`
		SChurn            int                    `json:"s_churn"`
		STmax             int                    `json:"s_tmax"`
		STmin             int                    `json:"s_tmin"`
		SNoise            int                    `json:"s_noise"`
		OverrideSettings  map[string]interface{} `json:"override_settings"`
		SamplerIndex      string                 `json:"sampler_index"`
	}

	// Txt2Img API response
	StableDiffusionResponse struct {
		Images []string `json:"images"`
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
