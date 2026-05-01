package internal

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/alvinunreal/tmuxai/config"
)

// =====================================================================
// TestParseSKILL_FrontmatterValid
// =====================================================================

func TestParseSKILL_FrontmatterValid(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantName    string
		wantDesc    string
		wantDisable bool
	}{
		{
			name:        "basic frontmatter",
			input:       "---\nname: hello-world\ndescription: Says hello\ndisable-model-invocation: false\n---\n# Hello World Skill\n\nThis skill prints hello.\n",
			wantName:    "hello-world",
			wantDesc:    "Says hello",
			wantDisable: false,
		},
		{
			name:        "disabled skill",
			input:       "---\nname: manual-mode\ndescription: Manual only\ndisable-model-invocation: true\n---\nManual content here\n",
			wantName:    "manual-mode",
			wantDesc:    "Manual only",
			wantDisable: true,
		},
		{
			name:        "extra frontmatter fields ignored",
			input:       "---\nname: extra-fields\ndescription: Has extras\nauthor: test\nversion: 1.0\n---\nExtra body\n",
			wantName:    "extra-fields",
			wantDesc:    "Has extras",
			wantDisable: false,
		},
		{
			name:        "YAML value containing dashes",
			input:       "---\nname: dash-in-value\ndescription: \"Setup --- and teardown\"\n---\nBody content\n",
			wantName:    "dash-in-value",
			wantDesc:    "Setup --- and teardown",
			wantDisable: false,
		},
		{
			name:        "multi-line description",
			input:       "---\nname: multi-desc\ndescription: |\n  Line one\n  Line two\n---\nBody content\n",
			wantName:    "multi-desc",
			wantDesc:    "Line one\nLine two",
			wantDisable: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			meta, body, err := ParseSkillMd([]byte(tc.input))
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if meta == nil {
				t.Fatal("expected frontmatter meta, got nil")
			}
			extractedName, extractedDesc, extractedDisabled := extractSkillMeta(meta)
			if extractedName != tc.wantName {
				t.Errorf("name: got %q, want %q", extractedName, tc.wantName)
			}
			if extractedDesc != tc.wantDesc {
				t.Errorf("description: got %q, want %q", extractedDesc, tc.wantDesc)
			}
			if extractedDisabled != tc.wantDisable {
				t.Errorf("disabled: got %v, want %v", extractedDisabled, tc.wantDisable)
			}
			if body == "" {
				t.Error("expected non-empty body")
			}
		})
	}
}

// =====================================================================
// TestParseSKILL_FrontmatterMalformed
// =====================================================================

