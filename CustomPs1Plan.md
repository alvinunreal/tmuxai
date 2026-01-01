# Custom PS1 Plan

## Goal
Design a richer yet parseable zsh prompt for Prepare Mode, keeping the `[status]»` suffix intact and minimizing disruption to user shell configuration.

## Scope
- Files: `internal/exec_pane.go` (zsh branch of `PrepareExecPaneWithShell`), `internal/exec_pane.go` prompt regex if needed, `internal/chat_command.go` expectations for `/prepare`, tests in `internal/exec_pane_test.go` and `internal/chat_command_test.go` (zsh cases), docs snippet in `README.md` Prepare Mode.
- Shell: zsh only (bash/fish untouched).

## Prompt Design Specification (FINALIZED)

### Final Prompt Format
```zsh
setopt PROMPT_SUBST; export PROMPT='%F{green}%n@%m%f:%F{blue}%~%f%F{magenta}$(b=$(git symbolic-ref --short HEAD 2>/dev/null) && echo "($b)")%f[%T][%?]» '
```

### Components Breakdown
| Segment | Zsh Code | Color | Description |
|---------|----------|-------|-------------|
| User@Host | `%F{green}%n@%m%f` | Green | Username and hostname |
| Separator | `:` | None | Colon separator |
| Directory | `%F{blue}%~%f` | Blue | Current working directory (~ abbreviated) |
| Git Branch | `%F{magenta}$(...)%f` | Magenta | Optional branch name in parentheses |
| Time | `[%T]` | None | 24-hour time HH:MM:SS |
| Status | `[%?]` | None | Exit code of last command (CRITICAL for parsing) |
| Suffix | `» ` | None | Prompt suffix with space (CRITICAL for parsing) |

### Example Output

**In a git repository:**
```
user@hostname:~/project(main)[14:30:00][0]» 
```

**Outside a git repository:**
```
user@hostname:/tmp[14:30:00][0]» 
```

**After a failed command:**
```
user@hostname:~/project(main)[14:31:00][127]» 
```

### Technical Notes
- `setopt PROMPT_SUBST` enables command substitution in PROMPT (required for git branch)
- `%F{color}...%f` is zsh's built-in color syntax (no `%{ %}` needed - zsh handles these as non-printing)
- Git branch uses `git symbolic-ref --short HEAD 2>/dev/null` with `&& echo` for graceful fallback
- The `[%?]»` suffix remains unchanged for backward-compatible parsing

### Parsing Compatibility
The existing regex `.*\[(\d+)\]» ?(.*)$` correctly parses the new format:
- Captures status code from `[number]` before `»`
- Captures command after `» `
- Ignores all prefix content (colors, git branch, etc.)

## Approach
1. **Prompt Design**: ✅ COMPLETED - See specification above
2. **Implementation**: Update zsh case in `PrepareExecPaneWithShell` to set the new `PROMPT`. Ensure command is single export without unsetting hooks. Keep clear-screen command as is.
3. **Parsing**: Review `parseExecPaneCommandHistory` regex; adjust to anchor on final `[number]»` while allowing richer prefixes. Confirm command capture still works after the prompt.
4. **Tests**: Update zsh expectations in `internal/exec_pane_test.go` and `internal/chat_command_test.go` to match the new prompt string (presence of `PROMPT=` and key segments). Add/adjust parsing test fixture to include the richer zsh prompt and validate status/command extraction.
5. **Docs**: Refresh README Prepare Mode example to show the richer zsh prompt and note the stable `[status]»` suffix and minimal side effects.

## Validation
- Run or update unit tests touching prompt parsing and `/prepare` zsh handling.
- Manual sanity: ensure prompt still ends with `[code]»` and git branch segment is optional when git helpers are absent.

## Risks/Mitigations
- **Git helpers missing**: ✅ Handled - branch segment uses `&& echo` pattern, returns empty if git command fails.
- **Color escape issues**: ✅ Handled - zsh's `%F{}%f` syntax is automatically treated as non-printing.
- **Prompt length**: ✅ Handled - concise format with abbreviated home directory (`%~`) and short branch format `(branch)`.
