package baize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const maxResponseBytes = 2 << 20

const (
	maxCommandTemplatePageSize = 50
	maxCommandTargets          = 50
	maxParameterEntries        = 64
	maxParameterKeyLength      = 100
	maxParameterValueLength    = 4096
	maxPreviewLength           = 4096
	maxReasonLength            = 500
	maxTemplateFieldLength     = 200
	maxTemplateDescription     = 500
	maxTemplateParameters      = 32
	maxTemplateEnumValues      = 50
	maxTemplateCapabilities    = 32
	maxPrecheckItems           = 50
	maxTaskTargets             = 50
	maxApprovalPageSize        = 50
	maxApprovalItems           = 50
	maxAgentPageItems          = 100
	maxApprovalPolicies        = 10
)

var agentIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

var agentSortFields = map[string]struct{}{
	"created_at": {}, "createdAt": {}, "updated_at": {}, "updatedAt": {},
	"last_heartbeat_at": {}, "lastHeartbeatAt": {}, "registered_at": {}, "registeredAt": {},
	"hostname": {}, "alias": {}, "status": {}, "agent_version": {}, "agentVersion": {},
	"os_type": {}, "osType": {},
}

var commandRiskLevels = map[string]struct{}{
	"read_only": {}, "low": {}, "medium": {}, "high": {}, "critical": {},
}

var commandApprovalStatuses = map[string]struct{}{
	"pending": {}, "approved": {}, "rejected": {}, "expired": {},
}

type Client struct {
	baseURL   *url.URL
	http      *http.Client
	token     string
	userAgent string
}

type AgentSummary struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"displayName"`
	Status          string     `json:"status"`
	OperatingSystem string     `json:"operatingSystem"`
	Architecture    string     `json:"architecture"`
	AgentVersion    string     `json:"agentVersion,omitempty"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
}

type AgentPage struct {
	Items          []AgentSummary `json:"items"`
	Total          int            `json:"total"`
	Page           int            `json:"page"`
	PageSize       int            `json:"pageSize"`
	HasMore        bool           `json:"hasMore"`
	NextPage       int            `json:"nextPage,omitempty"`
	ItemsTruncated bool           `json:"itemsTruncated,omitempty"`
}

type AgentListOptions struct {
	Page         int
	PageSize     int
	Search       string
	Alias        string
	System       string
	Region       string
	AgentVersion string
	Architecture string
	Status       string
	GroupID      string
	SortBy       string
	SortOrder    string
}

// CommandTemplateParameter 描述模板允许接收的参数类型，不携带参数默认值或命令内容。
type CommandTemplateParameter struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Label       string   `json:"label,omitempty"`
	Required    bool     `json:"required"`
	EnumValues  []string `json:"enumValues,omitempty"`
	Min         *int     `json:"min,omitempty"`
	Max         *int     `json:"max,omitempty"`
	MinLength   *int     `json:"minLength,omitempty"`
	MaxLength   *int     `json:"maxLength,omitempty"`
	AllowSpaces bool     `json:"allowSpaces,omitempty"`
	Secret      bool     `json:"secret,omitempty"`
	Description string   `json:"description,omitempty"`
}

// CommandTemplateSummary 是 AI 可发现的命令模板摘要，刻意不返回命令正文和工作目录。
type CommandTemplateSummary struct {
	ID                   string                     `json:"id"`
	Name                 string                     `json:"name"`
	Description          string                     `json:"description,omitempty"`
	Category             string                     `json:"category,omitempty"`
	TimeoutSec           int                        `json:"timeoutSec"`
	Status               string                     `json:"status"`
	RiskLevel            string                     `json:"riskLevel"`
	RenderMode           string                     `json:"renderMode"`
	Version              int                        `json:"version"`
	Platform             string                     `json:"platform,omitempty"`
	Parameters           []CommandTemplateParameter `json:"parameters,omitempty"`
	ParametersTruncated  bool                       `json:"parametersTruncated,omitempty"`
	RequiredCapabilities []string                   `json:"requiredCapabilities,omitempty"`
}

type CommandTemplatePage struct {
	Items          []CommandTemplateSummary `json:"items"`
	Total          int                      `json:"total"`
	Page           int                      `json:"page"`
	PageSize       int                      `json:"pageSize"`
	HasMore        bool                     `json:"hasMore"`
	NextPage       int                      `json:"nextPage,omitempty"`
	ItemsTruncated bool                     `json:"itemsTruncated,omitempty"`
}

type CommandTemplateListOptions struct {
	Page      int
	PageSize  int
	Search    string
	Category  string
	RiskLevel string
	Platform  string
}

