// Package apiclient implements a minimal, dependency-light client for the
// ComfyUI HTTP/WebSocket API. It is intended to replace the richinsley/comfy2go
// dependency by talking to ComfyUI's native REST endpoints directly.
//
// Supported endpoints (confirmed against a live ComfyUI 0.30.0 server):
//   - POST /prompt            — queue a workflow graph
//   - WS   /ws?clientId=<id>  — execution progress + output stream
//   - GET  /view              — fetch an output artifact's bytes
//   - GET  /history/{id}      — output discovery fallback
//   - POST /interrupt         — stop the currently-executing prompt
//   - POST /free              — unload models / free VRAM
//
// The WebSocket protocol uses JSON messages of the shape {"type": "...", "data": {...}}.
// Built-in message types observed on ComfyUI 0.30.0: status, execution_start,
// execution_cached, executing, progress, progress_state, executed, execution_error,
// execution_success, execution_interrupted. Unknown types are preserved on
// Message.Raw and exposed via Message.Type so callers can ignore them gracefully
// across version drift.
package apiclient

import (
	"encoding/json"
	"fmt"
)

// ComfyUI WebSocket message type constants.
const (
	TypeStatus               = "status"
	TypeExecutionStart       = "execution_start"
	TypeExecutionCached      = "execution_cached"
	TypeExecuting            = "executing"
	TypeProgress             = "progress"
	TypeProgressState        = "progress_state" // ComfyUI >= 0.3.x whole-graph snapshot
	TypeExecuted             = "executed"
	TypeExecutionError       = "execution_error"
	TypeExecutionSuccess     = "execution_success"
	TypeExecutionInterrupted = "execution_interrupted"
)

// NodeState is one entry in a progress_state snapshot, describing a single
// node's execution status. State is "running" or "finished" (ComfyUI may add
// more states in the future; callers should treat unknown states as active).
type NodeState struct {
	Value         float64 `json:"value"`
	Max           float64 `json:"max"`
	State         string  `json:"state"`
	NodeID        string  `json:"node_id"`
	DisplayNodeID string  `json:"display_node_id"`
	RealNodeID    string  `json:"real_node_id"`
}

// IsRunning reports whether the node is currently executing.
//
//nolint:gocritic // hugeParam: NodeState is a small immutable snapshot; value semantics preferred.
func (n NodeState) IsRunning() bool {
	return n.State == "running"
}

// OutputItem describes a single output artifact emitted by a node (image, gif,
// or audio). All three fields are required by ComfyUI's /view endpoint.
type OutputItem struct {
	Filename  string `json:"filename"`
	Subfolder string `json:"subfolder"`
	Type      string `json:"type"`
}

// Message is the parsed form of a single ComfyUI WebSocket message. Only the
// fields relevant to the message Type are populated; the rest retain their zero
// values. Callers should switch on Type and read the relevant fields.
type Message struct {
	// Type is the raw ComfyUI message type string. Unknown types are reported
	// verbatim so callers can ignore them.
	Type string

	// PromptID is populated for every message that carries it (most do, including
	// progress and progress_state). The connection-level "status" message has none.
	PromptID string

	// Executing / Progress / Executed: the node id the event concerns.
	Node string

	// Executing: when data.node is null (execution finished for this prompt),
	// ExecutingDone is true.
	ExecutingDone bool

	// Progress: current step value and maximum step count.
	Value int
	Max   int

	// Executed: Output maps the output kind ("images", "gifs", "audio") to the
	// list of artifacts produced by Node.
	Output map[string][]OutputItem

	// ProgressState is the whole-graph snapshot for progress_state messages.
	// It maps node id -> node state. Use IsGenerating() / RunningNodeIDs() to
	// determine liveness.
	ProgressState map[string]NodeState

	// ExecutionError: the exception details reported by ComfyUI.
	ExceptionType    string
	ExceptionMessage string

	// Raw is the original JSON of the message's "data" payload, retained so
	// callers (or future fields) can inspect anything not yet modeled.
	Raw json.RawMessage
}

// IsTerminal reports whether this message signals the end of prompt execution
// (success, error, or interruption). Consumers can stop reading the message
// stream once a terminal message for their prompt id has arrived.
//
//nolint:gocritic // hugeParam: Message is an immutable snapshot; value semantics preferred.
func (m Message) IsTerminal() bool {
	switch m.Type {
	case TypeExecutionError, TypeExecutionSuccess, TypeExecutionInterrupted:
		return true
	default:
		return false
	}
}

