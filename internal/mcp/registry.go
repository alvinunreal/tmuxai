package mcp

import "sort"

// ToolEntry maps a fully-qualified tool name to its server and tool metadata.
// ServerInfo is stored as a value copy to avoid stale pointer references.
type ToolEntry struct {
	ServerName string
	ToolName   string
	ServerInfo ServerInfo
}

type Registry struct {
	tools map[string]ToolEntry
}

func NewRegistry(manager *MCPManager) *Registry {
	r := &Registry{tools: make(map[string]ToolEntry)}
	servers := manager.GetServerInfo()
	for i := range servers {
		si := servers[i]
		if si.Status != StatusHealthy {
			continue
		}
		for _, tool := range si.Tools {
			fqName := "mcp__" + si.Name + "__" + tool.Name
			r.tools[fqName] = ToolEntry{
				ServerName: si.Name,
				ToolName:   tool.Name,
				ServerInfo: si,
			}
		}
	}
	return r
}

func (r *Registry) Lookup(fqName string) (ToolEntry, bool) {
	e, ok := r.tools[fqName]
	return e, ok
}

func (r *Registry) AllNames() []string {
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}