type PrecheckItem struct {
	Code     string `json:"code"`
	Level    string `json:"level"`
	Message  string `json:"message"`
	AgentID  string `json:"agentId,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}

type CommandTemplateRenderOptions struct {
	TemplateID  string
	AgentIDs    []string
	Parameters  map[string]any
	DiagnosisID string
}

type CommandTemplateRenderResult struct {
	TemplateID        string         `json:"templateId"`
	TemplateName      string         `json:"templateName"`
	TemplateVersion   int            `json:"templateVersion"`
	RenderMode        string         `json:"renderMode"`
	RiskLevel         string         `json:"riskLevel"`
	RenderedPreview   string         `json:"renderedPreview,omitempty"`
	PreviewTruncated  bool           `json:"previewTruncated,omitempty"`
	CommandHash       string         `json:"commandHash,omitempty"`
	PrecheckPassed    bool           `json:"precheckPassed"`
	PrecheckTruncated bool           `json:"precheckTruncated,omitempty"`
	MissingParameters []string       `json:"missingParameters,omitempty"`
	BlockedReasons    []PrecheckItem `json:"blockedReasons,omitempty"`
	Warnings          []PrecheckItem `json:"warnings,omitempty"`
	DryRun            bool           `json:"dryRun"`
}

type CommandPlanCreateOptions struct {
	TemplateID     string
	Title          string
	TargetAgentIDs []string
	Parameters     map[string]any
	DiagnosisID    string
}

type PlanSummary struct {
	ID                string          `json:"id"`
	TemplateID        string          `json:"templateId"`
	TemplateName      string          `json:"templateName"`
	TemplateVersion   int             `json:"templateVersion"`
	Title             string          `json:"title"`
	RiskLevel         string          `json:"riskLevel"`
	RenderMode        string          `json:"renderMode"`
	CommandHash       string          `json:"commandHash,omitempty"`
	TimeoutSec        int             `json:"timeoutSec"`
	TargetAgentIDs    []string        `json:"targetAgentIds"`
	Precheck          PrecheckSummary `json:"precheck"`
	PrecheckTruncated bool            `json:"precheckTruncated,omitempty"`
	Warnings          []PrecheckItem  `json:"warnings,omitempty"`
	ApprovalRequired  bool            `json:"approvalRequired"`
	ApprovalReason    string          `json:"approvalReason,omitempty"`
	Status            string          `json:"status"`
	DiagnosisID       string          `json:"diagnosisId,omitempty"`
	CreatedTaskID     string          `json:"createdTaskId,omitempty"`
	CreatedAt         *time.Time      `json:"createdAt,omitempty"`
	UpdatedAt         *time.Time      `json:"updatedAt,omitempty"`
	CancelledAt       *time.Time      `json:"cancelledAt,omitempty"`
	ExecutedAt        *time.Time      `json:"executedAt,omitempty"`
}

type PrecheckSummary struct {
	PrecheckPassed    bool           `json:"precheckPassed"`
	MissingParameters []string       `json:"missingParameters,omitempty"`
	BlockedReasons    []PrecheckItem `json:"blockedReasons,omitempty"`
}

type CommandPlanExecuteOptions struct {
	AutoDispatch   *bool
	ConfirmRisk    bool
	ConfirmMessage string
	DebugSessionID string
}

// CommandPlanApprovalCreateOptions 是命令计划审批申请参数。
// 审批申请只记录后端审批单，不会执行命令计划。
type CommandPlanApprovalCreateOptions struct {
	PlanID    string
	Reason    string
	ExpiresAt *time.Time
}

// CommandPlanApprovalListOptions 是审批单列表的筛选参数。
type CommandPlanApprovalListOptions struct {
	Page        int
	PageSize    int
	PlanID      string
	Status      string
	RiskLevel   string
	RequesterID string
	ApproverID  string
	Search      string
}

// CommandPlanApprovalDecisionOptions 是审批人的决策参数。
// Approved=true 只表示向后端提交通过意见，是否有权限由白泽后端最终判断。
type CommandPlanApprovalDecisionOptions struct {
	Approved        bool
	DecisionMessage string
}

type PlanExecutionSummary struct {
	Plan PlanSummary `json:"plan"`
	Task TaskSummary `json:"task"`
}

// ApprovalPlanSnapshot 是审批时可供 AI 复核的脱敏计划快照。
// 参数快照、策略快照、命令正文和工作目录不会进入该结构。
type ApprovalPlanSnapshot struct {
	TemplateID      string          `json:"templateId,omitempty"`
	TemplateName    string          `json:"templateName,omitempty"`
	TemplateVersion int             `json:"templateVersion,omitempty"`
	Title           string          `json:"title,omitempty"`
	RiskLevel       string          `json:"riskLevel,omitempty"`
	CommandHash     string          `json:"commandHash,omitempty"`
	TimeoutSec      int             `json:"timeoutSec,omitempty"`
	TargetAgentIDs  []string        `json:"targetAgentIds,omitempty"`
	Precheck        PrecheckSummary `json:"precheck,omitempty"`
	Warnings        []PrecheckItem  `json:"warnings,omitempty"`
	Truncated       bool            `json:"truncated,omitempty"`
}

// CommandPlanApproval 是经过字段白名单处理的审批单摘要。
type CommandPlanApproval struct {
	ID                string                `json:"id"`
	PlanID            string                `json:"planId"`
	RiskLevel         string                `json:"riskLevel"`
	Status            string                `json:"status"`
	Reason            string                `json:"reason,omitempty"`
	DecisionMessage   string                `json:"decisionMessage,omitempty"`
	ExpiresAt         *time.Time            `json:"expiresAt,omitempty"`
	DecidedAt         *time.Time            `json:"decidedAt,omitempty"`
	CreatedAt         *time.Time            `json:"createdAt,omitempty"`
	UpdatedAt         *time.Time            `json:"updatedAt,omitempty"`
	PlanSnapshot      *ApprovalPlanSnapshot `json:"planSnapshot,omitempty"`
	SnapshotTruncated bool                  `json:"snapshotTruncated,omitempty"`
}

type CommandPlanApprovalPage struct {
	Items    []CommandPlanApproval `json:"items"`
	Total    int                   `json:"total"`
	Page     int                   `json:"page"`
	PageSize int                   `json:"pageSize"`
	HasMore  bool                  `json:"hasMore"`
	NextPage int                   `json:"nextPage,omitempty"`
}

// CommandPlanApprovalPolicySummary 是审批策略的最小公开摘要。
// 不向 MCP 暴露权限码、通知渠道或其它策略内部细节。
type CommandPlanApprovalPolicySummary struct {
	RiskLevel         string `json:"riskLevel"`
	Enabled           bool   `json:"enabled"`
	AllowSelfApproval bool   `json:"allowSelfApproval"`
}

type TaskTargetSummary struct {
	ID         string     `json:"id"`
	AgentID    string     `json:"agentId"`
	Status     string     `json:"status"`
	ExitCode   *int       `json:"exitCode,omitempty"`
	OutputSize int        `json:"outputSize"`
	StartedAt  *time.Time `json:"startedAt,omitempty"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`
}

type TaskSummary struct {
	ID               string              `json:"id"`
	TaskType         string              `json:"taskType"`
	Title            string              `json:"title"`
	TimeoutSec       int                 `json:"timeoutSec"`
	Status           string              `json:"status"`
	CreatedAt        *time.Time          `json:"createdAt,omitempty"`
	StartedAt        *time.Time          `json:"startedAt,omitempty"`
	FinishedAt       *time.Time          `json:"finishedAt,omitempty"`
	CancelledAt      *time.Time          `json:"cancelledAt,omitempty"`
	Targets          []TaskTargetSummary `json:"targets"`
	TargetsTruncated bool                `json:"targetsTruncated,omitempty"`
}

type commandTemplateRecord struct {
	CommandTemplateSummary
	Parameters           []commandTemplateParameterRecord `json:"parameters"`
	RequiredCapabilities []string                         `json:"requiredCapabilities"`
}

type commandTemplateParameterRecord struct {
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	Label       string   `json:"label"`
	Required    bool     `json:"required"`
	EnumValues  []string `json:"enumValues"`
	Min         *int     `json:"min"`
	Max         *int     `json:"max"`
	MinLength   *int     `json:"minLength"`
	MaxLength   *int     `json:"maxLength"`
	AllowSpaces bool     `json:"allowSpaces"`
	Secret      bool     `json:"secret"`
	Description string   `json:"description"`
}

type commandTemplateRenderRecord struct {
	TemplateID        string         `json:"templateId"`
	TemplateName      string         `json:"templateName"`
	TemplateVersion   int            `json:"templateVersion"`
	RenderMode        string         `json:"renderMode"`
	RiskLevel         string         `json:"riskLevel"`
	RenderedPreview   string         `json:"renderedPreview"`
	CommandHash       string         `json:"commandHash"`
	PrecheckPassed    bool           `json:"precheckPassed"`
	MissingParameters []string       `json:"missingParameters"`
	BlockedReasons    []PrecheckItem `json:"blockedReasons"`
	Warnings          []PrecheckItem `json:"warnings"`
	DryRun            bool           `json:"dryRun"`
}

type commandPlanRecord struct {
	ID               string          `json:"id"`
	TemplateID       string          `json:"templateId"`
	TemplateName     string          `json:"templateName"`
	TemplateVersion  int             `json:"templateVersion"`
	Title            string          `json:"title"`
	RiskLevel        string          `json:"riskLevel"`
	RenderMode       string          `json:"renderMode"`
	RenderedPreview  string          `json:"renderedPreview"`
	CommandHash      string          `json:"commandHash"`
	TimeoutSec       int             `json:"timeoutSec"`
	TargetAgentIDs   []string        `json:"targetAgentIds"`
	Precheck         json.RawMessage `json:"precheck"`
	Warnings         []PrecheckItem  `json:"warnings"`
	ApprovalRequired bool            `json:"approvalRequired"`
	ApprovalReason   string          `json:"approvalReason"`
	Status           string          `json:"status"`
	DiagnosisID      string          `json:"diagnosisId"`
	CreatedTaskID    string          `json:"createdTaskId"`
	CreatedAt        *time.Time      `json:"createdAt"`
	UpdatedAt        *time.Time      `json:"updatedAt"`
	CancelledAt      *time.Time      `json:"cancelledAt"`
	ExecutedAt       *time.Time      `json:"executedAt"`
}

type commandPlanExecutionRecord struct {
	Plan commandPlanRecord `json:"plan"`
	Task execTaskRecord    `json:"task"`
}

type execTaskRecord struct {
	ID          string             `json:"id"`
	TaskType    string             `json:"taskType"`
	Title       string             `json:"title"`
	TimeoutSec  int                `json:"timeoutSec"`
	Status      string             `json:"status"`
	CreatedAt   *time.Time         `json:"createdAt"`
	StartedAt   *time.Time         `json:"startedAt"`
	FinishedAt  *time.Time         `json:"finishedAt"`
	CancelledAt *time.Time         `json:"cancelledAt"`
	Targets     []execTargetRecord `json:"targets"`
}

type execTargetRecord struct {
	ID         string     `json:"id"`
	AgentID    string     `json:"agentId"`
	Status     string     `json:"status"`
	ExitCode   *int       `json:"exitCode"`
	OutputSize int        `json:"outputSize"`
	StartedAt  *time.Time `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt"`
}

