package internal

import (
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"gopkg.in/yaml.v3"
)

// ============================================================================
// Types
// ============================================================================

// Skill represents a discovered SKILL.md file.
type Skill struct {
	Name        string // from frontmatter, validated against regex
	Description string // from frontmatter
	Disabled    bool   // disable-model-invocation flag
	DirPath     string // absolute path to skill directory (home for ancillary files)
	FilePath    string // absolute path to SKILL.md
	Body        string // L2 content + manifest (populated on lazy load only)
	BodyLength  int    // char count of body + manifest (for budget enforcement)
	Loaded      bool   // whether body is currently injected
}

// SkillRegistry manages discovery, storage, and lazy loading of skills.
type SkillRegistry struct {
	Skills    map[string]*Skill    // keyed by name
	Config    *config.SkillsConfig // pointer to config for budget enforcement
	L1Block   string               // pre-rendered <available_skills> XML block
	UsedChars int                  // aggregate chars of currently loaded skills
}

// ============================================================================
// Constants
// ============================================================================

const (
	maxManifestFiles = 100  // max files to list per skill
	maxManifestChars = 1024 // max chars for manifest text
)

const (
	DefaultMaxL1Chars      = 8000
	DefaultMaxLoadedChars  = 32000
	DefaultMaxSkillChars   = 20000
	DefaultTruncateDescAt  = 200
	DefaultAutoMatchThresh = 0.1
)

// ============================================================================
// Frontmatter Parser (§14)
// ============================================================================

var skillNameRegex = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ParseSkillMd splits a SKILL.md byte slice into YAML frontmatter and body.
//
// Rules (§14.2):
//   - Opening/closing "---" lines signal frontmatter (matched line-by-line via splitFrontmatter).
//   - Standalone "---" lines (trimmed) serve as fences; embedded "---" in YAML values is preserved.
//   - Body may contain further "---" horizontal rules (not mistaken for fences unless alone on a line).
//   - No frontmatter → nil meta, entire content as body.
//   - Malformed YAML → error (caller should reject the skill).
func splitFrontmatter(content string) (frontmatter, body string, ok bool) {
	lines := strings.Split(content, "\n")
	// Find opening ---
	openIdx := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == "---" {
			openIdx = i
			break
		}
	}
	if openIdx == -1 {
		return "", "", false
	}
	// Find closing ---
	closeIdx := -1
	for i := openIdx + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			closeIdx = i
			break
		}
	}
	if closeIdx == -1 {
		return "", "", false
	}
	return strings.Join(lines[openIdx+1:closeIdx], "\n"), strings.Join(lines[closeIdx+1:], "\n"), true
}

func ParseSkillMd(raw []byte) (meta map[string]any, body string, err error) {
	content := string(raw)

	if !strings.HasPrefix(content, "---\n") && !strings.HasPrefix(content, "---\r\n") {
		return nil, content, nil
	}

	fmStr, bodyStr, ok := splitFrontmatter(content)
	if !ok {
		return nil, content, nil
	}

	fmStr = strings.TrimSpace(fmStr)
	bodyStr = strings.TrimSpace(bodyStr)

	if fmStr == "" {
		return nil, bodyStr, nil
	}

	fm := make(map[string]any)
	if err := yaml.Unmarshal([]byte(fmStr), &fm); err != nil {
		return nil, "", fmt.Errorf("invalid YAML frontmatter: %w", err)
	}

	return fm, bodyStr, nil
}

// extractSkillMeta extracts name, description, and disabled flag from parsed
// frontmatter. Missing fields return zero-values.
func extractSkillMeta(meta map[string]any) (name, description string, disabled bool) {
	if meta == nil {
		return
	}
	name, _ = meta["name"].(string)
	name = strings.TrimSpace(name)
	description, _ = meta["description"].(string)
	description = strings.TrimSpace(description)
	disabled, _ = meta["disable-model-invocation"].(bool)
	return
}

