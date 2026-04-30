package internal

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	bedrockruntime "github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	brtypes "github.com/aws/aws-sdk-go-v2/service/bedrockruntime/types"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
)

// defaultBedrockMaxTokens is used when the user hasn't configured max_tokens.
// Several model families (Titan, Cohere) default to very small response limits
// and will silently truncate replies otherwise.
const defaultBedrockMaxTokens int32 = 4096

// buildBedrockInferenceConfig maps user-facing model config onto the Bedrock
// InferenceConfiguration. MaxTokens always gets a value (user-supplied or
// default); Temperature is only set when the user opts in.
func buildBedrockInferenceConfig(modelCfg config.ModelConfig) *brtypes.InferenceConfiguration {
	maxTokens := modelCfg.MaxTokens
	if maxTokens <= 0 {
		maxTokens = defaultBedrockMaxTokens
	}
	inference := &brtypes.InferenceConfiguration{
		MaxTokens: aws.Int32(maxTokens),
	}
	if modelCfg.Temperature > 0 {
		inference.Temperature = aws.Float32(modelCfg.Temperature)
	}
	return inference
}

// getOrCreateBedrockClient returns a cached Bedrock runtime client, creating
// one when the region/profile tuple changes. Credentials flow through the
// default AWS credential chain (env, shared config, SSO, IAM role, etc.).
func (c *AiClient) getOrCreateBedrockClient(ctx context.Context, region, profile string) (*bedrockruntime.Client, error) {
	c.bedrockMu.Lock()
	defer c.bedrockMu.Unlock()

	cacheKey := region + "|" + profile
	if c.bedrockClient != nil && c.bedrockKey == cacheKey {
		return c.bedrockClient, nil
	}

	opts := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		opts = append(opts, awsconfig.WithRegion(region))
	}
	if profile != "" {
		opts = append(opts, awsconfig.WithSharedConfigProfile(profile))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	if cfg.Region == "" {
		return nil, fmt.Errorf("AWS region not set — specify `region` in the model config or set AWS_REGION")
	}

	client := bedrockruntime.NewFromConfig(cfg)
	c.bedrockClient = client
	c.bedrockKey = cacheKey
	return client, nil
}

// BedrockConverse sends messages to AWS Bedrock via the Converse API, which
// provides a unified interface across all Bedrock-hosted model families
// (Anthropic, Meta, Mistral, Amazon Titan/Nova, Cohere, AI21, etc.).
//
// modelCfg carries the current model's settings (region, profile, inference
// parameters). It is optional — a zero value falls back to default AWS
// credential resolution and a conservative default max_tokens.
func (c *AiClient) BedrockConverse(ctx context.Context, messages []Message, modelID string, modelCfg config.ModelConfig) (string, error) {
	if len(messages) == 0 {
		return "", fmt.Errorf("no messages provided")
	}

	client, err := c.getOrCreateBedrockClient(ctx, modelCfg.Region, modelCfg.AWSProfile)
	if err != nil {
		return "", err
	}

	// Split out system messages; Bedrock Converse accepts them separately.
	var systemBlocks []brtypes.SystemContentBlock
	var convMessages []brtypes.Message
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			systemBlocks = append(systemBlocks, &brtypes.SystemContentBlockMemberText{
				Value: msg.Content,
			})
		case "user":
			convMessages = append(convMessages, brtypes.Message{
				Role: brtypes.ConversationRoleUser,
				Content: []brtypes.ContentBlock{
					&brtypes.ContentBlockMemberText{Value: msg.Content},
				},
			})
		case "assistant":
			convMessages = append(convMessages, brtypes.Message{
				Role: brtypes.ConversationRoleAssistant,
				Content: []brtypes.ContentBlock{
					&brtypes.ContentBlockMemberText{Value: msg.Content},
				},
			})
		}
	}

	if len(convMessages) == 0 {
		return "", fmt.Errorf("no user/assistant messages to send")
	}

	// Bedrock requires a non-empty modelID. Reject early with a clear error
	// rather than letting the SDK produce a less obvious one.
	if strings.TrimSpace(modelID) == "" {
		return "", fmt.Errorf("bedrock model ID is empty — set `model` in the config (e.g. anthropic.claude-3-5-sonnet-20241022-v2:0)")
	}

	inference := buildBedrockInferenceConfig(modelCfg)

	logger.Debug("Sending Bedrock Converse request with model: %s (%d messages, max_tokens=%d)", modelID, len(convMessages), aws.ToInt32(inference.MaxTokens))

	out, err := client.Converse(ctx, &bedrockruntime.ConverseInput{
		ModelId:         aws.String(modelID),
		Messages:        convMessages,
		System:          systemBlocks,
		InferenceConfig: inference,
	})
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("request canceled or timed out: %w", ctx.Err())
		}
		logger.Error("Bedrock Converse failed: %v", err)
		return "", fmt.Errorf("bedrock API error: %w", err)
	}

	msgOut, ok := out.Output.(*brtypes.ConverseOutputMemberMessage)
	if !ok || msgOut == nil {
		return "", fmt.Errorf("bedrock returned unexpected output type (model: %s)", modelID)
	}

	var sb strings.Builder
	for _, block := range msgOut.Value.Content {
		if textBlock, ok := block.(*brtypes.ContentBlockMemberText); ok {
			sb.WriteString(textBlock.Value)
		}
	}

	response := sb.String()
	if response == "" {
		return "", fmt.Errorf("bedrock returned empty response (model: %s)", modelID)
	}

	logger.Debug("Received Bedrock response (%d characters)", len(response))
	return response, nil
}
