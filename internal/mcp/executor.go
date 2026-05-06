package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/alvinunreal/tmuxai/logger"
)

const (
	defaultCallTimeout = 30 * time.Second
	warnCharThreshold  = 63000
	hardCharLimit      = 84000
	binaryScanLimit    = 8192
)

func ExecuteToolCall(ctx context.Context, mgr *MCPManager, reg *Registry, fqName string, args json.RawMessage) (string, bool) {
	entry, ok := reg.Lookup(fqName)
	if !ok {
		return fmt.Sprintf("(tool not found: %s)", fqName), true
	}

	// Track in-flight calls so reload/shutdown can wait for completion
	mgr.TrackCallStart(entry.ServerName)
	defer mgr.TrackCallEnd(entry.ServerName)

	timeout := defaultCallTimeout
	if entry.ServerInfo.Config.TimeoutSeconds > 0 {
		timeout = time.Duration(entry.ServerInfo.Config.TimeoutSeconds) * time.Second
	}

	callCtx, cancel := context.WithTimeout(mgr.processLife, timeout)
	defer cancel()

	session := mgr.GetSession(entry.ServerName)
	if session == nil {
		if !LazyReconnect(mgr, entry.ServerName) {
			return fmt.Sprintf("(server %q unavailable)", entry.ServerName), true
		}
		session = mgr.GetSession(entry.ServerName)
	}

	var arguments map[string]any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &arguments); err != nil {
			return fmt.Sprintf("(invalid arguments for %s: %v)", fqName, err), true
		}
	}

	result, err := callWithRetry(callCtx, mgr, session, entry, arguments)
	if err != nil {
		if callCtx.Err() == context.DeadlineExceeded {
			return fmt.Sprintf("(timed out: %s.%s)", entry.ServerName, entry.ToolName), true
		}
		return fmt.Sprintf("(error calling %s: %v)", fqName, err), true
	}

	raw := extractText(result)
	return SanitizeResult(raw), result.IsError
}

// callWithRetry attempts a tool call and, on failure, tries one lazy reconnect before giving up.
func callWithRetry(ctx context.Context, mgr *MCPManager, session *mcpsdk.ClientSession, entry ToolEntry, arguments map[string]any) (*mcpsdk.CallToolResult, error) {
	result, err := session.CallTool(ctx, &mcpsdk.CallToolParams{
		Name:      entry.ToolName,
		Arguments: arguments,
	})
	if err != nil {
		logger.Info("MCP CallTool failed for %s.%s: %v, attempting reconnect", entry.ServerName, entry.ToolName, err)
		if LazyReconnect(mgr, entry.ServerName) {
			session = mgr.GetSession(entry.ServerName)
			if session != nil {
				result, err = session.CallTool(ctx, &mcpsdk.CallToolParams{
					Name:      entry.ToolName,
					Arguments: arguments,
				})
			}
		}
	}
	return result, err
}

func extractText(result *mcpsdk.CallToolResult) string {
	var b strings.Builder
	for _, c := range result.Content {
		if tc, ok := c.(*mcpsdk.TextContent); ok {
			b.WriteString(tc.Text)
		}
	}
	return b.String()
}

func SanitizeResult(raw string) string {
	scanEnd := len(raw)
	if scanEnd > binaryScanLimit {
		scanEnd = binaryScanLimit
	}
	for i := 0; i < scanEnd; i++ {
		if raw[i] == 0 {
			return "[Binary data suppressed]"
		}
	}

	var b strings.Builder
	b.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if ch < 32 && ch != '\t' && ch != '\n' && ch != '\r' {
			continue
		}
		b.WriteByte(ch)
	}
	cleaned := b.String()

	if len(cleaned) > warnCharThreshold {
		logger.Info("MCP tool result exceeds %d chars (%d), consider optimizing", warnCharThreshold, len(cleaned))
	}

	if len(cleaned) > hardCharLimit {
		cleaned = cleaned[:hardCharLimit] + "\n[truncated]"
	}

	return cleaned
}

func LazyReconnect(mgr *MCPManager, serverName string) bool {
	err := mgr.ReconnectServer(serverName)
	if err != nil {
		logger.Info("MCP lazy reconnect failed for %q: %v", serverName, err)
		return false
	}
	return true
}
