package internal

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alvinunreal/tmuxai/config"
	"github.com/alvinunreal/tmuxai/logger"
)

const (
	clientID             = "Iv1.b507a08c87ecfe98"
	deviceCodeURL        = "https://github.com/login/device/code"
	accessTokenURL       = "https://github.com/login/oauth/access_token"
	copilotTokenURL      = "https://api.github.com/copilot_internal/v2/token"
	copilotAuthTokenFile = ".copilot-auth-token"
)

// DeviceCodeResponse represents GitHub's device code response
type DeviceCodeResponse struct {
	DeviceCode      string `json:"device_code"`
	UserCode        string `json:"user_code"`
	VerificationURI string `json:"verification_uri"`
	ExpiresIn       int    `json:"expires_in"`
	Interval        int    `json:"interval"`
}

// AccessTokenResponse represents GitHub's access token response
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
}

// CopilotTokenResponse represents Copilot's API token response
type CopilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
}

// CopilotAuth manages GitHub Copilot authentication
type CopilotAuth struct {
	config          *config.Config
	client          *http.Client
	authToken       string
	copilotToken    *CopilotTokenResponse
	copilotTokenMux sync.RWMutex
}

// NewCopilotAuth creates a new CopilotAuth instance
func NewCopilotAuth(cfg *config.Config) *CopilotAuth {
	return &CopilotAuth{
		config: cfg,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// RequestDeviceCode initiates the OAuth device flow
func (c *CopilotAuth) RequestDeviceCode() (*DeviceCodeResponse, error) {
	data := url.Values{
		"client_id": {clientID},
		"scope":     {"read:user"},
	}

	req, err := http.NewRequest("POST", deviceCodeURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create device code request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request device code: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var deviceCode DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&deviceCode); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	return &deviceCode, nil
}

// PollForAccessToken polls GitHub for the access token
func (c *CopilotAuth) PollForAccessToken(deviceCode *DeviceCodeResponse) (*AccessTokenResponse, error) {
	data := url.Values{
		"client_id":   {clientID},
		"device_code": {deviceCode.DeviceCode},
		"grant_type":  {"urn:ietf:params:oauth:grant-type:device_code"},
	}

	req, err := http.NewRequest("POST", accessTokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create access token request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request access token: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp AccessTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode access token response: %w", err)
	}

	if tokenResp.Error != "" {
		if tokenResp.Error == "authorization_pending" {
			return nil, nil // Continue polling
		}
		return nil, fmt.Errorf("access token error: %s", tokenResp.Error)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("received empty access token")
	}

	return &tokenResp, nil
}

// GetCopilotAPIToken exchanges GitHub auth token for Copilot API token
func (c *CopilotAuth) GetCopilotAPIToken() (string, error) {
	c.copilotTokenMux.RLock()
	if c.copilotToken != nil && time.Now().Unix() < c.copilotToken.ExpiresAt-60 {
		token := c.copilotToken.Token
		c.copilotTokenMux.RUnlock()
		return token, nil
	}
	c.copilotTokenMux.RUnlock()

	c.copilotTokenMux.Lock()
	defer c.copilotTokenMux.Unlock()

	// Double-check after acquiring write lock
	if c.copilotToken != nil && time.Now().Unix() < c.copilotToken.ExpiresAt-60 {
		return c.copilotToken.Token, nil
	}

	if c.authToken == "" {
		// Try to load auth token from file
		if err := c.LoadAuthToken(); err != nil {
			return "", fmt.Errorf("no auth token available, please run /copilot login")
		}
	}

	req, err := http.NewRequest("GET", copilotTokenURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create copilot token request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.authToken))
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to request copilot token: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("copilot token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp CopilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("failed to decode copilot token response: %w", err)
	}

	c.copilotToken = &tokenResp
	logger.Debug("Got new Copilot API token, expires at %d", tokenResp.ExpiresAt)

	return tokenResp.Token, nil
}

// SaveAuthToken saves the GitHub auth token to file
func (c *CopilotAuth) SaveAuthToken(token string) error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	tokenPath := filepath.Join(configDir, copilotAuthTokenFile)
	if err := os.WriteFile(tokenPath, []byte(token), 0600); err != nil {
		return fmt.Errorf("failed to save auth token: %w", err)
	}

	c.authToken = token
	logger.Info("Auth token saved successfully")
	return nil
}

// LoadAuthToken loads the GitHub auth token from file
func (c *CopilotAuth) LoadAuthToken() error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	tokenPath := filepath.Join(configDir, copilotAuthTokenFile)
	data, err := os.ReadFile(tokenPath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("no saved auth token found")
		}
		return fmt.Errorf("failed to read auth token: %w", err)
	}

	c.authToken = strings.TrimSpace(string(data))
	logger.Debug("Loaded auth token from file")
	return nil
}

// RemoveAuthToken removes the saved auth token
func (c *CopilotAuth) RemoveAuthToken() error {
	configDir, err := config.GetConfigDir()
	if err != nil {
		return fmt.Errorf("failed to get config directory: %w", err)
	}

	tokenPath := filepath.Join(configDir, copilotAuthTokenFile)
	if err := os.Remove(tokenPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove auth token: %w", err)
	}

	c.authToken = ""
	c.copilotToken = nil
	logger.Info("Auth token removed successfully")
	return nil
}

// IsAuthenticated checks if we have a valid auth token
func (c *CopilotAuth) IsAuthenticated() bool {
	if c.authToken != "" {
		return true
	}
	// Try to load from file
	err := c.LoadAuthToken()
	return err == nil && c.authToken != ""
}

// ListModels fetches available models from Copilot API
func (c *CopilotAuth) ListModels() ([]string, error) {
	token, err := c.GetCopilotAPIToken()
	if err != nil {
		return nil, fmt.Errorf("failed to get copilot token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://api.githubcopilot.com/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create models request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "GithubCopilot/1.155.0")
	req.Header.Set("Editor-Plugin-Version", "copilot/1.155.0")
	req.Header.Set("Editor-Version", "vscode/1.85.1")
	req.Header.Set("Copilot-Integration-Id", "vscode-chat")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to request models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("models request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var modelsResp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&modelsResp); err != nil {
		return nil, fmt.Errorf("failed to decode models response: %w", err)
	}

	models := make([]string, len(modelsResp.Data))
	for i, model := range modelsResp.Data {
		models[i] = model.ID
	}

	return models, nil
}