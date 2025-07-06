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
		// All action tags are now handled separately below
		{"RequestAccomplished", false, true, func(r *AIResponse, v string) { r.RequestAccomplished = isTrue(v) }},
		{"ExecPaneSeemsBusy", false, true, func(r *AIResponse, v string) { r.ExecPaneSeemsBusy = isTrue(v) }},
		{"WaitingForUserResponse", false, true, func(r *AIResponse, v string) { r.WaitingForUserResponse = isTrue(v) }},
		{"NoComment", false, true, func(r *AIResponse, v string) { r.NoComment = isTrue(v) }},
		{"CreateExecPane", false, true, func(r *AIResponse, v string) { r.CreateExecPane = isTrue(v) }},
	}

	clean := response
	r := AIResponse{}
	cleanForMsg := clean

	// Generic regex for tags with optional pane_id
	reWithPaneID := func(tagName string) *regexp.Regexp {
		return regexp.MustCompile(fmt.Sprintf(`(?s)<%s(?: pane_id="([^"]*)")?>(.*?)</%s>`, tagName, tagName))
	}

	// Handle ExecCommand
	reExec := reWithPaneID("ExecCommand")
	execMatches := reExec.FindAllStringSubmatch(clean, -1)
	for _, match := range execMatches {
		if len(match) >= 3 {
			r.ExecCommand = append(r.ExecCommand, ExecCommandInfo{PaneID: match[1], Command: html.UnescapeString(strings.TrimSpace(match[2]))})
		}
	}
	cleanForMsg = reExec.ReplaceAllString(cleanForMsg, "")

	// Handle TmuxSendKeys
	reSendKeys := reWithPaneID("TmuxSendKeys")
	sendKeysMatches := reSendKeys.FindAllStringSubmatch(clean, -1)
	for _, match := range sendKeysMatches {
		if len(match) >= 3 {
			r.SendKeys = append(r.SendKeys, SendKeysInfo{PaneID: match[1], Keys: html.UnescapeString(match[2])})
		}
	}
	cleanForMsg = reSendKeys.ReplaceAllString(cleanForMsg, "")

	// Handle PasteMultilineContent
	rePaste := reWithPaneID("PasteMultilineContent")
	pasteMatches := rePaste.FindAllStringSubmatch(clean, -1)
	for _, match := range pasteMatches {
		if len(match) >= 3 {
			r.PasteMultilineContent = append(r.PasteMultilineContent, PasteInfo{PaneID: match[1], Content: html.UnescapeString(strings.TrimSpace(match[2]))})
		}
	}
	cleanForMsg = rePaste.ReplaceAllString(cleanForMsg, "")

	// Handle the simple boolean tags
	for _, t := range tags {
		reTag := regexp.MustCompile(fmt.Sprintf(`(?s)<%s>(.*?)</%s>`, t.name, t.name))
		tagMatches := reTag.FindAllStringSubmatch(clean, -1)
		for _, m := range tagMatches {
			if len(m) < 2 {
				continue
			}
			val := strings.TrimSpace(m[1])
			if !t.isBool {
				val = html.UnescapeString(val)
			}
			if t.isArray {
				t.setField(&r, val)
			} else {
				t.setField(&r, val)
			}
		}
		// For message: remove all tag blocks
		cleanForMsg = reTag.ReplaceAllString(cleanForMsg, "")
	}

	// Message: trim, collapse multiple newlines
	msg := strings.TrimSpace(cleanForMsg)
	msg = collapseBlankLines(msg)
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
