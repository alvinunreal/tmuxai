package system

import (
	"os"
)

// TmuxBackend implements the Multiplexer interface for tmux
type TmuxBackend struct{}

// NewTmuxBackend creates a new TmuxBackend instance
func NewTmuxBackend() *TmuxBackend {
	return &TmuxBackend{}
}

// Session operations

func (t *TmuxBackend) GetCurrentSession() (string, error) {
	return TmuxCurrentWindowTarget()
}

func (t *TmuxBackend) CreateSession() (string, error) {
	return TmuxCreateSession()
}

func (t *TmuxBackend) AttachSession(sessionId string) error {
	return TmuxAttachSession(sessionId)
}

// Pane operations

func (t *TmuxBackend) GetPaneDetails(target string) ([]PaneDetails, error) {
	tmuxPanes, err := TmuxPanesDetails(target)
	if err != nil {
		return nil, err
	}

	// Convert TmuxPaneDetails to generic PaneDetails
	panes := make([]PaneDetails, len(tmuxPanes))
	for i, tmuxPane := range tmuxPanes {
		panes[i] = PaneDetails{
			Id:                 tmuxPane.Id,
			CurrentPid:         tmuxPane.CurrentPid,
			CurrentCommand:     tmuxPane.CurrentCommand,
			CurrentCommandArgs: tmuxPane.CurrentCommandArgs,
			Content:            tmuxPane.Content,
			Shell:              tmuxPane.Shell,
			OS:                 tmuxPane.OS,
			LastLine:           tmuxPane.LastLine,
			IsActive:           tmuxPane.IsActive,
			IsTmuxAiPane:       tmuxPane.IsTmuxAiPane,
			IsTmuxAiExecPane:   tmuxPane.IsTmuxAiExecPane,
			IsPrepared:         tmuxPane.IsPrepared,
			IsSubShell:         tmuxPane.IsSubShell,
			HistorySize:        tmuxPane.HistorySize,
			HistoryLimit:       tmuxPane.HistoryLimit,
			MultiplexerType:    MultiplexerTmux,
		}
	}

	return panes, nil
}

func (t *TmuxBackend) CreateNewPane(target string) (string, error) {
	return TmuxCreateNewPane(target)
}

func (t *TmuxBackend) CapturePane(paneId string, maxLines int) (string, error) {
	return TmuxCapturePane(paneId, maxLines)
}

func (t *TmuxBackend) ClearPane(paneId string) error {
	return TmuxClearPane(paneId)
}

// Command execution

func (t *TmuxBackend) SendCommand(paneId string, command string) error {
	return TmuxSendCommandToPane(paneId, command, true)
}

func (t *TmuxBackend) SendKeys(paneId string, keys string) error {
	return TmuxSendCommandToPane(paneId, keys, false)
}

// Detection and information

func (t *TmuxBackend) IsAvailable() bool {
	return os.Getenv("TMUX_PANE") != ""
}

func (t *TmuxBackend) GetType() MultiplexerType {
	return MultiplexerTmux
}

func (t *TmuxBackend) GetCurrentPaneId() (string, error) {
	return TmuxCurrentPaneId()
}