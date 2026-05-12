# system/

## Responsibility
- Encapsulates tmux runtime operations and output formatting helpers used by the app’s command/workflow layer.
- Provides pane discovery/introspection (`tmux`), pane command delivery, session/pane lifecycle helpers, and text/code formatting utilities (`Cosmetics`, `InfoFormatter`, `TmuxPaneDetails` helpers).
- Handles shell/process metadata lookup to classify pane context (command, shell, subshell, readiness).

## Design
- Functional package-level API style: most tmux actions are exposed as package variables/functions (`TmuxPanesDetails`, `TmuxCapturePane`, `TmuxSendCommandToPane`, etc.) for easy overriding in tests.
- Clear separation by concern:
  - `tmux.go` / `tmux_send.go`: external `tmux` command execution, parsing, and control flow.
  - `utils.go`: environment/process/formatting utilities (`GetProcessArgs`, `GetOSDetails`, `EstimateTokenCount`, map helpers).
  - `formatter.go` / `cosmetics.go`: presentation-layer output rendering with ANSI color/highlighting.
  - `types.go`: core model (`TmuxPaneDetails`) plus string/formatting helpers and refresh logic.
- Uses direct subprocess execution (`os/exec`) and stderr capture for diagnostics; returns structured errors with contextual logging via `logger`.
- Minimal internal state: state is passed via explicit function arguments or returned structs, not stored globally.

## Data & Control Flow
- tmux-facing functions build command arguments, execute `tmux`, and parse command output:
  - `TmuxPanesDetails` parses `tmux list-panes` CSV fields into `TmuxPaneDetails`, enriches each pane with `GetProcessArgs` and shell/subshell classification, and filters by target when target is a pane ID.
  - `TmuxSendCommandToPane` splits multiline input, detects special-key syntax, dispatches as either literal `-l` send-keys or tokenized special-key arguments, then optionally appends `Enter`.
  - `buildSplitWindowArgs` validates/rewrites split args by rejecting reserved tmux flags before appending required `-t`, `-P`, `-F` options.
- Data enrichment helpers:
  - `TmuxPaneDetails.Refresh` captures pane content, derives last visible line, computes `IsPrepared` by prompt suffix (`»`), and updates shell field when command is a known shell.
  - `FormatInfo` / `String` render pane metadata; formatter methods render uniform colored sections/kv/progress/bool output.
  - `Cosmetics` and inline-code processor transform raw assistant text with markdown-like formatting and syntax highlighting.

## Integration Points
- External binaries: `tmux` is the primary dependency; commands used include `list-panes`, `capture-pane`, `send-keys`, `split-window`, `clear-history`, `kill-pane`, `new-session`, `attach-session`, `current-pid command lookup` flows via `ps`, `pgrep` in `GetProcessArgs`.
- Third-party libs: `github.com/fatih/color` for terminal colors, `github.com/alecthomas/chroma/*` for syntax highlighting.
- Internal dependency: `github.com/alvinunreal/tmuxai/logger` for error/debug logging.
- Environment/config integration: reads `TMUX_PANE` (`TmuxCurrentPaneId`) and uses runtime/OS info helpers for pane metadata.