// validateSkillName enforces kebab-case alphanumeric, 1–64 characters.
func validateSkillName(name string) bool {
	if len(name) == 0 || len(name) > 64 {
		return false
	}
	return skillNameRegex.MatchString(name)
}

// ============================================================================
// Registry Initialization (§5.1–§5.2)
// ============================================================================

// InitSkills scans the skills directory, parses each SKILL.md, and builds a
// SkillRegistry. Returns an empty registry (not an error) if the directory
// doesn't exist.
//
// Directory derivation (§5.1): the skills directory sits alongside the KB
// directory at ~/.config/tmuxai/skills/ — derived from the same config
// directory that GetKBDir() uses.
func InitSkills(sc *config.SkillsConfig) (*SkillRegistry, error) {
	reg := &SkillRegistry{
		Skills: make(map[string]*Skill),
		Config: sc,
	}
	reg.L1Block = reg.BuildL1Block()

	if sc != nil && !sc.AutoScan {
		return reg, nil
	}

	skillsDir := resolveSkillsDir()
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return reg, nil
		}
		return nil, fmt.Errorf("failed to read skills directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirPath := filepath.Join(skillsDir, entry.Name())
		skillFile := findSkillMd(dirPath)
		if skillFile == "" {
			continue
		}

		fi, statErr := os.Stat(skillFile)
		if statErr != nil {
			continue
		}
		if fi.Size() > 1<<20 {
			logger.Info("SKILL.md %s exceeds 1MB cap, skipping", skillFile)
			continue
		}

		raw, err := os.ReadFile(skillFile)
		if err != nil {
			continue
		}

		meta, body, err := ParseSkillMd(raw)
		if err != nil {
			continue
		}

		name, description, disabled := extractSkillMeta(meta)
		if name == "" || description == "" {
			continue
		}
		if !validateSkillName(name) {
			continue
		}
		if name != entry.Name() {
			continue
		}

		reg.Skills[name] = &Skill{
			Name:        name,
			Description: description,
			Disabled:    disabled,
			DirPath:     dirPath,
			FilePath:    skillFile,
			Body:        "",
			BodyLength:  len(body),
			Loaded:      false,
		}
	}

	reg.L1Block = reg.BuildL1Block()
	return reg, nil
}

// resolveSkillsDir locates the skills directory at ~/.config/tmuxai/skills/
// using the same derivation path as GetKBDir() — sibling to kb/ under the
// tmuxai config directory.
func resolveSkillsDir() string {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(configDir, "skills")
}

// findSkillMd searches a directory for a file named SKILL.md (case-insensitive).
func findSkillMd(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if !e.IsDir() && strings.EqualFold(e.Name(), "SKILL.md") {
			return filepath.Join(dir, e.Name())
		}
	}
	return ""
}

// ============================================================================
// L1 Block Builder (§5.3)
// ============================================================================

// BuildL1Block renders the <available_skills> XML block. Skills are sorted
// by name. Budget enforcement skips skills exceeding max chars.
func (r *SkillRegistry) BuildL1Block() string {
	maxChars := r.budgetMaxL1()
	var buf strings.Builder
	buf.WriteString("\n<available_skills>\n")

	names := make([]string, 0, len(r.Skills))
	for n := range r.Skills {
		names = append(names, n)
	}
	sort.Strings(names)

	trunc := r.budgetTruncateDesc()

	for _, name := range names {
		s := r.Skills[name]
		marker := ""
		if s.Disabled {
			marker = " [manual]"
		}

		desc := s.Description
		if len(desc) > trunc {
			desc = desc[:trunc] + "..."
		}

		line := fmt.Sprintf(
			"  <skill loaded=\"%v\">\n    <name>%s%s</name>\n    <description>%s</description>\n    <location>%s</location>\n  </skill>\n",
			s.Loaded, s.Name, marker, html.EscapeString(desc), html.EscapeString(s.FilePath))

		if buf.Len()+len(line) > maxChars {
			buf.WriteString("  <!-- remaining skills omitted due to L1 budget -->\n")
			break
		}
		buf.WriteString(line)
	}

	buf.WriteString("</available_skills>\n")
	return buf.String()
}

