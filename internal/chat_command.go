package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

const helpMessage = `Available commands:
- /info: Display system information
- /clear: Clear the chat history
- /reset: Reset the chat history
- /paste: Enter paste mode for multiline input
- /prepare: Prepare the pane for TmuxAI automation
- /watch <prompt>: Start watch mode
- /squash: Summarize the chat history
- /model: List available models and show current model
- /model <name>: Switch to a different model
- /kb: List available knowledge bases
- /kb load <name>: Load a knowledge base
- /kb unload <name>: Unload a knowledge base
- /kb unload --all: Unload all knowledge bases
- /exit: Exit the application`

var commands = []string{
	"/help",
	"/clear",
	"/reset",
	"/paste",
	"/exit",
	"/info",
	"/watch",
	"/prepare",
	"/config",
	"/squash",
	"/model",
	"/kb",
}

// checks if the given content is a command
func (m *Manager) IsMessageSubcommand(content string) bool {
	content = strings.TrimSpace(strings.ToLower(content)) // Normalize input

	// Any message starting with / is considered a command
	return strings.HasPrefix(content, "/")
}

// processes a command and returns a response
func (m *Manager) ProcessSubCommand(command string) {
	commandLower := strings.ToLower(strings.TrimSpace(command))
	logger.Info("Processing command: %s", command)

	// Get the first word from the command (e.g., "/watch" from "/watch something")
	parts := strings.Fields(commandLower)
	if len(parts) == 0 {
		m.Println("Empty command")
		return
	}

	commandPrefix := parts[0]

	// Process the command using prefix matching
	switch {
	case prefixMatch(commandPrefix, "/help"):
		m.Println(helpMessage)
		return

	case prefixMatch(commandPrefix, "/info"):
		m.formatInfo()
		return

	case prefixMatch(commandPrefix, "/prepare"):
		supportedShells := []string{"bash", "zsh", "fish"}
		m.InitExecPane()

		// Check if exec pane is a subshell
		if m.ExecPane.IsSubShell {
			if len(parts) > 1 {
				shell := parts[1]
				isSupported := false
				for _, supportedShell := range supportedShells {
					if shell == supportedShell {
						isSupported = true
						break
					}
				}
				if !isSupported {
					m.Println(fmt.Sprintf("Shell '%s' is not supported. Supported shells are: %s", shell, strings.Join(supportedShells, ", ")))
					return
				}
				m.PrepareExecPaneWithShell(shell)
			} else {
				m.Println("Shell detection is not supported on subshells.")
				m.Println("Please specify the shell manually: /prepare bash, /prepare zsh, or /prepare fish")
				return
			}
		} else {
			if len(parts) > 1 {
				shell := parts[1]
				isSupported := false
				for _, supportedShell := range supportedShells {
					if shell == supportedShell {
						isSupported = true
						break
					}
				}

				if !isSupported {
					m.Println(fmt.Sprintf("Shell '%s' is not supported. Supported shells are: %s", shell, strings.Join(supportedShells, ", ")))
					return
				}
				m.PrepareExecPaneWithShell(shell)
			} else {
				m.PrepareExecPane()
			}
		}

		// for latency over ssh connections
		time.Sleep(500 * time.Millisecond)
		m.ExecPane.Refresh(m.GetMaxCaptureLines())
		m.Messages = []ChatMessage{}

		fmt.Println(m.ExecPane.String())
		m.parseExecPaneCommandHistory()

		logger.Debug("Parsed exec history:")
		for _, history := range m.ExecHistory {
			logger.Debug(fmt.Sprintf("Command: %s\nOutput: %s\nCode: %d\n", history.Command, history.Output, history.Code))
		}

		return

	case prefixMatch(commandPrefix, "/clear"):
		m.Messages = []ChatMessage{}
		_ = system.TmuxClearPane(m.PaneId)
		return

	case prefixMatch(commandPrefix, "/reset"):
		m.Status = ""
		m.Messages = []ChatMessage{}
		_ = system.TmuxClearPane(m.PaneId)
		_ = system.TmuxClearPane(m.ExecPane.Id)
		return

	case prefixMatch(commandPrefix, "/exit"):
		logger.Info("Exit command received, stopping watch mode (if active) and exiting.")
		os.Exit(0)
		return

	case prefixMatch(commandPrefix, "/squash"):
		m.squashHistory()
		return

	case prefixMatch(commandPrefix, "/watch") || commandPrefix == "/w":
		parts := strings.Fields(command)
		if len(parts) > 1 {
			watchDesc := strings.Join(parts[1:], " ")
			startWatch := `
1. Find out if there is new content in the pane based on chat history.
2. Comment only considering the new content in this pane output.

Watch for: ` + watchDesc
			m.Status = "running"
			m.WatchMode = true
			m.startWatchMode(startWatch)
			return
		}
		m.Println("Usage: /watch <description>")
		return

	case prefixMatch(commandPrefix, "/config"):
		// Helper function to check if a key is allowed
		isKeyAllowed := func(key string) bool {
			for _, k := range AllowedConfigKeys {
				if k == key {
					return true
				}
			}
			return false
		}

		// Check if it's "config set" for a specific key
		if len(parts) >= 3 && parts[1] == "set" {
			key := parts[2]
			if !isKeyAllowed(key) {
				m.Println(fmt.Sprintf("Cannot set '%s'. Only these keys are allowed: %s", key, strings.Join(AllowedConfigKeys, ", ")))
				return
			}
			value := strings.Join(parts[3:], " ")
			m.SessionOverrides[key] = config.TryInferType(key, value)
			m.Println(fmt.Sprintf("Set %s = %v", key, m.SessionOverrides[key]))
			return
		} else {
			code, _ := system.HighlightCode("yaml", m.FormatConfig())
			fmt.Println(code)
			return
		}

	case prefixMatch(commandPrefix, "/kb"):
		// Handle KB commands: /kb, /kb list, /kb load <name>, /kb unload <name>
		if len(parts) == 1 || (len(parts) == 2 && parts[1] == "list") {
			// List all available knowledge bases
			kbs, err := m.listKBs()
			if err != nil {
				m.Println(fmt.Sprintf("Error listing knowledge bases: %v", err))
				return
			}

			if len(kbs) == 0 {
				m.Println("No knowledge bases found in " + config.GetKBDir())
				return
			}

			m.Println("Available knowledge bases:")
			totalTokens := 0
			loadedCount := 0

			for _, name := range kbs {
				_, loaded := m.LoadedKBs[name]
				status := "[ ]"
				tokens := ""
				if loaded {
					status = "[✓]"
					tokenCount := system.EstimateTokenCount(m.LoadedKBs[name])
					tokens = fmt.Sprintf(" (%d tokens)", tokenCount)
					totalTokens += tokenCount
					loadedCount++
				}
				m.Println(fmt.Sprintf("  %s %s%s", status, name, tokens))
			}

			if loadedCount > 0 {
				m.Println("")
				m.Println(fmt.Sprintf("Loaded: %d KB(s), %d tokens", loadedCount, totalTokens))
			}
			return

		} else if len(parts) >= 2 && parts[1] == "load" {
			if len(parts) < 3 {
				m.Println("Usage: /kb load <name>")
				return
			}

			name := parts[2]
			if _, loaded := m.LoadedKBs[name]; loaded {
				m.Println(fmt.Sprintf("Knowledge base '%s' is already loaded", name))
				return
			}

			if err := m.loadKB(name); err != nil {
				m.Println(fmt.Sprintf("Error loading KB '%s': %v", name, err))
				return
			}

			tokenCount := system.EstimateTokenCount(m.LoadedKBs[name])
			m.Println(fmt.Sprintf("✓ Loaded knowledge base: %s (%d tokens)", name, tokenCount))
			return

		} else if len(parts) >= 2 && parts[1] == "unload" {
			if len(parts) >= 3 && parts[2] == "--all" {
				// Unload all KBs
				if len(m.LoadedKBs) == 0 {
					m.Println("No knowledge bases are currently loaded")
					return
				}

				count := len(m.LoadedKBs)
				m.LoadedKBs = make(map[string]string)
				m.Println(fmt.Sprintf("✓ Unloaded all knowledge bases (%d KB(s))", count))
				return
			}

			if len(parts) < 3 {
				m.Println("Usage: /kb unload <name> or /kb unload --all")
				return
			}

			name := parts[2]
			if err := m.unloadKB(name); err != nil {
				m.Println(fmt.Sprintf("Error: %v", err))
				return
			}

			m.Println(fmt.Sprintf("✓ Unloaded knowledge base: %s", name))
			return

		} else {
			m.Println("Usage: /kb [list|load <name>|unload <name>|unload --all]")
			return
		}

	case prefixMatch(commandPrefix, "/model"):
		// Handle model commands: /model, /model <name>
		if len(parts) == 1 {
			// List available models and show current
			m.listModels()
			return
		} else if len(parts) >= 2 {
			modelName := strings.Join(parts[1:], " ")
			m.switchModel(modelName)
			return
		}

	default:
		m.Println(fmt.Sprintf("Unknown command: %s. Type '/help' to see available commands.", command))
		return
	}
}