type commandPlanApprovalRecord struct {
	ID              string          `json:"id"`
	PlanID          string          `json:"planId"`
	RiskLevel       string          `json:"riskLevel"`
	Status          string          `json:"status"`
	Reason          string          `json:"reason"`
	DecisionMessage string          `json:"decisionMessage"`
	ExpiresAt       *time.Time      `json:"expiresAt"`
	DecidedAt       *time.Time      `json:"decidedAt"`
	CreatedAt       *time.Time      `json:"createdAt"`
	UpdatedAt       *time.Time      `json:"updatedAt"`
	PlanSnapshot    json.RawMessage `json:"planSnapshot"`
}

type commandPlanApprovalPolicyRecord struct {
	RiskLevel         string `json:"riskLevel"`
	Enabled           bool   `json:"enabled"`
	AllowSelfApproval bool   `json:"allowSelfApproval"`
}

type commandPlanApprovalSnapshotRecord struct {
	TemplateID      string          `json:"templateId"`
	TemplateName    string          `json:"templateName"`
	TemplateVersion int             `json:"templateVersion"`
	Title           string          `json:"title"`
	RiskLevel       string          `json:"riskLevel"`
	CommandHash     string          `json:"commandHash"`
	TimeoutSec      int             `json:"timeoutSec"`
	TargetAgentIDs  []string        `json:"targetAgentIds"`
	Precheck        json.RawMessage `json:"precheck"`
	Warnings        []PrecheckItem  `json:"warnings"`
}

type agentRecord struct {
	ID              string     `json:"id"`
	Hostname        string     `json:"hostname"`
	Alias           string     `json:"alias"`
	Status          string     `json:"status"`
	OSType          string     `json:"osType"`
	OSVersion       string     `json:"osVersion"`
	Arch            string     `json:"arch"`
	AgentVersion    string     `json:"agentVersion"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt"`
}

type APIError struct {
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Baize returned HTTP %d", e.StatusCode)
}

type InputError struct {
	message string
}

func (e *InputError) Error() string {
	return e.message
}

func newInputError(message string) error {
	return &InputError{message: message}
}

func NewClient(apiURL, token string, allowHTTP bool, userAgent string) (*Client, error) {
	normalized, err := ValidateAPIURL(apiURL, allowHTTP)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, errors.New("invalid Baize API URL")
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "baize-mcp/dev"
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
				return errors.New("cross-origin redirect blocked")
			}
			return nil
		},
	}
	return &Client{baseURL: parsed, http: client, token: token, userAgent: userAgent}, nil
}

func ValidateAPIURL(raw string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("API URL must be an absolute http or https URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("API URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("API URL must not contain credentials, query parameters, or fragments")
	}
	if strings.TrimRight(parsed.EscapedPath(), "/") != "/api/v1" {
		return "", errors.New("API URL must end with /api/v1")
	}
	if parsed.Scheme == "http" && !allowHTTP && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("non-loopback HTTP requires explicit --allow-http confirmation")
	}
	parsed.Path = "/api/v1"
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	payload := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password}
	var data struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, []string{"auth", "login"}, nil, payload, &data, false); err != nil {
		return "", err
	}
	if strings.TrimSpace(data.Token) == "" {
		return "", errors.New("Baize login response did not include a session credential")
	}
	return data.Token, nil
}

func (c *Client) CheckSession(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, []string{"auth", "profile"}, nil, nil, nil, true)
}

func (c *Client) Logout(ctx context.Context) error {
	var data struct {
		Revoked bool `json:"revoked"`
	}
	return c.do(ctx, http.MethodPost, []string{"auth", "logout"}, nil, nil, &data, true)
}

