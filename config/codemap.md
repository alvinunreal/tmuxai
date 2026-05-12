# config/

## Responsibility
- Centralize tmuxai runtime configuration: define schema, establish defaults, load user/custom configuration, normalize values, and expose filesystem paths used by other subsystems (config dir, kb dir).

## Design
- `Config` is a single strongly-typed root struct (`config.go`) with nested structs for providers and feature areas (`OpenRouter`, `ModelConfig`, `KnowledgeBase`, `WebSearch`, `WebFetch`, `Tmux`, etc.) and `mapstructure` tags for Viper binding/unmarshal.
- `DefaultConfig()` provides an explicit baseline, including sane defaults and default URLs/providers/timeouts, plus initialized map/slice fields to avoid nils where reasonable.
- `Load(...)` performs canonicalized configuration resolution in one pass: choose config source (explicit path → `TMUXAI_CONFIG` env → default search paths), configure env binding (`TMUXAI_*`, dotted keys mapped to underscores), read config (optional), unmarshal into the struct, then post-process env variable expansion.
- `EnumerateConfigKeys` recursively derives dotted key names from struct tags to allow env binding for all fields, including nested structs.
- `ResolveEnvKeyInConfig` recursively traverses values via reflection, expanding `$VAR` references in strings; map values are deep-copied when needed for non-addressable map entries.
- `TryInferType` is a lightweight parser for dynamic string values (bool/int) used by callers needing best-effort type inference.
- Helper funcs include `GetConfigDir`, `GetConfigFilePath`, and `GetKBDir` for workspace path discovery and setup.

## Data & Control Flow
- Startup/configure call: `Load(configFilePath?)` → `DefaultConfig` → Viper search/path/env setup → `ReadInConfig` (non-fatal if missing file) → `Unmarshal` into defaults → `ResolveEnvKeyInConfig` mutation pass.
- On demand, config consumers read the returned `*Config`; they may call `GetConfigDir`/`GetConfigFilePath` for canonical paths or `GetKBDir` for KB storage (which lazily creates `~/.config/tmuxai/kb`).
- Env expansion is recursive across nested structs and maps, with pointers/maps guarded to avoid nil deref.

## Integration Points
- Exposes configuration values consumed by command handlers and service modules (LLM providers, web search/fetch, knowledge base, tmux actions).
- Reads env vars from the process (`TMUXAI_*`) and YAML config (`config.yaml` in `.` or `$HOME/.config/tmuxai` unless overridden).
- Filesystem integration via `os.UserHomeDir`, `os.MkdirAll`, and path construction in helpers to ensure config/kb directories exist when needed.
- External dependency surface: `github.com/spf13/viper` is used for file/env/config parsing/unmarshal.