func TestParseSKILL_FrontmatterMalformed(t *testing.T) {
	tests := []struct {
		name                string
		input               string
		expectNoFrontmatter bool // nil meta means treated-as-no-frontmatter (graceful)
		expectError         bool
		wantEmptyDesc       bool
	}{
		{
			name:                "missing frontmatter entirely",
			input:               "# Just a regular SKILL.md\nNo frontmatter here.\n",
			expectNoFrontmatter: true,
		},
		{
			name:                "empty description",
			input:               "---\nname: empty-desc\n---\nBody content\n",
			expectNoFrontmatter: false,
			wantEmptyDesc:       true,
		},
		{
			name:                "truncated opening delimiter",
			input:               "--\nname: trunc-open\ndescription: truncated\n---\nBody\n",
			expectNoFrontmatter: true,
		},
		{
			name:                "no closing delimiter",
			input:               "---\nname: no-close\ndescription: never closes\nJust runs forever...\n",
			expectNoFrontmatter: false,
		},
		{
			name:        "malformed YAML frontmatter",
			input:       "---\nname: broken-yaml\ndescription: [\ninvalid: {\n---\nBody\n",
			expectError: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Ensure no panic occurs
			meta, body, err := ParseSkillMd([]byte(tc.input))

			if tc.expectError {
				if err == nil {
					t.Error("expected error for malformed YAML, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.expectNoFrontmatter && meta != nil {
				t.Error("expected nil meta (no frontmatter), got non-nil")
			}

			// Extract with nil-safe function (should not panic)
			name, desc, _ := extractSkillMeta(meta)
			if tc.expectNoFrontmatter && name != "" {
				t.Logf("graceful degradation: name=%q body_len=%d", name, len(body))
			}
			if tc.wantEmptyDesc && desc != "" {
				t.Errorf("wanted empty description, got %q", desc)
			}

			// Body should contain something reasonable
			if !tc.expectNoFrontmatter && body == "" {
				t.Error("expected non-empty body for non-YAML-error case")
			}
		})
	}
}

// =====================================================================
// TestAutoMatchScoresThreshold
// =====================================================================

func TestAutoMatchScoresThreshold(t *testing.T) {
	sc := &config.SkillsConfig{
		AutoMatchThreshold: 0.1,
		MaxLoadedChars:     32000,
	}

	buildReg := func(t *testing.T, skills map[string]string) *SkillRegistry {
		t.Helper()
		reg := &SkillRegistry{
			Skills: make(map[string]*Skill),
			Config: sc,
		}
		for name, desc := range skills {
			reg.Skills[name] = &Skill{
				Name:        name,
				Description: desc,
				Disabled:    false,
				Loaded:      false,
			}
		}
		return reg
	}

	tests := []struct {
		name         string
		skills       map[string]string
		context      string
		setLoaded    bool // if true, marks the first skill as loaded
		setDisabled  bool // if true, marks the second skill as disabled
		wantContains []string
		wantMissing  []string
	}{
		{
			name: "matching keyword hits threshold",
			skills: map[string]string{
				"python-debugging": "Python debugger pdb trace breakpoint stack inspection debugging",
				"go-formatting":    "Go formatter golint staticcheck linting formatting",
			},
			context:      "I need to debug this python script with pdb traceback",
			wantContains: []string{"python-debugging"},
			wantMissing:  []string{"go-formatting"},
		},
		{
			name: "no relevant skills",
			skills: map[string]string{
				"python-debugging": "Python debugger pdb trace breakpoint stack inspection debugging",
				"go-formatting":    "Go formatter golint staticcheck linting formatting",
			},
			context:     "deploy kubernetes pod namespace cluster",
			wantMissing: []string{"python-debugging", "go-formatting"},
		},
		{
			name: "disabled skill excluded",
			skills: map[string]string{
				"active-skill":   "test keyword alpha beta gamma",
				"disabled-skill": "test keyword alpha beta gamma",
			},
			context:      "keyword alpha beta gamma test",
			setDisabled:  true,
			wantContains: []string{"active-skill"},
			wantMissing:  []string{"disabled-skill"},
		},
		{
			name: "loaded skill excluded",
			skills: map[string]string{
				"already-loaded": "keyword alpha beta gamma important",
				"unloaded-skill": "different distinct unique term rare",
			},
			context:     "important keyword alpha beta gamma",
			setLoaded:   true,
			wantMissing: []string{"already-loaded"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := buildReg(t, tc.skills)

			// Apply modifier flags
			skillList := make([]*Skill, 0, len(reg.Skills))
			for _, s := range reg.Skills {
				skillList = append(skillList, s)
			}
			sort.Slice(skillList, func(i, j int) bool {
				return skillList[i].Name < skillList[j].Name
			})

			if tc.setLoaded && len(skillList) > 0 {
				skillList[0].Loaded = true
			}
			if tc.setDisabled && len(skillList) > 1 {
				skillList[1].Disabled = true
			}

			results := reg.AutoMatch(tc.context)
			resultNames := make(map[string]bool)
			for _, s := range results {
				resultNames[s.Name] = true
			}

			for _, want := range tc.wantContains {
				if !resultNames[want] {
					t.Errorf("expected %q in results, got %v", want, resultNames)
				}
			}
			for _, miss := range tc.wantMissing {
				if resultNames[miss] {
					t.Errorf("did not expect %q in results, but it appeared", miss)
				}
			}
		})
	}
}

// =====================================================================
// TestLoadBudgetEnforcement
// =====================================================================

func TestLoadBudgetEnforcement(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "large-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	const bodyLen = 5000
	largeBody := strings.Repeat("a", bodyLen)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := "---\nname: large-skill\ndescription: A skill with a huge body\n---\n" + largeBody
	if err := os.WriteFile(skillFile, []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	_, parsedBody, err := ParseSkillMd([]byte(skillContent))
	if err != nil {
		t.Fatalf("ParseSkillMd: %v", err)
	}
	bodyChars := len(parsedBody)

	tests := []struct {
		name             string
		maxLoaded        int
		usedChars        int
		wantErr          bool
		wantSkillLoaded  bool
		wantUsedCharsGEQ int
	}{
		{
			name:            "budget exceeded – no prior usage",
			maxLoaded:       bodyChars - 100,
			usedChars:       0,
			wantErr:         true,
			wantSkillLoaded: false,
		},
		{
			name:            "budget exceeded – with prior usage",
			maxLoaded:       bodyChars + 300,
			usedChars:       400,
			wantErr:         true, // 400 + bodyChars > bodyChars + 500 ? 400+5000 > 5500 => true
			wantSkillLoaded: false,
		},
		{
			name:             "within budget – no prior usage",
			maxLoaded:        bodyChars + 1000,
			usedChars:        0,
			wantSkillLoaded:  true,
			wantUsedCharsGEQ: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reg := &SkillRegistry{
				Skills: map[string]*Skill{
					"large-skill": {
						Name:        "large-skill",
						Description: "A skill with a huge body",
						DirPath:     skillDir,
						FilePath:    skillFile,
						BodyLength:  bodyChars,
					},
				},
				Config: &config.SkillsConfig{
					MaxLoadedChars: tc.maxLoaded,
					MaxSkillChars:  20000,
				},
			}
			reg.UsedChars = tc.usedChars

			err := reg.Load("large-skill")

			if tc.wantErr && err == nil {
				t.Error("expected error due to budget exceeded, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Errorf("expected no error, got: %v", err)
			}

			if reg.Skills["large-skill"].Loaded != tc.wantSkillLoaded {
				t.Errorf("Loaded = %v, want %v", reg.Skills["large-skill"].Loaded, tc.wantSkillLoaded)
			}

			if !tc.wantSkillLoaded && reg.UsedChars != tc.usedChars {
				t.Errorf("UsedChars changed from %d to %d on failed load", tc.usedChars, reg.UsedChars)
			}

			if tc.wantSkillLoaded && reg.UsedChars < tc.wantUsedCharsGEQ {
				t.Errorf("UsedChars = %d, want >= %d", reg.UsedChars, tc.wantUsedCharsGEQ)
			}
		})
	}
}

func TestLoadMaxSkillCharsZeroMeansUnlimited(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "unlimited-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}

	const bodyLen = DefaultMaxSkillChars + 500
	bodyContent := strings.Repeat("x", bodyLen)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := "---\nname: unlimited-skill\ndescription: Per-skill cap disabled\n---\n" + bodyContent
	if err := os.WriteFile(skillFile, []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	reg := &SkillRegistry{
		Skills: map[string]*Skill{
			"unlimited-skill": {
				Name:        "unlimited-skill",
				Description: "Per-skill cap disabled",
				DirPath:     skillDir,
				FilePath:    skillFile,
			},
		},
		Config: &config.SkillsConfig{
			MaxLoadedChars: bodyLen + 1000,
			MaxSkillChars:  0,
		},
	}

	if err := reg.Load("unlimited-skill"); err != nil {
		t.Fatalf("Load returned error: %v", err)
	}

	loaded := reg.Skills["unlimited-skill"]
	if loaded.BodyLength != bodyLen {
		t.Fatalf("BodyLength = %d, want %d", loaded.BodyLength, bodyLen)
	}
	if strings.Contains(loaded.Body, "...[skill body truncated]") {
		t.Fatal("max_skill_chars: 0 should not truncate skill body")
	}
}

// =====================================================================
// TestLoadIdempotent
// =====================================================================

func TestLoadIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "idem-skill")
	os.MkdirAll(skillDir, 0755)

	bodyContent := strings.Repeat("x", 500)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := "---\nname: idem-skill\ndescription: Idempotent load test\n---\n" + bodyContent
	os.WriteFile(skillFile, []byte(skillContent), 0644)

	reg := &SkillRegistry{
		Skills: map[string]*Skill{
			"idem-skill": {
				Name:        "idem-skill",
				Description: "Idempotent load test",
				DirPath:     skillDir,
				FilePath:    skillFile,
			},
		},
		Config: &config.SkillsConfig{
			MaxLoadedChars: 10000,
			MaxSkillChars:  20000,
		},
	}

	// First load
	if err := reg.Load("idem-skill"); err != nil {
		t.Fatalf("first Load: %v", err)
	}
	firstUsed := reg.UsedChars
	if !reg.Skills["idem-skill"].Loaded {
		t.Fatal("skill should be loaded after first call")
	}

	// Second load – must return nil and NOT double-count
	if err := reg.Load("idem-skill"); err != nil {
		t.Fatalf("second Load: %v", err)
	}
	if reg.UsedChars != firstUsed {
		t.Errorf("UsedChars changed from %d to %d on idempotent load", firstUsed, reg.UsedChars)
	}
}

// =====================================================================
// TestLoadPerSkillTruncation
// =====================================================================

func TestLoadPerSkillTruncation(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "huge-skill")
	os.MkdirAll(skillDir, 0755)

	hugeBody := strings.Repeat("z", 25000) // exceeds DefaultMaxSkillChars (20000)
	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := "---\nname: huge-skill\ndescription: Huge body\n---\n" + hugeBody
	os.WriteFile(skillFile, []byte(skillContent), 0644)

	reg := &SkillRegistry{
		Skills: map[string]*Skill{
			"huge-skill": {
				Name:        "huge-skill",
				Description: "Huge body",
				DirPath:     skillDir,
				FilePath:    skillFile,
			},
		},
		Config: &config.SkillsConfig{
			MaxLoadedChars: 50000,
			MaxSkillChars:  10000, // enforce smaller cap for test
		},
	}

	if err := reg.Load("huge-skill"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	skill := reg.Skills["huge-skill"]
	if !skill.Loaded {
		t.Fatal("skill should be loaded")
	}
	if len(skill.Body) > 10000+len("\n\n...[skill body truncated]") {
		t.Errorf("Body length %d exceeds MaxSkillChars 10000 (+ truncation suffix)", len(skill.Body))
	}
	if !strings.Contains(skill.Body, "[skill body truncated]") {
		t.Error("truncated body should contain truncation marker")
	}
}

// =====================================================================
// TestUnload
// =====================================================================

func TestUnload(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "unload-me")
	os.MkdirAll(skillDir, 0755)

	skillFile := filepath.Join(skillDir, "SKILL.md")
	skillContent := "---\nname: unload-me\ndescription: Unload test\n---\nBody text here\n"
	os.WriteFile(skillFile, []byte(skillContent), 0644)

	reg := &SkillRegistry{
		Skills: map[string]*Skill{
			"unload-me": {
				Name:        "unload-me",
				Description: "Unload test",
				DirPath:     skillDir,
				FilePath:    skillFile,
			},
		},
		Config: &config.SkillsConfig{
			MaxLoadedChars: 32000,
			MaxSkillChars:  20000,
		},
	}

	// Load first
	if err := reg.Load("unload-me"); err != nil {
		t.Fatalf("Load: %v", err)
	}
	preUsed := reg.UsedChars
	if preUsed <= 0 {
		t.Fatal("UsedChars should be > 0 after load")
	}

	// Unload
	if err := reg.Unload("unload-me"); err != nil {
		t.Fatalf("Unload: %v", err)
	}
	skill := reg.Skills["unload-me"]
	if skill.Loaded {
		t.Error("Loaded should be false after unload")
	}
	if skill.Body != "" {
		t.Error("Body should be empty after unload")
	}
	if skill.BodyLength != 0 {
		t.Error("BodyLength should be 0 after unload")
	}
	if reg.UsedChars > 0 {
		t.Errorf("UsedChars should be 0 after unload, got %d", reg.UsedChars)
	}

	// Double-unload should error
	if err := reg.Unload("unload-me"); err == nil {
		t.Error("expected error when unloading non-loaded skill, got nil")
	}

	// Unload nonexistent skill should error
	if err := reg.Unload("ghost-skill"); err == nil {
		t.Error("expected error when unloading nonexistent skill, got nil")
	}
}