func (c *Client) ListAgents(ctx context.Context, options AgentListOptions) (AgentPage, error) {
	if options.Page < 1 {
		return AgentPage{}, newInputError("page must be at least 1")
	}
	if options.PageSize < 1 || options.PageSize > 100 {
		return AgentPage{}, newInputError("page size must be between 1 and 100")
	}
	filters := []struct {
		name  string
		value string
		limit int
		query string
	}{
		{name: "search", value: options.Search, limit: 200, query: "search"},
		{name: "alias", value: options.Alias, limit: 200, query: "alias"},
		{name: "system", value: options.System, limit: 200, query: "system"},
		{name: "region", value: options.Region, limit: 200, query: "region"},
		{name: "agent version", value: options.AgentVersion, limit: 64, query: "agent_version"},
		{name: "architecture", value: options.Architecture, limit: 64, query: "arch"},
		{name: "status", value: options.Status, limit: 64, query: "status"},
		{name: "sort field", value: options.SortBy, limit: 64, query: "sort_by"},
	}
	query := url.Values{
		"page":      {fmt.Sprintf("%d", options.Page)},
		"page_size": {fmt.Sprintf("%d", options.PageSize)},
	}
	for _, filter := range filters {
		value := strings.TrimSpace(filter.value)
		if len(value) > filter.limit {
			return AgentPage{}, newInputError(fmt.Sprintf("%s must not exceed %d characters", filter.name, filter.limit))
		}
		if value != "" {
			query.Set(filter.query, value)
		}
	}
	groupID := strings.TrimSpace(options.GroupID)
	if groupID != "" {
		if !agentIDPattern.MatchString(groupID) {
			return AgentPage{}, newInputError("group ID must be a UUID")
		}
		query.Set("group_id", strings.ToLower(groupID))
	}
	sortOrder := strings.ToLower(strings.TrimSpace(options.SortOrder))
	sortBy := strings.TrimSpace(options.SortBy)
	if sortBy != "" {
		if _, ok := agentSortFields[sortBy]; !ok {
			return AgentPage{}, newInputError("sort field is not supported")
		}
	}
	if sortOrder != "" {
		if sortOrder != "asc" && sortOrder != "desc" {
			return AgentPage{}, newInputError("sort order must be asc or desc")
		}
		query.Set("sort_order", sortOrder)
	}
	var data struct {
		Items    []agentRecord `json:"items"`
		Total    int           `json:"total"`
		Page     int           `json:"page"`
		PageSize int           `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"agents"}, query, nil, &data, true); err != nil {
		return AgentPage{}, err
	}
	items := make([]AgentSummary, 0, minInt(len(data.Items), maxAgentPageItems))
	itemsTruncated := false
	for index, item := range data.Items {
		if index >= maxAgentPageItems {
			itemsTruncated = true
			break
		}
		items = append(items, summarizeAgent(item))
	}
	page := data.Page
	if page < 1 {
		page = options.Page
	}
	pageSize := data.PageSize
	if pageSize < 1 {
		pageSize = options.PageSize
	}
	hasMore := itemsTruncated || page*pageSize < data.Total
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return AgentPage{Items: items, Total: data.Total, Page: page, PageSize: pageSize, HasMore: hasMore, NextPage: nextPage, ItemsTruncated: itemsTruncated}, nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (AgentSummary, error) {
	id = strings.TrimSpace(id)
	if !agentIDPattern.MatchString(id) {
		return AgentSummary{}, newInputError("agent ID must be a UUID")
	}
	var data agentRecord
	if err := c.do(ctx, http.MethodGet, []string{"agents", strings.ToLower(id)}, nil, nil, &data, true); err != nil {
		return AgentSummary{}, err
	}
	return summarizeAgent(data), nil
}

func (c *Client) ListCommandTemplates(ctx context.Context, options CommandTemplateListOptions) (CommandTemplatePage, error) {
	if options.Page < 1 {
		return CommandTemplatePage{}, newInputError("page must be at least 1")
	}
	if options.PageSize < 1 || options.PageSize > maxCommandTemplatePageSize {
		return CommandTemplatePage{}, newInputError(fmt.Sprintf("page size must be between 1 and %d", maxCommandTemplatePageSize))
	}
	query := url.Values{
		"page":      {fmt.Sprintf("%d", options.Page)},
		"page_size": {fmt.Sprintf("%d", options.PageSize)},
		"status":    {"enabled"},
	}
	riskLevel := strings.ToLower(strings.TrimSpace(options.RiskLevel))
	if riskLevel != "" {
		if _, ok := commandRiskLevels[riskLevel]; !ok {
			return CommandTemplatePage{}, newInputError("risk level must be read_only, low, medium, high, or critical")
		}
		query.Set("risk_level", riskLevel)
	}
	for name, value := range map[string]string{
		"search": options.Search, "category": options.Category, "platform": options.Platform,
	} {
		value = strings.TrimSpace(value)
		if len(value) > 200 {
			return CommandTemplatePage{}, newInputError(fmt.Sprintf("%s must not exceed 200 characters", name))
		}
		if value != "" {
			query.Set(name, value)
		}
	}
	var data struct {
		Items    []commandTemplateRecord `json:"items"`
		Total    int                     `json:"total"`
		Page     int                     `json:"page"`
		PageSize int                     `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"ops", "command-templates"}, query, nil, &data, true); err != nil {
		return CommandTemplatePage{}, err
	}
	items := make([]CommandTemplateSummary, 0, minInt(len(data.Items), maxCommandTemplatePageSize))
	itemsTruncated := false
	for index, item := range data.Items {
		if index >= maxCommandTemplatePageSize {
			itemsTruncated = true
			break
		}
		items = append(items, summarizeCommandTemplate(item))
	}
	page := data.Page
	if page < 1 {
		page = options.Page
	}
	pageSize := data.PageSize
	if pageSize < 1 {
		pageSize = options.PageSize
	}
	hasMore := itemsTruncated || page*pageSize < data.Total
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return CommandTemplatePage{Items: items, Total: data.Total, Page: page, PageSize: pageSize, HasMore: hasMore, NextPage: nextPage, ItemsTruncated: itemsTruncated}, nil
}

func (c *Client) PreviewCommandTemplate(ctx context.Context, options CommandTemplateRenderOptions) (CommandTemplateRenderResult, error) {
	templateID, err := validateUUID(options.TemplateID, "template ID")
	if err != nil {
		return CommandTemplateRenderResult{}, err
	}
	agentIDs, err := validateUUIDList(options.AgentIDs, maxCommandTargets, "agent IDs")
	if err != nil {
		return CommandTemplateRenderResult{}, err
	}
	parameters, err := validateParameters(options.Parameters)
	if err != nil {
		return CommandTemplateRenderResult{}, err
	}
	payload := map[string]any{"agentIds": agentIDs, "parameters": parameters, "dryRun": true}
	if diagnosisID := strings.TrimSpace(options.DiagnosisID); diagnosisID != "" {
		if _, err := validateUUID(diagnosisID, "diagnosis ID"); err != nil {
			return CommandTemplateRenderResult{}, err
		}
		payload["diagnosisId"] = strings.ToLower(diagnosisID)
	}
	var data commandTemplateRenderRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-templates", templateID, "render"}, nil, payload, &data, true); err != nil {
		return CommandTemplateRenderResult{}, err
	}
	return summarizeCommandTemplateRender(data), nil
}

