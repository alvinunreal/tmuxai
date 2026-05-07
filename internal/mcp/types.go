package mcp

type ServerConfig struct {
	Type           string            `json:"type,omitempty"` // "" | "stdio" | "sse" | "streamable-http"
	Command        string            `json:"command,omitempty"`
	Args           []string          `json:"args,omitempty"`
	URL            string            `json:"url,omitempty"`
	Env            map[string]string `json:"env,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Disabled       bool              `json:"disabled,omitempty"`
}

type MCPConfig struct {
	MCPServers map[string]ServerConfig `json:"mcpServers"`
}

type ToolDef struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema []byte `json:"inputSchema,omitempty"`
}

type ServerStatus string

const (
	StatusHealthy   ServerStatus = "healthy"
	StatusUnhealthy ServerStatus = "unhealthy"
	StatusPending   ServerStatus = "pending"
)

type ServerInfo struct {
	Name      string
	Config    ServerConfig
	Status    ServerStatus
	Tools     []ToolDef
	ErrMsg    string
	Transport string
}
