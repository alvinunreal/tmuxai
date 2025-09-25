package internal

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/session"
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

// CommandExecHistory represents a command execution history entry
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
	sessionStore     *session.FileStore
	currentSession   *session.Session
	autoSaveTicker   *time.Ticker
	autoSaveStop     chan bool

	// Functions for mocking
	confirmedToExec  func(command string, prompt string, edit bool) (bool, string)
	getTmuxPanesInXml func(config *config.Config) string
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" && cfg.AzureOpenAI.APIKey == "" {
		fmt.Println("An API key is required. Set OpenRouter or Azure OpenAI credentials in the config file or environment variables.")
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

	// Initialize session store
	sessionStore, err := session.NewFileStore()
	if err != nil {
		logger.Error("Failed to initialize session store: %v", err)
		// Continue without session support
		sessionStore = nil
	}

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecPane:         &system.TmuxPaneDetails{},
		OS:               os,
		SessionOverrides: make(map[string]interface{}),
		sessionStore:     sessionStore,
		autoSaveStop:     make(chan bool),
	}

	manager.confirmedToExec = manager.confirmedToExecFn
	manager.getTmuxPanesInXml = manager.getTmuxPanesInXmlFn

	manager.InitExecPane()

	// Start auto-save mechanism
	manager.startAutoSave()

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

// Session management methods

func (m *Manager) startAutoSave() {
	if m.sessionStore == nil {
		return
	}

	m.autoSaveTicker = time.NewTicker(30 * time.Second)
	go func() {
		for {
			select {
			case <-m.autoSaveTicker.C:
				if m.currentSession != nil {
					m.saveCurrentSession()
				}
			case <-m.autoSaveStop:
				return
			}
		}
	}()
}

func (m *Manager) stopAutoSave() {
	if m.autoSaveTicker != nil {
		m.autoSaveTicker.Stop()
		m.autoSaveStop <- true
	}
}

func (m *Manager) saveCurrentSession() {
	if m.sessionStore == nil || m.currentSession == nil {
		return
	}

	// Convert internal types to session types
	sessionMessages := make([]session.ChatMessage, len(m.Messages))
	for i, msg := range m.Messages {
		sessionMessages[i] = session.ChatMessage{
			Content:   msg.Content,
			FromUser:  msg.FromUser,
			Timestamp: msg.Timestamp,
		}
	}

	sessionExecHistory := make([]session.CommandExecHistory, len(m.ExecHistory))
	for i, hist := range m.ExecHistory {
		sessionExecHistory[i] = session.CommandExecHistory{
			Command: hist.Command,
			Output:  hist.Output,
			Code:    hist.Code,
		}
	}

	m.currentSession.Messages = sessionMessages
	m.currentSession.ExecHistory = sessionExecHistory

	// Generate summary if needed
	if m.currentSession.Summary == "" && len(m.Messages) > 0 {
		m.currentSession.Summary = m.generateSessionSummary()
	}

	if err := m.sessionStore.Save(m.currentSession); err != nil {
		logger.Error("Failed to save session: %v", err)
	}
}

func (m *Manager) generateSessionSummary() string {
	if len(m.Messages) == 0 {
		return "Empty session"
	}

	// Take first user message as summary (max 100 chars)
	for _, msg := range m.Messages {
		if msg.FromUser {
			content := msg.Content
			if len(content) > 100 {
				content = content[:97] + "..."
			}
			return content
		}
	}

	// Fallback to first message
	content := m.Messages[0].Content
	if len(content) > 100 {
		content = content[:97] + "..."
	}
	return content
}

func (m *Manager) LoadSession(sessionID string) error {
	if m.sessionStore == nil {
		return fmt.Errorf("session store not initialized")
	}

	loadedSession, err := m.sessionStore.Load(sessionID)
	if err != nil {
		return err
	}

	// Convert session types to internal types
	m.Messages = make([]ChatMessage, len(loadedSession.Messages))
	for i, msg := range loadedSession.Messages {
		m.Messages[i] = ChatMessage{
			Content:   msg.Content,
			FromUser:  msg.FromUser,
			Timestamp: msg.Timestamp,
		}
	}

	m.ExecHistory = make([]CommandExecHistory, len(loadedSession.ExecHistory))
	for i, hist := range loadedSession.ExecHistory {
		m.ExecHistory[i] = CommandExecHistory{
			Command: hist.Command,
			Output:  hist.Output,
			Code:    hist.Code,
		}
	}

	m.currentSession = loadedSession
	m.sessionStore.SetCurrent(sessionID)

	return nil
}

func (m *Manager) CreateNewSession(name string) {
	if m.sessionStore == nil {
		return
	}

	m.currentSession = &session.Session{
		ID:        fmt.Sprintf("%d", time.Now().Unix()),
		Name:      name,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	m.Messages = []ChatMessage{}
	m.ExecHistory = []CommandExecHistory{}
}

func (m *Manager) ListSessions(limit int) ([]session.SessionSummary, error) {
	if m.sessionStore == nil {
		return nil, fmt.Errorf("session store not initialized")
	}

	return m.sessionStore.List(limit)
}

func (m *Manager) DeleteSession(sessionID string) error {
	if m.sessionStore == nil {
		return fmt.Errorf("session store not initialized")
	}

	return m.sessionStore.Delete(sessionID)
}