func (c *Client) CreateCommandPlan(ctx context.Context, options CommandPlanCreateOptions) (PlanSummary, error) {
	templateID, err := validateUUID(options.TemplateID, "template ID")
	if err != nil {
		return PlanSummary{}, err
	}
	agentIDs, err := validateUUIDList(options.TargetAgentIDs, maxCommandTargets, "target agent IDs")
	if err != nil {
		return PlanSummary{}, err
	}
	parameters, err := validateParameters(options.Parameters)
	if err != nil {
		return PlanSummary{}, err
	}
	title := strings.TrimSpace(options.Title)
	if len(title) > 255 {
		return PlanSummary{}, newInputError("title must not exceed 255 characters")
	}
	payload := map[string]any{
		"templateId":     templateID,
		"title":          title,
		"targetAgentIds": agentIDs,
		"parameters":     parameters,
	}
	if diagnosisID := strings.TrimSpace(options.DiagnosisID); diagnosisID != "" {
		if _, err := validateUUID(diagnosisID, "diagnosis ID"); err != nil {
			return PlanSummary{}, err
		}
		payload["diagnosisId"] = strings.ToLower(diagnosisID)
	}
	var data commandPlanRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-plans"}, nil, payload, &data, true); err != nil {
		return PlanSummary{}, err
	}
	return summarizeCommandPlan(data), nil
}

func (c *Client) GetCommandPlan(ctx context.Context, id string) (PlanSummary, error) {
	planID, err := validateUUID(id, "command plan ID")
	if err != nil {
		return PlanSummary{}, err
	}
	var data commandPlanRecord
	if err := c.do(ctx, http.MethodGet, []string{"ops", "command-plans", planID}, nil, nil, &data, true); err != nil {
		return PlanSummary{}, err
	}
	return summarizeCommandPlan(data), nil
}

// CancelCommandPlan 取消尚未转换为执行任务的命令计划，并返回更新后的计划摘要。
func (c *Client) CancelCommandPlan(ctx context.Context, id, reason string) (PlanSummary, error) {
	planID, err := validateUUID(id, "command plan ID")
	if err != nil {
		return PlanSummary{}, err
	}
	reason = strings.TrimSpace(reason)
	if len(reason) > maxReasonLength {
		return PlanSummary{}, newInputError(fmt.Sprintf("cancel reason must not exceed %d characters", maxReasonLength))
	}
	payload := map[string]any{}
	if reason != "" {
		payload["reason"] = reason
	}
	var data commandPlanRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-plans", planID, "cancel"}, nil, payload, &data, true); err != nil {
		return PlanSummary{}, err
	}
	return summarizeCommandPlan(data), nil
}

// RequestCommandPlanApproval 为 ready 计划创建服务端审批申请。
func (c *Client) RequestCommandPlanApproval(ctx context.Context, options CommandPlanApprovalCreateOptions) (CommandPlanApproval, error) {
	planID, err := validateUUID(options.PlanID, "command plan ID")
	if err != nil {
		return CommandPlanApproval{}, err
	}
	reason := strings.TrimSpace(options.Reason)
	if reason == "" {
		return CommandPlanApproval{}, newInputError("approval reason is required")
	}
	if len(reason) > maxReasonLength {
		return CommandPlanApproval{}, newInputError(fmt.Sprintf("approval reason must not exceed %d characters", maxReasonLength))
	}
	payload := map[string]any{"reason": reason}
	if options.ExpiresAt != nil {
		if options.ExpiresAt.IsZero() || !options.ExpiresAt.After(time.Now()) {
			return CommandPlanApproval{}, newInputError("approval expiry must be in the future")
		}
		payload["expiresAt"] = options.ExpiresAt.UTC().Format(time.RFC3339)
	}
	var data commandPlanApprovalRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-plans", planID, "approvals"}, nil, payload, &data, true); err != nil {
		return CommandPlanApproval{}, err
	}
	return summarizeCommandPlanApproval(data), nil
}

