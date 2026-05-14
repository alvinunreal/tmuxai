package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/internal/mcp"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/fatih/color"
)

type AIResponse struct {
	Message                string
	SendKeys               []string
	ExecCommand            []string
	PasteMultilineContent  string
	RequestAccomplished    bool
	ExecPaneSeemsBusy      bool
	WaitingForUserResponse bool
	NoComment              bool
	MCPToolCalls           []mcp.MCPToolCall
}

type ManagerOptions struct {
	ForcedExecPaneID  string
	ForcedReadPaneIDs []string
}

// Parsed only when pane is prepared
type CommandExecHistory struct {
	Command string
	Output  string
	Code    int
}

// Manager represents the TmuxAI manager agent
type Manager struct {
	Config            *config.Config
	AiClient          *AiClient
	Status            string // running, waiting, done
	PaneId            string
	ExecPane          *system.TmuxPaneDetails
	Messages          []ChatMessage
	ExecHistory       []CommandExecHistory
	WatchMode         bool
	OS                string
	SessionOverrides  map[string]interface{} // session-only config overrides
	LoadedKBs         map[string]string      // Loaded knowledge bases (name -> content)
	LoadedSkills      map[string]string      // Loaded skill bodies + manifests (name -> content)
	Skills            *SkillRegistry         // Skill registry (discovery, L1, budget)
	ForcedExecPaneID  string
	ForcedReadPaneIDs map[string]bool

	SearchEngine *SearchEngine

	McpManager       *mcp.MCPManager
	McpRegistry      *mcp.Registry
	McpToolDefCached string
	mcpDirty         bool

	// Functions for mocking
	confirmedToExec   func(command string, prompt string, edit bool) (bool, string)
	getTmuxPanesInXml func(config *config.Config) string
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config, options ManagerOptions) (*Manager, error) {

	paneId, err := system.TmuxCurrentPaneId()
	if err != nil {
		// If we're not in a tmux session, start a new session and execute the same command
		paneId, err = system.TmuxCreateSession()
		if err != nil {
			return nil, fmt.Errorf("system.TmuxCreateSession failed: %w", err)
		}
		args := strings.Join(os.Args[1:], " ")

		_ = system.TmuxSendCommandToPane(paneId, "tmuxai "+args, true)
		// shell initialization may take some time
		time.Sleep(1 * time.Second)
		_ = system.TmuxSendCommandToPane(paneId, "Enter", false)
		err = system.TmuxAttachSession(paneId)
		if err != nil {
			return nil, fmt.Errorf("system.TmuxAttachSession failed: %w", err)
		}
		os.Exit(0)
	}

	aiClient := NewAiClient(cfg)
	os := system.GetOSDetails()

	manager := &Manager{
		Config:            cfg,
		AiClient:          aiClient,
		PaneId:            paneId,
		Messages:          []ChatMessage{},
		ExecPane:          &system.TmuxPaneDetails{},
		OS:                os,
		SessionOverrides:  make(map[string]interface{}),
		LoadedKBs:         make(map[string]string),
		LoadedSkills:      make(map[string]string),
		ForcedExecPaneID:  options.ForcedExecPaneID,
		ForcedReadPaneIDs: make(map[string]bool),
	}

	for _, paneID := range options.ForcedReadPaneIDs {
		manager.ForcedReadPaneIDs[paneID] = true
	}

	// Set the config manager in the AI client
	aiClient.SetConfigManager(manager)

	manager.confirmedToExec = manager.confirmedToExecFn
	manager.getTmuxPanesInXml = manager.getTmuxPanesInXmlFn

	if err := manager.InitExecPane(); err != nil {
		return nil, err
	}

	// Auto-load knowledge bases from config
	manager.autoLoadKBs()

	// Initialize skill registry if enabled
	if manager.Config.KnowledgeBase.Skills.Enabled {
		reg, err := InitSkills(&manager.Config.KnowledgeBase.Skills)
		if err != nil {
			logger.Info("Skill initialization failed: %v", err)
		} else {
			manager.Skills = reg
		}
	}

	// Initialize web search engine if enabled
	manager.initSearchEngine()

	manager.initMCP()

	return manager, nil
}

// Start starts the manager agent
func (m *Manager) Start(initMessage string) error {
	cliInterface := NewCLIInterface(m)
	if initMessage != "" {
		logger.Info("Initial task provided: %s", initMessage)
	}
	if err := cliInterface.Start(initMessage); err != nil {
		logger.Error("Failed to start CLI interface: %v", err)
		return err
	}

	return nil
}

