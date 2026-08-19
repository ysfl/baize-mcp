package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ysfl/baize-mcp/internal/baize"
	"github.com/ysfl/baize-mcp/internal/buildinfo"
	"github.com/ysfl/baize-mcp/internal/profile"
)

const maxToolOutputBytes = 64 << 10

type Client interface {
	CheckSession(context.Context) error
	ListAgents(context.Context, baize.AgentListOptions) (baize.AgentPage, error)
	GetAgent(context.Context, string) (baize.AgentSummary, error)
	ListCommandTemplates(context.Context, baize.CommandTemplateListOptions) (baize.CommandTemplatePage, error)
	PreviewCommandTemplate(context.Context, baize.CommandTemplateRenderOptions) (baize.CommandTemplateRenderResult, error)
	CreateCommandPlan(context.Context, baize.CommandPlanCreateOptions) (baize.PlanSummary, error)
	GetCommandPlan(context.Context, string) (baize.PlanSummary, error)
	CancelCommandPlan(context.Context, string, string) (baize.PlanSummary, error)
	RequestCommandPlanApproval(context.Context, baize.CommandPlanApprovalCreateOptions) (baize.CommandPlanApproval, error)
	ListCommandPlanApprovals(context.Context, baize.CommandPlanApprovalListOptions) (baize.CommandPlanApprovalPage, error)
	GetCommandPlanApproval(context.Context, string) (baize.CommandPlanApproval, error)
	DecideCommandPlanApproval(context.Context, string, baize.CommandPlanApprovalDecisionOptions) (baize.CommandPlanApproval, error)
	ListCommandPlanApprovalPolicies(context.Context) ([]baize.CommandPlanApprovalPolicySummary, error)
	ExecuteCommandPlan(context.Context, string, baize.CommandPlanExecuteOptions) (baize.PlanExecutionSummary, error)
	DirectExecTask(context.Context, baize.DirectExecTaskOptions) (baize.TaskSummary, error)
	GetExecTask(context.Context, string) (baize.TaskSummary, error)
	CancelExecTask(context.Context, string) error
}

type emptyInput struct{}

type connectionStatusOutput struct {
	Connected bool `json:"connected" jsonschema:"whether the saved session was accepted by Baize"`
}

// Options 控制 MCP 本地工作流偏好；权限、审批和审计仍由白泽服务端决定。
type Options struct {
	WorkflowMode string
}

type workflowStatusOutput struct {
	WorkflowMode         string                                   `json:"workflowMode"`
	ApprovalPolicies     []baize.CommandPlanApprovalPolicySummary `json:"approvalPolicies"`
	ApprovalPolicyAccess string                                   `json:"approvalPolicyAccess"`
	AuditControl         string                                   `json:"auditControl"`
}

const (
	approvalPolicyAccessAvailable  = "available"
	approvalPolicyAccessNotVisible = "not_visible"
)

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

type commandPlanCancelInput struct {
	ID     string `json:"id" jsonschema:"Baize command plan UUID"`
	Reason string `json:"reason,omitempty" jsonschema:"optional cancellation reason; do not include secrets"`
}

type commandPlanApprovalCreateInput struct {
	PlanID    string     `json:"planId" jsonschema:"Baize command plan UUID"`
	Reason    string     `json:"reason" jsonschema:"why this plan needs approval; do not include secrets"`
	ExpiresAt *time.Time `json:"expiresAt,omitempty" jsonschema:"optional RFC3339 expiry time; must be in the future"`
}

type commandPlanApprovalsListInput struct {
	Page        int    `json:"page,omitempty" jsonschema:"page number, starting at 1"`
	PageSize    int    `json:"pageSize,omitempty" jsonschema:"number of approval records per page, from 1 to 50"`
	PlanID      string `json:"planId,omitempty" jsonschema:"optional Baize command plan UUID"`
	Status      string `json:"status,omitempty" jsonschema:"optional approval status: pending, approved, rejected, or expired"`
	RiskLevel   string `json:"riskLevel,omitempty" jsonschema:"optional risk level: read_only, low, medium, high, or critical"`
	RequesterID string `json:"requesterId,omitempty" jsonschema:"optional requester filter"`
	ApproverID  string `json:"approverId,omitempty" jsonschema:"optional approver filter"`
	Search      string `json:"search,omitempty" jsonschema:"optional approval reason or decision search"`
}

