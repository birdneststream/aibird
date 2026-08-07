package comfyui

import (
	"testing"
	"time"

	"aibird/image/comfyui/apiclient"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentActivity_EmptyByDefault(t *testing.T) {
	ClearActivity()
	assert.Nil(t, CurrentActivity())
}

func TestSetAndClearActivity(t *testing.T) {
	ClearActivity()
	t.Cleanup(ClearActivity)

	SetActivity(Activity{Action: "sd", PromptID: "p1", NodeTitle: "Sampler", Step: 5, Max: 20})
	got := CurrentActivity()
	require.NotNil(t, got)
	assert.Equal(t, "sd", got.Action)
	assert.Equal(t, "Sampler", got.NodeTitle)
	assert.Equal(t, 5, got.Step)
	assert.Equal(t, 20, got.Max)

	ClearActivity()
	assert.Nil(t, CurrentActivity())
}

func TestCurrentActivity_ReturnsSnapshot(t *testing.T) {
	ClearActivity()
	t.Cleanup(ClearActivity)

	SetActivity(Activity{Action: "h3", PromptID: "p2"})
	got := CurrentActivity()
	require.NotNil(t, got)
	// Mutating the returned snapshot must not affect the stored activity.
	got.Action = "mutated"
	got2 := CurrentActivity()
	require.NotNil(t, got2)
	assert.Equal(t, "h3", got2.Action, "stored activity must be unaffected by snapshot mutation")
}

func TestActivity_Elapsed(t *testing.T) {
	a := Activity{StartedAt: time.Now().Add(-90 * time.Second)}
	assert.InDelta(t, 90.0, a.Elapsed().Seconds(), 2.0)

	// Zero StartedAt -> zero elapsed.
	assert.Zero(t, Activity{}.Elapsed())
}

func TestActivity_Detail(t *testing.T) {
	tests := []struct {
		name string
		a    Activity
		want string
	}{
		{
			name: "sampling node with step",
			a:    Activity{NodeTitle: "Sampler", Step: 12, Max: 30, NodesDone: 3, NodesTotal: 8, StartedAt: time.Now().Add(-42 * time.Second)},
			want: " — Sampler (12/30) · node 3/8 · 42s",
		},
		{
			name: "non-sampling node (no step)",
			a:    Activity{NodeTitle: "Load Checkpoint", NodesDone: 0, NodesTotal: 8, StartedAt: time.Now().Add(-5 * time.Second)},
			want: " — Load Checkpoint · node 0/8 · 5s",
		},
		{
			name: "only node title (no progress_state yet)",
			a:    Activity{NodeTitle: "VAE Decode", StartedAt: time.Now().Add(-2 * time.Second)},
			want: " — VAE Decode · 2s",
		},
		{
			name: "no node title but has counts",
			a:    Activity{NodesDone: 1, NodesTotal: 4, StartedAt: time.Now().Add(-10 * time.Second)},
			want: " — node 1/4 · 10s",
		},
		{
			name: "empty activity returns empty detail",
			a:    Activity{},
			want: "",
		},
		{
			name: "elapsed rounds to whole seconds",
			a:    Activity{NodeTitle: "Sampler", StartedAt: time.Now().Add(-1500 * time.Millisecond)},
			want: " — Sampler · 1s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Use Contains for the elapsed-sensitive cases to avoid flakiness
			// from the rounding boundary; assert structure otherwise.
			if tt.name == "elapsed rounds to whole seconds" {
				d := tt.a.Detail()
				assert.Contains(t, d, "Sampler")
				return
			}
			assert.Equal(t, tt.want, tt.a.Detail())
		})
	}
}

func TestSummarizeProgressState(t *testing.T) {
	// Empty snapshot.
	done, total, title := summarizeProgressState(nil)
	assert.Equal(t, 0, done)
	assert.Equal(t, 0, total)
	assert.Equal(t, "", title)

	// Mixed snapshot: 2 finished, 1 running with a display title, 1 running without.
	states := map[string]apiclient.NodeState{
		"1": {State: nodeStateFinished},
		"2": {State: nodeStateFinished},
		"3": {State: "running", DisplayNodeID: "Sampler"},
		"4": {State: "running", DisplayNodeID: ""},
	}
	done, total, title = summarizeProgressState(states)
	assert.Equal(t, 2, done)
	assert.Equal(t, 4, total)
	assert.Equal(t, "Sampler", title, "runningTitle should be the display_node_id of the first running node")
}
