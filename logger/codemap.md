# logger/

## Responsibility

- Provides a minimal logging subsystem for TmuxAI using a process-global singleton logger.
- Owns creation, configuration, and lifecycle of the application's log output file.
- Exposes both instance methods (`(*Logger).Info/Error/Debug`) and package-level convenience functions (`logger.Info/Error/Debug`) for call sites.

## Design

- `Logger` encapsulates:
  - `logFile *os.File` (the writable destination file),
  - `logger *log.Logger` (standard library logger with `log.LstdFlags`),
  - `mu sync.Mutex` (serializes writes and close operations).
- Package-level singleton state is `instance *Logger` with `once sync.Once` to guarantee one-time initialization.
- `Init()` calls `newLogger()` exactly once via `once.Do`; `GetInstance()`/global helpers assume initialization may occur elsewhere.
- File location strategy is deterministic: `$HOME/.config/tmuxai/tmuxai.log`, with `MkdirAll` for the directory and append/create mode for the file.
- Logging calls prepend severity tags (`[INFO]`, `[ERROR]`, `[DEBUG]`) to formatted messages and delegate to `log.Logger.Printf`.

## Data & Control Flow

- `Init` → `once.Do` → `newLogger`:
  1. Resolve `$HOME` via `os.UserHomeDir`.
  2. Ensure `~/.config/tmuxai` exists.
  3. Open `tmuxai.log` with append/create/write flags.
  4. Construct `log.Logger` and assign to global `instance`.
- `GetInstance` returns an error if `instance == nil` (not initialized).
- Package-level `Info/Error/Debug` functions check `instance != nil` and forward to the singleton instance methods; otherwise they no-op.
- Instance `Info/Error/Debug` lock `mu`, format+print to file-backed logger, then unlock.
- `Close` locks `mu` and closes `logFile`.

## Integration Points

- Imported by application entrypoints/components that need runtime log emission.
- Consumers should call `logger.Init()` during startup before first logging; after that use either:
  - package-level `logger.Info(...)` etc. (simplest), or
  - `logger.GetInstance()` to obtain explicit `*Logger` when dependency-injected usage is desired.
- The log file path (`~/.config/tmuxai/tmuxai.log`) is the primary external integration surface for operators and diagnostics tooling.
