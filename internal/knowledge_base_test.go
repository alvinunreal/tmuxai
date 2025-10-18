package internal

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

// TestLoadKB tests loading a knowledge base
func TestLoadKB(t *testing.T) {
	// Create temp directory for KB files
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("Failed to create KB directory: %v", err)
	}

	// Create a test KB file
	kbContent := "# Test KB\nThis is test content."
	kbPath := filepath.Join(kbDir, "test.md")
	if err := os.WriteFile(kbPath, []byte(kbContent), 0644); err != nil {
		t.Fatalf("Failed to create test KB file: %v", err)
	}

	// Create a manager with custom KB path
	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Test loading KB
	err := mgr.loadKB("test")
	if err != nil {
		t.Fatalf("loadKB() failed: %v", err)
	}

	// Verify KB was loaded
	content, exists := mgr.LoadedKBs["test"]
	if !exists {
		t.Fatal("KB was not loaded into LoadedKBs map")
	}

	if content != kbContent {
		t.Errorf("Expected content %q, got %q", kbContent, content)
	}
}

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

// TestListKBs tests listing available knowledge bases
func TestListKBs(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("Failed to create KB directory: %v", err)
	}

	// Create test KB files
	kbFiles := []string{"test1.md", "test2.md", "test3.md"}
	for _, file := range kbFiles {
		path := filepath.Join(kbDir, file)
		if err := os.WriteFile(path, []byte("content"), 0644); err != nil {
			t.Fatalf("Failed to create KB file %s: %v", file, err)
		}
	}

	// Also create a non-markdown file (should be ignored)
	txtPath := filepath.Join(kbDir, "readme.txt")
	if err := os.WriteFile(txtPath, []byte("content"), 0644); err != nil {
		t.Fatalf("Failed to create txt file: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Test listing KBs
	kbs, err := mgr.listKBs()
	if err != nil {
		t.Fatalf("listKBs() failed: %v", err)
	}

	// Verify we got the right number (only .md files)
	if len(kbs) != 3 {
		t.Errorf("Expected 3 KBs, got %d", len(kbs))
	}

	// Verify the names are correct (without .md extension)
	expectedNames := map[string]bool{"test1": true, "test2": true, "test3": true}
	for _, name := range kbs {
		if !expectedNames[name] {
			t.Errorf("Unexpected KB name: %s", name)
		}
	}
}

// TestListKBsEmptyDir tests listing KBs when directory is empty
func TestListKBsEmptyDir(t *testing.T) {
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

	kbs, err := mgr.listKBs()
	if err != nil {
		t.Fatalf("listKBs() failed: %v", err)
	}

	if len(kbs) != 0 {
		t.Errorf("Expected 0 KBs in empty directory, got %d", len(kbs))
	}
}

// TestListKBsNonExistentDir tests listing KBs when directory doesn't exist
func TestListKBsNonExistentDir(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "nonexistent")

	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	kbs, err := mgr.listKBs()
	if err != nil {
		t.Fatalf("listKBs() should not error on non-existent directory: %v", err)
	}

	if len(kbs) != 0 {
		t.Errorf("Expected 0 KBs for non-existent directory, got %d", len(kbs))
	}
}

// TestAutoLoadKBs tests auto-loading KBs from config
func TestAutoLoadKBs(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("Failed to create KB directory: %v", err)
	}

	// Create test KB files
	kb1Content := "KB1 content"
	kb2Content := "KB2 content"
	if err := os.WriteFile(filepath.Join(kbDir, "kb1.md"), []byte(kb1Content), 0644); err != nil {
		t.Fatalf("Failed to create kb1.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kbDir, "kb2.md"), []byte(kb2Content), 0644); err != nil {
		t.Fatalf("Failed to create kb2.md: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.KnowledgeBase.AutoLoad = []string{"kb1", "kb2"}
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Mock Println to avoid output during tests
	mgr.Println = func(msg string) {}

	// Test auto-loading
	mgr.autoLoadKBs()

	// Verify both KBs were loaded
	if len(mgr.LoadedKBs) != 2 {
		t.Errorf("Expected 2 loaded KBs, got %d", len(mgr.LoadedKBs))
	}

	if mgr.LoadedKBs["kb1"] != kb1Content {
		t.Errorf("KB1 content mismatch")
	}

	if mgr.LoadedKBs["kb2"] != kb2Content {
		t.Errorf("KB2 content mismatch")
	}
}

// TestLoadKBsFromCLI tests loading KBs from CLI flag
func TestLoadKBsFromCLI(t *testing.T) {
	tmpDir := t.TempDir()
	kbDir := filepath.Join(tmpDir, "kb")
	if err := os.MkdirAll(kbDir, 0755); err != nil {
		t.Fatalf("Failed to create KB directory: %v", err)
	}

	// Create test KB files
	kb1Content := "KB1 content"
	kb2Content := "KB2 content"
	if err := os.WriteFile(filepath.Join(kbDir, "kb1.md"), []byte(kb1Content), 0644); err != nil {
		t.Fatalf("Failed to create kb1.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(kbDir, "kb2.md"), []byte(kb2Content), 0644); err != nil {
		t.Fatalf("Failed to create kb2.md: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.KnowledgeBase.Path = kbDir
	cfg.OpenRouter.APIKey = "test-key"

	mgr := &Manager{
		Config:    cfg,
		LoadedKBs: make(map[string]string),
	}

	// Mock Println to avoid output during tests
	mgr.Println = func(msg string) {}

	// Test loading from CLI
	mgr.LoadKBsFromCLI([]string{"kb1", "kb2"})

	// Verify both KBs were loaded
	if len(mgr.LoadedKBs) != 2 {
		t.Errorf("Expected 2 loaded KBs, got %d", len(mgr.LoadedKBs))
	}

	if mgr.LoadedKBs["kb1"] != kb1Content {
		t.Errorf("KB1 content mismatch")
	}

	if mgr.LoadedKBs["kb2"] != kb2Content {
		t.Errorf("KB2 content mismatch")
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
