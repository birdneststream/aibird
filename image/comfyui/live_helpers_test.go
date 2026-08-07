//go:build live

// Shared fixtures/helpers for the live (build-tagged) ComfyUI integration tests
// in the comfyui package. Skipped without the "live" build tag.
package comfyui

import (
	"context"
	"testing"
	"time"

	"aibird/image/comfyui/apiclient"

	"github.com/stretchr/testify/require"
)

func newLiveClient(t *testing.T, host string) *apiclient.Client {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = ctx
	port := 8190
	if host == "" {
		host = "192.168.0.145"
	}
	c := apiclient.New(host, port)
	require.NotNil(t, c)
	return c
}

// liveGraphJSON is a minimal API-format SDXL text2image workflow. Node 6 is the
// positive prompt (CLIPTextEncode, _meta.title "Positive").
const liveGraphJSON = `{
  "4": {"class_type": "CheckpointLoaderSimple", "inputs": {"ckpt_name": "sdxl/mklanNextgenXl_v1.safetensors"}, "_meta": {"title": "Load Checkpoint"}},
  "5": {"class_type": "EmptyLatentImage", "inputs": {"width": 64, "height": 64, "batch_size": 1}, "_meta": {"title": "EmptyLatentImage"}},
  "6": {"class_type": "CLIPTextEncode", "inputs": {"clip": ["4", 1], "text": "masterpiece best quality"}, "_meta": {"title": "Positive"}},
  "7": {"class_type": "CLIPTextEncode", "inputs": {"clip": ["4", 1], "text": "bad"}, "_meta": {"title": "Negative"}},
  "3": {"class_type": "KSampler", "inputs": {"seed": 1, "steps": 3, "cfg": 7, "sampler_name": "euler", "scheduler": "normal", "denoise": 1, "model": ["4", 0], "positive": ["6", 0], "negative": ["7", 0], "latent_image": ["5", 0]}, "_meta": {"title": "Sampler"}},
  "8": {"class_type": "VAEDecode", "inputs": {"samples": ["3", 0], "vae": ["4", 2]}, "_meta": {"title": "VAE Decode"}},
  "9": {"class_type": "SaveImage", "inputs": {"filename_prefix": "aibird_live_exec", "images": ["8", 0]}, "_meta": {"title": "Save Image"}}
}`

const liveMetaTOML = `
name = "live"
command = "live"
type = "image"
accessLevel = 0

[promptTarget]
node = "Positive"
input = "text"

[parameters.seed]
type = "int"
default = -1

[[parameters.seed.targets]]
node = "Sampler"
input = "seed"
`
