package internal

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/internal/mcp"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

const helpMessage = `Available commands:
- /info: Display system information
- /clear: Clear the chat history
- /reset: Reset the chat history
- /prepare: Prepare the pane for TmuxAI automation
- /watch <prompt>: Start watch mode
- /squash: Summarize the chat history
- /model: List available models and show current model
- /model <name>: Switch to a different model
- /kb: List available knowledge bases
- /kb load <name>: Load a knowledge base
- /kb unload <name>: Unload a knowledge base
- /kb unload --all: Unload all knowledge bases
- /skill: List available skills
- /skill load <name>: Load a skill
- /skill unload <name>: Unload a skill
- /skill unload --all: Unload all skills
- /skill info <name>: Show skill details
- /skill validate: Re-scan and validate skills
- /websearch [-f N] <query>: Search the web (use -f N to auto-fetch top N results)
- /webfetch <url>: Fetch and extract content from a URL
- /mcp: List MCP servers and status
- /mcp tools [<server>]: List MCP tools
- /mcp load: Full reload MCP config (shutdown all, reconnect all)
- /mcp reload: Hot reload MCP config (incremental diff)
- /mcp unload: Disconnect all MCP servers
- /exit: Exit the application`

var commands = []string{
	"/help",
	"/clear",
	"/reset",
	"/exit",
	"/info",
	"/watch",
	"/prepare",
	"/config",
	"/squash",
	"/model",
	"/kb",
	"/skill",
	"/websearch",
	"/webfetch",
	"/mcp",
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
		if err := m.InitExecPane(); err != nil {
			m.Println(fmt.Sprintf("Error preparing exec pane: %v", err))
			return
		}

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

	case prefixMatch(commandPrefix, "/skill"):
		// Feature gate
		if m.Skills == nil || !m.Config.KnowledgeBase.Skills.Enabled {
			m.Println("Skills system is not enabled. Set knowledge_base.skills.enabled: true in config.")
			return
		}

		// /skill or /skill list → show all skills
		if len(parts) == 1 || (len(parts) == 2 && parts[1] == "list") {
			names := make([]string, 0, len(m.Skills.Skills))
			for name := range m.Skills.Skills {
				names = append(names, name)
			}
			sort.Strings(names)

			if len(names) == 0 {
				m.Println("No skills discovered.")
				return
			}

			m.Println("Available skills:")
			for _, name := range names {
				s := m.Skills.Skills[name]
				loaded := "[ ]"
				if s.Loaded {
					loaded = "[✓]"
				}
				disabled := ""
				if s.Disabled {
					disabled = " [manual]"
				}
				charInfo := ""
				if s.Loaded {
					charInfo = fmt.Sprintf(" (%d chars)", s.BodyLength)
				}
				m.Println(fmt.Sprintf("  %s %-25s%s%s", loaded, name, disabled, charInfo))
			}
			total := len(names)
			loadedCount := 0
			usedChars := 0
			for _, s := range m.Skills.Skills {
				if s.Loaded {
					loadedCount++
					usedChars += s.BodyLength
				}
			}
			if loadedCount > 0 {
				m.Println("")
				m.Println(fmt.Sprintf("Loaded: %d/%d skill(s), %d/%d chars",
					loadedCount, total, usedChars, m.Config.KnowledgeBase.Skills.MaxLoadedChars))
			}
			return
		}

		// /skill load <name>
		if len(parts) >= 2 && parts[1] == "load" {
			if len(parts) < 3 {
				m.Println("Usage: /skill load <name>")
				return
			}
			name := parts[2]
			if _, ok := m.Skills.Skills[name]; !ok {
				m.Println(fmt.Sprintf("Skill '%s' not found", name))
				return
			}
			if m.Skills.Skills[name].Loaded {
				m.Println(fmt.Sprintf("Skill '%s' is already loaded", name))
				return
			}
			if err := m.Skills.Load(name); err != nil {
				m.Println(fmt.Sprintf("Error loading skill '%s': %v", name, err))
				return
			}
			m.Skills.L1Block = m.Skills.BuildL1Block()
			m.LoadedSkills[name] = m.Skills.Skills[name].Body
			m.Println(fmt.Sprintf("✓ Loaded skill: %s (%d chars)", name, m.Skills.Skills[name].BodyLength))
			return
		}

		// /skill unload <name> or /skill unload --all
		if len(parts) >= 2 && parts[1] == "unload" {
			if len(parts) >= 3 && parts[2] == "--all" {
				names := make([]string, 0, len(m.Skills.Skills))
				for _, s := range m.Skills.Skills {
					if s.Loaded {
						names = append(names, s.Name)
					}
				}
				if len(names) == 0 {
					m.Println("No skills are currently loaded")
					return
				}
				for _, name := range names {
					if err := m.Skills.Unload(name); err != nil {
						m.Println(fmt.Sprintf("Error: %v", err))
						return
					}
					delete(m.LoadedSkills, name)
				}
				m.Skills.L1Block = m.Skills.BuildL1Block()
				m.Println(fmt.Sprintf("✓ Unloaded all skills (%d skill(s))", len(names)))
				return
			}
			if len(parts) < 3 {
				m.Println("Usage: /skill unload <name> or /skill unload --all")
				return
			}
			name := parts[2]
			if err := m.Skills.Unload(name); err != nil {
				m.Println(fmt.Sprintf("Error: %v", err))
				return
			}
			delete(m.LoadedSkills, name)
			m.Skills.L1Block = m.Skills.BuildL1Block()
			m.Println(fmt.Sprintf("✓ Unloaded skill: %s", name))
			return
		}

		// /skill info <name>
		if len(parts) >= 2 && parts[1] == "info" {
			if len(parts) < 3 {
				m.Println("Usage: /skill info <name>")
				return
			}
			name := parts[2]
			skill, ok := m.Skills.Skills[name]
			if !ok {
				m.Println(fmt.Sprintf("Skill '%s' not found", name))
				return
			}
			m.Println(fmt.Sprintf("Name:        %s", skill.Name))
			m.Println(fmt.Sprintf("Description: %s", skill.Description))
			m.Println(fmt.Sprintf("Disabled:    %v", skill.Disabled))
			m.Println(fmt.Sprintf("Loaded:      %v", skill.Loaded))
			m.Println(fmt.Sprintf("Body Size:   %d chars", skill.BodyLength))
			m.Println(fmt.Sprintf("Directory:   %s", skill.DirPath))
			m.Println(fmt.Sprintf("File:        %s", skill.FilePath))
			// Show ancillary files
			manifest := skill.BuildManifest()
			if manifest != "" {
				m.Println("")
				m.Println(manifest)
			}
			return
		}

		// /skill validate
		if len(parts) == 2 && parts[1] == "validate" {
			// Re-scan even when startup auto_scan is disabled so validate reports
			// all valid and invalid on-disk skills.
			validateConfig := m.Config.KnowledgeBase.Skills
			validateConfig.AutoScan = true
			newReg, err := InitSkills(&validateConfig)
			if err != nil {
				m.Println(fmt.Sprintf("Validation error: %v", err))
				return
			}

			names := make([]string, 0, len(newReg.Skills))
			for name := range newReg.Skills {
				names = append(names, name)
			}
			sort.Strings(names)

			m.Println(fmt.Sprintf("Validated %d skill(s):", len(names)))
			for _, name := range names {
				m.Println(fmt.Sprintf("  ✓ OK  %s", name))
			}

			if len(newReg.DiscoveryWarnings) > 0 {
				m.Println("")
				m.Println(fmt.Sprintf("Warnings (%d):", len(newReg.DiscoveryWarnings)))
				for _, warning := range newReg.DiscoveryWarnings {
					m.Println(fmt.Sprintf("  ✗ %s", warning))
				}
			}
			return
		}

		m.Println("Usage: /skill [list|load <name>|unload <name>|unload --all|info <name>|validate]")

	case prefixMatch(commandPrefix, "/websearch"):
		if !m.Config.WebSearch.Enabled {
			m.Println("Web search is not enabled. Configure web_search.enabled: true in your config.")
			return
		}
		if m.SearchEngine == nil {
			m.Println("Web search engine not initialized. Check your configuration.")
			return
		}
		// Parse -f N flag
		var fetchCount int
		hasFetchFlag := false
		for i, p := range parts[1:] {
			if p == "-f" && i+1 < len(parts[1:]) {
				n, err := strconv.Atoi(parts[1:][i+1])
				if err != nil || n < 1 {
					m.Println("Usage: /websearch [-f N] <query>")
					return
				}
				fetchCount = n
				hasFetchFlag = true
				break
			}
		}
		if !hasFetchFlag {
			if len(parts) < 2 {
				m.Println("Usage: /websearch [-f N] <query>")
				return
			}
			query := strings.Join(parts[1:], " ")
			m.handleWebSearch(query)
			return
		}
		// Extract the actual query (everything except -f and its argument)
		remainingQuery := make([]string, 0)
		skipNext := false
		for _, p := range parts[1:] {
			if skipNext {
				skipNext = false
				continue
			}
			if p == "-f" {
				skipNext = true
				continue
			}
			remainingQuery = append(remainingQuery, p)
		}
		if len(remainingQuery) < 1 {
			m.Println("Usage: /websearch [-f N] <query>")
			return
		}
		// Perform search (budget still enforced by SearchEngine)
		query := strings.Join(remainingQuery, " ")
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.Config.WebSearch.TimeoutSeconds)*time.Second)
		defer cancel()
		searchResp := m.SearchEngine.Search(ctx, query)
		if searchResp.Error != nil {
			m.Println(fmt.Sprintf("Search failed: %v", searchResp.Error))
			return
		}
		formatted := FormatSearchResultsBlock(query, searchResp.Provider, searchResp.Results)
		fmt.Println(formatted)
		m.Messages = append(m.Messages, ChatMessage{
			Content:   formatted,
			FromUser:  false,
			Timestamp: time.Now(),
		})
		// Auto-fetch top N results
		actualCount := fetchCount
		if actualCount > len(searchResp.Results) {
			actualCount = len(searchResp.Results)
		}
		if actualCount > 0 {
			fmt.Printf("\nFetching top %d result(s)...\n", actualCount)
			fetchMaxChars := m.Config.WebSearch.FetchMaxChars
			if fetchMaxChars <= 0 {
				fetchMaxChars = m.Config.WebFetch.MaxChars
			}
			for i := 0; i < actualCount; i++ {
				urlStr := searchResp.Results[i].URL
				// M3: Each fetch gets its own fresh context with full timeout budget.
				fetchCtx, fetchCancel := context.WithTimeout(
					context.Background(),
					time.Duration(m.Config.WebFetch.TimeoutSeconds)*time.Second,
				)
				fetchResp := FetchWithFallbacks(fetchCtx, urlStr, fetchMaxChars, m.Config.WebFetch.TimeoutSeconds, false)
				fetchCancel()
				sourceLabel := ""
				if fetchResp.Source == "wayback" {
					sourceLabel = " via fallback: wayback"
				}
				chrs := utf8.RuneCountInString(fetchResp.Content)
				// Skip appending garbage content to LLM context.
				// Symmetric with direct fetch (/webfetch): Source == "" && chars < 150
				if fetchResp.Source == "" && chrs < 150 {
					fmt.Printf("...%s: all fetch methods returned minimal content, skipping\n", urlStr)
					continue
				}
				fmt.Printf("...fetched: %s (%d chars)%s\n", urlStr, chrs, sourceLabel)
				formatted := FormatFetchResultsBlock(urlStr, fetchResp.Content)
				m.Messages = append(m.Messages, ChatMessage{
					Content:   formatted,
					FromUser:  false,
					Timestamp: time.Now(),
				})
			}
		}
		return

	case prefixMatch(commandPrefix, "/webfetch"):
		if !m.Config.WebFetch.Enabled {
			m.Println("Web fetch is not enabled. Configure web_fetch.enabled: true in your config.")
			return
		}
		if len(parts) < 2 {
			m.Println("Usage: /webfetch <url>")
			return
		}
		urlStr := strings.Join(parts[1:], " ")
		m.handleWebFetch(urlStr)
		return

	case prefixMatch(commandPrefix, "/mcp"):
		// Allow /mcp load even when MCP is not yet configured
		if m.McpManager == nil {
			if len(parts) >= 2 && parts[1] == "load" {
				m.reloadMcp()
				return
			}
			m.Println("MCP not configured. Create ~/.config/tmuxai/mcp.json and use /mcp load.")
			return
		}
		if len(parts) == 1 || (len(parts) == 2 && parts[1] == "list") {
			m.showMcpServers()
			return
		} else if len(parts) >= 2 && parts[1] == "tools" {
			serverFilter := ""
			if len(parts) >= 3 {
				serverFilter = parts[2]
			}
			m.showMcpTools(serverFilter)
			return
		} else if len(parts) >= 2 && parts[1] == "load" {
			m.reloadMcp()
			return
		} else if len(parts) >= 2 && parts[1] == "reload" {
			m.reloadMcpIncremental()
			return
		} else if len(parts) >= 2 && parts[1] == "unload" {
			m.unloadMcp()
			return
		}
		m.Println("Usage: /mcp [list|tools <server>|load|reload|unload]")

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

	// Display loaded skills information
	if m.Skills != nil && len(m.LoadedSkills) > 0 {
		formatLine("Loaded Skills", fmt.Sprintf("%d (%d chars)", len(m.LoadedSkills), m.Skills.UsedChars))
	}

	// Display MCP information
	// MCP server status and tool count summary
	if m.McpManager != nil {
		servers := m.McpManager.GetServerInfo()
		if len(servers) > 0 {
			active := 0
			unhealthy := 0
			disabled := 0
			totalTools := 0
			for _, s := range servers {
				switch s.Status {
				case mcp.StatusHealthy:
					active++
					totalTools += len(s.Tools)
				case mcp.StatusUnhealthy:
					unhealthy++
				}
				if s.Config.Disabled {
					disabled++
				}
			}
			// Estimate tokens from the actual tool definitions text
			mcpTokens := system.EstimateTokenCount(m.ensureMcpToolDefs())
			fmt.Println(formatter.FormatSection("\nMCP"))
			formatLine("Active", fmt.Sprintf("%d (total tools: %d, ~%d tokens)", active, totalTools, mcpTokens))
			if unhealthy > 0 {
				formatLine("Unhealthy", unhealthy)
			}
			if disabled > 0 {
				formatLine("Disabled", disabled)
			}
		}
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

// handleWebSearch performs a web search and injects results into context.
func (m *Manager) handleWebSearch(query string) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(m.Config.WebSearch.TimeoutSeconds)*time.Second)
	defer cancel()

	fmt.Printf("Searching for \"%s\"...\n", query)

	resp := m.SearchEngine.Search(ctx, query)
	if resp.Error != nil {
		m.Println(fmt.Sprintf("Search failed: %v", resp.Error))
		return
	}

	formatted := FormatSearchResultsBlock(query, resp.Provider, resp.Results)
	fmt.Println(formatted)

	// Inject into chat history so the LLM can see results on the next interaction
	m.Messages = append(m.Messages, ChatMessage{
		Content:   formatted,
		FromUser:  false,
		Timestamp: time.Now(),
	})
}

