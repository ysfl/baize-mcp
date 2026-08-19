package baize

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	cronQueryViews    = map[string]struct{}{"list": {}, "detail": {}, "logs": {}}
	cronScheduleTypes = map[string]struct{}{"cron": {}, "once": {}}
	cronSortFields    = map[string]struct{}{
		"name": {}, "cron_expr": {}, "target_count": {}, "created_at": {}, "updated_at": {},
		"next_run_at": {}, "last_run_at": {}, "enabled": {}, "schedule_type": {},
	}
	runbookQueryViews = map[string]struct{}{"list": {}, "detail": {}, "audit": {}}
	runbookStatuses   = map[string]struct{}{"draft": {}, "enabled": {}, "deprecated": {}, "archived": {}}
	runbookRiskLevels = map[string]struct{}{"read_only": {}, "low": {}, "medium": {}, "high": {}, "critical": {}}
)

type CronJobsQueryOptions struct {
	View          string
	ID            string
	Page          int
	PageSize      int
	Enabled       *bool
	ScheduleType  string
	TargetAgentID string
	Search        string
	SortBy        string
	SortOrder     string
}

type CronJobsQueryResult struct {
	ReadResultBoundary
	View   string           `json:"view"`
	Page   *ReadPageMeta    `json:"page,omitempty"`
	Items  []CronJobSummary `json:"items,omitempty"`
	Detail *CronJobSummary  `json:"detail,omitempty"`
	Logs   []CronLogSummary `json:"logs,omitempty"`
}

type CronJobSummary struct {
	ID              string     `json:"id"`
	Name            string     `json:"name"`
	ScheduleType    string     `json:"scheduleType"`
	CronExpr        string     `json:"cronExpr,omitempty"`
	RunAt           *time.Time `json:"runAt,omitempty"`
	Timezone        string     `json:"timezone,omitempty"`
	TimeoutSec      int        `json:"timeoutSec"`
	TargetAgentIDs  []string   `json:"targetAgentIds,omitempty"`
	TargetCount     int        `json:"targetCount"`
	Enabled         bool       `json:"enabled"`
	LastRunAt       *time.Time `json:"lastRunAt,omitempty"`
	NextRunAt       *time.Time `json:"nextRunAt,omitempty"`
	ConsumedAt      *time.Time `json:"consumedAt,omitempty"`
	CreatedAt       *time.Time `json:"createdAt,omitempty"`
	UpdatedAt       *time.Time `json:"updatedAt,omitempty"`
	CommandExcluded bool       `json:"commandExcluded"`
}

