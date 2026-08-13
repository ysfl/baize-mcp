package mcpserver

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ysfl/baize-mcp/internal/baize"
	"github.com/ysfl/baize-mcp/internal/buildinfo"
)

type Client interface {
	CheckSession(context.Context) error
	ListAgents(context.Context, baize.AgentListOptions) (baize.AgentPage, error)
	GetAgent(context.Context, string) (baize.AgentSummary, error)
}

type emptyInput struct{}

type connectionStatusOutput struct {
	Connected bool `json:"connected" jsonschema:"whether the saved session was accepted by Baize"`
}

type agentsListInput struct {
	Page         int    `json:"page,omitempty" jsonschema:"page number, starting at 1"`
	PageSize     int    `json:"pageSize,omitempty" jsonschema:"number of agents per page, from 1 to 100"`
	Search       string `json:"search,omitempty" jsonschema:"optional hostname, alias, IP, system, architecture, or version search"`
	Alias        string `json:"alias,omitempty" jsonschema:"optional agent alias filter"`
	System       string `json:"system,omitempty" jsonschema:"optional operating system name or version filter"`
	Region       string `json:"region,omitempty" jsonschema:"optional country or city filter"`
	AgentVersion string `json:"agentVersion,omitempty" jsonschema:"optional Baize agent version filter"`
	Architecture string `json:"architecture,omitempty" jsonschema:"optional system architecture filter"`
	Status       string `json:"status,omitempty" jsonschema:"optional Baize agent status filter"`
	GroupID      string `json:"groupId,omitempty" jsonschema:"optional Baize group UUID"`
	SortBy       string `json:"sortBy,omitempty" jsonschema:"optional sort field: created_at, createdAt, updated_at, updatedAt, last_heartbeat_at, lastHeartbeatAt, registered_at, registeredAt, hostname, alias, status, agent_version, agentVersion, os_type, or osType"`
	SortOrder    string `json:"sortOrder,omitempty" jsonschema:"optional sort direction: asc or desc"`
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
		if err := client.CheckSession(ctx); err != nil {
			return nil, connectionStatusOutput{}, toolError(err)
		}
		return nil, connectionStatusOutput{Connected: true}, nil
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_agents_list",
		"List Baize agents",
		"Returns a paginated list of agents with privacy-protected status information. Addresses, fingerprints, capabilities, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentsListInput) (*mcp.CallToolResult, baize.AgentPage, error) {
		if input.Page == 0 {
			input.Page = 1
		}
		if input.PageSize == 0 {
			input.PageSize = 20
		}
		page, err := client.ListAgents(ctx, baize.AgentListOptions{
			Page:         input.Page,
			PageSize:     input.PageSize,
			Search:       strings.TrimSpace(input.Search),
			Alias:        strings.TrimSpace(input.Alias),
			System:       strings.TrimSpace(input.System),
			Region:       strings.TrimSpace(input.Region),
			AgentVersion: strings.TrimSpace(input.AgentVersion),
			Architecture: strings.TrimSpace(input.Architecture),
			Status:       strings.TrimSpace(input.Status),
			GroupID:      strings.TrimSpace(input.GroupID),
			SortBy:       strings.TrimSpace(input.SortBy),
			SortOrder:    strings.TrimSpace(input.SortOrder),
		})
		return nil, page, toolError(err)
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_agent_get",
		"Get Baize agent",
		"Returns privacy-protected status information for one agent. Addresses, fingerprints, capabilities, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentGetInput) (*mcp.CallToolResult, baize.AgentSummary, error) {
		item, err := client.GetAgent(ctx, input.ID)
		return nil, item, toolError(err)
	})

	return server
}

func toolError(err error) error {
	if err == nil {
		return nil
	}
	var inputErr *baize.InputError
	if errors.As(err, &inputErr) {
		return errors.New(inputErr.Error())
	}
	var apiErr *baize.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case 401:
			return errors.New("the saved Baize session is no longer valid")
		case 403:
			return errors.New("Baize denied this read request")
		case 404:
			return errors.New("the requested Baize resource was not found")
		case 429:
			return errors.New("Baize temporarily limited this request")
		default:
			return errors.New("Baize could not complete this request")
		}
	}
	return errors.New("the Baize request could not be completed")
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
