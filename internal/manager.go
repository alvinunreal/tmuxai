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

// TodoItem represents a single TODO item
type TodoItem struct {
	ID          int       `json:"id"`
	Description string    `json:"description"`
	Status      string    `json:"status"` // "pending", "in_progress", "completed"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// TodoList represents a collection of TODO items
type TodoList struct {
	ID        int        `json:"id"`
	Title     string     `json:"title"`
	Items     []TodoItem `json:"items"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
}

// GetProgress returns completion statistics for the TODO list
func (tl *TodoList) GetProgress() (completed, total int) {
	total = len(tl.Items)
	for _, item := range tl.Items {
		if item.Status == "completed" {
			completed++
		}
	}
	return
}

// GetCurrentItem returns the first non-completed TODO item, or nil if all are done
func (tl *TodoList) GetCurrentItem() *TodoItem {
	for i := range tl.Items {
		if tl.Items[i].Status != "completed" {
			return &tl.Items[i]
		}
	}
	return nil
}

type AIResponse struct {
	Message                string
	SendKeys               []string
	ExecCommand            []string
	PasteMultilineContent  string
	RequestAccomplished    bool
	ExecPaneSeemsBusy      bool
	WaitingForUserResponse bool
	NoComment              bool
	// TODO-related fields
	CreateTodoList         []string `json:"create_todo_list"` // List of TODO items to create
	UpdateTodoStatus       string   `json:"update_todo_status"` // "pending", "in_progress", "completed"
	UpdateTodoID           int      `json:"update_todo_id"` // ID of TODO item to update
	TodoCompleted          bool     `json:"todo_completed"` // Mark current TODO as completed
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
	
	// TODO tracking
	CurrentTodoList  *TodoList `json:"current_todo_list,omitempty"`
	TodoHistory      []TodoList `json:"todo_history,omitempty"`

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

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		PaneId:           paneId,
		Messages:         []ChatMessage{},
		ExecPane:         &system.TmuxPaneDetails{},
		OS:               os,
		SessionOverrides: make(map[string]interface{}),
	}

	manager.confirmedToExec = manager.confirmedToExecFn
	manager.getTmuxPanesInXml = manager.getTmuxPanesInXmlFn

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
	CreateTodoList: %v
	UpdateTodoStatus: %s
	UpdateTodoID: %d
	TodoCompleted: %v
`,
		ai.Message,
		ai.SendKeys,
		ai.ExecCommand,
		ai.PasteMultilineContent,
		ai.RequestAccomplished,
		ai.ExecPaneSeemsBusy,
		ai.WaitingForUserResponse,
		ai.NoComment,
		ai.CreateTodoList,
		ai.UpdateTodoStatus,
		ai.UpdateTodoID,
		ai.TodoCompleted,
	)
}

// CreateTodoList creates a new TODO list for the current task
func (m *Manager) CreateTodoList(title string, items []string) *TodoList {
	todoList := &TodoList{
		ID:        len(m.TodoHistory) + 1,
		Title:     title,
		Items:     make([]TodoItem, 0, len(items)),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	for i, item := range items {
		todoItem := TodoItem{
			ID:          i + 1,
			Description: item,
			Status:      "pending",
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
		todoList.Items = append(todoList.Items, todoItem)
	}

	m.CurrentTodoList = todoList
	return todoList
}

// UpdateTodoItem updates the status of a specific TODO item
func (m *Manager) UpdateTodoItem(itemID int, status string) bool {
	if m.CurrentTodoList == nil {
		return false
	}

	for i := range m.CurrentTodoList.Items {
		if m.CurrentTodoList.Items[i].ID == itemID {
			m.CurrentTodoList.Items[i].Status = status
			m.CurrentTodoList.Items[i].UpdatedAt = time.Now()
			m.CurrentTodoList.UpdatedAt = time.Now()
			return true
		}
	}
	return false
}

// MarkCurrentTodoCompleted marks the current active TODO item as completed
func (m *Manager) MarkCurrentTodoCompleted() bool {
	if m.CurrentTodoList == nil {
		return false
	}

	currentItem := m.CurrentTodoList.GetCurrentItem()
	if currentItem == nil {
		return false
	}

	return m.UpdateTodoItem(currentItem.ID, "completed")
}

// CompleteTodoList marks the todo list as completed and moves it to history
func (m *Manager) CompleteTodoList() {
	if m.CurrentTodoList != nil {
		m.TodoHistory = append(m.TodoHistory, *m.CurrentTodoList)
		m.CurrentTodoList = nil
	}
}

// FormatTodoList formats the TODO list for display in the chat pane
func (m *Manager) FormatTodoList() string {
	if m.CurrentTodoList == nil {
		return ""
	}

	var builder strings.Builder
	completed, total := m.CurrentTodoList.GetProgress()
	
	// Title with progress
	title := color.New(color.FgCyan, color.Bold).Sprintf("📋 %s", m.CurrentTodoList.Title)
	progress := color.New(color.FgWhite).Sprintf(" (%d/%d)", completed, total)
	builder.WriteString(title + progress + "\n")

	// TODO items
	for _, item := range m.CurrentTodoList.Items {
		var checkbox, status string
		switch item.Status {
		case "completed":
			checkbox = color.New(color.FgGreen).Sprint("☑")
			status = color.New(color.FgGreen).Sprint(item.Description)
		case "in_progress":
			checkbox = color.New(color.FgYellow).Sprint("🔄")
			status = color.New(color.FgYellow).Sprint(item.Description)
		default: // pending
			checkbox = color.New(color.FgWhite).Sprint("☐")
			status = color.New(color.FgWhite).Sprint(item.Description)
		}
		builder.WriteString(fmt.Sprintf("  %s %s\n", checkbox, status))
	}

	return builder.String()
}