// handleWebFetch fetches content from a URL and injects it into context.
// Uses the full fallback chain: direct → Wayback Machine.
func (m *Manager) handleWebFetch(rawURL string) {
	cfg := m.Config.WebFetch
	// Progress feedback before blocking network call
	fmt.Printf("Fetching %s...\n", rawURL)

	// Wrap with context timeout
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.TimeoutSeconds)*time.Second)
	defer cancel()

	// Use unified fallback chain
	resp := FetchWithFallbacks(ctx, rawURL, cfg.MaxChars, cfg.TimeoutSeconds, cfg.AllowedRedirects)

	// Guard: skip injecting empty/minimal content into LLM context.
	// Symmetric with auto-fetch path (websearch -f N).
	// Threshold 150 matches auto-fetch; needsFallback uses 80 for a different purpose.
	charCount := utf8.RuneCountInString(resp.Content)
	if resp.Source == "" && charCount < 150 {
		m.Println(fmt.Sprintf("⚠ %s — all fetch methods returned minimal content (%d chars). This page may require JavaScript rendering.", rawURL, charCount))
		return
	}

	// Build source label for display
	sourceLabel := ""
	switch resp.Source {
	case "wayback":
		sourceLabel = " (wayback archive)"
	}

	// Inject FULL content into chat history so the LLM can see it
	formatted := FormatFetchResultsBlock(rawURL, resp.Content)
	m.Messages = append(m.Messages, ChatMessage{
		Content:   formatted,
		FromUser:  false,
		Timestamp: time.Now(),
	})

	// Print condensed status to terminal (full content stays in LLM context)
	tokenEstimate := (charCount + 3) / 4
	m.Println(fmt.Sprintf("✓ Fetched %d chars (≈%d tokens)%s from %s", charCount, tokenEstimate, sourceLabel, rawURL))
}

