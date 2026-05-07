package mcp

import (
	"encoding/json"
	"testing"
)

func TestParseMCPToolCallsSingle(t *testing.T) {
	input := `<MCPToolCall>{"name":"read_file","arguments":{"path":"/tmp/x"}}</MCPToolCall>`
	calls, cleaned := ParseMCPToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read_file" {
		t.Errorf("Expected name read_file, got %s", calls[0].Name)
	}
	var args map[string]string
	if err := json.Unmarshal(calls[0].Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args["path"] != "/tmp/x" {
		t.Errorf("Expected path /tmp/x, got %s", args["path"])
	}
	if cleaned != "" {
		t.Errorf("Expected empty cleaned string, got %q", cleaned)
	}
}

func TestParseMCPToolCallsMultiple(t *testing.T) {
	input := `before <MCPToolCall>{"name":"a","arguments":{}}</MCPToolCall> middle <MCPToolCall>{"name":"b","arguments":{}}</MCPToolCall> after`
	calls, cleaned := ParseMCPToolCalls(input)
	if len(calls) != 2 {
		t.Fatalf("Expected 2 calls, got %d", len(calls))
	}
	if calls[0].Name != "a" || calls[1].Name != "b" {
		t.Errorf("Expected names a,b, got %s,%s", calls[0].Name, calls[1].Name)
	}
	if cleaned != "before  middle  after" {
		t.Errorf("Expected cleaned text, got %q", cleaned)
	}
}

func TestParseMCPToolCallsInvalidJSON(t *testing.T) {
	input := `<MCPToolCall>not-json</MCPToolCall><MCPToolCall>{"name":"valid","arguments":{}}</MCPToolCall>`
	calls, _ := ParseMCPToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 valid call (invalid skipped), got %d", len(calls))
	}
	if calls[0].Name != "valid" {
		t.Errorf("Expected name valid, got %s", calls[0].Name)
	}
}

func TestParseMCPToolCallsEmptyName(t *testing.T) {
	input := `<MCPToolCall>{"name":"","arguments":{}}</MCPToolCall>`
	calls, _ := ParseMCPToolCalls(input)
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls for empty name, got %d", len(calls))
	}
}

func TestParseMCPToolCallsEmptyJSON(t *testing.T) {
	input := `<MCPToolCall></MCPToolCall>`
	calls, _ := ParseMCPToolCalls(input)
	if len(calls) != 0 {
		t.Errorf("Expected 0 calls for empty JSON, got %d", len(calls))
	}
}

func TestParseMCPToolCallsNonePresent(t *testing.T) {
	input := `Just some text without any MCP tags.`
	calls, cleaned := ParseMCPToolCalls(input)
	if calls != nil {
		t.Errorf("Expected nil calls, got %v", calls)
	}
	if cleaned != input {
		t.Errorf("Expected original text returned, got %q", cleaned)
	}
}

func TestParseMCPToolCallsMixedWithOtherTags(t *testing.T) {
	input := `<ExecCommand>ls -la</ExecCommand><MCPToolCall>{"name":"search","arguments":{"q":"test"}}</MCPToolCall><TmuxSendKeys>enter</TmuxSendKeys>`
	calls, cleaned := ParseMCPToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "search" {
		t.Errorf("Expected search, got %s", calls[0].Name)
	}
	if cleaned != "<ExecCommand>ls -la</ExecCommand><TmuxSendKeys>enter</TmuxSendKeys>" {
		t.Errorf("Expected other tags preserved, got %q", cleaned)
	}
}

func TestParseMCPToolCallsMultiline(t *testing.T) {
	input := "<MCPToolCall>{\n  \"name\": \"read\",\n  \"arguments\": {\"path\": \"/foo\"}\n}</MCPToolCall>"
	calls, _ := ParseMCPToolCalls(input)
	if len(calls) != 1 {
		t.Fatalf("Expected 1 call, got %d", len(calls))
	}
	if calls[0].Name != "read" {
		t.Errorf("Expected read, got %s", calls[0].Name)
	}
}
