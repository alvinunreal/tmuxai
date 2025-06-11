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
	ExecPane         *system.PaneDetails
	Messages         []ChatMessage
	ExecHistory      []CommandExecHistory
	WatchMode        bool
	OS               string
	SessionOverrides map[string]interface{} // session-only config overrides
	Multiplexer      system.Multiplexer     // New field for multiplexer interface
	TargetWindow     string                 // Current window/session target
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" {
		fmt.Println("OpenRouter API key is required. Set it in the config file or as an environment variable: TMUXAI_OPENROUTER_API_KEY")
		return nil, fmt.Errorf("OpenRouter API key is required")
	}

	// Initialize multiplexer
	multiplexer, err := system.NewMultiplexer()
	if err != nil {
		// If we're not in a multiplexer session, try to create one
		// For now, default to tmux for backward compatibility
		multiplexer = system.NewTmuxBackend()
		paneId, err := multiplexer.CreateSession()
		if err != nil {
			return nil, fmt.Errorf("failed to create session: %w", err)
		}
		args := strings.Join(os.Args[1:], " ")

		multiplexer.SendCommand(paneId, "tmuxai "+args)
		// shell initialization may take some time
		time.Sleep(1 * time.Second)
		multiplexer.SendKeys(paneId, "Enter")
		err = multiplexer.AttachSession(paneId)
		if err != nil {
			return nil, fmt.Errorf("failed to attach to session: %w", err)
		}
		os.Exit(0)
	}

	// Get current pane and session info
	paneId, err := multiplexer.GetCurrentPaneId()
	if err != nil {
		return nil, fmt.Errorf("failed to get current pane ID: %w", err)
	}

	targetWindow, err := multiplexer.GetCurrentSession()
	if err != nil {
		return nil, fmt.Errorf("failed to get current session: %w", err)
	}

	aiClient := NewAiClient(&cfg.OpenRouter)
	osDetails := system.GetOSDetails()

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecPane:         &system.PaneDetails{},
		OS:               osDetails,
		SessionOverrides: make(map[string]interface{}),
		Multiplexer:      multiplexer,
		TargetWindow:     targetWindow,
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