type CronLogSummary struct {
	ID           string     `json:"id"`
	CronJobID    string     `json:"cronJobId"`
	ExecTaskID   string     `json:"execTaskId,omitempty"`
	Status       string     `json:"status"`
	TriggeredAt  *time.Time `json:"triggeredAt,omitempty"`
	FinishedAt   *time.Time `json:"finishedAt,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

type cronJobRecord struct {
	ID             string          `json:"id"`
	Name           string          `json:"name"`
	ScheduleType   string          `json:"scheduleType"`
	CronExpr       string          `json:"cronExpr"`
	RunAt          *time.Time      `json:"runAt"`
	Timezone       string          `json:"timezone"`
	Command        string          `json:"command"`
	WorkDir        string          `json:"workDir"`
	TimeoutSec     int             `json:"timeoutSec"`
	TargetAgentIDs json.RawMessage `json:"targetAgentIds"`
	Enabled        bool            `json:"enabled"`
	OperatorID     string          `json:"operatorId"`
	OperatorName   string          `json:"operatorName"`
	LastRunAt      *time.Time      `json:"lastRunAt"`
	NextRunAt      *time.Time      `json:"nextRunAt"`
	ConsumedAt     *time.Time      `json:"consumedAt"`
	CreatedAt      *time.Time      `json:"createdAt"`
	UpdatedAt      *time.Time      `json:"updatedAt"`
}

type cronLogRecord struct {
	ID           string     `json:"id"`
	CronJobID    string     `json:"cronJobId"`
	ExecTaskID   string     `json:"execTaskId"`
	Status       string     `json:"status"`
	TriggeredAt  *time.Time `json:"triggeredAt"`
	FinishedAt   *time.Time `json:"finishedAt"`
	ErrorMessage string     `json:"errorMessage"`
}

func (c *Client) QueryCronJobs(ctx context.Context, options CronJobsQueryOptions) (CronJobsQueryResult, error) {
	view := strings.ToLower(strings.TrimSpace(options.View))
	if view == "" {
		view = "list"
	}
	if !isAllowedReadValue(view, cronQueryViews) {
		return CronJobsQueryResult{}, newInputError("cron view must be list, detail, or logs")
	}
	boundary := newReadResultBoundary("Scheduled command text, working directories and operator identity are excluded. Log errors are bounded and conservatively redacted.")
	result := CronJobsQueryResult{ReadResultBoundary: boundary, View: view}
	if view == "detail" {
		id, err := validateUUID(options.ID, "cron job ID")
		if err != nil {
			return CronJobsQueryResult{}, err
		}
		var data cronJobRecord
		if err := c.do(ctx, http.MethodGet, []string{"cron", "jobs", id}, nil, nil, &data, true); err != nil {
			return CronJobsQueryResult{}, err
		}
		item, truncated := summarizeCronJob(data)
		result.Truncated = truncated
		result.Detail = &item
		return result, nil
	}
	page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
	if err != nil {
		return CronJobsQueryResult{}, err
	}
	query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}}
	if view == "logs" {
		id, err := validateUUID(options.ID, "cron job ID")
		if err != nil {
			return CronJobsQueryResult{}, err
		}
		var data struct {
			Items    []cronLogRecord `json:"items"`
			Total    int             `json:"total"`
			Page     int             `json:"page"`
			PageSize int             `json:"pageSize"`
		}
		if err := c.do(ctx, http.MethodGet, []string{"cron", "jobs", id, "logs"}, query, nil, &data, true); err != nil {
			return CronJobsQueryResult{}, err
		}
		for index, item := range data.Items {
			if index >= maxReadPageSize {
				result.Truncated = true
				break
			}
			errorMessage, changed, truncated := redactBoundedReadText(item.ErrorMessage, maxReadTextLength)
			result.RedactionApplied = result.RedactionApplied || changed
			result.Truncated = result.Truncated || truncated
			result.Logs = append(result.Logs, CronLogSummary{ID: item.ID, CronJobID: item.CronJobID, ExecTaskID: item.ExecTaskID, Status: trimPublicText(item.Status, 40), TriggeredAt: item.TriggeredAt, FinishedAt: item.FinishedAt, ErrorMessage: errorMessage})
		}
		actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
		pageMeta := makeReadPageMeta(data.Total, actualPage, actualPageSize, result.Truncated)
		result.Page = &pageMeta
		return result, nil
	}
	if options.Enabled != nil {
		query.Set("enabled", strconv.FormatBool(*options.Enabled))
	}
	scheduleType := strings.ToLower(strings.TrimSpace(options.ScheduleType))
	if !isAllowedReadValue(scheduleType, cronScheduleTypes) {
		return CronJobsQueryResult{}, newInputError("schedule type must be cron or once")
	}
	setOptionalQuery(query, "schedule_type", scheduleType)
	if options.TargetAgentID != "" {
		agentID, err := validateUUID(options.TargetAgentID, "target agent ID")
		if err != nil {
			return CronJobsQueryResult{}, err
		}
		query.Set("target_agent_id", agentID)
	}
	search, err := validateReadFilter(options.Search, "cron search", 120)
	if err != nil {
		return CronJobsQueryResult{}, err
	}
	setOptionalQuery(query, "search", search)
	sortBy := strings.ToLower(strings.TrimSpace(options.SortBy))
	if !isAllowedReadValue(sortBy, cronSortFields) {
		return CronJobsQueryResult{}, newInputError("cron sort field is not supported")
	}
	setOptionalQuery(query, "sort_by", sortBy)
	sortOrder := strings.ToLower(strings.TrimSpace(options.SortOrder))
	if sortOrder != "" && sortOrder != "asc" && sortOrder != "desc" {
		return CronJobsQueryResult{}, newInputError("sort order must be asc or desc")
	}
	setOptionalQuery(query, "sort_order", sortOrder)
	var data struct {
		Items    []cronJobRecord `json:"items"`
		Total    int             `json:"total"`
		Page     int             `json:"page"`
		PageSize int             `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"cron", "jobs"}, query, nil, &data, true); err != nil {
		return CronJobsQueryResult{}, err
	}
	for index, item := range data.Items {
		if index >= maxReadPageSize {
			result.Truncated = true
			break
		}
		summary, truncated := summarizeCronJob(item)
		result.Truncated = result.Truncated || truncated
		result.Items = append(result.Items, summary)
	}
	actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
	pageMeta := makeReadPageMeta(data.Total, actualPage, actualPageSize, result.Truncated)
	result.Page = &pageMeta
	return result, nil
}

