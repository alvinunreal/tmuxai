<br/>
<div align="center">
  <a href="https://github.com/alvinunreal/tmuxai">
    <img src="https://tmuxai.dev/gh.svg?v=2" alt="TmuxAI Logo" width="100%">
  </a>
  <h3 align="center">TmuxAI</h3>
  <p align="center">
    Your intelligent pair programmer directly within your tmux sessions.
    <br/>
    <br/>
    <a href="https://github.com/alvinunreal/tmuxai/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/github/license/alvinunreal/tmuxai?style=flat-square"></a>
    <a href="https://github.com/alvinunreal/tmuxai/releases/latest"><img alt="Release" src="https://img.shields.io/github/v/release/alvinunreal/tmuxai?style=flat-square"></a>
    <a href="https://github.com/alvinunreal/tmuxai/issues"><img alt="Issues" src="https://img.shields.io/github/issues/alvinunreal/tmuxai?style=flat-square"></a>
    <br/>
    <br/>
    <br/>
    <a href="https://tmuxai.dev/screenshots" target="_blank">Screenshots</a> |
    <a href="https://github.com/alvinunreal/tmuxai/issues/new?labels=bug&template=bug_report.md" target="_blank">Report Bug</a> |
    <a href="https://github.com/alvinunreal/tmuxai/issues/new?labels=enhancement&template=feature_request.md" target="_blank">Request Feature</a>
    <br/>
    <br/>
    <a href="https://tmuxai.dev/tmux-cheat-sheet/" target="_blank">Tmux Cheat Sheet</a> |
    <a href="https://tmuxai.dev/tmux-getting-started/" target="_blank">Tmux Getting Started</a> |
    <a href="https://tmuxai.dev/tmux-config/" target="_blank">Tmux Config Generator</a>
  </p>
</div>

## Table of Contents

