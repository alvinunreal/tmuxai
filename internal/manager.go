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

// Parsed only when pane is prepared
type CommandExecHistory struct {
	Command string
	Output  string
	Code    int
}

type ExecCommandInfo struct {
	Command string
	PaneID  string
}

type SendKeysInfo struct {
	Keys   string
	PaneID string
}

type PasteInfo struct {
	Content string
	PaneID  string
}

type AIResponse struct {
	Message                string
	SendKeys               []SendKeysInfo
	ExecCommand            []ExecCommandInfo
	PasteMultilineContent  []PasteInfo
	RequestAccomplished    bool
	ExecPaneSeemsBusy      bool
	WaitingForUserResponse bool
	NoComment              bool
	CreateExecPane         bool
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
	LastExecPaneID   string
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" {
		fmt.Println("OpenRouter API key is required. Set it in the config file or as an environment variable: TMUXAI_OPENROUTER_API_KEY")
		return nil, fmt.Errorf("OpenRouter API key is required")
	}

	paneId, err := system.TmuxCurrentPaneId()
	if err != nil {
		// If we're not in a tmux session, start a new session and execute the same command
		paneId, err = system.TmuxCreateSession()
		if err != nil {
			return nil, fmt.Errorf("system.TmuxCreateSession failed: %w", err)
		}
		args := strings.Join(os.Args[1:], " ")

		system.TmuxSendCommandToPane(paneId, "tmuxai "+args, true)
		// shell initialization may take some time
		time.Sleep(1 * time.Second)
		system.TmuxSendCommandToPane(paneId, "Enter", false)
		err = system.TmuxAttachSession(paneId)
		if err != nil {
			return nil, fmt.Errorf("system.TmuxAttachSession failed: %w", err)
		}
		os.Exit(0)
	}

	aiClient := NewAiClient(&cfg.OpenRouter)
	os := system.GetOSDetails()

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecPane:         &system.TmuxPaneDetails{},
		OS:               os,
		SessionOverrides: make(map[string]interface{}),
		LastExecPaneID:   "",
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
	var execCommands []string
	for _, cmd := range ai.ExecCommand {
		execCommands = append(execCommands, fmt.Sprintf("{Cmd: %s, PaneID: %s}", cmd.Command, cmd.PaneID))
	}
	var sendKeys []string
	for _, sk := range ai.SendKeys {
		sendKeys = append(sendKeys, fmt.Sprintf("{Keys: %s, PaneID: %s}", sk.Keys, sk.PaneID))
	}
	var pasteContent []string
	for _, pc := range ai.PasteMultilineContent {
		pasteContent = append(pasteContent, fmt.Sprintf("{Content: %s, PaneID: %s}", pc.Content, pc.PaneID))
	}

	return fmt.Sprintf(`
	Message: %s
	SendKeys: %v
	ExecCommand: %v
	PasteMultilineContent: %v
	RequestAccomplished: %v
	ExecPaneSeemsBusy: %v
	WaitingForUserResponse: %v
	NoComment: %v
	CreateExecPane: %v
`,
		ai.Message,
		sendKeys,
		execCommands,
		pasteContent,
		ai.RequestAccomplished,
		ai.ExecPaneSeemsBusy,
		ai.WaitingForUserResponse,
		ai.NoComment,
		ai.CreateExecPane,
	)
}
