package internal

import (
	"fmt"
	"html"
	"regexp"
	"strings"

	"github.com/alvinunreal/tmuxai/internal/mcp"
)

// Fix #10: Pre-compile all tag regex patterns at package level
type tagRegexes struct {
	tag       *regexp.Regexp
	codeBlock *regexp.Regexp
	backtick  *regexp.Regexp
	boolPat   *regexp.Regexp
	leftover  *regexp.Regexp
}

var tagNames = []string{
	"TmuxSendKeys", "ExecCommand", "PasteMultilineContent",
	"RequestAccomplished", "ExecPaneSeemsBusy", "WaitingForUserResponse", "NoComment",
}

var tagPatterns = func() map[string]*tagRegexes {
	patterns := make(map[string]*tagRegexes, len(tagNames))
	for _, name := range tagNames {
		patterns[name] = &tagRegexes{
			tag:       regexp.MustCompile(fmt.Sprintf(`(?s)<%s>(.*?)</%s>`, name, name)),
			codeBlock: regexp.MustCompile(fmt.Sprintf("(?s)```(?:xml)?\\s*<%s>.*?</%s>\\s*```", name, name)),
			backtick:  regexp.MustCompile(fmt.Sprintf("`<%s>.*?</%s>`", name, name)),
			boolPat:   regexp.MustCompile(fmt.Sprintf(`(?s)(<%s>\s*</%s>|<%s>\s*|`+"```<%s>```"+`|<%s/>)`, name, name, name, name, name)),
			leftover:  regexp.MustCompile(fmt.Sprintf(`(?m)^\s*(<%s>\s*|`+"```<%s>```"+`)?\s*$`, name, name)),
		}
	}
	return patterns
}()

// Fix #13: Pre-compile MCPToolCall code-block stripping patterns
var (
	mcpCodeBlockRe = regexp.MustCompile(`(?s)` + "```(?:xml)?\\s*<MCPToolCall>.*?</MCPToolCall>\\s*```")
	mcpBacktickRe  = regexp.MustCompile("`<MCPToolCall>.*?</MCPToolCall>`")
	mcpTagRe       = regexp.MustCompile(`(?s)<MCPToolCall>.*?</MCPToolCall>`)
	multiNewlineRe = regexp.MustCompile(`\n{2,}`)
)

func (m *Manager) parseAIResponse(response string) (AIResponse, error) {
	// Tag mapping: tag name -> field
	type tagInfo struct {
		name     string
		isArray  bool
		isBool   bool
		setField func(*AIResponse, string)
	}
	tags := []tagInfo{
		{"TmuxSendKeys", true, false, func(r *AIResponse, v string) { r.SendKeys = append(r.SendKeys, v) }},
		{"ExecCommand", true, false, func(r *AIResponse, v string) { r.ExecCommand = append(r.ExecCommand, v) }},
		{"PasteMultilineContent", false, false, func(r *AIResponse, v string) { r.PasteMultilineContent = v }},
		{"RequestAccomplished", false, true, func(r *AIResponse, v string) { r.RequestAccomplished = isTrue(v) }},
		{"ExecPaneSeemsBusy", false, true, func(r *AIResponse, v string) { r.ExecPaneSeemsBusy = isTrue(v) }},
		{"WaitingForUserResponse", false, true, func(r *AIResponse, v string) { r.WaitingForUserResponse = isTrue(v) }},
		{"NoComment", false, true, func(r *AIResponse, v string) { r.NoComment = isTrue(v) }},
	}

	clean := response
	r := AIResponse{}
	cleanForMsg := clean
	for _, t := range tags {
		pats := tagPatterns[t.name]
		tagMatches := pats.tag.FindAllStringSubmatch(clean, -1)
		for _, m := range tagMatches {
			if len(m) < 2 {
				continue
			}
			val := strings.TrimSpace(m[1])
			if !t.isBool {
				val = html.UnescapeString(val)
			}
			t.setField(&r, val)
		}
		// For message: remove all tag blocks, including code/backtick wrappers
		cleanForMsg = pats.codeBlock.ReplaceAllString(cleanForMsg, "")
		cleanForMsg = pats.backtick.ReplaceAllString(cleanForMsg, "")
		cleanForMsg = pats.tag.ReplaceAllString(cleanForMsg, "")
	}

	// Special handling: tags that may appear as <TagName> or ```<TagName>``` (no value)
	for _, t := range tags {
		if !t.isBool {
			continue
		}
		if tagPatterns[t.name].boolPat.MatchString(clean) {
			t.setField(&r, "1")
		}
	}

	// Fix #13: Unwrap code-fenced MCPToolCall tags before parsing
	cleanForMcp := mcpCodeBlockRe.ReplaceAllStringFunc(clean, func(match string) string {
		if inner := mcpTagRe.FindString(match); inner != "" {
			return inner
		}
		return match
	})
	mcpCalls, _ := mcp.ParseMCPToolCalls(cleanForMcp)
	r.MCPToolCalls = mcpCalls

	// Fix #13: Strip code-block-wrapped MCPToolCall from display message
	cleanForMsg = mcpCodeBlockRe.ReplaceAllString(cleanForMsg, "")
	cleanForMsg = mcpBacktickRe.ReplaceAllString(cleanForMsg, "")
	_, cleanForMsg = mcp.ParseMCPToolCalls(cleanForMsg)

	// Message: trim, collapse multiple newlines
	msg := strings.TrimSpace(cleanForMsg)
	msg = collapseBlankLines(msg)
	// Remove any leftover tag lines (e.g. <TagName>) that may not have been removed
	for _, t := range tags {
		msg = tagPatterns[t.name].leftover.ReplaceAllString(msg, "")
	}
	msg = strings.TrimSpace(msg)
	r.Message = msg

	return r, nil
}

// Helper: check if string is "1" or "true" (case-insensitive)
func isTrue(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true"
}

// Collapse multiple blank lines to a single newline
func collapseBlankLines(s string) string {
	return multiNewlineRe.ReplaceAllString(s, "\n")
}