- [About The Project](#about-the-project)
  - [Human-Inspired Interface](#human-inspired-interface)
- [Installation](#installation)
  - [Quick Install](#quick-install)
  - [Homebrew](#homebrew)
  - [Manual Download](#manual-download)
- [Post-Installation Setup](#post-installation-setup)
- [TmuxAI Layout](#tmuxai-layout)
- [Observe Mode](#observe-mode)
- [Prepare Mode](#prepare-mode)
- [Watch Mode](#watch-mode)
  - [Activating Watch Mode](#activating-watch-mode)
  - [Example Use Cases](#example-use-cases)
- [Squashing](#squashing)
  - [What is Squashing?](#what-is-squashing)
  - [Manual Squashing](#manual-squashing)
- [Core Commands](#core-commands)
- [Command-Line Usage](#command-line-usage)
- [Configuration](#configuration)
  - [Environment Variables](#environment-variables)
  - [Session-Specific Configuration](#session-specific-configuration)
  - [Using Other AI Providers](#using-other-ai-providers)
- [Contributing](#contributing)
- [License](#license)

## About The Project

![Product Demo](https://tmuxai.dev/demo.png)

TmuxAI is an intelligent terminal assistant that lives inside your tmux sessions. Unlike other CLI AI tools, TmuxAI observes and understands the content of your tmux panes, providing assistance without requiring you to change your workflow or interrupt your terminal sessions.

Think of TmuxAI as a _pair programmer_ that sits beside you, watching your terminal environment exactly as you see it. It can understand what you're working on across multiple panes, help solve problems and execute commands on your behalf in a dedicated execution pane.

### Human-Inspired Interface

TmuxAI's design philosophy mirrors the way humans collaborate at the terminal. Just as a colleague sitting next to you would observe your screen, understand context from what's visible, and help accordingly, TmuxAI:

1. **Observes**: Reads the visible content in all your panes
2. **Communicates**: Uses a dedicated chat pane for interaction
3. **Acts**: Can execute commands in a separate execution pane (with your permission)

This approach provides powerful AI assistance while respecting your existing workflow and maintaining the familiar terminal environment you're already comfortable with.

## Installation

TmuxAI requires only tmux to be installed on your system. It's designed to work on Unix-based operating systems including Linux and macOS.

### Quick Install

The fastest way to install TmuxAI is using the installation script:

```bash
# install tmux if not already installed
curl -fsSL https://get.tmuxai.dev | bash
```

This installs TmuxAI to `/usr/local/bin/tmuxai` by default. If you need to install to a different location or want to see what the script does before running it, you can view the source at [get.tmuxai.dev](https://get.tmuxai.dev).

### Homebrew

If you use Homebrew, you can install TmuxAI with:

```bash
brew install tmuxai
```

### Manual Download

You can also download pre-built binaries from the [GitHub releases page](https://github.com/alvinunreal/tmuxai/releases).

After downloading, make the binary executable and move it to a directory in your PATH:

```bash
chmod +x ./tmuxai
sudo mv ./tmuxai /usr/local/bin/
```

## Post-Installation Setup

After installing TmuxAI, you need to configure your API key to start using it:

1. **Set the API Key**  
   TmuxAI uses the OpenRouter endpoint by default. Set your API key by adding the following to your shell configuration (e.g., `~/.bashrc`, `~/.zshrc`):

   ```bash
   export TMUXAI_OPENROUTER_API_KEY="your-api-key-here"
   ```

2. **Start TmuxAI**

   ```bash
   tmuxai
   ```

## TmuxAI Layout

![Panes](https://tmuxai.dev/shots/panes.png?lastmode=1)

TmuxAI is designed to operate within a single tmux window, with one instance of
TmuxAI running per window and organizes your workspace using the following pane structure:

1. **Chat Pane**: This is where you interact with the AI. It features a REPL-like interface with syntax highlighting, auto-completion, and readline shortcuts.

2. **Exec Pane**: TmuxAI selects (or creates) a pane where commands can be executed.

3. **Read-Only Panes**: All other panes in the current window serve as additional context. TmuxAI can read their content but does not interact with them.

## Observe Mode

![Observe Mode](https://tmuxai.dev/shots/demo-observe.png)
_TmuxAI sent the first ping command and is waiting for the countdown to check for the next step_

TmuxAI operates by default in "observe mode". Here's how the interaction flow works:

1. **User types a message** in the Chat Pane.

2. **TmuxAI captures context** from all visible panes in your current tmux window (excluding the Chat Pane itself). This includes:

   - Current command with arguments
   - Detected shell type
   - User's operating system
   - Current content of each pane

3. **TmuxAI processes your request** by sending user's message, the current pane context, and chat history to the AI.

4. **The AI responds** with information, which may include a suggested command to run.

5. **If a command is suggested**, TmuxAI will:

   - Check if the command matches whitelist or blacklist patterns
   - Ask for your confirmation (unless the command is whitelisted)
   - Execute the command in the designated Exec Pane if approved
   - Wait for the `wait_interval` (default: 5 seconds) (You can pause/resume the countdown with `space` or `enter` to stop the countdown)
   - Capture the new output from all panes
   - Send the updated context back to the AI to continue helping you

6. **The conversation continues** until your task is complete.

![Observe Mode Flowchart](https://tmuxai.dev/shots/observe-mode.png)

## Prepare Mode

![Prepare Mode](https://tmuxai.dev/shots/demo-prepare.png?lastmode=1)
_TmuxAI customized the pane prompt and sent the first ping command. Instead of the countdown, it's waiting for command completion_

Prepare mode is an optional feature that enhances TmuxAI's ability to work with your terminal by customizing
your shell prompt and tracking command execution with better precision. This
enhancement eliminates the need for arbitrary wait intervals and provides the AI
with more detailed information about your commands and their results.

When you enable Prepare Mode, TmuxAI will:

1. **Detects your current shell** in the execution pane (supports bash, zsh, and fish)
2. **Customizes your shell prompt** to include special markers that TmuxAI can recognize
3. **Will track command execution history** including exit codes, and per-command outputs
4. **Will detect command completion** instead of using fixed wait time intervals

To activate Prepare Mode, simply use:

```
TmuxAI » /prepare
```

**Prepared Fish Example:**

```shell
$ function fish_prompt; set -l s $status; printf '%s@%s:%s[%s][%d]» ' $USER (hostname -s) (prompt_pwd) (date +"%H:%M") $s; end
username@hostname:~/r/tmuxai[21:05][0]»
```

## Watch Mode

![Watch Mode](https://tmuxai.dev/shots/demo-watch.png)
_TmuxAI watching user shell commands and better alternatives_

Watch Mode transforms TmuxAI into a proactive assistant that continuously
monitors your terminal activity and provides suggestions based on what you're
doing.

### Activating Watch Mode

To enable Watch Mode, use the `/watch` command followed by a description of what you want TmuxAI to look for:

```
TmuxAI » /watch spot and suggest more efficient alternatives to my shell commands
```

When activated, TmuxAI will:

1. Start capturing the content of all panes in your current tmux window at regular intervals (`wait_interval` configuration)
2. Analyze content based on your specified watch goal and provide suggestions when appropriate

### Example Use Cases

Watch Mode could be valuable for scenarios such as:

- **Learning shell efficiency**: Get suggestions for more concise commands as you work

  ```
  TmuxAI » /watch spot and suggest more efficient alternatives to my shell commands
  ```

- **Detecting common errors**: Receive warnings about potential issues or mistakes

  ```
  TmuxAI » /watch flag commands that could expose sensitive data or weaken system security
  ```

- **Log Monitoring and Error Detection**: Have TmuxAI monitor log files or terminal output for errors

  ```
  TmuxAI » /watch monitor log output for errors, warnings, or critical issues and suggest fixes
  ```

## Squashing

As you work with TmuxAI, your conversation history grows, adding to the context
provided to the AI model with each interaction. Different AI models have
different context size limits and pricing structures based on token usage. To
manage this, TmuxAI implements a simple context management feature called
"squashing."

### What is Squashing?

Squashing is TmuxAI's built-in mechanism for summarizing chat history to manage
token usage.

When your context grows too large, TmuxAI condenses previous
messages into a more compact summary.

You can check your current context utilization at any time using the `/info` command:

```bash
TmuxAI » /info

Context
────────

Messages            15
Context Size~       16500 tokens
                    ████████░░ 82.5%
Max Size            20000 tokens
```

This example shows that the context is at 82.5% capacity (16,500 tokens out of 20,000). When the context size reaches 80% of the configured maximum (`max_context_size` in your config), TmuxAI automatically triggers squashing.

### Manual Squashing

If you'd like to manage your context before reaching the automatic threshold, you can trigger squashing manually with the `/squash` command:

```bash
TmuxAI » /squash
```

## Core Commands

| Command                     | Description                                                      |
| --------------------------- | ---------------------------------------------------------------- |
| `/info`                     | Display system information, pane details, and context statistics |
| `/clear`                    | Clear chat history.                                              |
| `/reset`                    | Clear chat history and reset all panes.                          |
| `/config`                   | View current configuration settings                              |
| `/config set <key> <value>` | Override configuration for current session                       |
| `/squash`                   | Manually trigger context summarization                           |
| `/prepare`                  | Initialize Prepared Mode for the Exec Pane                       |
| `/watch <description>`      | Enable Watch Mode with specified goal                            |
| `/exit`                     | Exit TmuxAI                                                      |

## Command-Line Usage

You can start `tmuxai` with an initial message or task file from the command line:

- **Direct Message:**

  ```sh
  tmuxai your initial message
  ```

- **Task File:**
  ```sh
  tmuxai -f path/to/your_task.txt
  ```

## Configuration

The configuration can be managed through a YAML file, environment variables, or via runtime commands.

TmuxAI looks for its configuration file at `~/.config/tmuxai/config.yaml`.
For a sample configuration file, see [config.example.yaml](https://github.com/alvinunreal/tmuxai/blob/main/config.example.yaml).

### Environment Variables

All configuration options can also be set via environment variables, which take precedence over the config file. Use the prefix `TMUXAI_` followed by the uppercase configuration key:

```bash
# Examples
export TMUXAI_DEBUG=true
export TMUXAI_MAX_CAPTURE_LINES=300
export TMUXAI_OPENROUTER_API_KEY="your-api-key-here"
export TMUXAI_OPENROUTER_MODEL="..."
export TMUXAI_AZURE_OPENAI_API_KEY="your-azure-api-key"
export TMUXAI_AZURE_OPENAI_API_BASE="https://your-resource.openai.azure.com/"
export TMUXAI_AZURE_OPENAI_API_VERSION="2025-04-01-preview"
export TMUXAI_AZURE_OPENAI_DEPLOYMENT_NAME="gpt-4o"
```

You can also use environment variables directly within your configuration file values. The application will automatically expand these variables when loading the configuration:

```yaml
# Example config.yaml with environment variable expansion
openrouter:
  api_key: "${OPENAI_API_KEY}"
  base_url: https://api.openai.com/v1
```

### Session-Specific Configuration

You can override some configuration values for your current TmuxAI session using the `/config` command:

```bash
# View current configuration
TmuxAI » /config

# Override a configuration value for this session
TmuxAI » /config set max_capture_lines 300
TmuxAI » /config set openrouter.model gpt-4o-mini
```

These changes will persist only for the current session and won't modify your config file.

### Using Other AI Providers

OpenRouter is OpenAI API-compatible, so you can direct TmuxAI at OpenAI or any other OpenAI API-compatible endpoint by customizing the `base_url`.

For OpenAI:

```yaml
openrouter:
  api_key: sk-proj-XXX
  model: o4-mini-2025-04-16
  base_url: https://api.openai.com/v1
```

For Anthropic’s Claude:

```yaml
openrouter:
  api_key: sk-proj-XXX
  model: claude-3-7-sonnet-20250219
  base_url: https://api.anthropic.com/v1
```

For Gemini:

```yaml
openrouter:
  model: gemini-2.5-pro-preview-06-05
  api_key: XXXX
  base_url: https://generativelanguage.googleapis.com/v1beta/openai/
```

For local Ollama:

```yaml
openrouter:
  api_key: api-key
  model: gemma3:1b
  base_url: http://localhost:11434/v1
```

For Azure OpenAI:

```yaml
azure_openai:
  api_key: "your-azure-openai-key"
  api_base: "https://your-resource.openai.azure.com/"
  api_version: "2025-04-01-preview"
  deployment_name: "gpt-4o"
```

_Prompts are currently tuned for Gemini 2.5 by default; behavior with other models may vary._

## Contributing

If you have a suggestion that would make this better, please fork the repo and create a pull request.
You can also simply open an issue.
<br>
Don't forget to give the project a star!

## License

Distributed under the Apache License. See [Apache License](https://github.com/alvinunreal/tmuxai/blob/main/LICENSE) for more information.