// ListCommandPlanApprovals 分页查询当前登录账号服务端授权范围内的审批单。
func (c *Client) ListCommandPlanApprovals(ctx context.Context, options CommandPlanApprovalListOptions) (CommandPlanApprovalPage, error) {
	if options.Page < 1 {
		return CommandPlanApprovalPage{}, newInputError("page must be at least 1")
	}
	if options.PageSize < 1 || options.PageSize > maxApprovalPageSize {
		return CommandPlanApprovalPage{}, newInputError(fmt.Sprintf("page size must be between 1 and %d", maxApprovalPageSize))
	}
	query := url.Values{
		"page":      {fmt.Sprintf("%d", options.Page)},
		"page_size": {fmt.Sprintf("%d", options.PageSize)},
	}
	if planID := strings.TrimSpace(options.PlanID); planID != "" {
		validated, err := validateUUID(planID, "command plan ID")
		if err != nil {
			return CommandPlanApprovalPage{}, err
		}
		query.Set("planId", validated)
	}
	status := strings.ToLower(strings.TrimSpace(options.Status))
	if status != "" {
		if _, ok := commandApprovalStatuses[status]; !ok {
			return CommandPlanApprovalPage{}, newInputError("approval status must be pending, approved, rejected, or expired")
		}
		query.Set("status", status)
	}
	riskLevel := strings.ToLower(strings.TrimSpace(options.RiskLevel))
	if riskLevel != "" {
		if _, ok := commandRiskLevels[riskLevel]; !ok {
			return CommandPlanApprovalPage{}, newInputError("risk level must be read_only, low, medium, high, or critical")
		}
		query.Set("riskLevel", riskLevel)
	}
	for name, value := range map[string]string{
		"requesterId": options.RequesterID,
		"approverId":  options.ApproverID,
		"search":      options.Search,
	} {
		value = strings.TrimSpace(value)
		if len(value) > 200 {
			return CommandPlanApprovalPage{}, newInputError(fmt.Sprintf("%s must not exceed 200 characters", name))
		}
		if value != "" {
			query.Set(name, value)
		}
	}
	var data struct {
		Items    []commandPlanApprovalRecord `json:"items"`
		Total    int                         `json:"total"`
		Page     int                         `json:"page"`
		PageSize int                         `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"ops", "command-plan-approvals"}, query, nil, &data, true); err != nil {
		return CommandPlanApprovalPage{}, err
	}
	items := make([]CommandPlanApproval, 0, minInt(len(data.Items), maxApprovalItems))
	itemsTruncated := false
	for index, item := range data.Items {
		if index >= maxApprovalItems {
			itemsTruncated = true
			break
		}
		items = append(items, summarizeCommandPlanApproval(item))
	}
	page := data.Page
	if page < 1 {
		page = options.Page
	}
	pageSize := data.PageSize
	if pageSize < 1 {
		pageSize = options.PageSize
	}
	// The server total remains authoritative; the local item bound protects the AI response if a proxy ignores page_size.
	hasMore := itemsTruncated || page*pageSize < data.Total
	nextPage := 0
	if hasMore {
		nextPage = page + 1
	}
	return CommandPlanApprovalPage{Items: items, Total: data.Total, Page: page, PageSize: pageSize, HasMore: hasMore, NextPage: nextPage}, nil
}

// GetCommandPlanApproval 返回一条经过脱敏的审批单。
func (c *Client) GetCommandPlanApproval(ctx context.Context, id string) (CommandPlanApproval, error) {
	approvalID, err := validateUUID(id, "command plan approval ID")
	if err != nil {
		return CommandPlanApproval{}, err
	}
	var data commandPlanApprovalRecord
	if err := c.do(ctx, http.MethodGet, []string{"ops", "command-plan-approvals", approvalID}, nil, nil, &data, true); err != nil {
		return CommandPlanApproval{}, err
	}
	return summarizeCommandPlanApproval(data), nil
}

// DecideCommandPlanApproval 向白泽提交审批决策。
// 当前登录账号是否可以审批由后端最终判断。
func (c *Client) DecideCommandPlanApproval(ctx context.Context, id string, options CommandPlanApprovalDecisionOptions) (CommandPlanApproval, error) {
	approvalID, err := validateUUID(id, "command plan approval ID")
	if err != nil {
		return CommandPlanApproval{}, err
	}
	message := strings.TrimSpace(options.DecisionMessage)
	if len(message) > maxReasonLength {
		return CommandPlanApproval{}, newInputError(fmt.Sprintf("decision message must not exceed %d characters", maxReasonLength))
	}
	if !options.Approved && message == "" {
		return CommandPlanApproval{}, newInputError("a rejection decision message is required")
	}
	payload := map[string]any{"approved": options.Approved}
	if message != "" {
		payload["decisionMessage"] = message
	}
	var data commandPlanApprovalRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-plan-approvals", approvalID, "decision"}, nil, payload, &data, true); err != nil {
		return CommandPlanApproval{}, err
	}
	return summarizeCommandPlanApproval(data), nil
}

// ListCommandPlanApprovalPolicies 返回当前账号可读取的审批策略最小摘要。
func (c *Client) ListCommandPlanApprovalPolicies(ctx context.Context) ([]CommandPlanApprovalPolicySummary, error) {
	var data struct {
		Items []commandPlanApprovalPolicyRecord `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"ops", "command-plan-approval-policies"}, nil, nil, &data, true); err != nil {
		return nil, err
	}
	items := make([]CommandPlanApprovalPolicySummary, 0, minInt(len(data.Items), maxApprovalPolicies))
	for index, item := range data.Items {
		if index >= maxApprovalPolicies {
			break
		}
		items = append(items, CommandPlanApprovalPolicySummary{
			RiskLevel:         trimPublicText(item.RiskLevel, maxTemplateFieldLength),
			Enabled:           item.Enabled,
			AllowSelfApproval: item.AllowSelfApproval,
		})
	}
	return items, nil
}

func (c *Client) ExecuteCommandPlan(ctx context.Context, id string, options CommandPlanExecuteOptions) (PlanExecutionSummary, error) {
	planID, err := validateUUID(id, "command plan ID")
	if err != nil {
		return PlanExecutionSummary{}, err
	}
	payload := map[string]any{"confirmRisk": options.ConfirmRisk}
	if options.AutoDispatch != nil {
		payload["autoDispatch"] = *options.AutoDispatch
	}
	if message := strings.TrimSpace(options.ConfirmMessage); message != "" {
		if len(message) > maxReasonLength {
			return PlanExecutionSummary{}, newInputError(fmt.Sprintf("confirm message must not exceed %d characters", maxReasonLength))
		}
		payload["confirmMessage"] = message
	}
	if debugSessionID := strings.TrimSpace(options.DebugSessionID); debugSessionID != "" {
		if _, err := validateUUID(debugSessionID, "debug session ID"); err != nil {
			return PlanExecutionSummary{}, err
		}
		payload["debugSessionId"] = strings.ToLower(debugSessionID)
	}
	var data commandPlanExecutionRecord
	if err := c.do(ctx, http.MethodPost, []string{"ops", "command-plans", planID, "execute"}, nil, payload, &data, true); err != nil {
		return PlanExecutionSummary{}, err
	}
	return PlanExecutionSummary{Plan: summarizeCommandPlan(data.Plan), Task: summarizeExecTask(data.Task)}, nil
}

func (c *Client) GetExecTask(ctx context.Context, id string) (TaskSummary, error) {
	taskID, err := validateUUID(id, "execution task ID")
	if err != nil {
		return TaskSummary{}, err
	}
	var data execTaskRecord
	if err := c.do(ctx, http.MethodGet, []string{"ops", "tasks", taskID}, nil, nil, &data, true); err != nil {
		return TaskSummary{}, err
	}
	return summarizeExecTask(data), nil
}

func (c *Client) CancelExecTask(ctx context.Context, id string) error {
	taskID, err := validateUUID(id, "execution task ID")
	if err != nil {
		return err
	}
	return c.do(ctx, http.MethodPost, []string{"ops", "tasks", taskID, "cancel"}, nil, nil, nil, true)
}

func validateUUID(value, label string) (string, error) {
	value = strings.TrimSpace(value)
	if !agentIDPattern.MatchString(value) {
		return "", newInputError(label + " must be a UUID")
	}
	return strings.ToLower(value), nil
}

func validateUUIDList(values []string, max int, label string) ([]string, error) {
	if len(values) < 1 {
		return nil, newInputError(label + " are required")
	}
	if len(values) > max {
		return nil, newInputError(fmt.Sprintf("%s cannot exceed %d", label, max))
	}
	result := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized, err := validateUUID(value, strings.TrimSuffix(label, "s")+" ID")
		if err != nil {
			return nil, err
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, newInputError(label + " are required")
	}
	return result, nil
}

