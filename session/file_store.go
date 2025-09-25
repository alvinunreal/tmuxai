package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
)

type FileStore struct {
	baseDir string
}

func NewFileStore() (*FileStore, error) {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get config dir: %w", err)
	}

	baseDir := filepath.Join(configDir, "sessions")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	return &FileStore{
		baseDir: baseDir,
	}, nil
}

func (fs *FileStore) Save(session *Session) error {
	if session.ID == "" {
		session.ID = generateSessionID()
	}

	session.UpdatedAt = time.Now()
	if session.CreatedAt.IsZero() {
		session.CreatedAt = session.UpdatedAt
	}

	data, err := session.ToJSON()
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	filename := filepath.Join(fs.baseDir, session.ID+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	// Update the current session marker
	if err := fs.SetCurrent(session.ID); err != nil {
		logger.Error("Failed to set current session: %v", err)
	}

	// Update index file for quick listing
	if err := fs.updateIndex(); err != nil {
		logger.Error("Failed to update index: %v", err)
	}

	return nil
}

func (fs *FileStore) Load(id string) (*Session, error) {
	filename := filepath.Join(fs.baseDir, id+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("session not found: %s", id)
		}
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	return SessionFromJSON(data)
}

func (fs *FileStore) List(limit int) ([]SessionSummary, error) {
	indexFile := filepath.Join(fs.baseDir, "index.json")
	data, err := os.ReadFile(indexFile)
	if err != nil {
		if os.IsNotExist(err) {
			// If index doesn't exist, rebuild it
			if err := fs.updateIndex(); err != nil {
				return nil, err
			}
			data, err = os.ReadFile(indexFile)
			if err != nil {
				return nil, err
			}
		} else {
			return nil, fmt.Errorf("failed to read index file: %w", err)
		}
	}

	var summaries []SessionSummary
	if err := json.Unmarshal(data, &summaries); err != nil {
		return nil, fmt.Errorf("failed to unmarshal index: %w", err)
	}

	// Sort by UpdatedAt descending
	sort.Slice(summaries, func(i, j int) bool {
		return summaries[i].UpdatedAt.After(summaries[j].UpdatedAt)
	})

	if limit > 0 && len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries, nil
}

func (fs *FileStore) Delete(id string) error {
	filename := filepath.Join(fs.baseDir, id+".json")
	if err := os.Remove(filename); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("session not found: %s", id)
		}
		return fmt.Errorf("failed to delete session file: %w", err)
	}

	// Update index
	if err := fs.updateIndex(); err != nil {
		logger.Error("Failed to update index after delete: %v", err)
	}

	return nil
}

func (fs *FileStore) GetCurrent() (*Session, error) {
	currentFile := filepath.Join(fs.baseDir, "current")
	data, err := os.ReadFile(currentFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil // No current session
		}
		return nil, fmt.Errorf("failed to read current session marker: %w", err)
	}

	sessionID := strings.TrimSpace(string(data))
	if sessionID == "" {
		return nil, nil
	}

	return fs.Load(sessionID)
}

func (fs *FileStore) SetCurrent(id string) error {
	currentFile := filepath.Join(fs.baseDir, "current")
	return os.WriteFile(currentFile, []byte(id), 0644)
}

func (fs *FileStore) updateIndex() error {
	files, err := filepath.Glob(filepath.Join(fs.baseDir, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to list session files: %w", err)
	}

	var summaries []SessionSummary
	for _, file := range files {
		// Skip index.json
		if strings.HasSuffix(file, "index.json") {
			continue
		}

		data, err := os.ReadFile(file)
		if err != nil {
			logger.Error("Failed to read session file %s: %v", file, err)
			continue
		}

		session, err := SessionFromJSON(data)
		if err != nil {
			logger.Error("Failed to parse session file %s: %v", file, err)
			continue
		}

		summary := SessionSummary{
			ID:        session.ID,
			Name:      session.Name,
			Summary:   session.Summary,
			UpdatedAt: session.UpdatedAt,
		}
		summaries = append(summaries, summary)
	}

	indexData, err := json.MarshalIndent(summaries, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	indexFile := filepath.Join(fs.baseDir, "index.json")
	if err := os.WriteFile(indexFile, indexData, 0644); err != nil {
		return fmt.Errorf("failed to write index file: %w", err)
	}

	return nil
}

func generateSessionID() string {
	return fmt.Sprintf("%d", time.Now().Unix())
}