// IsGenerating reports whether, according to a progress_state snapshot, at least
// one node is currently running. This is the authoritative "generation is still
// happening" signal and is more reliable than the step-level "progress" message
// (which only fires during active sampling). Returns false for non-progress_state
// messages or snapshots where every node is finished.
//
// Consumers can use this to distinguish "working on a non-sampling step (e.g.
// model load, VAE decode)" from "stalled".
//
//nolint:gocritic // hugeParam: Message is an immutable snapshot; value semantics preferred.
func (m Message) IsGenerating() bool {
	if m.Type != TypeProgressState || m.ProgressState == nil {
		return false
	}
	for _, ns := range m.ProgressState {
		if ns.IsRunning() {
			return true
		}
	}
	return false
}

// RunningNodeIDs returns the node ids that are currently in the "running" state
// in a progress_state snapshot. Returns nil for non-progress_state messages.
//
//nolint:gocritic // hugeParam: Message is an immutable snapshot; value semantics preferred.
func (m Message) RunningNodeIDs() []string {
	if m.ProgressState == nil {
		return nil
	}
	var out []string
	for id, ns := range m.ProgressState {
		if ns.IsRunning() {
			out = append(out, id)
		}
	}
	return out
}

// wsEnvelope is the on-the-wire shape of a ComfyUI WebSocket message.
type wsEnvelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

type dataCommon struct {
	PromptID string `json:"prompt_id"`
}

type dataExecuting struct {
	PromptID string  `json:"prompt_id"`
	Node     *string `json:"node"` // nullable: null means "execution finished"
}

type dataProgress struct {
	PromptID string `json:"prompt_id"`
	Node     string `json:"node"`
	Value    int    `json:"value"`
	Max      int    `json:"max"`
}

type dataProgressState struct {
	PromptID string               `json:"prompt_id"`
	Nodes    map[string]NodeState `json:"nodes"`
}

type dataExecuted struct {
	PromptID string                  `json:"prompt_id"`
	Node     string                  `json:"node"`
	Output   map[string][]OutputItem `json:"output"`
}

type dataExecutionError struct {
	PromptID         string `json:"prompt_id"`
	ExceptionType    string `json:"exception_type"`
	ExceptionMessage string `json:"exception_message"`
}

// parseMessage decodes a raw WebSocket text frame into a Message. Unknown
// message types are still returned (with Type set) so callers can ignore them.
func parseMessage(raw []byte) (Message, error) {
	var env wsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return Message{}, fmt.Errorf("comfyui ws: invalid message envelope: %w", err)
	}

	msg := Message{Type: env.Type, Raw: env.Data}
	switch env.Type {
	case TypeExecutionStart, TypeExecutionCached, TypeExecutionSuccess,
		TypeExecutionInterrupted:
		var d dataCommon
		_ = json.Unmarshal(env.Data, &d) // best-effort; these carry optional prompt_id
		msg.PromptID = d.PromptID

	case TypeExecuting:
		var d dataExecuting
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return msg, fmt.Errorf("comfyui ws: invalid executing data: %w", err)
		}
		msg.PromptID = d.PromptID
		if d.Node == nil {
			msg.ExecutingDone = true
		} else {
			msg.Node = *d.Node
		}

	case TypeProgress:
		var d dataProgress
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return msg, fmt.Errorf("comfyui ws: invalid progress data: %w", err)
		}
		msg.PromptID = d.PromptID
		msg.Node = d.Node
		msg.Value = d.Value
		msg.Max = d.Max

	case TypeProgressState:
		var d dataProgressState
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return msg, fmt.Errorf("comfyui ws: invalid progress_state data: %w", err)
		}
		msg.PromptID = d.PromptID
		msg.ProgressState = d.Nodes

	case TypeExecuted:
		var d dataExecuted
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return msg, fmt.Errorf("comfyui ws: invalid executed data: %w", err)
		}
		msg.PromptID = d.PromptID
		msg.Node = d.Node
		msg.Output = d.Output

	case TypeExecutionError:
		var d dataExecutionError
		if err := json.Unmarshal(env.Data, &d); err != nil {
			return msg, fmt.Errorf("comfyui ws: invalid execution_error data: %w", err)
		}
		msg.PromptID = d.PromptID
		msg.ExceptionType = d.ExceptionType
		msg.ExceptionMessage = d.ExceptionMessage

	case TypeStatus:
		// Connection-level status: {status:{exec_info:{queue_remaining:N}}, sid:"..."}.
		// No prompt_id; nothing else to model here.

	default:
		// Unknown type: try to surface a prompt_id if one is present at the top
		// level of data (many future ComfyUI messages are likely to include it),
		// then leave Raw populated for the caller.
		var d dataCommon
		_ = json.Unmarshal(env.Data, &d)
		msg.PromptID = d.PromptID
	}
	return msg, nil
}
