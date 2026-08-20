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
	GetOverview(context.Context, baize.OverviewOptions) (baize.OverviewSummary, error)
	ObserveAgent(context.Context, baize.AgentObserveOptions) (baize.AgentObserveResult, error)
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
	DispatchExecTask(context.Context, string) (baize.TaskSummary, error)
	GetExecTaskOutput(context.Context, baize.ExecTaskOutputOptions) (baize.ExecTaskOutputSummary, error)
	CancelExecTask(context.Context, string) error
	StartRuntimeDiagnosis(context.Context, baize.RuntimeDiagnosisStartOptions) (baize.RuntimeDiagnosisSummary, error)
	GetRuntimeDiagnosis(context.Context, string) (baize.RuntimeDiagnosisDetail, error)
	GetRuntimeDiagnosisAIContext(context.Context, string) (baize.RuntimeDiagnosisAIContext, error)
	QueryLogs(context.Context, baize.LogsQueryOptions) (baize.LogQueryResult, error)
	ListAlerts(context.Context, baize.AlertsListOptions) (baize.AlertIncidentPage, error)
	ChangeAlert(context.Context, baize.AlertChangeOptions) (baize.AlertChangeResult, error)
	ListCertificates(context.Context, baize.CertificatesListOptions) (baize.CertificateTargetPage, error)
	QueryAssets(context.Context, baize.AssetsQueryOptions) (baize.AssetQueryResult, error)
	QueryCronJobs(context.Context, baize.CronJobsQueryOptions) (baize.CronJobsQueryResult, error)
	QueryRunbooks(context.Context, baize.RunbooksQueryOptions) (baize.RunbooksQueryResult, error)
	ObserveNginx(context.Context, baize.NginxObserveOptions) (baize.NginxObserveResult, error)
	ObserveSecurity(context.Context, baize.SecurityObserveOptions) (baize.SecurityObserveResult, error)
	GetSystemRelease(context.Context, baize.SystemReleaseOptions) (baize.SystemReleaseResult, error)
	GetSubscription(context.Context, baize.SubscriptionOptions) (baize.SubscriptionResult, error)
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

type agentObserveInput struct {
	AgentID string     `json:"agentId" jsonschema:"Baize agent UUID"`
	View    string     `json:"view" jsonschema:"observation view: health, metrics, processes, storage, docker, nginx, host_profile, or control_plane"`
	Metric  string     `json:"metric,omitempty" jsonschema:"optional process metric: cpu, memory, read_rate, write_rate, rx_rate, or tx_rate"`
	From    *time.Time `json:"from,omitempty" jsonschema:"optional RFC3339 start time for processes view; defaults to the last 15 minutes"`
	To      *time.Time `json:"to,omitempty" jsonschema:"optional RFC3339 end time for processes view; defaults to now"`
	Limit   int        `json:"limit,omitempty" jsonschema:"optional process result limit; storage and other views use their fixed safety bounds"`
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

type execTaskDispatchInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
}

type execTaskOutputInput struct {
	TaskID       string `json:"taskId" jsonschema:"Baize execution task UUID"`
	TargetID     string `json:"targetId,omitempty" jsonschema:"optional execution target UUID"`
	Limit        int    `json:"limit,omitempty" jsonschema:"optional output records per target, from 1 to 200; defaults to 50"`
	Mode         string `json:"mode,omitempty" jsonschema:"optional output mode: tail or page; defaults to tail"`
	AfterSeq     *int   `json:"afterSeq,omitempty" jsonschema:"optional continue after this output sequence"`
	BeforeSeq    *int   `json:"beforeSeq,omitempty" jsonschema:"optional read before this output sequence"`
	TargetLimit  int    `json:"targetLimit,omitempty" jsonschema:"optional number of targets, maximum 20"`
	TargetOffset int    `json:"targetOffset,omitempty" jsonschema:"optional target offset for multi-target tasks"`
}

type execTaskCancelInput struct {
	ID string `json:"id" jsonschema:"Baize execution task UUID"`
}

