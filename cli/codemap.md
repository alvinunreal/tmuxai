# cli/

## Responsibility
- Defines and wires the user-facing command-line interface surface for `tmuxai`.
- Owns CLI argument/flag parsing, bootstrap behavior (version/config/initial request), and startup orchestration for a manager-based interactive session.
- Delegates operational behavior to shared packages (`config`, `internal`, `logger`) and remains thin by only translating CLI intent into manager options and lifecycle calls.

## Design
- Uses Cobra’s `*cobra.Command` as a single root command (`tmuxai [request message]`) with global flags and a `PersistentPreRun` version gate.
- Keeps parse state in package-level vars (`taskFileFlag`, `kbFlag`, `modelFlag`, pane selectors, and booleans) bound once in `init()`.
- Root `Run` is single-threaded bootstrap flow: load config, normalize/resolve request source, construct `internal.ManagerOptions`, create `internal.Manager`, apply CLI overrides, then start interaction.
- Uses simple signal handling (`SIGTERM`, `SIGHUP`) to ensure manager cleanup before process exit.
- Exports only `Execute()` as the entrypoint used by `main.go`, preserving a clean boundary between runtime bootstrap and command registration.

## Data & Control Flow
1. `main()` initializes logging and invokes `cli.Execute()`.
2. `Execute()` delegates to Cobra’s `rootCmd.Execute()`.
3. On invocation, `PersistentPreRun` checks `--version` and exits with `internal.Version/Commit/Date`.
4. `Run` loads configuration via `config.Load(configFileFlag)`, then builds initial request text from args or `--file` content.
5. It maps CLI flags into `internal.ManagerOptions`:
   - `--exec-pane` -> `ForcedExecPaneID`
   - `--read-panes` -> `ForcedReadPaneIDs`
6. `internal.NewManager(cfg, options)` is called, then async signal goroutine registers cleanup hooks.
7. Late-bound overrides are applied on the manager before launch:
   - `--kb` -> `mgr.LoadKBsFromCLI(...)`
   - `--model` -> `mgr.SetModelsDefault(...)`
   - `--yolo` -> `mgr.SessionOverrides["yolo"] = true`
8. `mgr.Start(initMessage)` transfers control to the interactive loop; CLI layer no longer manages domain logic.

## Integration Points
- `github.com/alvinunreal/tmuxai/config`: reads effective config and environment overrides via `config.Load(...)`.
- `github.com/alvinunreal/tmuxai/internal`: constructs and drives the runtime `Manager` and calls high-level methods for KB loading, model switching, and session execution.
- `github.com/alvinunreal/tmuxai/logger`: structured runtime logging for config/mode/bootstrap and error reporting.
- `github.com/spf13/cobra`: provides command framework, flag binding, and parsing.
- `main.go`: only consumer of `cli.Execute()`, keeping CLI package as the terminal API boundary.