func summarizeCronJob(data cronJobRecord) (CronJobSummary, bool) {
	var targetAgentIDs []string
	if len(data.TargetAgentIDs) > 0 {
		_ = json.Unmarshal(data.TargetAgentIDs, &targetAgentIDs)
	}
	targetCount := len(targetAgentIDs)
	targetAgentIDs, truncated := trimPublicStringList(targetAgentIDs, 64, maxCommandTargets)
	return CronJobSummary{ID: data.ID, Name: trimPublicText(data.Name, 255), ScheduleType: trimPublicText(data.ScheduleType, 40), CronExpr: trimPublicText(data.CronExpr, 100), RunAt: data.RunAt, Timezone: trimPublicText(data.Timezone, 64), TimeoutSec: data.TimeoutSec, TargetAgentIDs: targetAgentIDs, TargetCount: targetCount, Enabled: data.Enabled, LastRunAt: data.LastRunAt, NextRunAt: data.NextRunAt, ConsumedAt: data.ConsumedAt, CreatedAt: data.CreatedAt, UpdatedAt: data.UpdatedAt, CommandExcluded: true}, truncated
}

type RunbooksQueryOptions struct {
	View      string
	ID        string
	Page      int
	PageSize  int
	Status    string
	Category  string
	RiskLevel string
	AIUsable  *bool
	Search    string
	Action    string
}

type RunbooksQueryResult struct {
	ReadResultBoundary
	View   string           `json:"view"`
	Page   *ReadPageMeta    `json:"page,omitempty"`
	Items  []RunbookSummary `json:"items,omitempty"`
	Detail *RunbookDetail   `json:"detail,omitempty"`
	Audit  []RunbookAudit   `json:"audit,omitempty"`
}

type RunbookSummary struct {
	ID                   string     `json:"id"`
	Name                 string     `json:"name"`
	Description          string     `json:"description,omitempty"`
	Category             string     `json:"category,omitempty"`
	Status               string     `json:"status"`
	RiskLevel            string     `json:"riskLevel"`
	Version              int        `json:"version"`
	RequiredCapabilities []string   `json:"requiredCapabilities,omitempty"`
	AIUsable             bool       `json:"aiUsable"`
	CreatedAt            *time.Time `json:"createdAt,omitempty"`
	UpdatedAt            *time.Time `json:"updatedAt,omitempty"`
}

type RunbookDetail struct {
	Runbook                   RunbookSummary `json:"runbook"`
	Steps                     []RunbookStep  `json:"steps"`
	DefinitionContentExcluded bool           `json:"definitionContentExcluded"`
}

type RunbookStep struct {
	ID                  string `json:"id"`
	StepKey             string `json:"stepKey"`
	StepOrder           int    `json:"stepOrder"`
	StepType            string `json:"stepType"`
	Name                string `json:"name"`
	Required            bool   `json:"required"`
	RiskLevel           string `json:"riskLevel"`
	CommandTemplateID   string `json:"commandTemplateId,omitempty"`
	DiagnosisTargetType string `json:"diagnosisTargetType,omitempty"`
	TimeoutSec          int    `json:"timeoutSec"`
}