type runtimeDiagnosisStartInput struct {
	AgentID      string `json:"agentId" jsonschema:"Baize agent UUID"`
	TargetType   string `json:"targetType" jsonschema:"read-only diagnosis target: pid, port, process_name, file, service, or container"`
	TargetValue  string `json:"targetValue" jsonschema:"target value validated by Baize; do not provide a shell command"`
	TimeHint     string `json:"timeHint,omitempty" jsonschema:"optional time or incident hint; this does not execute a command"`
	SourceModule string `json:"sourceModule,omitempty" jsonschema:"optional caller label"`
	TimeoutSec   int    `json:"timeoutSec,omitempty" jsonschema:"optional timeout from 1 to 10 seconds"`
	MaxResults   int    `json:"maxResults,omitempty" jsonschema:"optional result limit from 1 to 50"`
}

type runtimeDiagnosisGetInput struct {
	ID string `json:"id" jsonschema:"Baize runtime diagnosis UUID"`
}

type runtimeDiagnosisAIContextGetInput struct {
	ID string `json:"id" jsonschema:"Baize runtime diagnosis UUID"`
}

type logsQueryInput struct {
	Source           string `json:"source,omitempty"`
	AgentID          string `json:"agentId,omitempty"`
	Level            string `json:"level,omitempty"`
	Module           string `json:"module,omitempty"`
	Search           string `json:"search,omitempty"`
	TaskID           string `json:"taskId,omitempty"`
	SinceMinutes     int    `json:"sinceMinutes,omitempty"`
	SinceTimestampMS int64  `json:"sinceTimestampMs,omitempty"`
	Limit            int    `json:"limit,omitempty"`
	WindowMinutes    int    `json:"windowMinutes,omitempty"`
}

type alertsListInput struct {
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"pageSize,omitempty"`
	Status   string `json:"status,omitempty"`
	Severity string `json:"severity,omitempty"`
}

type alertChangeInput struct {
	IncidentID string `json:"incidentId" jsonschema:"Baize alert incident UUID"`
	Action     string `json:"action" jsonschema:"alert action: acknowledge or resolve"`
}

type certificatesListInput struct {
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"pageSize,omitempty"`
	Search   string `json:"search,omitempty"`
}

type assetsQueryInput struct {
	View        string `json:"view,omitempty"`
	ID          string `json:"id,omitempty"`
	Page        int    `json:"page,omitempty"`
	PageSize    int    `json:"pageSize,omitempty"`
	Status      string `json:"status,omitempty"`
	Environment string `json:"environment,omitempty"`
	Provider    string `json:"provider,omitempty"`
	Search      string `json:"search,omitempty"`
	Days        int    `json:"days,omitempty"`
}

type cronJobsQueryInput struct {
	View          string `json:"view,omitempty"`
	ID            string `json:"id,omitempty"`
	Page          int    `json:"page,omitempty"`
	PageSize      int    `json:"pageSize,omitempty"`
	Enabled       *bool  `json:"enabled,omitempty"`
	ScheduleType  string `json:"scheduleType,omitempty"`
	TargetAgentID string `json:"targetAgentId,omitempty"`
	Search        string `json:"search,omitempty"`
	SortBy        string `json:"sortBy,omitempty"`
	SortOrder     string `json:"sortOrder,omitempty"`
}

type runbooksQueryInput struct {
	View      string `json:"view,omitempty"`
	ID        string `json:"id,omitempty"`
	Page      int    `json:"page,omitempty"`
	PageSize  int    `json:"pageSize,omitempty"`
	Status    string `json:"status,omitempty"`
	Category  string `json:"category,omitempty"`
	RiskLevel string `json:"riskLevel,omitempty"`
	AIUsable  *bool  `json:"aiUsable,omitempty"`
	Search    string `json:"search,omitempty"`
	Action    string `json:"action,omitempty"`
}

type nginxObserveInput struct {
	View     string     `json:"view" jsonschema:"observation view: sites, site, overview, latest, upstream, slow_requests, or response_time"`
	AgentID  string     `json:"agentId,omitempty" jsonschema:"optional Baize agent UUID for agent-scoped views"`
	SiteID   string     `json:"siteId,omitempty" jsonschema:"optional Nginx site UUID for site view"`
	From     *time.Time `json:"from,omitempty" jsonschema:"optional RFC3339 start time for slow_requests or response_time"`
	To       *time.Time `json:"to,omitempty" jsonschema:"optional RFC3339 end time for slow_requests or response_time"`
	Page     int        `json:"page,omitempty" jsonschema:"optional page number for slow_requests"`
	PageSize int        `json:"pageSize,omitempty" jsonschema:"optional page size from 1 to 50"`
}