// --- web search command handlers below this line ---

func (m *Manager) showMcpServers() {
	servers := m.McpManager.GetServerInfo()
	if len(servers) == 0 {
		m.Println("No MCP servers configured.")
		return
	}

	var b strings.Builder
	b.WriteString("MCP Servers:\n")
	totalTools := 0
	totalTokens := 0
	for _, s := range servers {
		var icon string
		switch s.Status {
		case mcp.StatusHealthy:
			icon = "✓"
		case mcp.StatusUnhealthy:
			if s.Config.Disabled {
				icon = "○"
			} else {
				icon = "✗"
			}
		default:
			icon = "○"
		}
		toolCount := len(s.Tools)
		detail := fmt.Sprintf("(%s)", s.Transport)
		if s.Status == mcp.StatusHealthy {
			detail += fmt.Sprintf(" %d tools", toolCount)
			totalTools += toolCount
			tokenEst := 0
			for _, t := range s.Tools {
				tokenEst += (len(t.Name) + len(t.Description)) / 4
			}
			if tokenEst > 0 {
				detail += fmt.Sprintf(" (~%d tokens)", tokenEst)
				totalTokens += tokenEst
			}
		} else if s.ErrMsg != "" {
			detail += fmt.Sprintf(" error: %s", s.ErrMsg)
		}
		fmt.Fprintf(&b, "  %s %s %s\n", icon, s.Name, detail)
	}

	active := 0
	for _, s := range servers {
		if s.Status == mcp.StatusHealthy {
			active++
		}
	}
	fmt.Fprintf(&b, "\nTotal: %d/%d active, %d tools (~%d tokens)", active, len(servers), totalTools, totalTokens)
	m.Println(b.String())
}