type RunbookAudit struct {
	ID        string     `json:"id"`
	RunbookID string     `json:"runbookId,omitempty"`
	Action    string     `json:"action"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

type runbookRecord struct {
	ID                   string          `json:"id"`
	Name                 string          `json:"name"`
	Description          string          `json:"description"`
	Category             string          `json:"category"`
	Status               string          `json:"status"`
	RiskLevel            string          `json:"riskLevel"`
	Version              int             `json:"version"`
	RequiredCapabilities json.RawMessage `json:"requiredCapabilities"`
	Inputs               json.RawMessage `json:"inputs"`
	ContextBindings      json.RawMessage `json:"contextBindings"`
	Verification         json.RawMessage `json:"verification"`
	Rollback             json.RawMessage `json:"rollback"`
	AIUsable             bool            `json:"aiUsable"`
	CreatedBy            string          `json:"createdBy"`
	UpdatedBy            string          `json:"updatedBy"`
	CreatedAt            *time.Time      `json:"createdAt"`
	UpdatedAt            *time.Time      `json:"updatedAt"`
}

type runbookStepRecord struct {
	ID                  string          `json:"id"`
	StepKey             string          `json:"stepKey"`
	StepOrder           int             `json:"stepOrder"`
	StepType            string          `json:"stepType"`
	Name                string          `json:"name"`
	Required            bool            `json:"required"`
	RiskLevel           string          `json:"riskLevel"`
	CommandTemplateID   string          `json:"commandTemplateId"`
	DiagnosisTargetType string          `json:"diagnosisTargetType"`
	InputBindings       json.RawMessage `json:"inputBindings"`
	Conditions          json.RawMessage `json:"conditions"`
	Verification        json.RawMessage `json:"verification"`
	Rollback            json.RawMessage `json:"rollback"`
	ManualInstruction   string          `json:"manualInstruction"`
	TimeoutSec          int             `json:"timeoutSec"`
}

type runbookAuditRecord struct {
	ID           string          `json:"id"`
	RunbookID    string          `json:"runbookId"`
	InstanceID   string          `json:"instanceId"`
	Action       string          `json:"action"`
	OperatorID   string          `json:"operatorId"`
	OperatorName string          `json:"operatorName"`
	ClientIP     string          `json:"clientIp"`
	Detail       json.RawMessage `json:"detail"`
	CreatedAt    *time.Time      `json:"createdAt"`
}

func (c *Client) QueryRunbooks(ctx context.Context, options RunbooksQueryOptions) (RunbooksQueryResult, error) {
	view := strings.ToLower(strings.TrimSpace(options.View))
	if view == "" {
		view = "list"
	}
	if !isAllowedReadValue(view, runbookQueryViews) {
		return RunbooksQueryResult{}, newInputError("runbook view must be list, detail, or audit")
	}
	boundary := newReadResultBoundary("Runbook inputs, bindings, conditions, manual instructions, verification bodies, rollback bodies, operator identity, client IP and audit details are excluded.")
	result := RunbooksQueryResult{ReadResultBoundary: boundary, View: view}
	if view == "detail" {
		id, err := validateUUID(options.ID, "runbook ID")
		if err != nil {
			return RunbooksQueryResult{}, err
		}
		var data struct {
			Runbook runbookRecord       `json:"runbook"`
			Steps   []runbookStepRecord `json:"steps"`
		}
		if err := c.do(ctx, http.MethodGet, []string{"ops", "runbooks", id}, nil, nil, &data, true); err != nil {
			return RunbooksQueryResult{}, err
		}
		runbook, truncated, redacted := summarizeRunbook(data.Runbook)
		result.Truncated = truncated || len(data.Steps) > 32
		result.RedactionApplied = redacted
		detail := &RunbookDetail{Runbook: runbook, DefinitionContentExcluded: true}
		for index, item := range data.Steps {
			if index >= 32 {
				break
			}
			detail.Steps = append(detail.Steps, RunbookStep{ID: item.ID, StepKey: trimPublicText(item.StepKey, 80), StepOrder: item.StepOrder, StepType: trimPublicText(item.StepType, 40), Name: trimPublicText(item.Name, 160), Required: item.Required, RiskLevel: trimPublicText(item.RiskLevel, 40), CommandTemplateID: item.CommandTemplateID, DiagnosisTargetType: trimPublicText(item.DiagnosisTargetType, 40), TimeoutSec: item.TimeoutSec})
		}
		result.Detail = detail
		return result, nil
	}
	page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
	if err != nil {
		return RunbooksQueryResult{}, err
	}
	query := url.Values{"page": {fmt.Sprintf("%d", page)}, "page_size": {fmt.Sprintf("%d", pageSize)}}
	if view == "audit" {
		id, err := validateUUID(options.ID, "runbook ID")
		if err != nil {
			return RunbooksQueryResult{}, err
		}
		action := strings.ToLower(strings.TrimSpace(options.Action))
		if action != "" && action != "create_definition" && action != "update_definition" {
			return RunbooksQueryResult{}, newInputError("runbook audit action must be create_definition or update_definition")
		}
		setOptionalQuery(query, "action", action)
		var data struct {
			Items    []runbookAuditRecord `json:"items"`
			Total    int                  `json:"total"`
			Page     int                  `json:"page"`
			PageSize int                  `json:"pageSize"`
		}
		if err := c.do(ctx, http.MethodGet, []string{"ops", "runbooks", id, "audit-logs"}, query, nil, &data, true); err != nil {
			return RunbooksQueryResult{}, err
		}
		for index, item := range data.Items {
			if index >= maxReadPageSize {
				result.Truncated = true
				break
			}
			result.Audit = append(result.Audit, RunbookAudit{ID: item.ID, RunbookID: item.RunbookID, Action: trimPublicText(item.Action, 40), CreatedAt: item.CreatedAt})
		}
		actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
		pageMeta := makeReadPageMeta(data.Total, actualPage, actualPageSize, result.Truncated)
		result.Page = &pageMeta
		return result, nil
	}
	status := strings.ToLower(strings.TrimSpace(options.Status))
	riskLevel := strings.ToLower(strings.TrimSpace(options.RiskLevel))
	if !isAllowedReadValue(status, runbookStatuses) {
		return RunbooksQueryResult{}, newInputError("runbook status is not supported")
	}
	if !isAllowedReadValue(riskLevel, runbookRiskLevels) {
		return RunbooksQueryResult{}, newInputError("runbook risk level is not supported")
	}
	category, err := validateReadFilter(options.Category, "runbook category", 80)
	if err != nil {
		return RunbooksQueryResult{}, err
	}
	search, err := validateReadFilter(options.Search, "runbook search", maxReadSearchLength)
	if err != nil {
		return RunbooksQueryResult{}, err
	}
	setOptionalQuery(query, "status", status)
	setOptionalQuery(query, "riskLevel", riskLevel)
	setOptionalQuery(query, "category", category)
	setOptionalQuery(query, "search", search)
	if options.AIUsable != nil {
		query.Set("aiUsable", strconv.FormatBool(*options.AIUsable))
	}
	var data struct {
		Items    []runbookRecord `json:"items"`
		Total    int             `json:"total"`
		Page     int             `json:"page"`
		PageSize int             `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"ops", "runbooks"}, query, nil, &data, true); err != nil {
		return RunbooksQueryResult{}, err
	}
	for index, item := range data.Items {
		if index >= maxReadPageSize {
			result.Truncated = true
			break
		}
		summary, truncated, redacted := summarizeRunbook(item)
		result.Truncated = result.Truncated || truncated
		result.RedactionApplied = result.RedactionApplied || redacted
		result.Items = append(result.Items, summary)
	}
	actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
	pageMeta := makeReadPageMeta(data.Total, actualPage, actualPageSize, result.Truncated)
	result.Page = &pageMeta
	return result, nil
}

func summarizeRunbook(data runbookRecord) (RunbookSummary, bool, bool) {
	var capabilities []string
	if len(data.RequiredCapabilities) > 0 {
		_ = json.Unmarshal(data.RequiredCapabilities, &capabilities)
	}
	capabilities, truncated := trimPublicStringList(capabilities, maxTemplateFieldLength, maxTemplateCapabilities)
	description, redacted, descriptionTruncated := redactBoundedReadText(data.Description, maxReadTextLength)
	return RunbookSummary{ID: data.ID, Name: trimPublicText(data.Name, 120), Description: description, Category: trimPublicText(data.Category, 80), Status: trimPublicText(data.Status, 40), RiskLevel: trimPublicText(data.RiskLevel, 40), Version: data.Version, RequiredCapabilities: capabilities, AIUsable: data.AIUsable, CreatedAt: data.CreatedAt, UpdatedAt: data.UpdatedAt}, truncated || descriptionTruncated, redacted
}