type commandPlanApprovalGetInput struct {
	ID string `json:"id" jsonschema:"Baize command plan approval UUID"`
}

type commandPlanApprovalDecideInput struct {
	ID              string `json:"id" jsonschema:"Baize command plan approval UUID"`
	Approved        bool   `json:"approved" jsonschema:"true submits approval; false rejects it; Baize checks the signed-in account permission"`
	DecisionMessage string `json:"decisionMessage,omitempty" jsonschema:"short decision explanation; required when approved is false"`
}

type commandPlanExecuteInput struct {
	ID             string `json:"id" jsonschema:"Baize command plan UUID"`
	AutoDispatch   *bool  `json:"autoDispatch,omitempty" jsonschema:"optional; false creates a pending task without dispatching it"`
	ConfirmRisk    bool   `json:"confirmRisk,omitempty" jsonschema:"explicitly confirm high-risk execution when Baize requires it"`
	ConfirmMessage string `json:"confirmMessage,omitempty" jsonschema:"short reason for the risk confirmation"`
	DebugSessionID string `json:"debugSessionId,omitempty" jsonschema:"optional Baize-verified debug session UUID; do not invent this value"`
}

type directExecTaskInput struct {
	TemplateID     string         `json:"templateId,omitempty" jsonschema:"optional enabled Baize command template UUID; use this or command"`
	Command        string         `json:"command,omitempty" jsonschema:"optional exact custom command; mutually exclusive with templateId; server must have it in the direct execution allowlist"`
	Title          string         `json:"title" jsonschema:"human-readable execution title"`
	WorkDir        string         `json:"workDir,omitempty" jsonschema:"optional working directory; custom allowlisted commands cannot use one"`
	TimeoutSec     int            `json:"timeoutSec,omitempty" jsonschema:"optional execution timeout in seconds"`
	AutoDispatch   *bool          `json:"autoDispatch,omitempty" jsonschema:"optional; defaults to dispatching the task"`
	ConfirmRisk    bool           `json:"confirmRisk,omitempty" jsonschema:"explicitly confirm high-risk execution"`
	ConfirmMessage string         `json:"confirmMessage,omitempty" jsonschema:"short risk confirmation reason; do not include secrets"`
	Parameters     map[string]any `json:"parameters,omitempty" jsonschema:"template parameter values; scalar values only"`
	TargetAgentIDs []string       `json:"targetAgentIds" jsonschema:"one or more Baize agent UUIDs"`
}

type execTaskGetInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
}

type execTaskCancelInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
}

func New(client Client) *mcp.Server {
	return NewWithOptions(client, Options{WorkflowMode: profile.WorkflowModeMulti})
}

