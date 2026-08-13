package mcpserver

import (
	"context"
	"errors"
	"fmt"
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
	ListCommandTemplates(context.Context, baize.CommandTemplateListOptions) (baize.CommandTemplatePage, error)
	PreviewCommandTemplate(context.Context, baize.CommandTemplateRenderOptions) (baize.CommandTemplateRenderResult, error)
	CreateCommandPlan(context.Context, baize.CommandPlanCreateOptions) (baize.PlanSummary, error)
	GetCommandPlan(context.Context, string) (baize.PlanSummary, error)
	ExecuteCommandPlan(context.Context, string, baize.CommandPlanExecuteOptions) (baize.PlanExecutionSummary, error)
	GetExecTask(context.Context, string) (baize.TaskSummary, error)
	CancelExecTask(context.Context, string) error
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

type commandTemplatesListInput struct {
	Page      int    `json:"page,omitempty" jsonschema:"page number, starting at 1"`
	PageSize  int    `json:"pageSize,omitempty" jsonschema:"number of enabled templates per page, from 1 to 50"`
	Search    string `json:"search,omitempty" jsonschema:"optional template name or description search"`
	Category  string `json:"category,omitempty" jsonschema:"optional template category"`
	RiskLevel string `json:"riskLevel,omitempty" jsonschema:"optional risk level: read_only, low, medium, high, or critical"`
	Platform  string `json:"platform,omitempty" jsonschema:"optional target platform filter"`
}

type commandTemplatePreviewInput struct {
	TemplateID  string         `json:"templateId" jsonschema:"enabled Baize command template UUID"`
	AgentIDs    []string       `json:"agentIds" jsonschema:"one or more Baize agent UUIDs"`
	Parameters  map[string]any `json:"parameters,omitempty" jsonschema:"template parameter values; scalar values only"`
	DiagnosisID string         `json:"diagnosisId,omitempty" jsonschema:"optional related diagnosis UUID"`
}

type commandPlanCreateInput struct {
	TemplateID     string         `json:"templateId" jsonschema:"enabled Baize command template UUID"`
	Title          string         `json:"title,omitempty" jsonschema:"optional human-readable plan title"`
	TargetAgentIDs []string       `json:"targetAgentIds" jsonschema:"one or more Baize agent UUIDs"`
	Parameters     map[string]any `json:"parameters,omitempty" jsonschema:"template parameter values; scalar values only"`
	DiagnosisID    string         `json:"diagnosisId,omitempty" jsonschema:"optional related diagnosis UUID"`
}

type commandPlanGetInput struct {
	ID string `json:"id" jsonschema:"Baize command plan UUID"`
}

type commandPlanExecuteInput struct {
	ID             string `json:"id" jsonschema:"Baize command plan UUID"`
	AutoDispatch   *bool  `json:"autoDispatch,omitempty" jsonschema:"optional; false creates a pending task without dispatching it"`
	ConfirmRisk    bool   `json:"confirmRisk,omitempty" jsonschema:"explicitly confirm high-risk execution when Baize requires it"`
	ConfirmMessage string `json:"confirmMessage,omitempty" jsonschema:"short reason for the risk confirmation"`
	DebugSessionID string `json:"debugSessionId,omitempty" jsonschema:"optional Baize-verified debug session UUID; do not invent this value"`
}

type execTaskGetInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
}

type execTaskCancelInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
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

	mcp.AddTool(server, readOnlyTool(
		"baize_command_templates_list",
		"List Baize command templates",
		"Lists enabled, server-defined command templates that the signed-in account can use. Command bodies and work directories are not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandTemplatesListInput) (*mcp.CallToolResult, baize.CommandTemplatePage, error) {
		if input.Page == 0 {
			input.Page = 1
		}
		if input.PageSize == 0 {
			input.PageSize = 20
		}
		page, err := client.ListCommandTemplates(ctx, baize.CommandTemplateListOptions{
			Page: input.Page, PageSize: input.PageSize, Search: strings.TrimSpace(input.Search),
			Category: strings.TrimSpace(input.Category), RiskLevel: strings.TrimSpace(input.RiskLevel), Platform: strings.TrimSpace(input.Platform),
		})
		return nil, page, toolError(err)
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_template_preview",
		"Preview a Baize command template",
		"Validates a server-defined command template for selected agents and returns a bounded, privacy-protected preview. It never creates a plan or runs a command.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandTemplatePreviewInput) (*mcp.CallToolResult, baize.CommandTemplateRenderResult, error) {
		result, err := client.PreviewCommandTemplate(ctx, baize.CommandTemplateRenderOptions{
			TemplateID: strings.TrimSpace(input.TemplateID), AgentIDs: input.AgentIDs, Parameters: input.Parameters, DiagnosisID: strings.TrimSpace(input.DiagnosisID),
		})
		return nil, result, toolError(err)
	})

	mcp.AddTool(server, writeTool(
		"baize_command_plan_create",
		"Create a Baize command plan",
		"Creates a server-validated, auditable command plan from an enabled template. This does not dispatch a command to any agent.",
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanCreateInput) (*mcp.CallToolResult, baize.PlanSummary, error) {
		plan, err := client.CreateCommandPlan(ctx, baize.CommandPlanCreateOptions{
			TemplateID: strings.TrimSpace(input.TemplateID), Title: strings.TrimSpace(input.Title), TargetAgentIDs: input.TargetAgentIDs,
			Parameters: input.Parameters, DiagnosisID: strings.TrimSpace(input.DiagnosisID),
		})
		return nil, plan, writeToolError(err)
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_plan_get",
		"Get a Baize command plan",
		"Returns the bounded status, risk, target and precheck information for one command plan. The rendered command itself is not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanGetInput) (*mcp.CallToolResult, baize.PlanSummary, error) {
		plan, err := client.GetCommandPlan(ctx, input.ID)
		return nil, plan, toolError(err)
	})

	mcp.AddTool(server, writeTool(
		"baize_command_plan_execute",
		"Execute a Baize command plan",
		"Converts a ready command plan into a Baize execution task. Baize applies the signed-in account permissions, risk confirmation, approval, audit, and dispatch rules.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanExecuteInput) (*mcp.CallToolResult, baize.PlanExecutionSummary, error) {
		result, err := client.ExecuteCommandPlan(ctx, input.ID, baize.CommandPlanExecuteOptions{
			AutoDispatch: input.AutoDispatch, ConfirmRisk: input.ConfirmRisk, ConfirmMessage: strings.TrimSpace(input.ConfirmMessage), DebugSessionID: strings.TrimSpace(input.DebugSessionID),
		})
		return nil, result, writeToolError(err)
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_exec_task_get",
		"Get a Baize execution task",
		"Returns bounded task and per-agent progress for one remote execution task. Command text, environment values, output, and operator identity are not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskGetInput) (*mcp.CallToolResult, baize.TaskSummary, error) {
		task, err := client.GetExecTask(ctx, input.ID)
		return nil, task, toolError(err)
	})

	mcp.AddTool(server, writeTool(
		"baize_exec_task_cancel",
		"Cancel a Baize execution task",
		"Requests cancellation of a pending or running remote execution task. Baize records the action and applies its existing permission and task-state rules.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskCancelInput) (*mcp.CallToolResult, emptyInput, error) {
		err := client.CancelExecTask(ctx, input.ID)
		return nil, emptyInput{}, writeToolError(err)
	})

	return server
}

func toolError(err error) error {
	return toolErrorWithAction(err, "read")
}

func writeToolError(err error) error {
	return toolErrorWithAction(err, "write")
}

func toolErrorWithAction(err error, action string) error {
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
			return fmt.Errorf("Baize denied this %s request", action)
		case 404:
			return errors.New("the requested Baize resource was not found")
		case 409:
			return errors.New("Baize could not complete this request because the current task state requires confirmation or approval")
		case 429:
			return errors.New("Baize temporarily limited this request")
		default:
			return errors.New("Baize could not complete this request")
		}
	}
	return errors.New("the Baize request could not be completed")
}

func writeTool(name, title, description string, destructive bool) *mcp.Tool {
	return &mcp.Tool{
		Name: name, Title: title, Description: description,
		Annotations: &mcp.ToolAnnotations{DestructiveHint: &destructive, IdempotentHint: false, OpenWorldHint: boolPtr(false), ReadOnlyHint: false},
	}
}

func boolPtr(value bool) *bool {
	return &value
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
