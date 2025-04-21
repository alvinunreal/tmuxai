// Package system provides utilities for message formatting and code highlighting.
package system

import (
	"fmt"
	"regexp"
)

// Cosmetics processes a message string, applying terminal formatting to markdown code blocks
// (triple backticks, optionally with language) and inline code (single backticks).
// - Code blocks are highlighted using HighlightCode.
// - Inline code is rendered with a gray background and yellow text (ANSI codes).
// All other text is left as-is.
func Cosmetics(message string) string {
	// Regex for code blocks: ```lang\ncode\n```
	codeBlockRe := regexp.MustCompile("(?s)```([a-zA-Z0-9]*)\\n(.*?)\\n```")
	// Regex for inline code: `code`
	inlineCodeRe := regexp.MustCompile("`([^`]+)`")

	result := ""
	lastIndex := 0

	// Find all code blocks
	codeBlocks := codeBlockRe.FindAllStringSubmatchIndex(message, -1)
	for _, block := range codeBlocks {
		start, end := block[0], block[1]
		langStart, langEnd := block[2], block[3]
		codeStart, codeEnd := block[4], block[5]

		// Process text before this code block (may contain inline code)
		segment := message[lastIndex:start]
		result += processInlineCode(segment, inlineCodeRe)

		// Extract language and code
		lang := message[langStart:langEnd]
		code := message[codeStart:codeEnd]

		// Highlight code block
		highlighted, err := HighlightCode(lang, code)
		if err != nil {
			// Fallback: print as plain code block
			highlighted = fmt.Sprintf("```%s\n%s\n```", lang, code)
		}
		result += highlighted
		lastIndex = end
	}
	// Process any remaining text after the last code block
	result += processInlineCode(message[lastIndex:], inlineCodeRe)
	return result
}

// processInlineCode finds inline code (single backticks) and applies ANSI formatting.
func processInlineCode(text string, inlineCodeRe *regexp.Regexp) string {
	const (
		bgGray   = "\033[48;5;240m"
		fgYellow = "\033[38;5;226m"
		reset    = "\033[0m"
	)
	result := ""
	lastIndex := 0
	matches := inlineCodeRe.FindAllStringSubmatchIndex(text, -1)
	for _, m := range matches {
		start, end := m[0], m[1]
		codeStart, codeEnd := m[2], m[3]
		// Add text before inline code
		result += text[lastIndex:start]
		// Add formatted inline code
		code := text[codeStart:codeEnd]
		result += bgGray + fgYellow + code + reset
		lastIndex = end
	}
	// Add any remaining text
	result += text[lastIndex:]
	return result
}
