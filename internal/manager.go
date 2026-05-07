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

