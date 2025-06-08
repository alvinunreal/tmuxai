package system

// PaneDetails represents the details of a multiplexer pane.
// This is a common structure that both Tmux and Zellij implementations will use.
type PaneDetails struct {
	Id                 string
	IsActive           int // Using int for boolean-like values from tmux; 0 or 1
	CurrentPid         int
	CurrentCommand     string
	CurrentCommandArgs []string // Store as a slice of strings
	HistorySize        int
	HistoryLimit       int
	IsSubShell         bool
	IsTmuxAiPane       bool   // Specific to tmuxai's main pane
	IsTmuxAiExecPane   bool   // Specific to tmuxai's execution pane
	IsPrepared         bool   // If the pane is prepared for exec (e.g. PS1 set)
	Shell              string // Shell running in the pane (e.g. bash, zsh)
	OS                 string // OS details if available
	Content            string // Captured content of the pane
	LastLine           string // Last line of the pane content
}

// Multiplexer defines the interface for interacting with a terminal multiplexer.
type Multiplexer interface {
	// GetCurrentPaneId returns the ID of the current pane.
	GetCurrentPaneId() (string, error)

	// CreateNewPane creates a new pane.
	// target: specifies the target window or pane for splitting.
	// command: optional command to run in the new pane.
	CreateNewPane(target string, command ...string) (string, error)

	// GetPaneDetails returns details for panes.
	// target: can be a window ID, session ID, or specific pane ID (depending on implementation).
	GetPaneDetails(target string) ([]PaneDetails, error)

	// CapturePane captures the content of a specific pane.
	// paneId: the ID of the pane to capture.
	// maxLines: the maximum number of lines to capture from the scrollback history.
	CapturePane(paneId string, maxLines int) (string, error)

	// SendCommandToPane sends a command (sequence of characters) to a specific pane.
	// paneId: the ID of the pane to send the command to.
	// command: the command string to send.
	// autoenter: whether to press Enter after sending the command.
	SendCommandToPane(paneId string, command string, autoenter bool) error

	// CreateSession creates a new multiplexer session.
	// command: optional command to run in the initial pane of the new session.
	CreateSession(command ...string) (string, error) // Returns new pane/session ID

	// AttachSession attaches the current terminal to an existing session.
	// sessionId: the ID of the session to attach to.
	AttachSession(sessionId string) error

	// ClearPane clears the content and history of a pane.
	// paneId: the ID of the pane to clear.
	ClearPane(paneId string) error

	// IsInsideSession checks if currently running inside a multiplexer session.
	IsInsideSession() bool

	// GetType returns a string indicating the type of multiplexer (e.g., "tmux", "zellij").
	GetType() string
}
