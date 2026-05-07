package mcp

import (
	"strings"
	"testing"
)

func TestSanitizeResultNormalText(t *testing.T) {
	raw := "Hello, world!"
	result := SanitizeResult(raw)
	if result != raw {
		t.Errorf("Expected passthrough, got %q", result)
	}
}

func TestSanitizeResultNullByte(t *testing.T) {
	raw := "hello\x00world"
	result := SanitizeResult(raw)
	if result != "[Binary data suppressed]" {
		t.Errorf("Expected suppressed binary, got %q", result)
	}
}

func TestSanitizeResultNullByteAtEnd(t *testing.T) {
	raw := "hello\x00"
	result := SanitizeResult(raw)
	if result != "[Binary data suppressed]" {
		t.Errorf("Expected suppressed binary, got %q", result)
	}
}

func TestSanitizeResultControlChars(t *testing.T) {
	raw := "hel\x01lo\x02wor\x07ld"
	result := SanitizeResult(raw)
	if result != "helloworld" {
		t.Errorf("Expected control chars stripped, got %q", result)
	}
}

func TestSanitizeResultPreservesTabNewlineCR(t *testing.T) {
	raw := "line1\nline2\ttab\rcr"
	result := SanitizeResult(raw)
	if result != raw {
		t.Errorf("Expected tab/newline/cr preserved, got %q", result)
	}
}

func TestSanitizeResultTruncation(t *testing.T) {
	raw := strings.Repeat("a", 100000)
	result := SanitizeResult(raw)
	if len(result) != hardCharLimit+len("\n[truncated]") {
		t.Errorf("Expected length %d, got %d", hardCharLimit+len("\n[truncated]"), len(result))
	}
	if !strings.HasSuffix(result, "\n[truncated]") {
		t.Errorf("Expected truncation indicator, got suffix: %q", result[len(result)-20:])
	}
}

func TestSanitizeResultAtThreshold(t *testing.T) {
	raw := strings.Repeat("a", 63001)
	result := SanitizeResult(raw)
	if len(result) != 63001 {
		t.Errorf("Expected no truncation at threshold, got length %d", len(result))
	}
}

func TestSanitizeResultJustUnderLimit(t *testing.T) {
	raw := strings.Repeat("b", hardCharLimit)
	result := SanitizeResult(raw)
	if len(result) != hardCharLimit {
		t.Errorf("Expected no truncation at exactly limit, got length %d", len(result))
	}
}

func TestSanitizeResultJustOverLimit(t *testing.T) {
	raw := strings.Repeat("c", hardCharLimit+1)
	result := SanitizeResult(raw)
	if !strings.HasSuffix(result, "\n[truncated]") {
		t.Errorf("Expected truncation indicator for input over limit")
	}
}

func TestSanitizeResultEmpty(t *testing.T) {
	result := SanitizeResult("")
	if result != "" {
		t.Errorf("Expected empty string, got %q", result)
	}
}

func TestSanitizeResultNullInLargePayload(t *testing.T) {
	raw := strings.Repeat("x", 4000) + "\x00" + strings.Repeat("y", 20000)
	result := SanitizeResult(raw)
	if result != "[Binary data suppressed]" {
		t.Errorf("Expected binary suppressed when null byte in first 8K, got %q", result[:30])
	}
}

func TestSanitizeResultNullPastScanLimit(t *testing.T) {
	raw := strings.Repeat("x", 9000) + "\x00" + strings.Repeat("y", 1000)
	result := SanitizeResult(raw)
	if result == "[Binary data suppressed]" {
		t.Error("Should not suppress when null byte is past binaryScanLimit")
	}
}

func TestRegistryLookupFound(t *testing.T) {
	r := &Registry{tools: map[string]ToolEntry{
		"mcp__srv__tool1": {ServerName: "srv", ToolName: "tool1"},
	}}
	entry, ok := r.Lookup("mcp__srv__tool1")
	if !ok {
		t.Error("Expected to find entry")
	}
	if entry.ToolName != "tool1" {
		t.Errorf("Expected tool1, got %s", entry.ToolName)
	}
}

func TestRegistryLookupNotFound(t *testing.T) {
	r := &Registry{tools: map[string]ToolEntry{}}
	_, ok := r.Lookup("mcp__srv__tool1")
	if ok {
		t.Error("Expected not found")
	}
}

func TestRegistryAllNames(t *testing.T) {
	r := &Registry{tools: map[string]ToolEntry{
		"mcp__z__tool":  {ServerName: "z", ToolName: "tool"},
		"mcp__a__tool":  {ServerName: "a", ToolName: "tool"},
		"mcp__m__tool":  {ServerName: "m", ToolName: "tool"},
	}}
	names := r.AllNames()
	if len(names) != 3 {
		t.Fatalf("Expected 3 names, got %d", len(names))
	}
	if names[0] != "mcp__a__tool" || names[1] != "mcp__m__tool" || names[2] != "mcp__z__tool" {
		t.Errorf("Expected sorted names, got %v", names)
	}
}

func TestRegistryAllNamesEmpty(t *testing.T) {
	r := &Registry{tools: map[string]ToolEntry{}}
	names := r.AllNames()
	if len(names) != 0 {
		t.Errorf("Expected empty, got %v", names)
	}
}
