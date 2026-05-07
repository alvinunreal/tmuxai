package internal

import (
	"reflect"
	"testing"

	"github.com/alvinunreal/tmuxai/internal/mcp"
)

func TestParseAIResponse_MCPToolCall(t *testing.T) {
	mgr := &Manager{}
	input := "Let me look that up.\n<MCPToolCall>{\"name\":\"read_file\",\"arguments\":{\"path\":\"/tmp/x\"}}</MCPToolCall>"
	got, err := mgr.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.MCPToolCalls) != 1 {
		t.Fatalf("Expected 1 MCPToolCall, got %d", len(got.MCPToolCalls))
	}
	if got.MCPToolCalls[0].Name != "read_file" {
		t.Errorf("Expected name read_file, got %s", got.MCPToolCalls[0].Name)
	}
	if got.Message != "Let me look that up." {
		t.Errorf("Expected message without MCP tag, got %q", got.Message)
	}
}

func TestParseAIResponse_MCPToolCallOnly(t *testing.T) {
	mgr := &Manager{}
	input := `<MCPToolCall>{"name":"search","arguments":{"q":"hello"}}</MCPToolCall>`
	got, err := mgr.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got.MCPToolCalls) != 1 {
		t.Fatalf("Expected 1 MCPToolCall, got %d", len(got.MCPToolCalls))
	}
	if got.MCPToolCalls[0].Name != "search" {
		t.Errorf("Expected search, got %s", got.MCPToolCalls[0].Name)
	}
}

func TestParseAIResponse_ExistingTagsUnaffectedByMCP(t *testing.T) {
	mgr := &Manager{}
	input := "<TmuxSendKeys>ls</TmuxSendKeys><MCPToolCall>{\"name\":\"tool\",\"arguments\":{}}</MCPToolCall><RequestAccomplished>1</RequestAccomplished>"
	got, err := mgr.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := AIResponse{
		SendKeys:            []string{"ls"},
		RequestAccomplished: true,
		MCPToolCalls:        []mcp.MCPToolCall{{Name: "tool", Arguments: []byte("{}")}},
	}
	if !reflect.DeepEqual(got.SendKeys, want.SendKeys) {
		t.Errorf("SendKeys: got %v, want %v", got.SendKeys, want.SendKeys)
	}
	if got.RequestAccomplished != want.RequestAccomplished {
		t.Errorf("RequestAccomplished: got %v, want %v", got.RequestAccomplished, want.RequestAccomplished)
	}
	if len(got.MCPToolCalls) != 1 || got.MCPToolCalls[0].Name != "tool" {
		t.Errorf("MCPToolCalls: got %v, want 1 call named tool", got.MCPToolCalls)
	}
}

func TestParseAIResponse_MCPToolCallsRemovedFromMessage(t *testing.T) {
	mgr := &Manager{}
	input := "Check this.\n<MCPToolCall>{\"name\":\"foo\",\"arguments\":{}}</MCPToolCall>\nDone."
	got, err := mgr.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Message != "Check this.\nDone." {
		t.Errorf("Expected MCP tags removed from message, got %q", got.Message)
	}
}

func TestParseAIResponse_NoMCPToolCalls(t *testing.T) {
	mgr := &Manager{}
	input := "Just a normal response."
	got, err := mgr.parseAIResponse(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.MCPToolCalls != nil {
		t.Errorf("Expected nil MCPToolCalls, got %v", got.MCPToolCalls)
	}
	if got.Message != "Just a normal response." {
		t.Errorf("Expected original message, got %q", got.Message)
	}
}
