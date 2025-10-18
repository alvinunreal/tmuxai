package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/system"
)

// loadKB loads a knowledge base file by name
func (m *Manager) loadKB(name string) error {
	kbDir, err := config.GetKBDir()
	if err != nil {
		return fmt.Errorf("failed to get KB directory: %w", err)
	}

	kbPath := filepath.Join(kbDir, name+".md")
	content, err := os.ReadFile(kbPath)
	if err != nil {
		return fmt.Errorf("failed to read KB file: %w", err)
	}

	m.LoadedKBs[name] = string(content)
	return nil
}

// unloadKB removes a knowledge base from memory
func (m *Manager) unloadKB(name string) error {
	if _, exists := m.LoadedKBs[name]; !exists {
		return fmt.Errorf("knowledge base '%s' is not loaded", name)
	}
	delete(m.LoadedKBs, name)
	return nil
}

// listKBs returns information about available knowledge bases
func (m *Manager) listKBs() ([]KBInfo, error) {
	kbDir, err := config.GetKBDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get KB directory: %w", err)
	}

	entries, err := os.ReadDir(kbDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KB directory doesn't exist yet, return empty list
			return []KBInfo{}, nil
		}
		return nil, fmt.Errorf("failed to read KB directory: %w", err)
	}

	var kbs []KBInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		loaded := false
		tokens := 0

		if content, exists := m.LoadedKBs[name]; exists {
			loaded = true
			tokens = system.EstimateTokenCount(content)
		}

		kbs = append(kbs, KBInfo{
			Name:   name,
			Loaded: loaded,
			Tokens: tokens,
		})
	}

	return kbs, nil
}

// autoLoadKBs loads knowledge bases specified in the config
func (m *Manager) autoLoadKBs() {
	if len(m.Config.KnowledgeBase.AutoLoad) == 0 {
		return
	}

	for _, name := range m.Config.KnowledgeBase.AutoLoad {
		if err := m.loadKB(name); err != nil {
			m.Println(fmt.Sprintf("Warning: Failed to auto-load KB '%s': %v", name, err))
		}
	}
}

// getTotalLoadedKBTokens returns the total token count of all loaded KBs
func (m *Manager) getTotalLoadedKBTokens() int {
	total := 0
	for _, content := range m.LoadedKBs {
		total += system.EstimateTokenCount(content)
	}
	return total
}

// KBInfo holds information about a knowledge base
type KBInfo struct {
	Name   string
	Loaded bool
	Tokens int
}