// =====================================================================
// TestBuildL1Block
// =====================================================================

func TestBuildL1Block(t *testing.T) {
	sc := &config.SkillsConfig{
		MaxL1Chars:     8000,
		TruncateDescAt: 50, // short truncate for test
	}
	reg := &SkillRegistry{
		Skills: map[string]*Skill{
			"aaa-short": {
				Name:        "aaa-short",
				Description: "Short desc",
				FilePath:    "/fake/path/aaa",
				Loaded:      false,
			},
			"bbb-long-desc": {
				Name:        "bbb-long-desc",
				Description: "This is a very long description that should be truncated because it exceeds the truncate limit set in config",
				FilePath:    "/fake/path/bbb",
				Loaded:      true,
			},
			"ccc-disabled": {
				Name:        "ccc-disabled",
				Description: "Manual skill",
				Disabled:    true,
				FilePath:    "/fake/path/ccc",
				Loaded:      false,
			},
		},
		Config: sc,
	}

	block := reg.BuildL1Block()

	// Structure checks
	if !strings.Contains(block, "<available_skills>") {
		t.Error("block should contain <available_skills>")
	}
	if !strings.Contains(block, "</available_skills>") {
		t.Error("block should contain </available_skills>")
	}

	// Sorting: aaa before bbb before ccc
	aaaPos := strings.Index(block, "aaa-short")
	bbbPos := strings.Index(block, "bbb-long-desc")
	cccPos := strings.Index(block, "ccc-disabled")
	if aaaPos >= bbbPos || bbbPos >= cccPos || cccPos < 0 {
		t.Logf("positions: aaa=%d bbb=%d ccc=%d", aaaPos, bbbPos, cccPos)
		t.Error("skills should be sorted alphabetically")
	}

	// Disabled marker
	if !strings.Contains(block, "[manual]") {
		t.Error("disabled skill should have [manual] marker")
	}

	// Loaded attribute on bbb
	if !strings.Contains(block, `loaded="true"`) {
		t.Error("bbb should have loaded=\"true\"")
	}

	// Description truncation on bbb
	if strings.Contains(block, "very long description that should be truncated because it exceeds") {
		t.Error("bbb description should be truncated")
	}
}

