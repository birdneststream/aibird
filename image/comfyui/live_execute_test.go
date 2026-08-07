//go:build live

// Validates the full direct-client execute path against a real ComfyUI server
// using a synthetic API-format workflow. Exercises: sidecar meta load,
// applyParameters (named inputs), graph clone, ApplyInput, QueuePrompt,
// WS stream with prompt_id filtering, View download. Run manually:
//
//	COMFYUI_HOST=192.168.0.145 go test -tags live -run TestLiveExecutePath -v ./image/comfyui/...
package comfyui

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLiveExecutePath(t *testing.T) {
	host := os.Getenv("COMFYUI_HOST")
	if host == "" {
		t.Skip("COMFYUI_HOST not set")
	}

	// Write the API-format graph + sidecar meta to a temp dir.
	dir := t.TempDir()
	graphPath := filepath.Join(dir, "live.json")
	require.NoError(t, os.WriteFile(graphPath, []byte(liveGraphJSON), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "live.meta.toml"), []byte(liveMetaTOML), 0644))

	irc := newState(nil, "live", "a tiny red square")
	metaData, err := GetAibirdMeta(graphPath)
	require.NoError(t, err)

	// Use the real execute path with a short ctx. processWithMeta needs a
	// *state.State with Config; we drive executeAndCollect directly to avoid
	// IRC plumbing, injecting an apiclient built from config.
	client := newLiveClient(t, host)
	defer func() {
		fctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = client.Free(fctx, true, true)
	}()

	cudaDevice := "cuda:0"
	updates, err := applyParameters(irc, metaData, "a tiny red square", cudaDevice)
	require.NoError(t, err)

	// executeAndCollect needs an *state.State for the wan rate-limit branch; the
	// synthetic flow has no "wan" model so it won't trigger. Reuse irc, with a
	// bounded ctx.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	filename, err := executeAndCollect(ctx, irc, client, "live", graphPath, updates, metaData)
	require.NoError(t, err)
	require.NotEmpty(t, filename)

	f, err := os.Open(filename)
	require.NoError(t, err)
	defer f.Close()
	n, err := io.Copy(io.Discard, f)
	require.NoError(t, err)
	require.Greater(t, n, int64(0))
	t.Logf("live execute path produced %s (%d bytes)", filename, n)
}
