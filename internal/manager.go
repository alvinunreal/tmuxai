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

// ActionStep represents a single action or message from the AI response sequence.
type ActionStep struct {
	Type    string // "message", "sendKeys", "execCommand", "pasteMultiline", "execAndWait"
	Content string // The text message, keys to send, command to execute, or content to paste
}

// AIResponse represents the parsed response from the AI, including a sequence of actions.
type AIResponse struct {
	Sequence []ActionStep // Ordered sequence of actions and messages
	State    string       // Final state determined by the AI (e.g., RequestAccomplished)
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
	Status           string // running, question, done
	PaneId           string
	ExecPane         *system.TmuxPaneDetails
	Messages         []ChatMessage
	ExecHistory      []CommandExecHistory
	WatchMode        bool
	OS               string
	SessionOverrides map[string]interface{} // session-only config overrides
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
	tmuxaiColor := color.New(color.FgHiGreen)
	arrowColor := color.New(color.FgHiYellow)
	stateColor := color.New(color.FgHiWhite)

	var stateSymbol string
	switch m.Status {
	case "running":
		stateSymbol = "▶"
	case "question":
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

// String representation for AIResponse
func (ai *AIResponse) String() string {
	var sequenceStr strings.Builder
	for i, step := range ai.Sequence {
		sequenceStr.WriteString(fmt.Sprintf("\n  Step %d: Type=%s, Content=`%s`", i+1, step.Type, step.Content))
	}
	if len(ai.Sequence) == 0 {
		sequenceStr.WriteString("\n  (No actions in sequence)")
	}

	return fmt.Sprintf(`
	State: %s
	Sequence: %s
`,
		ai.State,
		sequenceStr.String(),
	)
}
