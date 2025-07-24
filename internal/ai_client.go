package internal

import (
	"bytes"
	"context"
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

// AiClient represents an AI client for interacting with OpenAI-compatible APIs including Azure OpenAI
type AiClient struct {
	config *config.Config
	client *http.Client
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents a request to the chat completion API
type ChatCompletionRequest struct {
	Model    string    `json:"model,omitempty"`
	Messages []Message `json:"messages"`
}

// ChatCompletionChoice represents a choice in the chat completion response
type ChatCompletionChoice struct {
	Index   int     `json:"index"`
	Message Message `json:"message"`
}

// ChatCompletionResponse represents a response from the chat completion API
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Choices []ChatCompletionChoice `json:"choices"`
}

func NewAiClient(cfg *config.Config) *AiClient {
	return &AiClient{
		config: cfg,
		client: &http.Client{},
	}
}

// GetResponseFromChatMessages gets a response from the AI based on chat messages
func (c *AiClient) GetResponseFromChatMessages(ctx context.Context, chatMessages []ChatMessage, model string) (string, error) {
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

		aiMessages = append(aiMessages, Message{
			Role:    role,
			Content: msg.Content,
		})
	}

	logger.Info("Sending %d messages to AI", len(aiMessages))

	// Get response from AI
	response, err := c.ChatCompletion(ctx, aiMessages, model)
	if err != nil {
		return "", err
	}

	return response, nil
}

// ChatCompletion sends a chat completion request to the OpenRouter API
func (c *AiClient) ChatCompletion(ctx context.Context, messages []Message, model string) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:    model,
		Messages: messages,
	}

	// determine endpoint and headers based on configuration
	var url string
	var apiKeyHeader string
	var apiKey string

	if c.config.AzureOpenAI.APIKey != "" {
		// Use Azure OpenAI endpoint
		base := strings.TrimSuffix(c.config.AzureOpenAI.APIBase, "/")
		url = fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=%s",
			base,
			c.config.AzureOpenAI.DeploymentName,
			c.config.AzureOpenAI.APIVersion)
		apiKeyHeader = "api-key"
		apiKey = c.config.AzureOpenAI.APIKey

		// Azure endpoint doesn't expect model in body
		reqBody.Model = ""
	} else {
		// default OpenRouter/OpenAI compatible endpoint
		baseURL := strings.TrimSuffix(c.config.OpenRouter.BaseURL, "/")
		url = baseURL + "/chat/completions"
		apiKeyHeader = "Authorization"
		apiKey = "Bearer " + c.config.OpenRouter.APIKey
	}

	reqJSON, err := json.Marshal(reqBody)
	if err != nil {
		logger.Error("Failed to marshal request: %v", err)
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqJSON))
	if err != nil {
		logger.Error("Failed to create request: %v", err)
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(apiKeyHeader, apiKey)

	req.Header.Set("HTTP-Referer", "https://github.com/alvinunreal/tmuxai")
	req.Header.Set("X-Title", "TmuxAI")

	// Log the request details for debugging before sending
	logger.Debug("Sending API request to: %s with model: %s", url, model)

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		if ctx.Err() == context.Canceled {
			return "", fmt.Errorf("request canceled: %w", ctx.Err())
		}
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

	// Log the raw response for debugging
	logger.Debug("API response status: %d, response size: %d bytes", resp.StatusCode, len(body))

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		logger.Error("API returned error: %s", body)
		return "", fmt.Errorf("API returned error: %s", body)
	}

	// Parse the response
	var completionResp ChatCompletionResponse
	if err := json.Unmarshal(body, &completionResp); err != nil {
		logger.Error("Failed to unmarshal response: %v, body: %s", err, body)
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Return the response content
	if len(completionResp.Choices) > 0 {
		responseContent := completionResp.Choices[0].Message.Content
		logger.Debug("Received AI response (%d characters): %s", len(responseContent), responseContent)
		return responseContent, nil
	}

	// Enhanced error for no completion choices
	logger.Error("No completion choices returned. Raw response: %s", string(body))
	return "", fmt.Errorf("no completion choices returned (model: %s, status: %d)", model, resp.StatusCode)
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
