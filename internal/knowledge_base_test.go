package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)


// TestLoadKBNonExistent tests loading a non-existent KB
func TestLoadKBNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("Failed to create KB directory: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Try to load non-existent KB
	err := mgr.loadKB("nonexistent")
	if err == nil {
		t.Fatal("Expected error when loading non-existent KB, got nil")
	}
}

// TestUnloadKB tests unloading a knowledge base
func TestUnloadKB(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config: cfg,
		LoadedKBs: map[string]string{
			"test": "test content",
		},
	}

	// Test unloading KB
	err := mgr.unloadKB("test")
	if err != nil {
		t.Fatalf("unloadKB() failed: %v", err)
	}

	// Verify KB was unloaded
	if _, exists := mgr.LoadedKBs["test"]; exists {
		t.Fatal("KB still exists in LoadedKBs after unloading")
	}
}

// TestUnloadKBNonLoaded tests unloading a KB that isn't loaded
func TestUnloadKBNonLoaded(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Try to unload non-loaded KB
	err := mgr.unloadKB("test")
	if err == nil {
		t.Fatal("Expected error when unloading non-loaded KB, got nil")
	}
}


// TestGetTotalLoadedKBTokens tests token counting for loaded KBs
func TestGetTotalLoadedKBTokens(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config: cfg,
		LoadedKBs: map[string]string{
			"kb1": "Short content",
			"kb2": "Another piece of content with more words",
		},
	}

	tokens := mgr.getTotalLoadedKBTokens()

	// We can't test exact token count, but it should be > 0
	if tokens <= 0 {
		t.Errorf("Expected positive token count, got %d", tokens)
	}
}