func validateParameters(values map[string]any) (map[string]any, error) {
	if len(values) > maxParameterEntries {
		return nil, newInputError(fmt.Sprintf("parameters cannot contain more than %d entries", maxParameterEntries))
	}
	result := make(map[string]any, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > maxParameterKeyLength {
			return nil, newInputError("parameter names must be non-empty and no longer than 100 characters")
		}
		switch normalized := value.(type) {
		case nil, string, bool, float64, int, int64, json.Number:
			if text, ok := normalized.(string); ok && len(text) > maxParameterValueLength {
				return nil, newInputError(fmt.Sprintf("parameter %q exceeds the allowed length", key))
			}
			result[key] = normalized
		default:
			return nil, newInputError(fmt.Sprintf("parameter %q must be a scalar value", key))
		}
	}
	return result, nil
}

func summarizeCommandTemplate(item commandTemplateRecord) CommandTemplateSummary {
	summary := item.CommandTemplateSummary
	summary.Name = trimPublicText(summary.Name, maxTemplateFieldLength)
	summary.Description = trimPublicText(summary.Description, maxTemplateDescription)
	summary.Category = trimPublicText(summary.Category, maxTemplateFieldLength)
	summary.Platform = trimPublicText(summary.Platform, maxTemplateFieldLength)
	parameters := make([]CommandTemplateParameter, 0, minInt(len(item.Parameters), maxTemplateParameters))
	for index, parameter := range item.Parameters {
		if index >= maxTemplateParameters {
			summary.ParametersTruncated = true
			break
		}
		enumValues := make([]string, 0, minInt(len(parameter.EnumValues), maxTemplateEnumValues))
		for enumIndex, enumValue := range parameter.EnumValues {
			if enumIndex >= maxTemplateEnumValues {
				summary.ParametersTruncated = true
				break
			}
			enumValues = append(enumValues, trimPublicText(enumValue, maxTemplateFieldLength))
		}
		parameters = append(parameters, CommandTemplateParameter{
			Name: trimPublicText(parameter.Name, maxTemplateFieldLength), Type: trimPublicText(parameter.Type, maxTemplateFieldLength),
			Label: trimPublicText(parameter.Label, maxTemplateFieldLength), Required: parameter.Required,
			EnumValues: enumValues, Min: parameter.Min, Max: parameter.Max, MinLength: parameter.MinLength,
			MaxLength: parameter.MaxLength, AllowSpaces: parameter.AllowSpaces, Secret: parameter.Secret,
			Description: trimPublicText(parameter.Description, maxTemplateDescription),
		})
	}
	summary.Parameters = parameters
	summary.RequiredCapabilities = make([]string, 0, minInt(len(item.RequiredCapabilities), maxTemplateCapabilities))
	for index, capability := range item.RequiredCapabilities {
		if index >= maxTemplateCapabilities {
			summary.ParametersTruncated = true
			break
		}
		summary.RequiredCapabilities = append(summary.RequiredCapabilities, trimPublicText(capability, maxTemplateFieldLength))
	}
	return summary
}

func summarizeCommandTemplateRender(item commandTemplateRenderRecord) CommandTemplateRenderResult {
	preview, previewTruncated := trimPublicTextWithFlag(item.RenderedPreview, maxPreviewLength)
	blockedReasons, blockedTruncated := trimPrecheckItems(item.BlockedReasons)
	warnings, warningsTruncated := trimPrecheckItems(item.Warnings)
	missingParameters, missingTruncated := trimPublicStringList(item.MissingParameters, maxTemplateFieldLength, maxTemplateParameters)
	return CommandTemplateRenderResult{
		TemplateID: item.TemplateID, TemplateName: trimPublicText(item.TemplateName, maxTemplateFieldLength), TemplateVersion: item.TemplateVersion,
		RenderMode: trimPublicText(item.RenderMode, maxTemplateFieldLength), RiskLevel: trimPublicText(item.RiskLevel, maxTemplateFieldLength), RenderedPreview: preview, PreviewTruncated: previewTruncated,
		CommandHash: trimPublicText(item.CommandHash, maxTemplateFieldLength), PrecheckPassed: item.PrecheckPassed, MissingParameters: missingParameters,
		PrecheckTruncated: blockedTruncated || warningsTruncated || missingTruncated, BlockedReasons: blockedReasons, Warnings: warnings, DryRun: item.DryRun,
	}
}

func summarizeCommandPlan(item commandPlanRecord) PlanSummary {
	var precheck PrecheckSummary
	if len(item.Precheck) > 0 {
		_ = json.Unmarshal(item.Precheck, &precheck)
	}
	var missingTruncated, blockedTruncated bool
	precheck.MissingParameters, missingTruncated = trimPublicStringList(precheck.MissingParameters, maxTemplateFieldLength, maxTemplateParameters)
	precheck.BlockedReasons, blockedTruncated = trimPrecheckItems(precheck.BlockedReasons)
	warnings, warningsTruncated := trimPrecheckItems(item.Warnings)
	targetAgentIDs, targetsTruncated := trimPublicStringList(item.TargetAgentIDs, 0, maxCommandTargets)
	return PlanSummary{
		ID: item.ID, TemplateID: item.TemplateID, TemplateName: trimPublicText(item.TemplateName, maxTemplateFieldLength), TemplateVersion: item.TemplateVersion,
		Title: trimPublicText(item.Title, maxReasonLength), RiskLevel: trimPublicText(item.RiskLevel, maxTemplateFieldLength), RenderMode: trimPublicText(item.RenderMode, maxTemplateFieldLength),
		CommandHash: trimPublicText(item.CommandHash, maxTemplateFieldLength),
		TimeoutSec:  item.TimeoutSec, TargetAgentIDs: targetAgentIDs, Precheck: precheck,
		PrecheckTruncated: missingTruncated || blockedTruncated || warningsTruncated || targetsTruncated, Warnings: warnings, ApprovalRequired: item.ApprovalRequired, ApprovalReason: trimPublicText(item.ApprovalReason, maxReasonLength),
		Status: trimPublicText(item.Status, maxTemplateFieldLength), DiagnosisID: item.DiagnosisID, CreatedTaskID: item.CreatedTaskID, CreatedAt: item.CreatedAt,
		UpdatedAt: item.UpdatedAt, CancelledAt: item.CancelledAt, ExecutedAt: item.ExecutedAt,
	}
}