func (m *Manager) Println(msg string) {
	fmt.Println(m.GetPrompt() + msg)
}

func (m *Manager) GetConfig() *config.Config {
	return m.Config
}

// getPrompt returns the prompt string with color
func (m *Manager) GetPrompt() string {
	// Check if custom prompt template is configured
	customPrompt := m.getCustomPromptTemplate()
	if customPrompt != "" {
		return m.renderPromptTemplate(customPrompt)
	}

	// Default prompt behavior (backward compatible)
	tmuxaiColor := color.New(color.FgGreen, color.Bold)
	arrowColor := color.New(color.FgYellow, color.Bold)
	stateColor := color.New(color.FgMagenta, color.Bold)
	modelColor := color.New(color.FgCyan, color.Bold)

	var stateSymbol string
	switch m.Status {
	case "running":
		stateSymbol = "▶"
	case "waiting":
		stateSymbol = "?"
	case "done":
		stateSymbol = "✓"
	default:
		stateSymbol = ""
	}
	if m.WatchMode {
		stateSymbol = "∞"
	}

	prompt := tmuxaiColor.Sprint("TmuxAI")

	// Show current model if it's not the default or first available model
	currentModel := m.GetModelsDefault()
	availableModels := m.GetAvailableModels()
	if len(availableModels) > 0 {
		// Get the "expected" model (configured default or first available)
		expectedModel := m.Config.DefaultModel
		if expectedModel == "" && len(availableModels) > 0 {
			expectedModel = availableModels[0] // First model as default
		}

		// Show model if current is different from expected
		if currentModel != "" && currentModel != expectedModel {
			prompt += " " + modelColor.Sprint("["+currentModel+"]")
		}
	}

	if stateSymbol != "" {
		prompt += " " + stateColor.Sprint("["+stateSymbol+"]")
	}
	prompt += arrowColor.Sprint(" » ")
	return prompt
}

// getCustomPromptTemplate returns the custom prompt template if configured
func (m *Manager) getCustomPromptTemplate() string {
	// Check session override first
	if override, exists := m.SessionOverrides["prompts.prompt"]; exists {
		if val, ok := override.(string); ok {
			return val
		}
	}
	return m.Config.Prompts.Prompt
}

// getCurrentContextSize calculates the total token count of all messages
func (m *Manager) getCurrentContextSize() int {
	var totalTokens int
	for _, msg := range m.Messages {
		totalTokens += system.EstimateTokenCount(msg.Content)
	}
	return totalTokens
}

