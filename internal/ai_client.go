package internal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
)

// AiClient represents an AI client for interacting with OpenRouter API
type AiClient struct {
	config *config.OpenRouterConfig
	client *http.Client
}

// Message represents a chat message
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ChatCompletionRequest represents a request to the chat completion API
type ChatCompletionRequest struct {
	Model      string      `json:"model"`
	Messages   []Message   `json:"messages"`
	Tools      []Tool      `json:"tools,omitempty"`
	ToolChoice interface{} `json:"tool_choice,omitempty"`
}

// FunctionCall represents a function call made by the AI
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a tool call made by the AI
type ToolCall struct {
	ID       string       `json:"id"`
	Type     string       `json:"type"`
	Function FunctionCall `json:"function"`
}

// ChatCompletionChoice represents a choice in the chat completion response
type ChatCompletionChoice struct {
	Index     int        `json:"index"`
	Message   Message    `json:"message"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
}

// ChatCompletionResponse represents a response from the chat completion API
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Choices []ChatCompletionChoice `json:"choices"`
}

func NewAiClient(cfg *config.OpenRouterConfig) *AiClient {
	return &AiClient{
		config: cfg,
		client: &http.Client{},
	}
}

// GetResponseFromChatMessages gets a response from the AI based on chat messages
func (c *AiClient) GetResponseFromChatMessages(chatMessages []ChatMessage, model string) (string, error) {
	// Convert chat messages to AI client format
	aiMessages := []Message{}

	for i, msg := range chatMessages {
		var role string

		if i == 0 && !msg.FromUser {
			role = "system"
		} else if msg.FromUser {
			role = "user"
		} else {
			role = "assistant"
		}

		// Parse the content to check for tool calls
		message := Message{
			Role:    role,
			Content: msg.Content,
		}

		// If this is an assistant message, check for tool calls
		if role == "assistant" {
			// We don't need to extract tool calls from previous messages
			// as they're already in the XML format
		}

		aiMessages = append(aiMessages, message)
	}

	logger.Info("Sending %d messages to AI", len(aiMessages))

	// Get response from AI
	response, err := c.ChatCompletion(aiMessages, model)
	if err != nil {
		return "", err
	}

	return response, nil
}

// ChatCompletion sends a chat completion request to the OpenRouter API
func (c *AiClient) ChatCompletion(messages []Message, model string) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:      model,
		Messages:   messages,
		Tools:      tools,
		ToolChoice: "auto",
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("Failed to marshal request: %v", err)
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.config.BaseURL + "/chat/completions"

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		logger.Error("Failed to create request: %v", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)

	req.Header.Set("HTTP-Referer", "https://github.com/alvinunreal/tmuxai")
	req.Header.Set("X-Title", "TmuxAI")

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		logger.Error("Failed to send request: %v", err)
		return "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Error("Failed to read response: %v", err)
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		logger.Error("API returned error: %s", body)
		return "", fmt.Errorf("API returned error: %s", body)
	}

	// Parse the response
	var completionResp ChatCompletionResponse
	if err := json.Unmarshal(body, &completionResp); err != nil {
		logger.Error("Failed to unmarshal response: %v", err)
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Return the response content
	if len(completionResp.Choices) > 0 {
		choice := completionResp.Choices[0]

		// Format the response with function calls
		var response strings.Builder

		// Add the message content if it exists
		if choice.Message.Content != "" {
			response.WriteString(choice.Message.Content)
		}

		// Process each tool call
		for _, toolCall := range choice.Message.ToolCalls {
			functionName := toolCall.Function.Name
			arguments := toolCall.Function.Arguments

			// Add a newline if there's already content
			if response.Len() > 0 {
				response.WriteString("\n\n")
			}

			// Format the function call
			response.WriteString(fmt.Sprintf("function: %s\narguments: %s", functionName, arguments))
		}

		responseContent := response.String()
		logger.Debug("Received AI response (%d characters): %s", len(responseContent), responseContent)
		return responseContent, nil
	}

	logger.Error("No completion choices returned")
	return "", fmt.Errorf("no completion choices returned")
}

func debugChatMessages(chatMessages []ChatMessage, response string) {

	timestamp := time.Now().Format("20060102-150405")
	configDir, _ := config.GetConfigDir()

	debugDir := fmt.Sprintf("%s/debug", configDir)
	if _, err := os.Stat(debugDir); os.IsNotExist(err) {
		os.Mkdir(debugDir, 0755)
	}

	debugFileName := fmt.Sprintf("%s/debug-%s.txt", debugDir, timestamp)

	file, err := os.Create(debugFileName)
	if err != nil {
		logger.Error("Failed to create debug file: %v", err)
		return
	}
	defer file.Close()

	file.WriteString("==================    SENT CHAT MESSAGES ==================\n\n")

	for i, msg := range chatMessages {
		role := "assistant"
		if msg.FromUser {
			role = "user"
		}
		if i == 0 && !msg.FromUser {
			role = "system"
		}
		timeStr := msg.Timestamp.Format(time.RFC3339)

		file.WriteString(fmt.Sprintf("Message %d: Role=%s, Time=%s\n", i+1, role, timeStr))
		file.WriteString(fmt.Sprintf("Content:\n%s\n\n", msg.Content))
	}

	file.WriteString("==================    RECEIVED RESPONSE ==================\n\n")
	file.WriteString(response)
	file.WriteString("\n\n==================    END DEBUG ==================\n")
}
