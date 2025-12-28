package internal

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/BoringDystopiaDevelopment/tmuxai/logger"
	"github.com/BoringDystopiaDevelopment/tmuxai/system"
	"github.com/briandowns/spinner"
)

func (m *Manager) needSquash() bool {
	totalTokens := 0

	isPrepared := m.ExecPane != nil && m.ExecPane.IsPrepared
	totalTokens += system.EstimateTokenCount(m.chatAssistantPrompt(isPrepared).Content)
	totalTokens += m.getTotalLoadedKBTokens()

	for _, msg := range m.Messages {
		totalTokens += system.EstimateTokenCount(msg.Content)
	}

	threshold := int(float64(m.GetMaxContextSize()) * 0.8)
	return totalTokens > threshold
}

func (m *Manager) squashHistory() {
	if len(m.Messages) < 2 {
		return
	}

	messagesToSummarize := m.Messages[:len(m.Messages)-1]

	summarizedHistory, err := m.summarizeChatHistory(messagesToSummarize)
	if err != nil {
		logger.Error("Failed to summarize chat history: %v", err)
		return
	}

	m.Messages = []ChatMessage{
		{
			Content:   summarizedHistory,
			FromUser:  false,
			Timestamp: time.Now(),
		},
	}
	logger.Debug("Context successfully reduced through summarization")
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

	summary, err := m.AiClient.GetResponseFromChatMessages(ctx, summarizationMessage, m.GetModel())
	if err != nil {
		return "", err
	}

	if m.Config.Debug {
		debugChatMessages(summarizationMessage, summary)
	}

	s.Stop()
	return fmt.Sprintf("CHAT HISTORY SUMMARY:\n%s", summary), nil
}
