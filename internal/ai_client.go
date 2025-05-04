package internal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// AiClient represents an AI client for interacting with OpenRouter API or OpenAI API
type AiClient struct {
	config                  *config.OpenRouterConfig
	client                  *http.Client
	apiProvider             string
	apiUrl                  string
	apiKey                  string
	model                   string
	openaiClient            openai.Client
	openaiClientInitialized bool
}

// Message represents a chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatCompletionRequest represents a request to the chat completion API
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
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
	client := &AiClient{
		config:      &cfg.OpenRouter,
		client:      &http.Client{},
		apiProvider: cfg.ApiProvider,
		apiUrl:      cfg.ApiUrl,
		apiKey:      cfg.ApiKey,
		model:       cfg.Model,
	}

	// Initialize OpenAI client if using that provider
	if client.apiProvider == "openai" {
		// Set environment variables for the OpenAI client
		if client.apiKey != "" {
			os.Setenv("OPENAI_API_KEY", client.apiKey)
		}

		// Initialize the OpenAI client
		client.openaiClient = openai.NewClient()
		client.openaiClientInitialized = true

		// Set custom base URL if specified
		if client.apiUrl != "" {
			// Create a new client with custom base URL
			opts := []option.RequestOption{
				option.WithBaseURL(client.apiUrl),
			}
			client.openaiClient = openai.NewClient(opts...)
		}
	}

	return client
}

// convertChatMessageToAIFormat converts a ChatMessage to the format needed for OpenAI API
// and returns the role, content, and any tool calls information
func convertChatMessageToAIFormat(msg ChatMessage, index int, messages []ChatMessage) (string, string, string) {
	// Determine role
	role := "assistant"
	if msg.FromUser {
		role = "user"
	}
	if index == 0 && !msg.FromUser {
		role = "system"
	}

	// Basic content from the message
	content := msg.Content

	// Format tool calls if present
	var toolCallsFormatted string
	if len(msg.ToolCalls) > 0 {
		toolCallsData := make([]map[string]interface{}, 0)

		for _, tc := range msg.ToolCalls {
			// Create a tool call representation
			toolCall := map[string]interface{}{
				"type": tc.Type,
				"function": map[string]string{
					"name":      tc.Type,
					"arguments": tc.Content,
				},
			}
			toolCallsData = append(toolCallsData, toolCall)
		}

		// Convert to JSON string
		toolCallsBytes, _ := json.Marshal(map[string]interface{}{
			"tool_calls": toolCallsData,
		})
		toolCallsFormatted = string(toolCallsBytes)
	}

	return role, content, toolCallsFormatted
}

// GetResponseFromChatMessages gets a response from the AI based on chat messages
func (c *AiClient) GetResponseFromChatMessages(chatMessages []ChatMessage, model string, tools []openai.ChatCompletionToolParam) (string, error) {
	// Convert chat messages to AI client format
	aiMessages := []Message{}

	for i, msg := range chatMessages {
		role, content, _ := convertChatMessageToAIFormat(msg, i, chatMessages)

		// Basic message without tool calls
		message := Message{
			Role:    role,
			Content: content,
		}

		aiMessages = append(aiMessages, message)
	}

	logger.Info("Sending %d messages to AI", len(aiMessages))

	// Use appropriate provider based on config
	var response string
	var err error

	if c.apiProvider == "openai" && c.openaiClientInitialized {
		// Use OpenAI client
		modelToUse := model
		if modelToUse == "" {
			modelToUse = c.model
		}

		response, err = c.openaiChatCompletion(aiMessages, modelToUse, tools, chatMessages)
	} else {
		// Use the original implementation
		response, err = c.ChatCompletion(aiMessages, model)
	}

	if err != nil {
		return "", err
	}

	return response, nil
}

// openaiChatCompletion sends a chat completion request using the official OpenAI client
func (c *AiClient) openaiChatCompletion(messages []Message, model string, tools []openai.ChatCompletionToolParam, originalMessages []ChatMessage) (string, error) {
	ctx := context.Background()

	// Convert our messages to OpenAI's format
	openaiMessages := []openai.ChatCompletionMessageParamUnion{}

	for i, msg := range messages {
		switch msg.Role {
		case "system":
			openaiMessages = append(openaiMessages, openai.SystemMessage(msg.Content))
		case "user":
			openaiMessages = append(openaiMessages, openai.UserMessage(msg.Content))
		case "assistant":
			// Check if the original message had tool calls
			if i < len(originalMessages) && len(originalMessages[i].ToolCalls) > 0 {
				_, content, toolCallsJSON := convertChatMessageToAIFormat(originalMessages[i], i, originalMessages)

				// Include tool calls in the message content with a format that won't confuse the AI
				completeContent := content
				if toolCallsJSON != "" {
					// Append the tool calls in a way that makes it clear they were previous actions
					completeContent += "\n\n[Previous tool calls: " + toolCallsJSON + "]"
				}

				// Use regular assistant message with enhanced content
				openaiMessages = append(openaiMessages, openai.AssistantMessage(completeContent))
			} else {
				// Regular assistant message without tool calls
				openaiMessages = append(openaiMessages, openai.AssistantMessage(msg.Content))
			}
		}
	}

	// Create completion params
	params := openai.ChatCompletionNewParams{
		Messages: openaiMessages,
		Model:    model,
		Tools:    tools,
	}

	// Call OpenAI API
	completion, err := c.openaiClient.Chat.Completions.New(ctx, params)
	if err != nil {
		logger.Error("Failed to get completion from OpenAI: %v", err)
		return "", fmt.Errorf("failed to get completion from OpenAI: %w", err)
	}

	if len(completion.Choices) == 0 {
		logger.Error("No completion choices returned from OpenAI")
		return "", fmt.Errorf("no completion choices returned from OpenAI")
	}

	// Convert completion response to JSON to include all tool call information
	completionJSON, err := json.Marshal(completion)
	if err != nil {
		logger.Error("Failed to marshal completion to JSON: %v", err)
		return "", fmt.Errorf("failed to marshal completion to JSON: %w", err)
	}

	responseJSON := string(completionJSON)
	logger.Debug("Received AI response: %s", responseJSON)

	return responseJSON, nil
}

