package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BoringDystopiaDevelopment/tmuxai/config"
	"github.com/BoringDystopiaDevelopment/tmuxai/logger"
	"github.com/BoringDystopiaDevelopment/tmuxai/system"
)

// loadKB loads a knowledge base file by name
func (m *Manager) loadKB(name string) error {
	kbDir := config.GetKBDir()
	kbPath := filepath.Join(kbDir, name)

	content, err := os.ReadFile(kbPath)
	if err != nil {
		return fmt.Errorf("failed to read KB file '%s': %w", name, err)
	}

	m.LoadedKBs[name] = string(content)
	logger.Info("Loaded knowledge base: %s", name)
	return nil
}

// unloadKB removes a knowledge base from memory
func (m *Manager) unloadKB(name string) error {
	if _, exists := m.LoadedKBs[name]; !exists {
		return fmt.Errorf("knowledge base '%s' is not loaded", name)
	}

	delete(m.LoadedKBs, name)
	logger.Info("Unloaded knowledge base: %s", name)
	return nil
}

// listKBs returns a list of all available knowledge bases with their loaded status
func (m *Manager) listKBs() ([]string, error) {
	kbDir := config.GetKBDir()

	entries, err := os.ReadDir(kbDir)
	if err != nil {
		if os.IsNotExist(err) {
			// KB directory doesn't exist yet
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read KB directory: %w", err)
	}

	var kbs []string
	for _, entry := range entries {
		if !entry.IsDir() {
			kbs = append(kbs, entry.Name())
		}
	}

	return kbs, nil
}

// autoLoadKBs loads knowledge bases specified in the config
func (m *Manager) autoLoadKBs() {
	if len(m.Config.KnowledgeBase.AutoLoad) == 0 {
		return
	}

	logger.Info("Auto-loading knowledge bases: %v", m.Config.KnowledgeBase.AutoLoad)

	for _, name := range m.Config.KnowledgeBase.AutoLoad {
		if err := m.loadKB(name); err != nil {
			logger.Error("Failed to auto-load KB '%s': %v", name, err)
			m.Println(fmt.Sprintf("Warning: Failed to auto-load KB '%s': %v", name, err))
		}
	}
}

// LoadKBsFromCLI loads knowledge bases specified via CLI flag
func (m *Manager) LoadKBsFromCLI(kbNames []string) {
	if len(kbNames) == 0 {
		return
	}

	logger.Info("Loading knowledge bases from CLI: %v", kbNames)

	for _, name := range kbNames {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		if err := m.loadKB(name); err != nil {
			logger.Error("Failed to load KB '%s' from CLI: %v", name, err)
			m.Println(fmt.Sprintf("Warning: Failed to load KB '%s': %v", name, err))
		}
	}
}

// getTotalLoadedKBTokens calculates the total token count of all loaded KBs
func (m *Manager) getTotalLoadedKBTokens() int {
	total := 0
	for _, content := range m.LoadedKBs {
		total += system.EstimateTokenCount(content)
	}
	return total
}
