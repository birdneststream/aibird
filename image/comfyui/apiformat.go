package comfyui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"aibird/logger"

	"github.com/BurntSushi/toml"
)

// APIGraph is a ComfyUI workflow in API JSON format: a flat map of node id to
// node definition. This is the shape ComfyUI's "Save (API Format)" produces and
// the shape POST /prompt expects as the "prompt" body. (UI/editor-format files
// with nodes/links/groups arrays are no longer supported.)
type APIGraph map[string]APINode

// APINode is one node in an API-format graph.
type APINode struct {
	ClassType string                 `json:"class_type"`
	Inputs    map[string]interface{} `json:"inputs"`
	Meta      APINodeMeta            `json:"_meta"`
}

// APINodeMeta carries the node's editor title (preserved by ComfyUI's
// API-format export for real nodes; used to match meta targets by title).
type APINodeMeta struct {
	Title string `json:"title"`
}

// LoadAPIGraph reads and parses a ComfyUI API-format workflow JSON file.
func LoadAPIGraph(path string) (*APIGraph, error) {
	if strings.Contains(path, "..") {
		return nil, fmt.Errorf("invalid workflow file path: %s", path)
	}
	data, err := os.ReadFile(path) // #nosec G304 - path traversal guarded above
	if err != nil {
		return nil, fmt.Errorf("read api graph %s: %w", path, err)
	}
	var graph APIGraph
	if err := json.Unmarshal(data, &graph); err != nil {
		return nil, fmt.Errorf("decode api graph %s: %w", path, err)
	}
	if len(graph) == 0 {
		return nil, fmt.Errorf("api graph %s has no nodes", path)
	}
	return &graph, nil
}

// Clone returns a deep-enough copy of the graph so the caller can mutate it
// without affecting the on-disk-parsed original. Inputs maps are copied; the
// values themselves (strings/numbers/[]interface) are reused read-only.
func (g *APIGraph) Clone() *APIGraph {
	out := make(APIGraph, len(*g))
	for id, n := range *g {
		in := make(map[string]interface{}, len(n.Inputs))
		for k, v := range n.Inputs {
			in[k] = v
		}
		out[id] = APINode{ClassType: n.ClassType, Inputs: in, Meta: n.Meta}
	}
	return &out
}

// resolveNodeIDs returns the ids of all nodes matching ref. A node matches if
// its id (the graph key) equals ref, OR its _meta.title equals ref, OR its
// class_type equals ref. Matching by id is preferred for unambiguous targeting
// (API-format node ids are unique; titles like "Load VAE" can collide). Matching
// by title/class is retained for readability and matches the historical
// apply-to-all-matching semantics. Returns an error if no node matches, so
// misconfigured meta fails loudly instead of silently skipping.
func (g *APIGraph) resolveNodeIDs(ref string) ([]string, error) {
	// Direct id match is always unambiguous; return it immediately.
	if _, ok := (*g)[ref]; ok {
		return []string{ref}, nil
	}
	var ids []string
	for id, n := range *g {
		if n.Meta.Title == ref || n.ClassType == ref {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("no workflow node matches %q (by id, _meta.title, or class_type)", ref)
	}
	return ids, nil
}

// ApplyInput sets a named input on every node matching nodeRef. When
// appendIfString is true and the existing input is a non-empty string, the new
// string value is appended to it (space-separated) instead of replacing it.
// This carries forward the prompt-target concatenation behavior. A missing
// input name on the target node is NOT an error — some inputs are optional and
// may be absent until set.
func (g *APIGraph) ApplyInput(nodeRef, inputName string, value interface{}, appendIfString bool) error {
	ids, err := g.resolveNodeIDs(nodeRef)
	if err != nil {
		return err
	}
	for _, id := range ids {
		node := (*g)[id]
		if appendIfString {
			if existing, ok := node.Inputs[inputName].(string); ok && existing != "" {
				if sv, ok := value.(string); ok {
					value = existing + " " + sv
				}
			}
		}
		node.Inputs[inputName] = value
		(*g)[id] = node
		logger.Debug("applied input", "node", id, "ref", nodeRef, "input", inputName, "value", value)
	}
	return nil
}

// PromptJSON returns the graph in the map form POST /prompt expects
// (map[nodeID]{class_type, inputs, _meta}). Note nodes are absent from
// API-format exports, so there is nothing to strip.
func (g *APIGraph) PromptJSON() map[string]interface{} {
	out := make(map[string]interface{}, len(*g))
	for id, n := range *g {
		out[id] = map[string]interface{}{
			"class_type": n.ClassType,
			"inputs":     n.Inputs,
			"_meta":      n.Meta,
		}
	}
	return out
}

// SidecarMetaPath returns the sidecar metadata path for a given workflow file:
// the same base name with a ".meta.toml" suffix
// (e.g. "comfyuijson/sd.json" -> "comfyuijson/sd.meta.toml").
func SidecarMetaPath(workflowFile string) string {
	ext := filepath.Ext(workflowFile)
	return strings.TrimSuffix(workflowFile, ext) + ".meta.toml"
}

// loadSidecarMeta reads and decodes the sidecar .meta.toml for a workflow.
func loadSidecarMeta(workflowFile string) (*AibirdMeta, error) {
	metaPath := SidecarMetaPath(workflowFile)
	if strings.Contains(metaPath, "..") {
		return nil, fmt.Errorf("invalid workflow file path: %s", workflowFile)
	}
	data, err := os.ReadFile(metaPath) // #nosec G304 - path traversal guarded above
	if err != nil {
		return nil, fmt.Errorf("read sidecar meta %s: %w (expected a .meta.toml next to the workflow)", metaPath, err)
	}
	var meta AibirdMeta
	if _, err := toml.Decode(string(data), &meta); err != nil {
		logger.Error("Failed to decode sidecar meta TOML", "file", metaPath, "error", err)
		return nil, fmt.Errorf("decode sidecar meta %s: %w", metaPath, err)
	}
	return &meta, nil
}
