package mcp

import (
	"encoding/json"
	"strings"
	"testing"

	"aibird/text"
)

func TestToolJSONSchemas(t *testing.T) {
	// Verify tool schemas are valid JSON
	tools := DefaultMcpTools()
	if len(tools) != 2 {
		t.Fatalf("expected 2 default tools, got %d", len(tools))
	}

	for _, tool := range tools {
		if tool.Type != "function" {
			t.Errorf("tool %s: expected type 'function', got %q", tool.Function.Name, tool.Type)
		}
		if tool.Function.Name == "" {
			t.Error("tool name must not be empty")
		}
		if tool.Function.Description == "" {
			t.Error("tool description must not be empty")
		}
		if !json.Valid(tool.Function.Parameters) {
			t.Errorf("tool %s: parameters is not valid JSON", tool.Function.Name)
		}

		// Verify parameters have required fields
		var params map[string]interface{}
		if err := json.Unmarshal(tool.Function.Parameters, &params); err != nil {
			t.Errorf("tool %s: failed to unmarshal parameters: %v", tool.Function.Name, err)
		}
		if params["type"] != "object" {
			t.Errorf("tool %s: expected parameters type 'object', got %v", tool.Function.Name, params["type"])
		}
	}
}

func TestMcpRequestSerialization(t *testing.T) {
	content := "test message"
	req := McpRequest{
		ToolChoice: "auto",
		Tools:      DefaultMcpTools(),
	}
	req.Model = "test-model"
	req.Stream = false
	req.Messages = []text.Message{
		{Role: "user", Content: content},
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("failed to marshal McpRequest: %v", err)
	}

	// Verify key fields are present in JSON
	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"model":"test-model"`) {
		t.Error("expected model in JSON output")
	}
	if !strings.Contains(jsonStr, `"tool_choice":"auto"`) {
		t.Error("expected tool_choice in JSON output")
	}
	if !strings.Contains(jsonStr, `"web_search"`) {
		t.Error("expected web_search tool in JSON output")
	}
	if !strings.Contains(jsonStr, `"scrape_url"`) {
		t.Error("expected scrape_url tool in JSON output")
	}
}

func TestMcpResponseDeserialization(t *testing.T) {
	// Response with tool_calls
	jsonStr := `{
		"id": "chatcmpl-123",
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": null,
				"tool_calls": [{
					"id": "call_abc123",
					"type": "function",
					"function": {
						"name": "web_search",
						"arguments": "{\"query\":\"Go 1.25 release\"}"
					}
				}]
			},
			"finish_reason": "tool_calls"
		}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 20, "total_tokens": 120}
	}`

	var resp McpResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal McpResponse: %v", err)
	}

	if resp.ID != "chatcmpl-123" {
		t.Errorf("expected id 'chatcmpl-123', got %q", resp.ID)
	}
	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	choice := resp.Choices[0]
	if choice.FinishReason != "tool_calls" {
		t.Errorf("expected finish_reason 'tool_calls', got %q", choice.FinishReason)
	}
	if choice.Message.Content != nil {
		t.Errorf("expected null content, got %v", choice.Message.Content)
	}
	if len(choice.Message.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(choice.Message.ToolCalls))
	}
	tc := choice.Message.ToolCalls[0]
	if tc.ID != "call_abc123" {
		t.Errorf("expected tool call id 'call_abc123', got %q", tc.ID)
	}
	if tc.Function.Name != "web_search" {
		t.Errorf("expected function name 'web_search', got %q", tc.Function.Name)
	}

	// Verify arguments can be parsed
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
		t.Fatalf("failed to parse tool call arguments: %v", err)
	}
	if args["query"] != "Go 1.25 release" {
		t.Errorf("expected query 'Go 1.25 release', got %v", args["query"])
	}
}

func TestMcpResponseWithContent(t *testing.T) {
	jsonStr := `{
		"id": "chatcmpl-456",
		"model": "test-model",
		"choices": [{
			"index": 0,
			"message": {
				"role": "assistant",
				"content": "The answer is 42."
			},
			"finish_reason": "stop"
		}]
	}`

	var resp McpResponse
	if err := json.Unmarshal([]byte(jsonStr), &resp); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if len(resp.Choices) != 1 {
		t.Fatalf("expected 1 choice, got %d", len(resp.Choices))
	}
	if resp.Choices[0].Message.Content == nil || *resp.Choices[0].Message.Content != "The answer is 42." {
		t.Error("expected content 'The answer is 42.'")
	}
	if resp.Choices[0].FinishReason != "stop" {
		t.Errorf("expected finish_reason 'stop', got %q", resp.Choices[0].FinishReason)
	}
	if len(resp.Choices[0].Message.ToolCalls) != 0 {
		t.Error("expected no tool calls")
	}
}

func TestToolMessageSerialization(t *testing.T) {
	msg := ToolMessage{
		Role:       "tool",
		Content:    "Search result data here",
		ToolCallID: "call_abc123",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("failed to marshal ToolMessage: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"role":"tool"`) {
		t.Error("expected role:tool in JSON")
	}
	if !strings.Contains(jsonStr, `"tool_call_id":"call_abc123"`) {
		t.Error("expected tool_call_id in JSON")
	}

	// Verify round-trip
	var decoded ToolMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal ToolMessage: %v", err)
	}
	if decoded.Role != "tool" {
		t.Errorf("expected role 'tool', got %q", decoded.Role)
	}
	if decoded.ToolCallID != "call_abc123" {
		t.Errorf("expected tool_call_id 'call_abc123', got %q", decoded.ToolCallID)
	}
}

func TestStringPtr(t *testing.T) {
	s := "hello"
	p := StringPtr(s)
	if p == nil || *p != s {
		t.Errorf("expected pointer to %q", s)
	}
}

func TestNullPtr(t *testing.T) {
	p := NullPtr()
	if p != nil {
		t.Error("expected nil pointer")
	}
}