func summarizeCommandPlanApproval(item commandPlanApprovalRecord) CommandPlanApproval {
	snapshot, snapshotTruncated := summarizeApprovalPlanSnapshot(item.PlanSnapshot)
	return CommandPlanApproval{
		ID:                item.ID,
		PlanID:            item.PlanID,
		RiskLevel:         trimPublicText(item.RiskLevel, maxTemplateFieldLength),
		Status:            trimPublicText(item.Status, maxTemplateFieldLength),
		Reason:            trimPublicText(item.Reason, maxReasonLength),
		DecisionMessage:   trimPublicText(item.DecisionMessage, maxReasonLength),
		ExpiresAt:         item.ExpiresAt,
		DecidedAt:         item.DecidedAt,
		CreatedAt:         item.CreatedAt,
		UpdatedAt:         item.UpdatedAt,
		PlanSnapshot:      snapshot,
		SnapshotTruncated: snapshotTruncated,
	}
}

func summarizeApprovalPlanSnapshot(raw json.RawMessage) (*ApprovalPlanSnapshot, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false
	}
	var item commandPlanApprovalSnapshotRecord
	if err := json.Unmarshal(raw, &item); err != nil {
		return nil, true
	}
	var precheck PrecheckSummary
	if len(item.Precheck) > 0 {
		_ = json.Unmarshal(item.Precheck, &precheck)
	}
	var missingTruncated, blockedTruncated bool
	precheck.MissingParameters, missingTruncated = trimPublicStringList(precheck.MissingParameters, maxTemplateFieldLength, maxTemplateParameters)
	precheck.BlockedReasons, blockedTruncated = trimPrecheckItems(precheck.BlockedReasons)
	warnings, warningsTruncated := trimPrecheckItems(item.Warnings)
	targets, targetsTruncated := trimPublicStringList(item.TargetAgentIDs, 0, maxCommandTargets)
	snapshot := &ApprovalPlanSnapshot{
		TemplateID:      item.TemplateID,
		TemplateName:    trimPublicText(item.TemplateName, maxTemplateFieldLength),
		TemplateVersion: item.TemplateVersion,
		Title:           trimPublicText(item.Title, maxReasonLength),
		RiskLevel:       trimPublicText(item.RiskLevel, maxTemplateFieldLength),
		CommandHash:     trimPublicText(item.CommandHash, maxTemplateFieldLength),
		TimeoutSec:      item.TimeoutSec,
		TargetAgentIDs:  targets,
		Precheck:        precheck,
		Warnings:        warnings,
		Truncated:       missingTruncated || blockedTruncated || warningsTruncated || targetsTruncated,
	}
	return snapshot, snapshot.Truncated
}

func summarizeExecTask(item execTaskRecord) TaskSummary {
	targets := make([]TaskTargetSummary, 0, minInt(len(item.Targets), maxTaskTargets))
	targetsTruncated := false
	for index, target := range item.Targets {
		if index >= maxTaskTargets {
			targetsTruncated = true
			break
		}
		targets = append(targets, TaskTargetSummary{ID: target.ID, AgentID: target.AgentID, Status: trimPublicText(target.Status, maxTemplateFieldLength), ExitCode: target.ExitCode, OutputSize: target.OutputSize, StartedAt: target.StartedAt, FinishedAt: target.FinishedAt})
	}
	return TaskSummary{ID: item.ID, TaskType: trimPublicText(item.TaskType, maxTemplateFieldLength), Title: trimPublicText(item.Title, maxReasonLength), TimeoutSec: item.TimeoutSec, Status: trimPublicText(item.Status, maxTemplateFieldLength), CreatedAt: item.CreatedAt, StartedAt: item.StartedAt, FinishedAt: item.FinishedAt, CancelledAt: item.CancelledAt, Targets: targets, TargetsTruncated: targetsTruncated}
}

func trimPrecheckItems(items []PrecheckItem) ([]PrecheckItem, bool) {
	if len(items) == 0 {
		return nil, false
	}
	result := make([]PrecheckItem, 0, minInt(len(items), maxPrecheckItems))
	for index, item := range items {
		if index >= maxPrecheckItems {
			return result, true
		}
		item.Code = trimPublicText(item.Code, 100)
		item.Level = trimPublicText(item.Level, 32)
		item.Message = trimPublicText(item.Message, maxReasonLength)
		item.Hostname = trimPublicText(item.Hostname, 255)
		result = append(result, item)
	}
	return result, false
}

func trimPublicStringList(values []string, itemMax, listMax int) ([]string, bool) {
	if len(values) == 0 {
		return nil, false
	}
	result := make([]string, 0, minInt(len(values), listMax))
	for index, value := range values {
		if index >= listMax {
			return result, true
		}
		if itemMax > 0 {
			value = trimPublicText(value, itemMax)
		}
		result = append(result, value)
	}
	return result, false
}

func minInt(left, right int) int {
	if left < right {
		return left
	}
	return right
}

func trimPublicText(value string, max int) string {
	trimmed, _ := trimPublicTextWithFlag(value, max)
	return trimmed
}

func trimPublicTextWithFlag(value string, max int) (string, bool) {
	value = strings.TrimSpace(value)
	if max <= 0 {
		return "", value != ""
	}
	if len(value) <= max {
		return value, false
	}
	var builder strings.Builder
	builder.Grow(max)
	for _, r := range value {
		runeSize := utf8.RuneLen(r)
		if runeSize < 0 || builder.Len()+runeSize > max {
			break
		}
		builder.WriteRune(r)
	}
	return builder.String(), true
}

func (c *Client) do(ctx context.Context, method string, segments []string, query url.Values, payload any, output any, authenticated bool) error {
	endpoint := c.baseURL.JoinPath(segments...)
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return errors.New("encode Baize request")
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return errors.New("create Baize request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		if c.token == "" {
			return errors.New("Baize session credential is unavailable")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errors.New("Baize request timed out or was cancelled")
		}
		return errors.New("Baize request failed before a response was received")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("read Baize response")
	}
	if len(raw) > maxResponseBytes {
		return errors.New("Baize response exceeded the allowed size")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{StatusCode: resp.StatusCode}
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) != nil {
		return errors.New("Baize response was not valid JSON")
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("Baize response did not match the expected format")
	}
	return nil
}

func summarizeAgent(item agentRecord) AgentSummary {
	displayName := strings.TrimSpace(item.Alias)
	if displayName == "" {
		displayName = strings.TrimSpace(item.Hostname)
	}
	if displayName == "" {
		displayName = item.ID
	}
	operatingSystem := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(item.OSType),
		strings.TrimSpace(item.OSVersion),
	}, " "))
	return AgentSummary{
		ID:              item.ID,
		DisplayName:     displayName,
		Status:          item.Status,
		OperatingSystem: operatingSystem,
		Architecture:    item.Arch,
		AgentVersion:    item.AgentVersion,
		LastHeartbeatAt: item.LastHeartbeatAt,
	}
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
