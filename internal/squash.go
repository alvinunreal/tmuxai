package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/logger"
	"github.com/alvinunreal/tmuxai/system"
	"github.com/briandowns/spinner"
)

// needSquash checks if the current context size is approaching the max limit
func (m *Manager) needSquash() bool {
	totalTokens := 0
	for _, msg := range m.Messages {
		totalTokens += system.EstimateTokenCount(msg.Content)
	}

	threshold := int(float64(m.GetMaxContextSize()) * 0.8)
	return totalTokens > threshold
}

// manageContext handles context reduction by summarizing chat history
func (m *Manager) squashHistory() {
	var systemMessage ChatMessage
	var assistantBaseMessage ChatMessage
	var hasSystemMessage bool
	var hasAssistantBaseMessage bool

	// Find system and initial assistant messages to preserve
	for i, msg := range m.Messages {
		if i == 0 && !msg.FromUser {
			systemMessage = msg
			hasSystemMessage = true
			continue
		}
		if i == 1 && !msg.FromUser && hasSystemMessage {
			assistantBaseMessage = msg
			hasAssistantBaseMessage = true
			break
		}
	}

	// Messages to be summarized (exclude system and initial assistant message)
	var messagesToSummarize []ChatMessage
	startIdx := 0
	if hasSystemMessage {
		startIdx++
	}
	if hasAssistantBaseMessage {
		startIdx++
	}

	// Only summarize if we have messages beyond the base ones
	if startIdx < len(m.Messages)-1 {
		messagesToSummarize = m.Messages[startIdx : len(m.Messages)-1] // Exclude the most recent user message

		// Request summarization from AI
		summarizedHistory, err := m.summarizeChatHistory(messagesToSummarize)
		if err != nil {
			logger.Error("Failed to summarize chat history: %v", err)
			return
		}

		// Build new context with summarized history
		var newHistory []ChatMessage

		// Add system message if present
		if hasSystemMessage {
			newHistory = append(newHistory, systemMessage)
		}

		// Add assistant base message if present
		if hasAssistantBaseMessage {
			newHistory = append(newHistory, assistantBaseMessage)
		}

		// Add the summary as a system message
		newHistory = append(newHistory, ChatMessage{
			Content:   summarizedHistory,
			FromUser:  false,
			Timestamp: time.Now(),
		})

		m.Messages = newHistory
		logger.Debug("Context successfully reduced through summarization")
	}
}

// summarizeChatHistory asks the AI to summarize the chat history
func (m *Manager) summarizeChatHistory(messages []ChatMessage) (string, error) {
	s := spinner.New(spinner.CharSets[26], 100*time.Millisecond)
	s.Start()

	// Convert messages to a readable format for summarization
	var chatLog strings.Builder
	for _, msg := range messages {
		role := "Assistant"
		if msg.FromUser {
			role = "User"
		}

		chatLog.WriteString(fmt.Sprintf("[%s]: %s\n\n", role, msg.Content))
	}

	// Create a summarization prompt
	summarizationPrompt := fmt.Sprintf(
		"Below is a chat history between a user and an assistant. Please provide a concise summary of the key points, decisions, and context from this conversation. Focus on the most important information that would be needed to continue the conversation effectively:\n\n%s",
		chatLog.String(),
	)

	// Create a temporary AI client for summarization to avoid affecting the main conversation
	summarizationMessage := []ChatMessage{
		{
			Content:   summarizationPrompt,
			FromUser:  true,
			Timestamp: time.Now(),
		},
	}

	// Create a context for the summarization request (no timeout to support local LLMs with large contexts)
	ctx := context.Background()

	summary, err := m.AiClient.GetResponseFromChatMessages(ctx, summarizationMessage, m.GetOpenRouterModel())
	if err != nil {
		return "", err
	}

	if m.Config.Debug {
		debugChatMessages(summarizationMessage, summary)
	}

	s.Stop()
	return fmt.Sprintf("CHAT HISTORY SUMMARY:\n%s", summary), nil
}