// ============================================================================
// Ancillary File Manifest (§6.4)
// ============================================================================

// BuildManifest walks the skill directory, collects non-SKILL.md files,
// and returns a formatted manifest. Caps at maxManifestFiles / maxManifestChars.
// Returns "" if there are no ancillary files.
func (s *Skill) BuildManifest() string {
	var files []string
	err := filepath.WalkDir(s.DirPath, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(p), "SKILL.md") {
			return nil
		}
		files = append(files, p)
		return nil
	})
	if err != nil || len(files) == 0 {
		return ""
	}

	sort.Strings(files)
	if len(files) > maxManifestFiles {
		files = files[:maxManifestFiles]
	}

	var buf strings.Builder
	buf.WriteString("--- Skill File Manifest: " + s.Name + " ---\n")
	buf.WriteString("The following helper files are available in the skill directory.\n")
	buf.WriteString("Use their full paths to read (cat/head) or execute (bash) them as needed.\n\n")

	for _, f := range files {
		rel, _ := filepath.Rel(s.DirPath, f)
		line := fmt.Sprintf("  %-35s → %s\n", rel, f)
		if buf.Len()+len(line) > maxManifestChars {
			buf.WriteString("  ... (manifest truncated)\n")
			break
		}
		buf.WriteString(line)
	}
	buf.WriteString("--- End Manifest ---\n")
	return buf.String()
}

// ============================================================================
// Load / Unload (§7.3)
// ============================================================================

// Load reads a skill body + manifest from disk, enforces budgets, and stores it.
func (r *SkillRegistry) Load(name string) error {
	skill, ok := r.Skills[name]
	if !ok {
		return fmt.Errorf("skill '%s' not found", name)
	}
	if skill.Loaded {
		return nil
	}

	fi, statErr := os.Stat(skill.FilePath)
	if statErr == nil && fi.Size() > 1<<20 {
		return fmt.Errorf("skill '%s' SKILL.md exceeds 1MB cap", name)
	}

	raw, err := os.ReadFile(skill.FilePath)
	if err != nil {
		return fmt.Errorf("read skill '%s': %w", name, err)
	}

	_, body, err := ParseSkillMd(raw)
	if err != nil {
		return fmt.Errorf("parse skill '%s': %w", name, err)
	}

	manifest := skill.BuildManifest()
	content := body
	if manifest != "" {
		content = body + "\n\n" + manifest
	}

	bodyChars := len(content)

	// Per-skill cap. A configured value of 0 means unlimited; the 1MB
	// SKILL.md hard cap above still applies.
	maxSkill := r.budgetMaxSkill()
	if maxSkill > 0 && bodyChars > maxSkill {
		content = content[:maxSkill] + "\n\n...[skill body truncated]"
		bodyChars = maxSkill
	}

	// Aggregate cap.
	maxLoaded := r.budgetMaxLoaded()
	if r.UsedChars+bodyChars > maxLoaded {
		return fmt.Errorf(
			"skill '%s' (%d chars) would exceed aggregate budget (%d/%d)",
			name, bodyChars, r.UsedChars, maxLoaded)
	}

	skill.Body = content
	skill.BodyLength = bodyChars
	skill.Loaded = true
	r.UsedChars += bodyChars
	r.Skills[name] = skill
	return nil
}

// Unload clears a loaded skill's body and restores UsedChars.
func (r *SkillRegistry) Unload(name string) error {
	skill, ok := r.Skills[name]
	if !ok || !skill.Loaded {
		return fmt.Errorf("skill '%s' is not loaded", name)
	}

	r.UsedChars -= skill.BodyLength
	if r.UsedChars < 0 {
		r.UsedChars = 0
	}
	skill.Body = ""
	skill.BodyLength = 0
	skill.Loaded = false
	r.Skills[name] = skill
	return nil
}

// ============================================================================
// Auto-Match (§7.2)
// ============================================================================

