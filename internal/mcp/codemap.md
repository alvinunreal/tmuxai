# internal/mcp/

## Responsibility
- Own the MCP integration layer: load/validate MCP server config, manage MCP client sessions, expose discovered tool metadata, execute remote MCP tool calls, and parse `<MCPToolCall>` directives from model output.
- Keep lifecycle safety (startup, reconnect, shutdown, and process cleanup) while tracking server health and availability for the rest of tmuxai.

## Design
- **Config types (`types.go`)**: `MCPConfig` and `ServerConfig` describe server transport/auth/runtime knobs; `ServerInfo` and `ToolDef` model runtime state and tool metadata.
- **Lifecycle manager (`MCPManager`)**: centralized coordinator with mutex-protected maps for:
  - server metadata (`servers`)
  - active sessions (`sessions`)
  - stdio process handles for process-group cleanup (`cmds`)
  - in-flight request counters (`inFlight`, lock-free sync.Map + atomic).
  Plus cached tool-definition text for prompt inclusion.
- **Transport/path abstraction**: transport resolution defaults empty `type` to command→stdio or URL→sse, then creates SDK transport for stdio / SSE / streamable-HTTP.
- **Validation/parsing**:
  - `LoadConfig` reads JSON, applies `${VAR}` expansion, validates required transport fields, and returns `nil,nil` if file is absent.
  - `ParseMCPToolCalls` extracts JSON calls from `<MCPToolCall>...</MCPToolCall>` blocks and strips them from text.
- **Registry**: `Registry` builds fully-qualified names `mcp__<server>__<tool>` from healthy servers for deterministic tool lookup.

## Data & Control Flow
1. **Load phase**: `DefaultConfigPath -> LoadConfig -> Validate`; expansion and structural validation happen before runtime use.
2. **Startup**: `NewMCPManager(cfg)` creates context and empty state; `Init` runs `initServer` for each configured server (parallel).
3. **initServer**: set pending status, create transport, connect (`Client.Connect`), fetch tools (`Tools`), then publish healthy status and tool list; failures are recorded as unhealthy with error text.
4. **Execution**:
   - Caller resolves a fully-qualified name via `Registry.Lookup`.
   - `ExecuteToolCall` increments in-flight counter, enforces timeout, gets session, parses arguments, calls tool, performs one reconnect retry on failure (`callWithRetry`), normalizes output (`extractText` + `SanitizeResult`), and returns error flag.
5. **Change/recovery**:
   - `Reload` computes add/restart/remove sets via config diff, drains in-flight calls, tears down impacted servers, reinitializes targets, updates config.
   - `ReconnectServer` targets one server; `Shutdown` cancels manager context, closes all sessions, and kills stdio process groups.
6. **Observability helpers**: `GetServerInfo`, `ListTools`, `ToolDefinitionsBlock`, and `InvalidateCache` support status/query and cached prompt formatting.

## Integration Points
- Depends on **go-sdk MCP** (`github.com/modelcontextprotocol/go-sdk/mcp`) for transport/session/call APIs.
- Emits structured diagnostics through **tmuxai/logger**.
- Reads/expands environment from the host process and loads config from `${HOME}/.config/tmuxai/mcp.json` via stdlib OS/JSON APIs.
- Invoked by higher application layers that build tool registries, execute tools on user/model requests, and trigger config reload/shutdown events.
