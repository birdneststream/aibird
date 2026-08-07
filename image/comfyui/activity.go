package comfyui

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// nodeStateFinished is the ComfyUI progress_state value indicating a node has
// completed execution (the other observed value is "running"). Kept as a const
// so the magic string isn't duplicated.
const nodeStateFinished = "finished"

// Activity is the live state of the currently-executing ComfyUI generation.
// There is at most one active generation at a time (single GPU, serialized
// queue), so it is tracked as a package-level singleton.
//
// executeAndCollect writes to it on every WebSocket frame; !status reads it to
// surface "what is the GPU doing right now" detail (current node, sampling step,
// overall node completion, elapsed) alongside the queue's command name.
type Activity struct {
	// Action is the workflow/command name being generated (e.g. "sd", "h3").
	Action string
	// PromptID is the ComfyUI prompt id for this generation.
	PromptID string
	// NodeTitle is the human-readable title of the currently-executing node
	// (resolved from the graph's _meta.title / class_type, or progress_state's
	// display_node_id). Empty until the first executing/progress_state frame.
	NodeTitle string
	// Step and Max are the per-node sampling progress (e.g. step 12 of 30).
	// Both are 0 while a non-sampling node (model load, VAE decode) runs.
	Step int
	Max  int
	// NodesDone and NodesTotal are the whole-graph completion counts derived
	// from progress_state snapshots. Both are 0 until the first progress_state
	// frame arrives.
	NodesDone  int
	NodesTotal int
	// StartedAt is when this generation began (for elapsed-time display).
	StartedAt time.Time
}

var (
	activityMu sync.RWMutex
	activity   *Activity
)

// SetActivity records (or replaces) the current generation's live state.
// Called by executeAndCollect on each progress frame. Safe for concurrent use.
// Takes a value (not a pointer) so the stored entry is a snapshot, not an alias
// of the caller's mutable local.
//
//nolint:gocritic // hugeParam: Activity is a small immutable snapshot; value copy is intentional.
func SetActivity(a Activity) {
	activityMu.Lock()
	activity = &a
	activityMu.Unlock()
}

// ClearActivity removes the current generation state. Called on terminal
// (success/error/interruption/cancel) so a stale entry can't linger.
func ClearActivity() {
	activityMu.Lock()
	activity = nil
	activityMu.Unlock()
}

// CurrentActivity returns a snapshot of the current generation state, or nil if
// no generation is active. The returned pointer is safe to read without
// further locking; callers should not mutate it.
func CurrentActivity() *Activity {
	activityMu.RLock()
	defer activityMu.RUnlock()
	if activity == nil {
		return nil
	}
	a := *activity
	return &a
}

// Elapsed returns the duration since the generation started, or 0 if StartedAt
// is unset.
//
//nolint:gocritic // hugeParam: read-only snapshot; value receiver preferred.
func (a Activity) Elapsed() time.Duration {
	if a.StartedAt.IsZero() {
		return 0
	}
	return time.Since(a.StartedAt)
}

// Detail renders the live-progress detail for the !status line, e.g.
// " — Sampler (12/30) · node 4/8 · 42s". Returns "" when there is nothing
// meaningful to show (no node title and no elapsed time).
//
//nolint:gocritic // hugeParam: read-only snapshot; value receiver preferred.
func (a Activity) Detail() string {
	var parts []string

	// Current node, with sampling step when the node emits progress.
	if a.NodeTitle != "" {
		if a.Max > 0 {
			parts = append(parts, fmt.Sprintf("%s (%d/%d)", a.NodeTitle, a.Step, a.Max))
		} else {
			parts = append(parts, a.NodeTitle)
		}
	}

	// Whole-graph completion (only once progress_state has reported).
	if a.NodesTotal > 0 {
		parts = append(parts, fmt.Sprintf("node %d/%d", a.NodesDone, a.NodesTotal))
	}

	// Elapsed time, rounded to the nearest second.
	if secs := int(a.Elapsed().Seconds()); secs > 0 {
		parts = append(parts, fmt.Sprintf("%ds", secs))
	}

	if len(parts) == 0 {
		return ""
	}
	return " — " + strings.Join(parts, " · ")
}
