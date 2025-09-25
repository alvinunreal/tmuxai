# GitHub Copilot Integration Implementation Plan

## Overview
This document outlines the implementation plan for adding GitHub Copilot support to TmuxAI, allowing users to leverage Copilot's AI models (GPT-4o, Claude 3.5 Sonnet, etc.) directly within their tmux sessions.

## Key Features
- OAuth device flow authentication (no manual token hunting)
- Automatic token refresh for Copilot's short-lived API tokens
- Model discovery and selection
- Seamless provider switching between Copilot, OpenRouter, and Azure OpenAI
- Zero-config startup (no API key required to launch TmuxAI)

## Architecture Changes

### 1. Remove Hard API Key Requirement
**File**: `internal/manager.go`
- Modify `NewManager()` to allow initialization without API keys
- Add `HasConfiguredProvider()` method to check if any provider is available
- Track provider status with new field: `ProviderStatus string`

### 2. Provider Priority System
Order of preference:
1. GitHub Copilot (if authenticated)
2. Azure OpenAI (if configured)
3. OpenRouter (if configured)
4. None (show setup instructions)

## Implementation Steps

### Step 1: Config Structure Extension
**File**: `config/config.go`
```go
type CopilotConfig struct {
    Enabled     bool   `mapstructure:"enabled"`
    Model       string `mapstructure:"model"`
    AuthToken   string // Loaded from file, not config
}
```

Add to main Config struct:
```go
type Config struct {
    // ... existing fields
    Copilot CopilotConfig `mapstructure:"copilot"`
}
```

### Step 2: Copilot Authentication Module
**New File**: `internal/copilot_auth.go`

Key components:
- `RequestDeviceCode()` - Initiate OAuth device flow
- `PollForAuth(deviceCode)` - Poll GitHub for auth completion
- `SaveAuthToken(token)` - Store in `~/.config/tmuxai/.copilot-auth-token`
- `LoadAuthToken()` - Read stored auth token
- `GetCopilotAPIToken(authToken)` - Exchange for short-lived API token
- Token cache with 1-hour TTL

OAuth endpoints:
- Device code: `https://github.com/login/device/code`
- Token exchange: `https://github.com/login/oauth/access_token`
- Copilot token: `https://api.github.com/copilot_internal/v2/token`

Client ID: `Iv1.b507a08c87ecfe98`

### Step 3: Extend AI Client
**File**: `internal/ai_client.go`

Modifications:
```go
type AiClient struct {
    config           *config.Config
    client           *http.Client
    copilotAuthToken string
    copilotAPIToken  *CopilotToken // Cached with expiry
}

type CopilotToken struct {
    Token     string
    ExpiresAt time.Time
}
```

Update `ChatCompletion()`:
- Check provider priority
- If Copilot: refresh API token if expired
- Set endpoint to `https://api.githubcopilot.com`
- Use Copilot token in Authorization header

### Step 4: Command Interface
**File**: `internal/chat_command.go`

New commands:
- `/copilot login` - Start OAuth device flow
- `/copilot logout` - Clear stored tokens
- `/copilot status` - Show auth status
- `/copilot models` - List available models

Implementation:
```go
case prefixMatch(commandPrefix, "/copilot"):
    m.processCopilotCommand(parts)
    return
```

### Step 5: Message Interception
**File**: `internal/process_message.go`

Before processing messages, check:
```go
if !m.HasConfiguredProvider() {
    m.ShowProviderSetupInstructions()
    return
}
```

Setup instructions:
```
No API provider configured. Please configure one:

• GitHub Copilot (recommended):
  /copilot login

• OpenRouter:
  export TMUXAI_OPENROUTER_API_KEY="your-key"

• Azure OpenAI:
  Set credentials in ~/.config/tmuxai/config.yaml

Run /help for more information.
```

### Step 6: Model Discovery
**Endpoint**: `GET https://api.githubcopilot.com/models`

Headers:
```
Authorization: Bearer {copilot_api_token}
Content-Type: application/json
Copilot-Integration-Id: vscode-chat
```

Parse response and cache available models.

## User Experience Flow

### First-Time Setup
1. User runs `tmuxai` (no API key required)
2. User sends message → sees provider setup instructions
3. User runs `/copilot login`
4. TmuxAI shows:
   ```
   First copy your one-time code:
   XXXX-XXXX

   Then visit: https://github.com/login/device

   Waiting for authentication...
   Press Enter to check status...
   ```
5. User completes GitHub auth
6. TmuxAI confirms: "Authentication successful!"
7. Ready to use

### Subsequent Usage
1. TmuxAI loads saved auth token on startup
2. Automatically refreshes Copilot API token as needed
3. Seamless experience with no manual intervention

## Configuration Example

```yaml
# ~/.config/tmuxai/config.yaml
copilot:
  enabled: true
  model: gpt-4o  # or claude-3-5-sonnet, o1-mini, etc.

# Copilot takes precedence if enabled and authenticated
openrouter:
  api_key: ${OPENROUTER_API_KEY}
  model: gpt-4o-mini
```

## Testing Plan

1. **Auth Flow Testing**
   - Test device code request
   - Test auth polling with timeout
   - Test token storage and retrieval
   - Test invalid/expired tokens

2. **Provider Switching**
   - Test Copilot → OpenRouter fallback
   - Test missing all providers
   - Test config override via `/config set`

3. **Model Discovery**
   - Test listing available models
   - Test model selection
   - Test invalid model handling

4. **Token Refresh**
   - Test automatic refresh on expiry
   - Test handling of refresh failures
   - Test concurrent request handling

## Error Handling

1. **Auth Failures**
   - Network errors → retry with backoff
   - Invalid device code → restart flow
   - Expired auth token → prompt re-login

2. **API Failures**
   - Token refresh fails → fall back to other providers
   - Model not available → suggest alternatives
   - Rate limits → show clear error message

## Security Considerations

1. **Token Storage**
   - Store auth token with 0600 permissions
   - Never log tokens
   - Clear tokens on `/copilot logout`

2. **Token Transmission**
   - Always use HTTPS
   - Include proper headers
   - No token in URL parameters

## Migration Path for Existing Users

1. Existing users continue working with current providers
2. Copilot becomes available via `/copilot login`
3. No breaking changes to existing configs
4. Can disable Copilot with `copilot.enabled: false`

## Implementation Timeline

- Phase 1: Core auth flow and token management
- Phase 2: AI client integration
- Phase 3: Command interface and UX
- Phase 4: Model discovery and advanced features
- Phase 5: Testing and documentation

## References

- [GitHub Device Flow](https://docs.github.com/en/apps/oauth-apps/building-oauth-apps/authorizing-oauth-apps#device-flow)
- [Copilot REST API](https://docs.github.com/en/copilot/using-github-copilot/using-github-copilot-in-the-command-line)
- [Shell-ask implementation](https://github.com/egoist/shell-ask)
- [Aider Copilot docs](https://aider.chat/docs/llms/copilot.html)