// =====================================================================
// TestBuildL1BlockBudgetCutoff
// =====================================================================

func TestBuildL1BlockBudgetCutoff(t *testing.T) {
	// Tiny budget to force early cutoff
	sc := &config.SkillsConfig{
		MaxL1Chars:     200, // very small
		TruncateDescAt: 200,
	}
	reg := &SkillRegistry{
		Skills: make(map[string]*Skill),
		Config: sc,
	}
	// Add 10 skills with descriptive names to exhaust budget
	for i := 0; i < 10; i++ {
		name := fmt.Sprintf("skill-%02d-with-long-name", i)
		reg.Skills[name] = &Skill{
			Name:        name,
			Description: fmt.Sprintf("Description for skill number %d with lots of text", i),
			FilePath:    "/fake/path",
		}
	}

	block := reg.BuildL1Block()
	if !strings.Contains(block, "remaining skills omitted") {
		t.Error("tiny budget should trigger budget cutoff comment")
	}
}

// =====================================================================
// TestBuildManifest
// =====================================================================

func TestBuildManifest(t *testing.T) {
	tempDir := t.TempDir()

	// Skill dir with ancillary files
	skillDir := filepath.Join(tempDir, "my-skill")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: my-skill\ndescription: test\n---\nBody\n"), 0644)
	os.WriteFile(filepath.Join(skillDir, "helper.sh"), []byte("#!/bin/bash\necho hi"), 0644)
	os.WriteFile(filepath.Join(skillDir, "README.md"), []byte("# Helper docs"), 0644)

	subDir := filepath.Join(skillDir, "snippets")
	os.MkdirAll(subDir, 0755)
	os.WriteFile(filepath.Join(subDir, "snippet1.txt"), []byte("snippet content"), 0644)

	skill := &Skill{Name: "my-skill", DirPath: skillDir}

	// Normal manifest
	manifest := skill.BuildManifest()
	if !strings.Contains(manifest, "Skill File Manifest") {
		t.Error("manifest should contain header")
	}
	if !strings.Contains(manifest, "helper.sh") {
		t.Error("manifest should list helper.sh")
	}
	if !strings.Contains(manifest, "README.md") {
		t.Error("manifest should list README.md")
	}
	if strings.Contains(manifest, "SKILL.md") {
		t.Error("manifest should NOT list SKILL.md")
	}

	// No ancillary files
	emptyDir := filepath.Join(tempDir, "bare-skill")
	os.MkdirAll(emptyDir, 0755)
	os.WriteFile(filepath.Join(emptyDir, "SKILL.md"), []byte("---\nname: bare-skill\ndescription: bare\n---\nBody\n"), 0644)
	bareSkill := &Skill{Name: "bare-skill", DirPath: emptyDir}
	emptyManifest := bareSkill.BuildManifest()
	if emptyManifest != "" {
		t.Error("bare skill should return empty manifest")
	}
}