// stopWords is the English stopword set used by tokenize.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "not": true, "you": true,
	"all": true, "can": true, "had": true, "her": true, "was": true,
	"one": true, "our": true, "out": true, "are": true, "has": true,
	"have": true, "from": true, "been": true, "some": true, "them": true,
	"than": true, "its": true, "over": true, "such": true, "that": true,
	"with": true, "will": true, "this": true, "each": true, "when": true,
	"use": true, "how": true, "which": true, "their": true, "what": true,
}

// tokenize lowercases, strips ASCII punctuation, drops stopwords and
// tokens ≤ 2 characters.
func tokenize(text string) []string {
	words := strings.Fields(strings.ToLower(text))
	var result []string
	for _, w := range words {
		w = strings.Trim(w, ".,;:!?()[]{}\"'/\\`~")
		if len(w) > 2 && !stopWords[w] {
			result = append(result, w)
		}
	}
	return result
}

type scoredSkill struct {
	skill *Skill
	score float64
}

// AutoMatch scores each eligible skill against the context string using
// IDF-weighted keyword overlap (§7.2). Excludes disabled and already-loaded
// skills. Returns candidates sorted by score descending, name ascending.
func (r *SkillRegistry) AutoMatch(context string) []*Skill {
	ctxTokens := tokenize(context)
	if len(ctxTokens) == 0 {
		return nil
	}

	// Build IDF: document frequency across skill descriptions.
	idf := make(map[string]float64)
	docCount := 0
	var eligible []*Skill

	for _, skill := range r.Skills {
		if skill.Disabled || skill.Loaded {
			continue
		}
		docCount++
		eligible = append(eligible, skill)
		seen := make(map[string]bool)
		for _, w := range tokenize(skill.Description) {
			if !seen[w] {
				idf[w]++
				seen[w] = true
			}
		}
	}
	if docCount == 0 {
		return nil
	}

	for w := range idf {
		idf[w] = math.Log(float64(docCount)/idf[w]) + 1
	}

	threshold := r.budgetMatchThreshold()
	var candidates []scoredSkill

	for _, skill := range eligible {
		descTokens := tokenize(skill.Description)
		if len(descTokens) == 0 {
			continue
		}

		matchSum := 0.0
		for _, w := range descTokens {
			for _, cw := range ctxTokens {
				if w == cw {
					matchSum += idf[w]
				}
			}
		}

		score := matchSum / float64(len(descTokens))
		if score >= threshold {
			candidates = append(candidates, scoredSkill{skill, score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].skill.Name < candidates[j].skill.Name
	})

	results := make([]*Skill, len(candidates))
	for i, c := range candidates {
		results[i] = c.skill
	}
	return results
}

// ============================================================================
// Budget Accessors (private helpers — defaults when config is nil/zero)
// ============================================================================

func (r *SkillRegistry) budgetMaxL1() int {
	if r.Config == nil || r.Config.MaxL1Chars <= 0 {
		return DefaultMaxL1Chars
	}
	return r.Config.MaxL1Chars
}

func (r *SkillRegistry) budgetMaxLoaded() int {
	if r.Config == nil || r.Config.MaxLoadedChars <= 0 {
		return DefaultMaxLoadedChars
	}
	return r.Config.MaxLoadedChars
}

func (r *SkillRegistry) budgetMaxSkill() int {
	if r.Config == nil {
		return DefaultMaxSkillChars
	}
	if r.Config.MaxSkillChars == 0 {
		return 0
	}
	if r.Config.MaxSkillChars < 0 {
		return DefaultMaxSkillChars
	}
	return r.Config.MaxSkillChars
}

func (r *SkillRegistry) budgetTruncateDesc() int {
	if r.Config == nil || r.Config.TruncateDescAt <= 0 {
		return DefaultTruncateDescAt
	}
	return r.Config.TruncateDescAt
}

func (r *SkillRegistry) budgetMatchThreshold() float64 {
	if r.Config == nil || r.Config.AutoMatchThreshold <= 0 {
		return DefaultAutoMatchThresh
	}
	return r.Config.AutoMatchThreshold
}