type securityObserveInput struct {
	View     string `json:"view" jsonschema:"observation view: exposure_overview, exposure_findings, exposure_scans, network_overview, network_observations, network_paths, or network_risks"`
	Page     int    `json:"page,omitempty" jsonschema:"optional page number"`
	PageSize int    `json:"pageSize,omitempty" jsonschema:"optional page size from 1 to 50"`
	AgentID  string `json:"agentId,omitempty" jsonschema:"optional Baize agent UUID for network views"`
}

type systemReleaseInput struct{}
type subscriptionInput struct{}

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
	registerOverviewTool(server, client)

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
		"baize_agent_observe",
		"Observe a Baize agent",
		"Reads one explicit, bounded observation view for an agent. Results identify excluded or redacted content and do not expose credentials, environment values, host-profile contents, or raw terminal data.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input agentObserveInput) (*mcp.CallToolResult, baize.AgentObserveResult, error) {
		result, err := client.ObserveAgent(ctx, baize.AgentObserveOptions{
			AgentID: strings.TrimSpace(input.AgentID), View: strings.TrimSpace(input.View), Metric: strings.TrimSpace(input.Metric), From: input.From, To: input.To, Limit: input.Limit,
		})
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_runtime_diagnosis_start",
		"Start a Baize runtime diagnosis",
		"Creates a bounded, read-only runtime diagnosis probe for one agent. Baize validates the target, checks the signed-in account and agent capability, records the diagnosis, and keeps command execution and approvals separate.",
		false,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input runtimeDiagnosisStartInput) (*mcp.CallToolResult, baize.RuntimeDiagnosisSummary, error) {
		result, err := client.StartRuntimeDiagnosis(ctx, baize.RuntimeDiagnosisStartOptions{
			AgentID: strings.TrimSpace(input.AgentID), TargetType: strings.TrimSpace(input.TargetType), TargetValue: strings.TrimSpace(input.TargetValue),
			TimeHint: strings.TrimSpace(input.TimeHint), SourceModule: strings.TrimSpace(input.SourceModule), TimeoutSec: input.TimeoutSec, MaxResults: input.MaxResults,
		})
		return toolOutput(result, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_runtime_diagnosis_get",
		"Get a Baize runtime diagnosis",
		"Returns a bounded diagnosis status and evidence counts for one previously started probe. Process commands, paths, port addresses, evidence values, credentials, and raw Agent output are excluded; missing detail is not evidence that the diagnosis failed.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input runtimeDiagnosisGetInput) (*mcp.CallToolResult, baize.RuntimeDiagnosisDetail, error) {
		result, err := client.GetRuntimeDiagnosis(ctx, strings.TrimSpace(input.ID))
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_runtime_diagnosis_ai_context_get",
		"Get bounded diagnosis AI context",
		"Reads the privacy-reduced AI context for one diagnosis when the signed-in account and Baize settings allow it. Commands, paths, evidence values, operator identity, audit references, credentials, and model endpoints are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input runtimeDiagnosisAIContextGetInput) (*mcp.CallToolResult, baize.RuntimeDiagnosisAIContext, error) {
		result, err := client.GetRuntimeDiagnosisAIContext(ctx, strings.TrimSpace(input.ID))
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_logs_query",
		"Query bounded Baize logs",
		"Reads one fixed log view: recent server logs, an on-demand agent log query, or an aggregate overview. Correlation identifiers and raw source paths are excluded; redaction and truncation are reported explicitly.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input logsQueryInput) (*mcp.CallToolResult, any, error) {
		result, err := client.QueryLogs(ctx, baize.LogsQueryOptions{Source: input.Source, AgentID: input.AgentID, Level: input.Level, Module: input.Module, Search: input.Search, TaskID: input.TaskID, SinceMinutes: input.SinceMinutes, SinceTimestampMS: input.SinceTimestampMS, Limit: input.Limit, WindowMinutes: input.WindowMinutes})
		return toolOutput[any](result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_alerts_list",
		"List Baize alerts",
		"Lists alerts visible to the signed-in account with bounded, privacy-reduced messages. Acknowledging user identity, resource identifiers, and diagnosis target values are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input alertsListInput) (*mcp.CallToolResult, any, error) {
		result, err := client.ListAlerts(ctx, baize.AlertsListOptions{Page: input.Page, PageSize: input.PageSize, Status: input.Status, Severity: input.Severity})
		return toolOutput[any](result, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_alert_change",
		"Acknowledge or resolve a Baize alert",
		"Requests acknowledgement or resolution for one visible Baize alert. Baize enforces the alert-management permission, current alert state, audit and any server-side rules; query the alert again to confirm the final status.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input alertChangeInput) (*mcp.CallToolResult, baize.AlertChangeResult, error) {
		result, err := client.ChangeAlert(ctx, baize.AlertChangeOptions{IncidentID: strings.TrimSpace(input.IncidentID), Action: strings.TrimSpace(input.Action)})
		return toolOutput(result, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_certificates_list",
		"List certificate status",
		"Lists certificate monitoring targets and their latest bounded status. Certificate file paths, subjects, issuers, private keys, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input certificatesListInput) (*mcp.CallToolResult, any, error) {
		result, err := client.ListCertificates(ctx, baize.CertificatesListOptions{Page: input.Page, PageSize: input.PageSize, Search: input.Search})
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_assets_query",
		"Query Baize assets",
		"Reads one fixed asset inventory view: list, summary, expiring, or detail. Asset IP addresses, notes, links, and credentials are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input assetsQueryInput) (*mcp.CallToolResult, any, error) {
		result, err := client.QueryAssets(ctx, baize.AssetsQueryOptions{View: input.View, ID: input.ID, Page: input.Page, PageSize: input.PageSize, Status: input.Status, Environment: input.Environment, Provider: input.Provider, Search: input.Search, Days: input.Days})
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_cron_jobs_query",
		"Query scheduled tasks",
		"Reads one fixed scheduled-task view: list, detail, or execution logs. Commands, working directories, and operator identity are excluded; this tool never runs or changes a task.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input cronJobsQueryInput) (*mcp.CallToolResult, any, error) {
		result, err := client.QueryCronJobs(ctx, baize.CronJobsQueryOptions{View: input.View, ID: input.ID, Page: input.Page, PageSize: input.PageSize, Enabled: input.Enabled, ScheduleType: input.ScheduleType, TargetAgentID: input.TargetAgentID, Search: input.Search, SortBy: input.SortBy, SortOrder: input.SortOrder})
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_runbooks_query",
		"Query Baize Runbooks",
		"Reads one fixed Runbook view: definitions, bounded step metadata, or definition audit events. Inputs, bindings, instructions, operator identity, client IP, and audit details are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input runbooksQueryInput) (*mcp.CallToolResult, any, error) {
		result, err := client.QueryRunbooks(ctx, baize.RunbooksQueryOptions{View: input.View, ID: input.ID, Page: input.Page, PageSize: input.PageSize, Status: input.Status, Category: input.Category, RiskLevel: input.RiskLevel, AIUsable: input.AIUsable, Search: input.Search, Action: input.Action})
		return toolOutput(result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_nginx_observe",
		"Observe Baize Nginx state",
		"Reads one fixed Nginx observation view. Client addresses, complete URLs, configuration contents, credentials, and raw slow-request details are excluded; missing detail is not evidence of failure.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input nginxObserveInput) (*mcp.CallToolResult, any, error) {
		result, err := client.ObserveNginx(ctx, baize.NginxObserveOptions{View: input.View, AgentID: input.AgentID, SiteID: input.SiteID, From: input.From, To: input.To, Page: input.Page, PageSize: input.PageSize})
		return toolOutput[any](result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_security_observe",
		"Observe Baize security state",
		"Reads one fixed exposure or network-entry security view. Results contain bounded risk codes, severities, statuses, counts, and summaries; addresses, paths, process details, and raw evidence are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input securityObserveInput) (*mcp.CallToolResult, any, error) {
		result, err := client.ObserveSecurity(ctx, baize.SecurityObserveOptions{View: input.View, AgentID: input.AgentID, Page: input.Page, PageSize: input.PageSize})
		return toolOutput[any](result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_system_release_get",
		"Get Baize release status",
		"Reads current and latest component versions, update availability, and bounded release notes. Images, digests, internal commits, upgrade commands, and source URLs are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, _ systemReleaseInput) (*mcp.CallToolResult, any, error) {
		result, err := client.GetSystemRelease(ctx, baize.SystemReleaseOptions{})
		return toolOutput[any](result, err, "read")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_subscription_get",
		"Get Baize subscription status",
		"Reads the account-visible subscription plan, license state, feature modes, limits, usage counters, and telemetry policy. Installation identity, license material, recovery targets, and upgrade URLs are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, _ subscriptionInput) (*mcp.CallToolResult, any, error) {
		result, err := client.GetSubscription(ctx, baize.SubscriptionOptions{})
		return toolOutput[any](result, err, "read")
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
		"Directly execute a remote task",
		"Creates an auditable task using a template or server-allowlisted command. Baize enforces permissions, agent scope, safety, risk confirmation, quotas, dispatch and audit.",
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
		"Returns bounded task and target progress. Commands, environment values, output and operator identity are excluded.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskGetInput) (*mcp.CallToolResult, baize.TaskSummary, error) {
		task, err := client.GetExecTask(ctx, input.ID)
		return toolOutput(task, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_exec_task_dispatch",
		"Dispatch a remote task",
		"Dispatches an existing pending task without changing its command or targets. Baize checks permission and task state.",
		true,
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskDispatchInput) (*mcp.CallToolResult, baize.TaskSummary, error) {
		task, err := client.DispatchExecTask(ctx, input.ID)
		return toolOutput(task, err, "write")
	})

	mcp.AddTool(server, readOnlyTool(
		"baize_exec_task_output_get",
		"Read Baize execution output",
		"Reads bounded output only after the user explicitly asks for task output. The result identifies truncation and conservative pattern redaction; it never returns task commands, environment values, credentials, or a complete unbounded terminal stream.",
	), func(ctx context.Context, _ *mcp.CallToolRequest, input execTaskOutputInput) (*mcp.CallToolResult, baize.ExecTaskOutputSummary, error) {
		output, err := client.GetExecTaskOutput(ctx, baize.ExecTaskOutputOptions{
			TaskID: strings.TrimSpace(input.TaskID), TargetID: strings.TrimSpace(input.TargetID), Limit: input.Limit, Mode: strings.TrimSpace(input.Mode),
			AfterSeq: input.AfterSeq, BeforeSeq: input.BeforeSeq, TargetLimit: input.TargetLimit, TargetOffset: input.TargetOffset,
		})
		return toolOutput(output, err, "read")
	})

	mcp.AddTool(server, writeTool(
		"baize_exec_task_cancel",
		"Cancel a Baize execution task",
		"Requests cancellation of a pending or running task. Baize records the action and checks permission and task state.",
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
		base := ""
		switch apiErr.StatusCode {
		case 401:
			base = "the saved Baize session is no longer valid"
		case 403:
			base = fmt.Sprintf("Baize denied this %s request", action)
		case 404:
			base = "the requested Baize resource was not found"
		case 409:
			base = "Baize could not complete this request because the current task state requires confirmation or approval"
		case 429:
			base = "Baize temporarily limited this request"
		default:
			base = "Baize could not complete this request"
		}
		// 只回传服务端契约中的稳定标识，帮助 AI 选择下一步；不回传原始错误、参数或 traceId。
		parts := make([]string, 0, 4)
		if apiErr.Reason != "" {
			parts = append(parts, "reason="+apiErr.Reason)
		}
		if apiErr.MessageKey != "" {
			parts = append(parts, "messageKey="+apiErr.MessageKey)
		}
		if apiErr.NextActionKey != "" {
			parts = append(parts, "nextActionKey="+apiErr.NextActionKey)
		}
		if apiErr.Retryable != nil {
			parts = append(parts, fmt.Sprintf("retryable=%t", *apiErr.Retryable))
		}
		if len(parts) > 0 {
			return fmt.Errorf("%s (%s)", base, strings.Join(parts, ", "))
		}
		return errors.New(base)
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