// NewWithOptions 创建带本地工作流偏好的 MCP 服务。
func NewWithOptions(client Client, options Options) *mcp.Server {
	workflowMode := options.WorkflowMode
	if workflowMode != profile.WorkflowModeSingle {
		workflowMode = profile.WorkflowModeMulti
	}
	server := mcp.NewServer(
		&mcp.Implementation{Name: "baize-mcp", Version: buildinfo.Version},
		&mcp.ServerOptions{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))},
	)

	mcp.AddTool(server, readOnlyTool(
		"baize_workflow_status",
		"Get Baize workflow mode",
		"Returns the local single-user or multi-user workflow preference and, when visible to the signed-in account, the server approval policy summary. It never changes permissions or disables Baize audit.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, workflowStatusOutput, error) {
		policies, err := client.ListCommandPlanApprovalPolicies(ctx)
		policyAccess := approvalPolicyAccessAvailable
		if err != nil {
			var apiErr *baize.APIError
			// 审批策略查看权限是管理能力；普通账号不能因看不到策略而失去本地模式状态。
			// 404 兼容尚未发布该只读端点的旧白泽服务，二者都不暴露底层响应细节。
			if errors.As(err, &apiErr) && (apiErr.StatusCode == 403 || apiErr.StatusCode == 404) {
				policies = []baize.CommandPlanApprovalPolicySummary{}
				policyAccess = approvalPolicyAccessNotVisible
				err = nil
			}
		}
		if policies == nil {
			policies = []baize.CommandPlanApprovalPolicySummary{}
		}
		result := workflowStatusOutput{
			WorkflowMode:         workflowMode,
			ApprovalPolicies:     policies,
			ApprovalPolicyAccess: policyAccess,
			AuditControl:         "server_managed",
		}
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_connection_status",
		"Check Baize connection",
		"Checks whether the saved local session can access Baize. Connection addresses and credentials are not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, _ emptyInput) (*mcp.CallToolResult, connectionStatusOutput, error) {
		if err := client.CheckSession(ctx); err != nil {
			return toolOutput(connectionStatusOutput{}, err, "read")
		}
		return toolOutput(connectionStatusOutput{Connected: true}, nil, "read")
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
		return toolOutput(page, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_agent_get",
		"Get Baize agent",
		"Returns privacy-protected status information for one agent. Addresses, fingerprints, capabilities, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentGetInput) (*mcp.CallToolResult, baize.AgentSummary, error) {
		item, err := client.GetAgent(ctx, input.ID)
		return toolOutput(item, err, "read")
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
		return toolOutput(page, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_template_preview",
		"Preview a Baize command template",
		"Validates a server-defined command template for selected agents and returns a bounded, privacy-protected preview. It never creates a plan or runs a command.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandTemplatePreviewInput) (*mcp.CallToolResult, baize.CommandTemplateRenderResult, error) {
		result, err := client.PreviewCommandTemplate(ctx, baize.CommandTemplateRenderOptions{
			TemplateID: strings.TrimSpace(input.TemplateID), AgentIDs: input.AgentIDs, Parameters: input.Parameters, DiagnosisID: strings.TrimSpace(input.DiagnosisID),
		})
		return toolOutput(result, err, "read")
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
		return toolOutput(plan, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_plan_get",
		"Get a Baize command plan",
		"Returns the bounded status, risk, target and precheck information for one command plan. The rendered command itself is not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanGetInput) (*mcp.CallToolResult, baize.PlanSummary, error) {
		plan, err := client.GetCommandPlan(ctx, input.ID)
		return toolOutput(plan, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_command_plan_cancel",
		"Cancel a Baize command plan",
		"Cancels a ready or otherwise not-yet-executed command plan. Baize records the action and applies its existing permission and state rules.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanCancelInput) (*mcp.CallToolResult, baize.PlanSummary, error) {
		plan, err := client.CancelCommandPlan(ctx, input.ID, strings.TrimSpace(input.Reason))
		return toolOutput(plan, err, "write")
	})

	mcp.AddTool(server, writeTool(
		"baize_command_plan_approval_create",
		"Request command-plan approval",
		"Creates a server-side approval request for a ready command plan. It never executes the plan; Baize decides whether the signed-in account may request approval.",
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanApprovalCreateInput) (*mcp.CallToolResult, baize.CommandPlanApproval, error) {
		approval, err := client.RequestCommandPlanApproval(ctx, baize.CommandPlanApprovalCreateOptions{
			PlanID: strings.TrimSpace(input.PlanID), Reason: strings.TrimSpace(input.Reason), ExpiresAt: input.ExpiresAt,
		})
		return toolOutput(approval, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_plan_approvals_list",
		"List command-plan approvals",
		"Lists approval records visible to the signed-in account. Results include bounded status and redacted plan-review fields, never policy details or command text.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanApprovalsListInput) (*mcp.CallToolResult, baize.CommandPlanApprovalPage, error) {
		if input.Page == 0 {
			input.Page = 1
		}
		if input.PageSize == 0 {
			input.PageSize = 20
		}
		page, err := client.ListCommandPlanApprovals(ctx, baize.CommandPlanApprovalListOptions{
			Page: input.Page, PageSize: input.PageSize, PlanID: strings.TrimSpace(input.PlanID), Status: strings.TrimSpace(input.Status),
			RiskLevel: strings.TrimSpace(input.RiskLevel), RequesterID: strings.TrimSpace(input.RequesterID), ApproverID: strings.TrimSpace(input.ApproverID), Search: strings.TrimSpace(input.Search),
		})
		return toolOutput(page, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_command_plan_approval_get",
		"Get command-plan approval",
		"Returns one approval record and a bounded, redacted plan snapshot for review. It excludes command text, parameters, policy details, operator identity, and credentials.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanApprovalGetInput) (*mcp.CallToolResult, baize.CommandPlanApproval, error) {
		approval, err := client.GetCommandPlanApproval(ctx, input.ID)
		return toolOutput(approval, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_command_plan_approval_decide",
		"Decide command-plan approval",
		"Submits approval or rejection for a pending command-plan approval. approved=true is allowed only when Baize grants the signed-in account the required permission; this tool does not execute the plan.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input commandPlanApprovalDecideInput) (*mcp.CallToolResult, baize.CommandPlanApproval, error) {
		approval, err := client.DecideCommandPlanApproval(ctx, input.ID, baize.CommandPlanApprovalDecisionOptions{
			Approved: input.Approved, DecisionMessage: strings.TrimSpace(input.DecisionMessage),
		})
		return toolOutput(approval, err, "write")
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
		return toolOutput(result, err, "write")
	})

	mcp.AddTool(server, writeTool(
		"baize_exec_task_direct",
		"Directly execute a Baize remote task",
		"Creates one auditable Baize execution task through the server direct-execution permission. Enabled templates or an exact server allowlisted custom command may be used. The server still enforces authentication, agent scope and capability, dangerous-command blocking, quotas, risk confirmation, and automatic security-review audit records.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input directExecTaskInput) (*mcp.CallToolResult, baize.TaskSummary, error) {
		task, err := client.DirectExecTask(ctx, baize.DirectExecTaskOptions{
			TemplateID: strings.TrimSpace(input.TemplateID), Command: strings.TrimSpace(input.Command), Title: strings.TrimSpace(input.Title), WorkDir: strings.TrimSpace(input.WorkDir),
			TimeoutSec: input.TimeoutSec, AutoDispatch: input.AutoDispatch, ConfirmRisk: input.ConfirmRisk, ConfirmMessage: strings.TrimSpace(input.ConfirmMessage),
			Parameters: input.Parameters, TargetAgentIDs: input.TargetAgentIDs,
		})
		return toolOutput(task, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_exec_task_get",
		"Get a Baize execution task",
		"Returns bounded task and per-agent progress for one remote execution task. Command text, environment values, output, and operator identity are not returned.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskGetInput) (*mcp.CallToolResult, baize.TaskSummary, error) {
		task, err := client.GetExecTask(ctx, input.ID)
		return toolOutput(task, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_exec_task_cancel",
		"Cancel a Baize execution task",
		"Requests cancellation of a pending or running remote execution task. Baize records the action and applies its existing permission and task-state rules.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskCancelInput) (*mcp.CallToolResult, emptyInput, error) {
		err := client.CancelExecTask(ctx, input.ID)
		return toolOutput(emptyInput{}, err, "write")
	})

	return server
}

// toolOutput 为结构化 MCP 结果设置稳定上限，避免代理或异常服务端响应把无界内容
// 重复写入 AI 对话；客户端摘要会先执行字段级限制。
func toolOutput[T any](value T, err error, action string) (*mcp.CallToolResult, T, error) {
	if err != nil {
		return nil, value, toolErrorWithAction(err, action)
	}
	raw, marshalErr := json.Marshal(value)
	if marshalErr != nil {
		return nil, value, errors.New("the Baize result could not be encoded")
	}
	if len(raw) > maxToolOutputBytes {
		return nil, value, errors.New("the Baize result exceeded the allowed context size")
	}
	return nil, value, nil
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
