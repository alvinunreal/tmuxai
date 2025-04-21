package internal

import (
	"fmt"
	"html"
	"regexp"
	"strings"
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
		{"ExecAndWait", false, false, func(r *AIResponse, v string) { r.ExecAndWait = v }},
		{"RequestAccomplished", false, true, func(r *AIResponse, v string) { r.RequestAccomplished = isTrue(v) }},
		{"ExecPaneSeemsBusy", false, true, func(r *AIResponse, v string) { r.ExecPaneSeemsBusy = isTrue(v) }},
		{"WaitingForUserResponse", false, true, func(r *AIResponse, v string) { r.WaitingForUserResponse = isTrue(v) }},
		{"NoComment", false, true, func(r *AIResponse, v string) { r.NoComment = isTrue(v) }},
	}

	clean := response
	tagPattern := `(?s)<%s>(.*?)</%s>`
	r := AIResponse{}
	cleanForMsg := clean
	for _, t := range tags {
		reTag := regexp.MustCompile(fmt.Sprintf(tagPattern, t.name, t.name))
		tagMatches := reTag.FindAllStringSubmatch(clean, -1)
		for _, m := range tagMatches {
			// m[0] is the full match, m[1] is the value
			if len(m) < 2 {
				continue // skip invalid match
			}
			val := strings.TrimSpace(m[1])
			// Decode XML entities for non-bool tags
			if !t.isBool {
				val = html.UnescapeString(val)
			}
			if t.isArray {
				t.setField(&r, val)
			} else {
				t.setField(&r, val)
			}
		}
		// For message: remove all tag blocks, including code/backtick wrappers
		// Remove code block: ```xml\n<tag>...</tag>\n```, ```\n<tag>...</tag>\n```
		cleanForMsg = regexp.MustCompile(fmt.Sprintf("(?s)```(?:xml)?\\s*<%s>.*?</%s>\\s*```", t.name, t.name)).ReplaceAllString(cleanForMsg, "")
		// Remove single backtick-wrapped tags: `<Tag>...</Tag>`
		cleanForMsg = regexp.MustCompile(fmt.Sprintf("`<%s>.*?</%s>`", t.name, t.name)).ReplaceAllString(cleanForMsg, "")
		// Remove plain tag: <Tag>...</Tag>
		cleanForMsg = reTag.ReplaceAllString(cleanForMsg, "")
	}

	// Special handling: tags that may appear as <TagName> or ```<TagName>``` (no value)
	// Set bool fields to true if such tag is present, even if no value
	for _, t := range tags {
		if !t.isBool {
			continue
		}
		// Match <TagName> or ```<TagName>```
		pat := fmt.Sprintf("(?s)(<%s>\\s*</%s>|<%s>\\s*|```<%s>```|<%s/>)", t.name, t.name, t.name, t.name, t.name)
		if regexp.MustCompile(pat).MatchString(clean) {
			t.setField(&r, "1")
		}
	}

	// Message: trim, collapse multiple newlines
	msg := strings.TrimSpace(cleanForMsg)
	msg = collapseBlankLines(msg)
	// Remove any leftover tag lines (e.g. <TagName>) that may not have been removed
	for _, t := range tags {
		// Remove lines that are just <TagName> or ```<TagName>```
		reLeftover := regexp.MustCompile(fmt.Sprintf("(?m)^\\s*(<%s>\\s*|```<%s>```)?\\s*$", t.name, t.name))
		msg = reLeftover.ReplaceAllString(msg, "")
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
	return mustCompile(`\n{2,}`).ReplaceAllString(s, "\n")
}

// mustCompile is a helper for regexp.MustCompile
func mustCompile(expr string) *regexp.Regexp {
	re, err := regexp.Compile(expr)
	if err != nil {
		panic(err)
	}
	return re
}
