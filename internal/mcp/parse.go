package mcp

import (
	"encoding/json"
	"regexp"

	"github.com/alvinunreal/tmuxai/logger"
)

type MCPToolCall struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

var mcpToolCallPattern = regexp.MustCompile(`(?s)<MCPToolCall>(.*?)</MCPToolCall>`)

func ParseMCPToolCalls(response string) ([]MCPToolCall, string) {
	matches := mcpToolCallPattern.FindAllStringSubmatch(response, -1)
	if len(matches) == 0 {
		return nil, response
	}

	var calls []MCPToolCall
	for _, m := range matches {
		body := m[1]
		var call MCPToolCall
		if err := json.Unmarshal([]byte(body), &call); err != nil {
			logger.Info("MCP: skipping invalid <MCPToolCall> JSON: %v", err)
			continue
		}
		if call.Name == "" {
			logger.Info("MCP: skipping <MCPToolCall> with empty name")
			continue
		}
		calls = append(calls, call)
	}

	cleaned := mcpToolCallPattern.ReplaceAllString(response, "")
	return calls, cleaned
}
