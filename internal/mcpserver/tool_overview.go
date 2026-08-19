package mcpserver

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ysfl/baize-mcp/internal/baize"
)

type overviewGetInput struct {
	GroupID string `json:"groupId,omitempty" jsonschema:"optional Baize group UUID; omit for the account-wide scope"`
	Limit   int    `json:"limit,omitempty" jsonschema:"optional number of highlighted abnormal nodes, from 1 to 20; defaults to 10"`
}

func registerOverviewTool(server *mcp.Server, client Client) {
	mcp.AddTool(server, readOnlyTool(
		"baize_overview_get",
		"Get Baize runtime overview",
		"Returns an account-scoped read-only runtime summary and a bounded list of abnormal nodes. Empty cached resource sections or an empty abnormal list are marked explicitly and must not be treated as proof that every node is healthy. Addresses, credentials, and internal ranking weights are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input overviewGetInput) (*mcp.CallToolResult, baize.OverviewSummary, error) {
		limit := input.Limit
		if limit == 0 {
			limit = baize.OverviewDefaultLimit
		}
		result, err := client.GetOverview(ctx, baize.OverviewOptions{
			GroupID: strings.TrimSpace(input.GroupID),
			Limit:   limit,
		})
		return toolOutput(result, err, "read")
	})
}
