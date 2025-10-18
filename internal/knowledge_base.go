package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
)

// autoLoadKBs loads knowledge bases specified in the config
func (m *Manager) autoLoadKBs() {
	for _, kbName := range m.Config.KnowledgeBase.AutoLoad {
		if err := m.loadKB(kbName); err != nil {
			logger.Error("Failed to auto-load KB '%s': %v", kbName, err)
		}
	}
}

// loadKB loads a knowledge base file into memory
func (m *Manager) loadKB(name string) error {
	kbDir, err := config.GetKBDir(m.Config.KnowledgeBase.Path)
	if err != nil {
		return fmt.Errorf("failed to get KB directory: %w", err)
	}

	kbPath := filepath.Join(kbDir, name+".md")
	content, err := os.ReadFile(kbPath)
	if err != nil {
		return fmt.Errorf("failed to read KB file: %w", err)
	}

	m.LoadedKBs[name] = string(content)
	logger.Info("Loaded KB: %s (%d tokens)", name, system.EstimateTokenCount(string(content)))
	return nil
}

// unloadKB removes a knowledge base from memory
func (m *Manager) unloadKB(name string) error {
	if _, exists := m.LoadedKBs[name]; !exists {
		return fmt.Errorf("KB '%s' is not loaded", name)
	}
	delete(m.LoadedKBs, name)
	logger.Info("Unloaded KB: %s", name)
	return nil
}

// listKBs returns a list of all available KBs with their loaded status
func (m *Manager) listKBs() ([]string, error) {
	kbDir, err := config.GetKBDir(m.Config.KnowledgeBase.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to get KB directory: %w", err)
	}

	entries, err := os.ReadDir(kbDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read KB directory: %w", err)
	}

	var kbs []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}

		name := strings.TrimSuffix(entry.Name(), ".md")
		loaded := ""
		tokens := 0

		if content, exists := m.LoadedKBs[name]; exists {
			loaded = "[✓] "
			tokens = system.EstimateTokenCount(content)
		} else {
			loaded = "[ ] "
		}

		var kbInfo string
		if tokens > 0 {
			kbInfo = fmt.Sprintf("%s%s (%d tokens)", loaded, name, tokens)
		} else {
			kbInfo = fmt.Sprintf("%s%s", loaded, name)
		}

		kbs = append(kbs, kbInfo)
	}

	return kbs, nil
}

// getTotalLoadedKBTokens returns the total token count of all loaded KBs
func (m *Manager) getTotalLoadedKBTokens() int {
	total := 0
	for _, content := range m.LoadedKBs {
		total += system.EstimateTokenCount(content)
	}
	return total
}