// Helper function to check if a command matches a prefix
func prefixMatch(command, target string) bool {
	return strings.HasPrefix(target, command)
}

// formats system information and tmux details into a readable string
func (m *Manager) formatInfo() {
	formatter := system.NewInfoFormatter()
	const labelWidth = 18 // Width of the label column
	formatLine := func(key string, value any) {
		fmt.Print(formatter.LabelColor.Sprintf("%-*s", labelWidth, key))
		fmt.Print("  ")
		fmt.Println(value)
	}
	// Display general information
	fmt.Println(formatter.FormatSection("\nGeneral"))
	formatLine("Version", Version)
	formatLine("Max Capture Lines", m.Config.MaxCaptureLines)
	formatLine("Wait Interval", m.Config.WaitInterval)

	// Display AI model information
	currentModelConfig, _ := m.GetCurrentModelConfig()
	currentDefault := m.GetModelsDefault()
	availableModels := m.GetAvailableModels()

	if len(availableModels) > 0 {
		// Show current model configuration
		modelName := currentDefault
		if modelName == "" && len(availableModels) > 0 {
			modelName = availableModels[0] // First model as default
		}
		if modelName != "" {
			formatLine("Model", modelName)
		}
		if modelConfig, exists := m.GetModelConfig(modelName); exists {
			formatLine("Provider", modelConfig.Provider)
		}
	} else {
		// Legacy configuration
		formatLine("Provider", currentModelConfig.Provider)
		formatLine("Model", currentModelConfig.Model)
	}

	// Display context information section
	fmt.Println(formatter.FormatSection("\nContext"))
	formatLine("Messages", len(m.Messages))
	var totalTokens int
	for _, msg := range m.Messages {
		totalTokens += system.EstimateTokenCount(msg.Content)
	}

	usagePercent := 0.0
	if m.GetMaxContextSize() > 0 {
		usagePercent = float64(totalTokens) / float64(m.GetMaxContextSize()) * 100
	}
	fmt.Print(formatter.LabelColor.Sprintf("%-*s", labelWidth, "Context Size~"))
	fmt.Print("  ") // Two spaces for separation
	fmt.Printf("%s\n", fmt.Sprintf("%d tokens", totalTokens))
	fmt.Printf("%-*s  %s\n", labelWidth, "", formatter.FormatProgressBar(usagePercent, 10))
	formatLine("Max Size", fmt.Sprintf("%d tokens", m.GetMaxContextSize()))

	// Display knowledge base information
	if len(m.LoadedKBs) > 0 {
		kbTokens := m.getTotalLoadedKBTokens()
		formatLine("Loaded KBs", fmt.Sprintf("%d (%d tokens)", len(m.LoadedKBs), kbTokens))
	}

	// Display tmux panes section
	fmt.Println()
	fmt.Println(formatter.FormatSection("Tmux Window Panes"))

	panes, _ := m.GetTmuxPanes()
	for _, pane := range panes {
		pane.Refresh(m.GetMaxCaptureLines())
		fmt.Println(pane.FormatInfo(formatter))
	}
}

