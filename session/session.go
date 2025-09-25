package session

import (
	"encoding/json"
	"time"
)

// ChatMessage represents a chat message
type ChatMessage struct {
	Content   string    `json:"content"`
	FromUser  bool      `json:"from_user"`
	Timestamp time.Time `json:"timestamp"`
}

// CommandExecHistory represents a command execution history entry
type CommandExecHistory struct {
	Command string `json:"command"`
	Output  string `json:"output"`
	Code    int    `json:"code"`
}

type Session struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Summary     string               `json:"summary"`
	Messages    []ChatMessage        `json:"messages"`
	ExecHistory []CommandExecHistory `json:"exec_history"`
	CreatedAt   time.Time            `json:"created_at"`
	UpdatedAt   time.Time            `json:"updated_at"`
	PaneContext string               `json:"pane_context"`
}

type SessionSummary struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SessionStore interface {
	Save(session *Session) error
	Load(id string) (*Session, error)
	List(limit int) ([]SessionSummary, error)
	Delete(id string) error
	GetCurrent() (*Session, error)
	SetCurrent(id string) error
}

func (s *Session) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

func SessionFromJSON(data []byte) (*Session, error) {
	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, err
	}
	return &session, nil
}