// =====================================================================
// TestBuildManifestFileCap
// =====================================================================

func TestBuildManifestFileCap(t *testing.T) {
	tempDir := t.TempDir()
	skillDir := filepath.Join(tempDir, "many-files")
	os.MkdirAll(skillDir, 0755)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("---\nname: many-files\ndescription: many\n---\nBody\n"), 0644)

	// Create more files than maxManifestFiles (100)
	for i := 0; i < 110; i++ {
		fn := fmt.Sprintf("file_%04d.txt", i)
		os.WriteFile(filepath.Join(skillDir, fn), []byte("content"), 0644)
	}

	skill := &Skill{Name: "many-files", DirPath: skillDir}
	manifest := skill.BuildManifest()
	if !strings.Contains(manifest, "manifest truncated") {
		t.Error("manifest should be truncated when exceeding file cap")
	}
}

// =====================================================================
// TestValidateDiscoverAndCapErrors (via InitSkills)
// =====================================================================

func TestValidateDiscoverAndCapErrors(t *testing.T) {
	tests := []struct {
		name         string
		setupFunc    func(baseTemp string)
		wantFound    map[string]bool
		wantNotFound []string
	}{
		{
			name: "discovers valid, skips corrupted",
			setupFunc: func(base string) {
				configDir := filepath.Join(base, ".config", "tmuxai")
				skillsDir := filepath.Join(configDir, "skills")

				// Valid skill
				validDir := filepath.Join(skillsDir, "valid-skill")
				os.MkdirAll(validDir, 0755)
				validContent := "---\nname: valid-skill\ndescription: A perfectly valid skill for testing\n---\nSome useful content here.\n"
				os.WriteFile(filepath.Join(validDir, "SKILL.md"), []byte(validContent), 0644)

				// Corrupted skill: YAML error
				corrDir := filepath.Join(skillsDir, "corrupt-yaml")
				os.MkdirAll(corrDir, 0755)
				corrContent := "---\nname: corrupt-yaml\ndescription: [{bad yaml\n---\nBody\n"
				os.WriteFile(filepath.Join(corrDir, "SKILL.md"), []byte(corrContent), 0644)

				// Invalid name skill: uppercase letters
				badNameDir := filepath.Join(skillsDir, "Bad-Name")
				os.MkdirAll(badNameDir, 0755)
				badNameContent := "---\nname: Bad-Name\ndescription: Has uppercase name\n---\nBody\n"
				os.WriteFile(filepath.Join(badNameDir, "SKILL.md"), []byte(badNameContent), 0644)

				// Missing name/description
				noNameDir := filepath.Join(skillsDir, "no-name")
				os.MkdirAll(noNameDir, 0755)
				noNameContent := "---\ndescription: No name provided\n---\nBody\n"
				os.WriteFile(filepath.Join(noNameDir, "SKILL.md"), []byte(noNameContent), 0644)

				// Empty directory (no SKILL.md)
				emptyDir := filepath.Join(skillsDir, "empty-dir")
				os.MkdirAll(emptyDir, 0755)
			},
			wantFound:    map[string]bool{"valid-skill": true},
			wantNotFound: []string{"corrupt-yaml", "Bad-Name", "no-name", "empty-dir"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			baseTemp := t.TempDir()
			tc.setupFunc(baseTemp)

			origHome := os.Getenv("HOME")
			os.Setenv("HOME", baseTemp)
			t.Cleanup(func() { os.Setenv("HOME", origHome) })

			sc := &config.SkillsConfig{
				AutoScan:           true,
				MaxL1Chars:         8000,
				MaxLoadedChars:     32000,
				MaxSkillChars:      20000,
				TruncateDescAt:     200,
				AutoMatchThreshold: 0.1,
			}

			reg, err := InitSkills(sc)
			if err != nil {
				t.Fatalf("InitSkills returned error: %v", err)
			}

			// Check wanted skills are found
			for wantName := range tc.wantFound {
				if _, ok := reg.Skills[wantName]; !ok {
					t.Errorf("expected skill %q to be discovered, got skills: %v",
						wantName, skillNames(reg))
				}
			}

			// Check unwanted skills are not found
			for _, notWant := range tc.wantNotFound {
				if _, ok := reg.Skills[notWant]; ok {
					t.Errorf("expected skill %q NOT to be discovered, but it was", notWant)
				}
			}

			if len(reg.DiscoveryWarnings) != len(tc.wantNotFound) {
				t.Errorf("DiscoveryWarnings length = %d, want %d: %v", len(reg.DiscoveryWarnings), len(tc.wantNotFound), reg.DiscoveryWarnings)
			}
			for _, notWant := range tc.wantNotFound {
				foundWarning := false
				for _, warning := range reg.DiscoveryWarnings {
					if strings.HasPrefix(warning, notWant+":") {
						foundWarning = true
						break
					}
				}
				if !foundWarning {
					t.Errorf("expected discovery warning for %q, got %v", notWant, reg.DiscoveryWarnings)
				}
			}

			// Valid skill attributes
			if s, ok := reg.Skills["valid-skill"]; ok {
				if s.Description != "A perfectly valid skill for testing" {
					t.Errorf("description: got %q, want %q", s.Description, "A perfectly valid skill for testing")
				}
				if s.Loaded {
					t.Error("skill should not be pre-loaded by InitSkills")
				}
				if s.FilePath == "" {
					t.Error("FilePath should be populated")
				}
			}

			// L1Block should be generated (non-empty)
			if reg.L1Block == "" {
				t.Error("L1Block should not be empty")
			}
		})
	}
}

