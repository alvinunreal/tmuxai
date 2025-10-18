package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLoadKB tests loading a knowledge base file
func TestLoadKB(t *testing.T) {
	// Create a temporary KB directory
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	require.NoError(t, os.MkdirAll(kbDir, 0o755))

	// Create a test KB file
	kbContent := "# Test KB\nThis is test knowledge base content."
	kbPath := filepath.Join(kbDir, "test.md")
	require.NoError(t, os.WriteFile(kbPath, []byte(kbContent), 0o644))

	// Mock GetKBDir to return our temp directory
	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config:    &config.Config{},
		LoadedKBs: make(map[string]string),
	}

	// Test loading KB
	err := manager.loadKB("test")
	assert.NoError(t, err)
	assert.Contains(t, manager.LoadedKBs, "test")
	assert.Equal(t, kbContent, manager.LoadedKBs["test"])
}

// TestLoadKBNonExistent tests loading a non-existent KB file
func TestLoadKBNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	require.NoError(t, os.MkdirAll(kbDir, 0o755))

	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config:    &config.Config{},
		LoadedKBs: make(map[string]string),
	}

	// Test loading non-existent KB
	err := manager.loadKB("nonexistent")
	assert.Error(t, err)
	assert.NotContains(t, manager.LoadedKBs, "nonexistent")
}

// TestUnloadKB tests unloading a knowledge base
func TestUnloadKB(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{},
		LoadedKBs: map[string]string{
			"test": "test content",
		},
	}

	// Test unloading existing KB
	err := manager.unloadKB("test")
	assert.NoError(t, err)
	assert.NotContains(t, manager.LoadedKBs, "test")
}

// TestUnloadKBNotLoaded tests unloading a KB that isn't loaded
func TestUnloadKBNotLoaded(t *testing.T) {
	manager := &Manager{
		Config:    &config.Config{},
		LoadedKBs: make(map[string]string),
	}

	// Test unloading non-loaded KB
	err := manager.unloadKB("notloaded")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "is not loaded")
}

// TestListKBs tests listing knowledge bases
func TestListKBs(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	require.NoError(t, os.MkdirAll(kbDir, 0o755))

	// Create test KB files
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "kb1.md"), []byte("KB 1 content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "kb2.md"), []byte("KB 2 content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "notmd.txt"), []byte("Not a KB"), 0o644))

	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config: &config.Config{},
		LoadedKBs: map[string]string{
			"kb1": "KB 1 content",
		},
	}

	// Test listing KBs
	kbs, err := manager.listKBs()
	assert.NoError(t, err)
	assert.Len(t, kbs, 2) // Only .md files

	// Check that kb1 is marked as loaded
	var kb1Info, kb2Info *KBInfo
	for i := range kbs {
		if kbs[i].Name == "kb1" {
			kb1Info = &kbs[i]
		} else if kbs[i].Name == "kb2" {
			kb2Info = &kbs[i]
		}
	}

	assert.NotNil(t, kb1Info)
	assert.True(t, kb1Info.Loaded)
	assert.Greater(t, kb1Info.Tokens, 0)

	assert.NotNil(t, kb2Info)
	assert.False(t, kb2Info.Loaded)
	assert.Equal(t, 0, kb2Info.Tokens)
}

// TestListKBsEmptyDir tests listing KBs when directory doesn't exist
func TestListKBsEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "nonexistent")

	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config:    &config.Config{},
		LoadedKBs: make(map[string]string),
	}

	// Should return empty list, not error
	kbs, err := manager.listKBs()
	assert.NoError(t, err)
	assert.Len(t, kbs, 0)
}

// TestAutoLoadKBs tests auto-loading knowledge bases from config
func TestAutoLoadKBs(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	require.NoError(t, os.MkdirAll(kbDir, 0o755))

	// Create test KB files
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "kb1.md"), []byte("KB 1 content"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "kb2.md"), []byte("KB 2 content"), 0o644))

	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config: &config.Config{
			KnowledgeBase: config.KnowledgeBaseConfig{
				AutoLoad: []string{"kb1", "kb2"},
			},
		},
		LoadedKBs: make(map[string]string),
	}

	// Test auto-loading
	manager.autoLoadKBs()

	assert.Len(t, manager.LoadedKBs, 2)
	assert.Contains(t, manager.LoadedKBs, "kb1")
	assert.Contains(t, manager.LoadedKBs, "kb2")
	assert.Equal(t, "KB 1 content", manager.LoadedKBs["kb1"])
	assert.Equal(t, "KB 2 content", manager.LoadedKBs["kb2"])
}

// TestAutoLoadKBsWithMissing tests auto-loading with some missing KBs
func TestAutoLoadKBsWithMissing(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	require.NoError(t, os.MkdirAll(kbDir, 0o755))

	// Create only one KB file
	require.NoError(t, os.WriteFile(filepath.Join(kbDir, "kb1.md"), []byte("KB 1 content"), 0o644))

	originalGetKBDir := config.GetKBDir
	config.GetKBDir = func() (string, error) {
		return kbDir, nil
	}
	defer func() {
		config.GetKBDir = originalGetKBDir
	}()

	manager := &Manager{
		Config: &config.Config{
			KnowledgeBase: config.KnowledgeBaseConfig{
				AutoLoad: []string{"kb1", "missing"},
			},
		},
		LoadedKBs: make(map[string]string),
	}

	// Test auto-loading with missing KB (should not fail completely)
	manager.autoLoadKBs()

	// Should have loaded kb1 but not missing
	assert.Contains(t, manager.LoadedKBs, "kb1")
	assert.NotContains(t, manager.LoadedKBs, "missing")
}

// TestGetTotalLoadedKBTokens tests token counting
func TestGetTotalLoadedKBTokens(t *testing.T) {
	manager := &Manager{
		Config: &config.Config{},
		LoadedKBs: map[string]string{
			"kb1": "Short content",
			"kb2": "This is a longer piece of content that should have more tokens",
		},
	}

	tokens := manager.getTotalLoadedKBTokens()
	assert.Greater(t, tokens, 0)
	// Should be roughly the sum of both KBs' tokens
	assert.Greater(t, tokens, 10) // At least some tokens
}