// formatCompactNumber formats a number in compact form (e.g., 37000 -> 37k)
func formatCompactNumber(n int) string {
	if n >= 1000000 {
		return fmt.Sprintf("%.1fm", float64(n)/1000000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%dk", n/1000)
	}
	return fmt.Sprintf("%d", n)
}

// getModelLabel returns the current model label for display
func (m *Manager) getModelLabel() string {
	currentModel := m.GetModelsDefault()
	if currentModel != "" {
		return currentModel
	}

	// Fallback to legacy model name
	if modelConfig, exists := m.GetCurrentModelConfig(); exists {
		return modelConfig.Model
	}
	return "unknown"
}

// renderPromptTemplate renders a prompt template with placeholders
func (m *Manager) renderPromptTemplate(template string) string {
	// Define colors
	tmuxaiColor := color.New(color.FgGreen, color.Bold)
	modelColor := color.New(color.FgCyan, color.Bold)
	stateColor := color.New(color.FgMagenta, color.Bold)
	contextColor := color.New(color.FgWhite)
	maxContextColor := color.New(color.FgWhite)
	percentColor := color.New(color.FgWhite)

	// Get state symbol
	var stateSymbol string
	switch m.Status {
	case "running":
		stateSymbol = "▶"
	case "waiting":
		stateSymbol = "?"
	case "done":
		stateSymbol = "✓"
	default:
		stateSymbol = ""
	}
	if m.WatchMode {
		stateSymbol = "∞"
	}

	// Calculate context info only when the template needs it. Prompt rendering can
	// happen frequently, and scanning all messages is unnecessary for compact
	// prompts such as "✨ ".
	contextSize := 0
	maxContext := 0
	contextPercent := 0.0
	needsContext := strings.Contains(template, "{context}") ||
		strings.Contains(template, "{max_context}") ||
		strings.Contains(template, "{context_percent}")
	if needsContext {
		contextSize = m.getCurrentContextSize()
		maxContext = m.GetMaxContextSize()
		if maxContext > 0 {
			contextPercent = float64(contextSize) / float64(maxContext) * 100
		}
	}

	// Build placeholder map
	placeholders := map[string]string{
		"{app}":             tmuxaiColor.Sprint("TmuxAI"),
		"{model}":           modelColor.Sprint(m.getModelLabel()),
		"{state}":           stateColor.Sprint(stateSymbol),
		"{context}":         contextColor.Sprint(formatCompactNumber(contextSize)),
		"{max_context}":     maxContextColor.Sprint(formatCompactNumber(maxContext)),
		"{context_percent}": percentColor.Sprint(fmt.Sprintf("%.0f%%", contextPercent)),
	}

	// Replace placeholders
	result := template
	for placeholder, value := range placeholders {
		result = strings.ReplaceAll(result, placeholder, value)
	}

	return result
}

func (ai *AIResponse) String() string {
	return fmt.Sprintf(`
	Message: %s
	SendKeys: %v
	ExecCommand: %v
	PasteMultilineContent: %s
	RequestAccomplished: %v
	ExecPaneSeemsBusy: %v
	WaitingForUserResponse: %v
	NoComment: %v
	MCPToolCalls: %d
`,
		ai.Message,
		ai.SendKeys,
		ai.ExecCommand,
		ai.PasteMultilineContent,
		ai.RequestAccomplished,
		ai.ExecPaneSeemsBusy,
		ai.WaitingForUserResponse,
		ai.NoComment,
		len(ai.MCPToolCalls),
	)
}

// initSearchEngine initializes the web search engine if enabled in config.
func (m *Manager) initSearchEngine() {
	cfg := m.Config.WebSearch
	if !cfg.Enabled {
		return
	}

	providers := make([]WebSearchProvider, 0)

	// Build providers list, putting default_provider first
	defaultProv := cfg.DefaultProvider
	if defaultProv == "" {
		defaultProv = "brave"
	}

	// Add default provider first
	if pcfg, ok := cfg.Providers[defaultProv]; ok {
		switch defaultProv {
		case "brave":
			providers = append(providers, NewBraveProvider(pcfg.APIKey, pcfg.BaseURL, nil, cfg.TimeoutSeconds))
		case "searxng":
			if prov, err := NewSearXNGProvider(pcfg.BaseURL, nil, cfg.TimeoutSeconds); err == nil {
				providers = append(providers, prov)
			} else {
				logger.Debug("Failed to init SearXNG provider: %v", err)
			}
		}
	}

	// Add remaining providers
	for name, pcfg := range cfg.Providers {
		if name == defaultProv {
			continue
		}
		switch name {
		case "brave":
			providers = append(providers, NewBraveProvider(pcfg.APIKey, pcfg.BaseURL, nil, cfg.TimeoutSeconds))
		case "searxng":
			if prov, err := NewSearXNGProvider(pcfg.BaseURL, nil, cfg.TimeoutSeconds); err == nil {
				providers = append(providers, prov)
			} else {
				logger.Debug("Failed to init SearXNG provider: %v", err)
			}
		}
	}

	if len(providers) > 0 {
		m.SearchEngine = NewSearchEngine(providers, cfg.MaxResults, cfg.MaxResultChars)
	}
}

func (m *Manager) initMCP() {
	mcpCfg, err := mcp.LoadConfig(mcp.DefaultConfigPath())
	if err != nil {
		logger.Info("MCP: config load failed: %v", err)
		return
	}
	if mcpCfg == nil || len(mcpCfg.MCPServers) == 0 {
		return
	}

	mgr := mcp.NewMCPManager(mcpCfg)
	if err := mgr.Init(); err != nil {
		logger.Info("MCP: init had errors: %v", err)
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
	if totalTools > 0 {
		logger.Info("MCP: loaded %d servers with %d tools", activeServers, totalTools)
	} else {
		logger.Info("MCP: %d servers configured but 0 healthy tools", len(servers))
	}
}

// Cleanup performs graceful shutdown of all managed resources.
// It must be called when the Manager is no longer needed.
func (m *Manager) Cleanup() {
	if m.McpManager != nil {
		logger.Info("Shutting down MCP servers...")
		m.McpManager.Shutdown()
		m.McpManager = nil
		m.McpRegistry = nil
	}
}

func (m *Manager) ensureMcpToolDefs() string {
	if m.McpManager == nil {
		return ""
	}
	if m.mcpDirty {
		m.McpToolDefCached = m.McpManager.ToolDefinitionsBlock()
		m.mcpDirty = false
	}
	return m.McpToolDefCached
}