// listModels displays available models and highlights the current one
func (m *Manager) listModels() {
	formatter := system.NewInfoFormatter()

	// Get current model configuration
	currentModelConfig, _ := m.GetCurrentModelConfig()
	currentDefault := m.GetModelsDefault()

	fmt.Println(formatter.FormatSection("\nAvailable Models"))

	// List configured models
	availableModels := m.GetAvailableModels()
	if len(availableModels) > 0 {
		for _, name := range availableModels {
			config, exists := m.GetModelConfig(name)
			if exists {
				status := " [ ]"
				if currentDefault == name {
					status = " [✓]"
				}
				fmt.Printf("%s %s (%s: %s)\n", status, name, config.Provider, config.Model)
			}
		}
	} else {
		fmt.Println("No model configurations found. Using legacy configuration.")
	}

	// Show current model from legacy config if no models configured
	if len(availableModels) == 0 || currentDefault == "" {
		fmt.Println("\nCurrent Model (Legacy):")
		fmt.Printf("  Provider: %s\n", currentModelConfig.Provider)
		fmt.Printf("  Model: %s\n", currentModelConfig.Model)
		if currentModelConfig.BaseURL != "" {
			fmt.Printf("  Base URL: %s\n", currentModelConfig.BaseURL)
		}
	} else {
		fmt.Println("\nCurrent Model:")
		fmt.Printf("  Configuration: %s\n", currentDefault)
		fmt.Printf("  Provider: %s\n", currentModelConfig.Provider)
		fmt.Printf("  Model: %s\n", currentModelConfig.Model)
		if currentModelConfig.BaseURL != "" {
			fmt.Printf("  Base URL: %s\n", currentModelConfig.BaseURL)
		}
	}

	if len(availableModels) > 0 {
		fmt.Println("\nUsage: /model <name> to switch models")
	}
}

// switchModel switches to the specified model configuration
func (m *Manager) switchModel(modelName string) {
	// Check if the model exists in configurations
	_, exists := m.GetModelConfig(modelName)
	if !exists {
		m.Println(fmt.Sprintf("Model '%s' not found. Available models: %s", modelName, strings.Join(m.GetAvailableModels(), ", ")))
		return
	}

	// Set the model as default for this session
	m.SetModelsDefault(modelName)

	// Get the model configuration to show details
	modelConfig, _ := m.GetModelConfig(modelName)

	m.Println(fmt.Sprintf("✓ Switched to %s (%s: %s)", modelName, modelConfig.Provider, modelConfig.Model))
}
