package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
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
}

// Parsed only when pane is prepared
type CommandExecHistory struct {
	Command string
	Output  string
	Code    int
}

// Manager represents the TmuxAI manager agent
type Manager struct {
	Config           *config.Config
	AiClient         *AiClient
	Status           string // running, waiting, done
	PaneId           string
	ExecPane         *system.TmuxPaneDetails
	Messages         []ChatMessage
	ExecHistory      []CommandExecHistory
	WatchMode        bool
	OS               string
	SessionOverrides map[string]interface{} // session-only config overrides
	ActiveModel      string                 // currently active model profile name

	// Functions for mocking
	confirmedToExec  func(command string, prompt string, edit bool) (bool, string)
	getTmuxPanesInXml func(config *config.Config) string
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" && cfg.AzureOpenAI.APIKey == "" && cfg.OpenAI.APIKey == "" {
		fmt.Println("An API key is required. Set OpenAI, OpenRouter, or Azure OpenAI credentials in the config file or environment variables.")
		return nil, fmt.Errorf("API key required")
	}

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
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecPane:         &system.TmuxPaneDetails{},
		OS:               os,
		SessionOverrides: make(map[string]interface{}),
		ActiveModel:      cfg.DefaultModel,
	}

	manager.confirmedToExec = manager.confirmedToExecFn
	manager.getTmuxPanesInXml = manager.getTmuxPanesInXmlFn

	// Initialize AI client with active model if set
	if manager.ActiveModel != "" {
		manager.UpdateAiClientForModel()
	}

	manager.InitExecPane()
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
`,
		ai.Message,
		ai.SendKeys,
		ai.ExecCommand,
		ai.PasteMultilineContent,
		ai.RequestAccomplished,
		ai.ExecPaneSeemsBusy,
		ai.WaitingForUserResponse,
		ai.NoComment,
	)
}

// SetActiveModel sets the active model and updates the AI client
func (m *Manager) SetActiveModel(modelName string) error {
	if modelName == "" {
		return fmt.Errorf("model name cannot be empty")
	}

	// Check if the model exists
	if _, exists := m.Config.Models[modelName]; !exists {
		return fmt.Errorf("model '%s' not found in configuration", modelName)
	}

	m.ActiveModel = modelName
	return m.UpdateAiClientForModel()
}

// UpdateAiClientForModel updates the AI client based on the active model
func (m *Manager) UpdateAiClientForModel() error {
	if m.ActiveModel == "" {
		return nil
	}

	modelCfg, exists := m.Config.Models[m.ActiveModel]
	if !exists {
		return fmt.Errorf("model '%s' not found", m.ActiveModel)
	}

	// Update config based on model provider
	switch strings.ToLower(modelCfg.Provider) {
	case "openai":
		m.SessionOverrides["openai.api_key"] = modelCfg.APIKey
		m.SessionOverrides["openai.model"] = modelCfg.Model
		if modelCfg.BaseURL != "" {
			m.SessionOverrides["openai.base_url"] = modelCfg.BaseURL
		}
	case "openrouter":
		m.SessionOverrides["openrouter.api_key"] = modelCfg.APIKey
		m.SessionOverrides["openrouter.model"] = modelCfg.Model
		if modelCfg.BaseURL != "" {
			m.SessionOverrides["openrouter.base_url"] = modelCfg.BaseURL
		}
	case "azure":
		m.SessionOverrides["azure_openai.api_key"] = modelCfg.APIKey
		if modelCfg.APIBase != "" {
			m.SessionOverrides["azure_openai.api_base"] = modelCfg.APIBase
		}
		if modelCfg.APIVersion != "" {
			m.SessionOverrides["azure_openai.api_version"] = modelCfg.APIVersion
		}
		if modelCfg.DeploymentName != "" {
			m.SessionOverrides["azure_openai.deployment_name"] = modelCfg.DeploymentName
		}
	default:
		return fmt.Errorf("unknown provider '%s' for model '%s'", modelCfg.Provider, m.ActiveModel)
	}

	// Recreate AI client with updated config
	m.AiClient = NewAiClient(m.Config)

	return nil
}

// GetActiveModelInfo returns information about the currently active model
func (m *Manager) GetActiveModelInfo() string {
	if m.ActiveModel == "" {
		// Fall back to showing the traditional config
		provider := "OpenRouter"
		model := m.GetModel()

		if m.GetOpenAIAPIKey() != "" {
			provider = "OpenAI"
		} else if m.GetAzureOpenAIAPIKey() != "" {
			provider = "Azure OpenAI"
		}

		return fmt.Sprintf("Provider: %s\nModel: %s\n(Using direct configuration, no model profile active)", provider, model)
	}

	modelCfg, exists := m.Config.Models[m.ActiveModel]
	if !exists {
		return fmt.Sprintf("Active model '%s' not found in configuration", m.ActiveModel)
	}

	var info strings.Builder
	info.WriteString(fmt.Sprintf("Active Model: %s\n", m.ActiveModel))
	info.WriteString(fmt.Sprintf("Provider: %s\n", modelCfg.Provider))
	info.WriteString(fmt.Sprintf("Model: %s\n", modelCfg.Model))
	if modelCfg.BaseURL != "" {
		info.WriteString(fmt.Sprintf("Base URL: %s\n", modelCfg.BaseURL))
	}
	if modelCfg.APIBase != "" {
		info.WriteString(fmt.Sprintf("API Base: %s\n", modelCfg.APIBase))
	}

	return info.String()
}

// ListModels returns a list of all configured model names
func (m *Manager) ListModels() []string {
	models := make([]string, 0, len(m.Config.Models))
	for name := range m.Config.Models {
		models = append(models, name)
	}
	return models
}
