# internal/

## Responsibility
- Orchestrate tmux-aware AI interaction: parse user chat/commands, manage session state, call AI providers, parse model outputs, and execute resulting shell/tmux actions.
- Enforce safety and guardrails around execution (risk scoring, confirmation/edit path, timeout/watch controls).
- Provide runtime services used by the CLI loop: knowledge base + skill loading, web search/fetch, pane context capture, command generation, and response formatting.

## Design
- `Manager` is the central stateful coordinator (`internal/manager.go`): holds config/session overrides, pane/window IDs/history, MCP/web/KB/skill registries, and provider/runtime dependencies.
- Command handling is split into command-style channels in `chat.go` and `chat_command.go` (slash command parsing + mutating operations) versus assistant-style dialog handled by `process_message.go`.
- AI providers are abstracted behind `AIClient` methods and provider-specific constructors/config (`ai_client.go`, `bedrock.go`), with downstream decoding centralized in shared parsers (`process_response.go`, `process_response_from_openai.go`/legacy parser in same flow).
- Prompt composition is centralized in `prompts.go`, while contextual data assembly is pulled from pane/KB/skill helpers (`pane_details.go`, `exec_pane.go`, `knowledge_base.go`, `skill_registry.go`).
- Execution actions are dispatched via typed `AIResponse` paths (`process_response.go` + `exec_pane.go`): run command, type, scroll/page/paste, ask follow-up, or request confirmation.
- History pressure is managed explicitly in `squash.go` with a summarization path that rewrites conversation state before subsequent model calls.

## Data & Control Flow
- `chat.go` reads input → if slash command, `chat_command.go` mutates runtime/session settings; otherwise routes to `process_message`.
- `process_message.go` composes request context (prompt + pane snapshot + KB/skills/context), calls `AIClient`, optionally performs follow-up recursion, and handles MCP tool-result loops up to a bounded depth.
- Raw provider responses pass through parsing (`process_response.go`), producing structured action models; high-risk actions are tagged and optionally interrupted by confirmation (`risk_scorer.go`, `confirm.go`).
- Approved actions execute through pane utilities (`exec_pane.go`) and/or web helpers (`web_search.go`, `web_fetch.go`), while results are appended to chat history and persisted in `Manager` state.
- On token-limit pressure or policy thresholds, `squash.go` condenses prior history and appends compact summaries back into active conversation state.

## Integration Points
- CLI/session boundary: `chat.go` and `chat_command.go` are the external command/state interface consumed by the running process UI/loop.
- AI runtime boundary: provider layer (`ai_client.go`, `bedrock.go`) receives request text/metadata and returns content that is interpreted by response-processing code.
- Tmux runtime boundary: pane/window discovery and command injection via `exec_pane.go`, `pane_details.go`, and `countdown.go` for watch/automation behavior.
- Tooling/service boundaries: KB/skills (`knowledge_base.go`, `skill_registry.go`), search/fetch (`web_search*.go`, `web_fetch.go`), and MCP integration (`internal/mcp/codemap.md`).
- Safety boundary: risk and confirmation pipeline (`risk_scorer.go`, `confirm.go`) is enforced before command execution.