func (m *Manager) showMcpTools(filter string) {
	servers := m.McpManager.GetServerInfo()
	var b strings.Builder
	found := false
	for _, s := range servers {
		if s.Status != mcp.StatusHealthy {
			continue
		}
		if filter != "" && !strings.EqualFold(s.Name, filter) {
			continue
		}
		if found {
			b.WriteString("\n")
		}
		found = true
		fmt.Fprintf(&b, "--- %s ---\n", s.Name)
		for _, t := range s.Tools {
			fqName := "mcp__" + s.Name + "__" + t.Name
			desc := t.Description
			if desc == "" {
				desc = "(no description)"
			}
			fmt.Fprintf(&b, "  %-40s %s\n", fqName, desc)
		}
	}
	if !found {
		if filter != "" {
			m.Println(fmt.Sprintf("No tools found for server '%s'.", filter))
		} else {
			m.Println("No MCP tools available.")
		}
		return
	}
	m.Println(strings.TrimRight(b.String(), "\n"))
}

func (m *Manager) reloadMcp() {
	if m.McpManager != nil {
		m.McpManager.Shutdown()
	}

	mcpCfg, err := mcp.LoadConfig(mcp.DefaultConfigPath())
	if err != nil {
		m.Println(fmt.Sprintf("Error loading MCP config: %v", err))
		m.McpManager = nil
		m.McpRegistry = nil
		return
	}
	if mcpCfg == nil || len(mcpCfg.MCPServers) == 0 {
		m.Println("No MCP servers configured.")
		m.McpManager = nil
		m.McpRegistry = nil
		return
	}

	mgr := mcp.NewMCPManager(mcpCfg)
	if err := mgr.Init(); err != nil {
		logger.Info("MCP reload: init had errors: %v", err)
	}

	servers := mgr.GetServerInfo()
	activeServers := 0
	totalTools := 0
	for _, s := range servers {
		if s.Status == mcp.StatusHealthy {
			activeServers++
			totalTools += len(s.Tools)
		}
	}

	m.McpManager = mgr
	m.McpRegistry = mcp.NewRegistry(mgr)
	m.mcpDirty = true

	m.Println(fmt.Sprintf("MCP reloaded: %d servers, %d tools", activeServers, totalTools))
	logger.Info("MCP reloaded: %d servers, %d tools", activeServers, totalTools)
}

