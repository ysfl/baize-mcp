package mcpserver

import (
	"context"
	"io"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ysfl/baize-mcp/internal/baize"
	"github.com/ysfl/baize-mcp/internal/buildinfo"
)

type Client interface {
	CurrentUser(context.Context) (baize.CurrentUser, error)
	ListAgents(context.Context, baize.AgentListOptions) (baize.AgentPage, error)
	GetAgent(context.Context, string) (baize.AgentSummary, error)
}

type emptyInput struct{}

type connectionStatusOutput struct {
	Connected bool `json:"connected" jsonschema:"whether the saved session was accepted by Baize"`
}

type agentsListInput struct {
	Page     int    `json:"page,omitempty" jsonschema:"page number, starting at 1"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"number of agents per page, from 1 to 100"`
	Search   string `json:"search,omitempty" jsonschema:"optional name, system, architecture, or version search"`
	Status   string `json:"status,omitempty" jsonschema:"optional Baize agent status filter"`
}

type agentGetInput struct {
	ID string `json:"id" jsonschema:"Baize agent UUID"`
}

func New(client Client) *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "baize-mcp", Version: buildinfo.Version},
		&mcp.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
	)

	mcp.AddTool(server, readOnlyTool(
		"baize_connection_status",
		"Check Baize connection",
		"Checks whether the saved local session can access Baize. Connection addresses and credentials are not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, connectionStatusOutput, error) {
		if _, err := client.CurrentUser(ctx); err != nil {
			return nil, connectionStatusOutput{}, err
		}
		return nil, connectionStatusOutput{Connected: true}, nil
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_agents_list",
		"List Baize agents",
		"Returns a paginated, privacy-reduced list of agents. Addresses, fingerprints, capabilities, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentsListInput) (*mcp.CallToolResult, baize.AgentPage, error) {
		if input.Page == 0 {
			input.Page = 1
		}
		if input.PageSize == 0 {
			input.PageSize = 20
		}
		page, err := client.ListAgents(ctx, baize.AgentListOptions{
			Page:     input.Page,
			PageSize: input.PageSize,
			Search:   strings.TrimSpace(input.Search),
			Status:   strings.TrimSpace(input.Status),
		})
		return nil, page, err
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_agent_get",
		"Get Baize agent",
		"Returns privacy-reduced details for one agent. Addresses, fingerprints, capabilities, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentGetInput) (*mcp.CallToolResult, baize.AgentSummary, error) {
		item, err := client.GetAgent(ctx, input.ID)
		return nil, item, err
	})

	return server
}

func readOnlyTool(name, title, description string) *mcp.Tool {
	falseValue := false
	return &mcp.Tool{
		Name:        name,
		Title:       title,
		Description: description,
		Annotations: &mcp.ToolAnnotations{
			DestructiveHint: &falseValue,
			IdempotentHint:  true,
			OpenWorldHint:   &falseValue,
			ReadOnlyHint:    true,
		},
	}
}
