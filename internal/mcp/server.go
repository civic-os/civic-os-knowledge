package mcpserver

import (
	"github.com/civic-os/civic-os-knowledge/internal/tools"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the server version. CI validates this matches the git tag.
const Version = "0.2.0"

// NewMCPServer creates a configured MCP server with all knowledge base tools registered.
func NewMCPServer(deps *tools.Deps) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{
			Name:    "civic-os-knowledge",
			Version: Version,
		},
		nil,
	)

	mcp.AddTool(server, tools.ReadTool(), tools.ReadHandler(deps))
	mcp.AddTool(server, tools.SearchTool(), tools.SearchHandler(deps))
	mcp.AddTool(server, tools.ListTool(), tools.ListHandler(deps))
	mcp.AddTool(server, tools.CreateTool(), tools.CreateHandler(deps))
	mcp.AddTool(server, tools.UpdateTool(), tools.UpdateHandler(deps))
	mcp.AddTool(server, tools.HistoryTool(), tools.HistoryHandler(deps))
	mcp.AddTool(server, tools.DiffTool(), tools.DiffHandler(deps))

	return server
}