// ChatCompletion sends a chat completion request to the OpenRouter API
func (c *AiClient) ChatCompletion(messages []Message, model string) (string, error) {
	reqBody := ChatCompletionRequest{
		Model:    model,
		Messages: messages,
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
		responseContent := completionResp.Choices[0].Message.Content
		logger.Debug("Received AI response (%d characters): %s", len(responseContent), responseContent)
		return responseContent, nil
	}

	logger.Error("No completion choices returned")
	return "", fmt.Errorf("no completion choices returned")
}

func debugChatMessages(chatMessages []ChatMessage, response string) {
	// Get the config directory
	configDir, err := config.GetConfigDir()
	if err != nil {
		logger.Error("Failed to get config directory: %s", err)
		return
	}

	// Create debug directory if it doesn't exist
	debugDir := fmt.Sprintf("%s/debug", configDir)
	err = os.MkdirAll(debugDir, 0755)
	if err != nil {
		logger.Error("Failed to create debug directory: %s", err)
		return
	}

	// Create file with timestamp
	fileName := fmt.Sprintf("%s/chat_%s.txt", debugDir, time.Now().Format("2006-01-02_15-04-05"))
	file, err := os.Create(fileName)
	if err != nil {
		logger.Error("Failed to create debug file: %s", err)
		return
	}
	defer file.Close()

	file.WriteString("==================    SENT CHAT MESSAGES ==================\n\n")

	for i, msg := range chatMessages {
		role, content, toolCallsJSON := convertChatMessageToAIFormat(msg, i, chatMessages)
		timeStr := msg.Timestamp.Format(time.RFC3339)

		file.WriteString(fmt.Sprintf("Message %d: Role=%s, Time=%s\n", i+1, role, timeStr))
		file.WriteString(fmt.Sprintf("Content:\n%s\n\n", content))

		// Log tool calls if present
		if len(msg.ToolCalls) > 0 {
			file.WriteString("Tool Calls:\n")
			file.WriteString(fmt.Sprintf("  Raw JSON: %s\n\n", toolCallsJSON))

			for j, tc := range msg.ToolCalls {
				file.WriteString(fmt.Sprintf("  Tool Call %d: Type=%s\n", j+1, tc.Type))

				// Try to pretty print the content if it's JSON
				var prettyContent string
				var contentMap map[string]interface{}
				if err := json.Unmarshal([]byte(tc.Content), &contentMap); err == nil {
					prettyBytes, _ := json.MarshalIndent(contentMap, "    ", "  ")
					prettyContent = string(prettyBytes)
					file.WriteString(fmt.Sprintf("    Arguments:\n%s\n", prettyContent))
				} else {
					file.WriteString(fmt.Sprintf("    Content: %s\n", tc.Content))
				}

				if tc.StateValue != "" {
					file.WriteString(fmt.Sprintf("    StateValue: %s\n", tc.StateValue))
				}
				file.WriteString("\n")
			}
		}
	}

	file.WriteString("==================    RECEIVED RESPONSE ==================\n\n")

	// Try to parse the response as JSON to format it nicely
	var responseObj map[string]interface{}
	if err := json.Unmarshal([]byte(response), &responseObj); err == nil {
		// Format nicely for tool calls
		if choices, ok := responseObj["choices"].([]interface{}); ok && len(choices) > 0 {
			choice := choices[0].(map[string]interface{})
			if message, ok := choice["message"].(map[string]interface{}); ok {
				// Output content if present
				if content, ok := message["content"].(string); ok && content != "" {
					file.WriteString(fmt.Sprintf("Content: %s\n\n", content))
				}

				// Format tool calls if present
				if toolCalls, ok := message["tool_calls"].([]interface{}); ok && len(toolCalls) > 0 {
					file.WriteString("Tool Calls:\n")
					for i, tc := range toolCalls {
						toolCall := tc.(map[string]interface{})
						if function, ok := toolCall["function"].(map[string]interface{}); ok {
							name := function["name"].(string)
							args := function["arguments"].(string)

							// Try to pretty print the arguments
							var prettyArgs string
							var argsMap map[string]interface{}
							if err := json.Unmarshal([]byte(args), &argsMap); err == nil {
								prettyBytes, _ := json.MarshalIndent(argsMap, "", "  ")
								prettyArgs = string(prettyBytes)
							} else {
								prettyArgs = args
							}

							file.WriteString(fmt.Sprintf("  %d. Function: %s\n", i+1, name))
							file.WriteString(fmt.Sprintf("     Arguments: %s\n\n", prettyArgs))
						}
					}
				}
			}

			// Add finish reason
			if reason, ok := choice["finish_reason"].(string); ok {
				file.WriteString(fmt.Sprintf("Finish Reason: %s\n", reason))
			}
		}

		// Add model info
		if model, ok := responseObj["model"].(string); ok {
			file.WriteString(fmt.Sprintf("Model: %s\n", model))
		}

		// Add other relevant metadata
		// file.WriteString("\nFull JSON Response:\n")
		// prettyJSON, _ := json.MarshalIndent(responseObj, "", "  ")
		// file.WriteString(string(prettyJSON))
	} else {
		// If not valid JSON, write as-is
		file.WriteString(response)
	}

	file.WriteString("\n\n==================    END DEBUG ==================\n")
}
