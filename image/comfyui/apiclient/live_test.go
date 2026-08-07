//go:build live

// Live integration smoke test against a real ComfyUI server. Skipped by
// default (requires the "live" build tag) and not run in CI. Run manually:
//
//	COMFYUI_HOST=192.168.0.145 COMFYUI_PORT=8190 \
//	    go test -tags live -run TestLiveSmoke ./image/comfyui/apiclient/...
package apiclient

import (
	"context"
	"io"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLiveSmoke(t *testing.T) {
	host := os.Getenv("COMFYUI_HOST")
	port := 8190
	if host == "" {
		t.Skip("COMFYUI_HOST not set; skipping live smoke test")
	}
	c := New(host, port)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Minimal text2image workflow: CheckpointLoader -> CLIPTextEncode(pos/neg) ->
	// EmptyLatentImage -> KSampler -> VAEDecode -> SaveImage. Uses the first
	// available checkpoint reported by /object_info so it needs no prior knowledge.
	ckpt := os.Getenv("COMFYUI_CKPT")
	if ckpt == "" {
		ckpt = "sdxl/mklanNextgenXl_v1.safetensors"
	}
	graph := map[string]any{
		"4": map[string]any{"class_type": "CheckpointLoaderSimple", "inputs": map[string]any{"ckpt_name": ckpt}},
		"5": map[string]any{"class_type": "EmptyLatentImage", "inputs": map[string]any{"width": 64, "height": 64, "batch_size": 1}},
		"6": map[string]any{"class_type": "CLIPTextEncode", "inputs": map[string]any{"clip": []any{"4", 1}, "text": "a tiny red square, low quality"}},
		"7": map[string]any{"class_type": "CLIPTextEncode", "inputs": map[string]any{"clip": []any{"4", 1}, "text": "bad"}},
		"3": map[string]any{"class_type": "KSampler", "inputs": map[string]any{
			"seed": 1, "steps": 2, "cfg": 7, "sampler_name": "euler", "scheduler": "normal", "denoise": 1.0,
			"model": []any{"4", 0}, "positive": []any{"6", 0}, "negative": []any{"7", 0}, "latent_image": []any{"5", 0}}},
		"8": map[string]any{"class_type": "VAEDecode", "inputs": map[string]any{"samples": []any{"3", 0}, "vae": []any{"4", 2}}},
		"9": map[string]any{"class_type": "SaveImage", "inputs": map[string]any{"filename_prefix": "aibird_smoke", "images": []any{"8", 0}}},
	}
	pid, err := c.QueuePrompt(ctx, graph)
	require.NoError(t, err)
	t.Logf("prompt_id=%s", pid)

	ch, stop, err := c.Connect(ctx)
	require.NoError(t, err)
	defer stop()

	var item OutputItem
	for m := range ch {
		t.Logf("  msg type=%-22s prompt=%s node=%s val=%d/%d generating=%v", m.Type, m.PromptID, m.Node, m.Value, m.Max, m.IsGenerating())
		if m.Type == TypeExecuted && m.PromptID == pid {
			if imgs, ok := m.Output["images"]; ok && len(imgs) > 0 {
				item = imgs[0]
			}
		}
		if m.IsTerminal() && m.PromptID == pid {
			stop()
		}
	}
	require.NotEmpty(t, item.Filename, "no output image received")

	rc, err := c.View(context.Background(), item.Filename, item.Subfolder, item.Type)
	require.NoError(t, err)
	defer rc.Close()
	n, err := io.Copy(io.Discard, rc)
	require.NoError(t, err)
	require.Greater(t, n, int64(0))
	t.Logf("downloaded %s (%d bytes)", item.Filename, n)

	require.NoError(t, c.Free(context.Background(), true, true))
	t.Log("LIVE SMOKE TEST PASSED")
}
