# Repository Atlas: tmuxai

## Project Responsibility
TmuxAI is a Go command-line application that embeds an AI pair-programming assistant into a tmux window. It observes tmux panes, builds prompt context from terminal state and configured knowledge sources, calls model providers, and can route approved actions back into tmux panes with safety checks.

## System Entry Points
- `main.go`: process entrypoint; initializes the singleton file logger and invokes the Cobra CLI.
- `cli/cli.go`: user-facing command surface, flag parsing, config loading, manager construction, and interactive session start.
- `go.mod`: module/dependency manifest for CLI, tmux formatting, provider SDKs, web extraction, MCP, and config dependencies.
- `.goreleaser.yml`: release packaging configuration for built binaries.
- `.github/workflows/*.yml`: CI/release automation for tests, stale issue/PR handling, and release flows.

## Runtime Architecture
1. `main.go` initializes `logger` and calls `cli.Execute()`.
2. `cli/` loads `config`, resolves CLI overrides, and constructs `internal.Manager`.
3. `internal/` captures tmux context via `system/`, composes prompts, loads KB/skills/tool definitions, calls AI providers, parses responses, and enforces confirmation/risk handling before execution.
4. `internal/mcp/` optionally manages MCP server sessions and remote tool execution for model-requested tool calls.
5. `system/` performs tmux subprocess calls and formats terminal-facing output.

## Directory Map
| Directory | Responsibility Summary | Detailed Map |
|-----------|------------------------|--------------|
| `cli/` | Cobra-based CLI adapter that translates flags, config, initial requests, and signals into `internal.Manager` lifecycle calls. | [cli/codemap.md](cli/codemap.md) |
| `config/` | Typed runtime configuration schema, defaults, Viper loading, environment binding/expansion, and config/KB path helpers. | [config/codemap.md](config/codemap.md) |
| `internal/` | Core orchestration layer for tmux-aware chat, prompt/context assembly, provider calls, response parsing, safety checks, execution, KB/skills, web helpers, and history management. | [internal/codemap.md](internal/codemap.md) |
| `internal/mcp/` | MCP integration layer for server config, client lifecycle, tool discovery, prompt definitions, tool-call parsing, execution, reconnect, reload, and shutdown. | [internal/mcp/codemap.md](internal/mcp/codemap.md) |
| `system/` | Tmux subprocess facade plus pane metadata enrichment, command delivery, OS/process helpers, and terminal formatting/cosmetics utilities. | [system/codemap.md](system/codemap.md) |
| `logger/` | Process-global file logger writing severity-tagged diagnostics to `~/.config/tmuxai/tmuxai.log`. | [logger/codemap.md](logger/codemap.md) |

## Key Integration Boundaries
- **Tmux boundary:** `system/` executes `tmux`, `ps`, and related process-introspection commands; `internal/` consumes these helpers to observe panes and send approved actions.
- **AI provider boundary:** `internal/ai_client.go` and provider-specific support code call configured model backends (OpenAI-compatible, OpenRouter, Azure, Gemini/GenAI, Bedrock, Copilot SDK paths as configured by code).
- **Tooling boundary:** `internal/mcp/`, web search/fetch helpers, skills, and knowledge-base loading provide non-tmux context and actions to the core manager.
- **Safety boundary:** `internal/risk_scorer.go`, confirmation handling, yolo/session overrides, and execution-pane routing mediate model-suggested commands before they reach tmux.