func TestInitSkillsHonorsAutoScanFalse(t *testing.T) {
	baseTemp := t.TempDir()
	configDir := filepath.Join(baseTemp, ".config", "tmuxai")
	skillsDir := filepath.Join(configDir, "skills")
	skillDir := filepath.Join(skillsDir, "valid-skill")
	if err := os.MkdirAll(skillDir, 0755); err != nil {
		t.Fatal(err)
	}
	skillContent := "---\nname: valid-skill\ndescription: A valid skill that should not be scanned\n---\nBody\n"
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillContent), 0644); err != nil {
		t.Fatal(err)
	}

	origHome := os.Getenv("HOME")
	os.Setenv("HOME", baseTemp)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	reg, err := InitSkills(&config.SkillsConfig{
		AutoScan:       false,
		MaxL1Chars:     8000,
		MaxLoadedChars: 32000,
		MaxSkillChars:  20000,
	})
	if err != nil {
		t.Fatalf("InitSkills returned error: %v", err)
	}
	if len(reg.Skills) != 0 {
		t.Fatalf("AutoScan false discovered skills: %v", skillNames(reg))
	}
	if len(reg.DiscoveryWarnings) != 0 {
		t.Fatalf("AutoScan false should not validate skills, got warnings: %v", reg.DiscoveryWarnings)
	}
}

// skillNames returns sorted skill names from a registry.
func skillNames(r *SkillRegistry) []string {
	names := make([]string, 0, len(r.Skills))
	for n := range r.Skills {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
