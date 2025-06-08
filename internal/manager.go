package internal

import (
	"fmt"
	"os"
	"os/exec"
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
	CurrentPaneId    string                 // Changed from PaneId
	Mux              system.Multiplexer     // Added Mux field
	ExecPane         *system.PaneDetails    // Changed type from TmuxPaneDetails
	Messages         []ChatMessage
	ExecHistory      []CommandExecHistory
	WatchMode        bool
	OS               string
	SessionOverrides map[string]interface{} // session-only config overrides
}

// NewManager creates a new manager agent
func NewManager(cfg *config.Config) (*Manager, error) {
	if cfg.OpenRouter.APIKey == "" {
		// This check can remain, or be moved after multiplexer detection if API key is only needed then.
		// For now, keeping it at the beginning.
		fmt.Println("OpenRouter API key is required. Set it in the config file or as an environment variable: TMUXAI_OPENROUTER_API_KEY")
		return nil, fmt.Errorf("OpenRouter API key is required")
	}

	var mux system.Multiplexer
	var currentPaneId string
	var err error

	if os.Getenv("ZELLIJ") == "0" && os.Getenv("ZELLIJ_PANE_ID") != "" {
		logger.Info("Zellij environment detected.")
		mux = &system.ZellijMultiplexer{}
	} else if os.Getenv("TMUX") != "" && os.Getenv("TMUX_PANE") != "" {
		logger.Info("Tmux environment detected.")
		mux = &system.TmuxMultiplexer{}
	} else {
		logger.Info("No existing Zellij or Tmux session detected. Attempting to start one...")
		var zellijErr error
		var tmuxErr error

		// Attempt to start Zellij
		logger.Info("Attempting to start Zellij...")
		zellijCmd := exec.Command("zellij", "attach", "--create")
		zellijCmd.Stdin = os.Stdin
		zellijCmd.Stdout = os.Stdout
		zellijCmd.Stderr = os.Stderr // Capture Zellij's errors for logging, though it might take over stderr

		if err := zellijCmd.Run(); err == nil {
			// If zellijCmd.Run() returns nil, it means Zellij started and then exited,
			// OR it has taken over the terminal and is running.
			// If it forked and the original process exited with 0, this indicates success.
			// If it took over, this Go process is likely about to be backgrounded or its output hidden.
			fmt.Println("Successfully started Zellij. Please re-run tmuxai inside the new Zellij session if it didn't start automatically.")
			logger.Info("Zellij started successfully. tmuxai will now exit.")
			os.Exit(0)
		} else {
			zellijErr = fmt.Errorf("zellij attach --create failed: %w", err)
			logger.Warning("Failed to start Zellij: %v", zellijErr)

			// Attempt to start Tmux
			logger.Info("Attempting to start a new Tmux session...")
			tempTmux := &system.TmuxMultiplexer{}
			newPaneId, errTmuxCreate := tempTmux.CreateSession() // command is optional
			if errTmuxCreate != nil {
				tmuxErr = fmt.Errorf("failed to create tmux session: %w", errTmuxCreate)
				logger.Error("Failed to create Tmux session: %v", tmuxErr)
				return nil, fmt.Errorf("no supported multiplexer found and failed to start a new one. Zellij error: [%v], Tmux error: [%v]", zellijErr, tmuxErr)
			}

			logger.Info("Tmux session created with pane ID: %s. Attempting to run tmuxai in it.", newPaneId)
			tmuxaiCmd := "tmuxai " + strings.Join(os.Args[1:], " ")
			if errSend := tempTmux.SendCommandToPane(newPaneId, tmuxaiCmd, true); errSend != nil {
				logger.Error("Failed to send command to new Tmux session: %v. Please start tmuxai manually in the new session.", errSend)
				// Don't necessarily fail here, session is running.
			}
			// Add a small delay for the command to register, then send Enter if needed.
			time.Sleep(500 * time.Millisecond)
			// It seems SendCommandToPane with autoenter=true should handle the Enter.
			// If not, an explicit Enter might be tempTmux.SendCommandToPane(newPaneId, "Enter", false)

			fmt.Printf("Tmux session created (Pane: %s). tmuxai is attempting to start within it.\n", newPaneId)
			fmt.Println("If tmuxai does not start automatically, please attach to the session (e.g., 'tmux attach') and run it.")

			// Attempt to attach to the new Tmux session
			logger.Info("Attempting to attach to the new Tmux session %s...", newPaneId)
			if errAttach := tempTmux.AttachSession(newPaneId); errAttach != nil {
				logger.Error("Failed to automatically attach to tmux session %s: %v. Please attach manually (e.g., 'tmux attach').", newPaneId, errAttach)
				// Even if attach fails, the session is running with tmuxai possibly starting.
			}
			// Whether attach succeeds or fails, the primary goal of starting tmux and tmuxai in it has been attempted.
			os.Exit(0)
		}
	}

	currentPaneId, err = mux.GetCurrentPaneId()
	if err != nil {
		// It's unlikely GetCurrentPaneId would fail if the environment variables were correctly detected,
		// but good to handle it.
		return nil, fmt.Errorf("failed to get current pane ID from %s: %w", mux.GetType(), err)
	}
	logger.Info("Current %s pane ID: %s", mux.GetType(), currentPaneId)

	aiClient := NewAiClient(&cfg.OpenRouter)
	// OS details might be obtainable via multiplexer in a more generic way later if needed
	osDetails := system.GetOSDetails() // Assuming this is a generic utility for now

	manager := &Manager{
		Config:           cfg,
		AiClient:         aiClient,
		Mux:              mux, // Assign the detected multiplexer
		CurrentPaneId:    currentPaneId, // Assign the fetched pane ID
		Messages:         []ChatMessage{},
		ExecPane:         &system.PaneDetails{}, // Initialize ExecPane; InitExecPane will populate it
		OS:               osDetails,
		SessionOverrides: make(map[string]interface{}),
	}

	// InitExecPane is responsible for finding or creating the execution pane.
	// It will need to be refactored to use manager.Mux in a later step.
	// For now, the call remains, and we assume it will be adapted or
	// it might error out if it still has hardcoded tmux calls.
	// The subtask asks to focus on detection logic.
	// If InitExecPane() call causes issues due to not being updated,
	// it might be temporarily commented out in a real dev cycle,
	// but per instructions, I'll keep the call.
	manager.InitExecPane() // This method will require refactoring in a later step.
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