func (m *Manager) unloadMcp() {
	if m.McpManager != nil {
		m.McpManager.Shutdown()
	}
	m.McpManager = nil
	m.McpRegistry = nil
	m.McpToolDefCached = ""
	m.mcpDirty = false
	m.Println("MCP unloaded.")
	logger.Info("MCP unloaded")
}

// reloadMcpIncremental is an alias for reloadMcp — full reconnect is sufficient for now.
// Incremental diffing (add/remove/change per-server) can be added later if needed.
func (m *Manager) reloadMcpIncremental() {
	if m.McpManager == nil {
		m.Println("No MCP manager initialized. Use /mcp load first.")
		return
	}

	mcpCfg, err := mcp.LoadConfig(mcp.DefaultConfigPath())
	if err != nil {
		m.Println(fmt.Sprintf("Error loading MCP config: %v", err))
		return
	}
	if mcpCfg == nil || len(mcpCfg.MCPServers) == 0 {
		m.Println("No MCP servers configured in config.")
		return
	}

	added, removed, restarted, kept, firstErr := m.McpManager.Reload(mcpCfg)
	if firstErr != nil {
		logger.Info("MCP incremental reload had errors: %v", firstErr)
	}

	m.McpRegistry = mcp.NewRegistry(m.McpManager)
	m.mcpDirty = true

	m.Println(fmt.Sprintf("MCP hot-reloaded: added=%d removed=%d restarted=%d kept=%d",
		added, removed, restarted, kept))
	logger.Info("MCP hot-reloaded: added=%d removed=%d restarted=%d kept=%d",
		added, removed, restarted, kept)
}
