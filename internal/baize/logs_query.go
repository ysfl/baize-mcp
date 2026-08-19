package baize

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	LogQuerySourceServer   = "server"
	LogQuerySourceAgent    = "agent"
	LogQuerySourceOverview = "overview"
	maxLogQueryItems       = 100
	maxLogQueryBytes       = 16 << 10
	maxLogOverviewItems    = 20
)

var logQueryLevels = map[string]struct{}{
	"debug": {}, "info": {}, "warn": {}, "error": {},
}

type LogsQueryOptions struct {
	Source           string
	AgentID          string
	Level            string
	Module           string
	Search           string
	TaskID           string
	SinceMinutes     int
	SinceTimestampMS int64
	Limit            int
	WindowMinutes    int
}

type LogQueryResult struct {
	ReadResultBoundary
	Source   string              `json:"source"`
	Status   string              `json:"status,omitempty"`
	Items    []LogLineSummary    `json:"items,omitempty"`
	Overview *LogOverviewSummary `json:"overview,omitempty"`
	Index    *LogIndexSummary    `json:"index,omitempty"`
}

type LogLineSummary struct {
	TimestampMS int64  `json:"timestampMs"`
	Time        string `json:"time,omitempty"`
	Level       string `json:"level"`
	Service     string `json:"service,omitempty"`
	Module      string `json:"module,omitempty"`
	Message     string `json:"message"`
	AgentID     string `json:"agentId,omitempty"`
	Error       string `json:"error,omitempty"`
}

type LogOverviewSummary struct {
	WindowMinutes int                 `json:"windowMinutes"`
	LevelCounts   []LogDimensionCount `json:"levelCounts,omitempty"`
	ModuleCounts  []LogDimensionCount `json:"moduleCounts,omitempty"`
	TopErrors     []LogErrorSummary   `json:"topErrors,omitempty"`
	GeneratedAt   *time.Time          `json:"generatedAt,omitempty"`
	Export        *LogExportSummary   `json:"export,omitempty"`
}

type LogDimensionCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type LogErrorSummary struct {
	Level      string     `json:"level"`
	Module     string     `json:"module,omitempty"`
	Message    string     `json:"message"`
	Error      string     `json:"error,omitempty"`
	Count      int64      `json:"count"`
	LastSeenAt *time.Time `json:"lastSeenAt,omitempty"`
}

type LogIndexSummary struct {
	Enabled       bool       `json:"enabled"`
	Started       bool       `json:"started"`
	Received      int64      `json:"received"`
	Indexed       int64      `json:"indexed"`
	Failed        int64      `json:"failed"`
	LastIndexedAt *time.Time `json:"lastIndexedAt,omitempty"`
}

type LogExportSummary struct {
	Enabled            bool       `json:"enabled"`
	Started            bool       `json:"started"`
	Format             string     `json:"format,omitempty"`
	EndpointConfigured bool       `json:"endpointConfigured"`
	Received           int64      `json:"received"`
	Exported           int64      `json:"exported"`
	Failed             int64      `json:"failed"`
	LastExportedAt     *time.Time `json:"lastExportedAt,omitempty"`
}

type runtimeLogLineRecord struct {
	TimestampMS int64  `json:"timestampMs"`
	Time        string `json:"time"`
	Level       string `json:"level"`
	Service     string `json:"service"`
	Module      string `json:"module"`
	Message     string `json:"message"`
	Source      string `json:"source"`
	RequestID   string `json:"requestId"`
	AgentID     string `json:"agentId"`
	TaskID      string `json:"taskId"`
	Fingerprint string `json:"fingerprint"`
	Error       string `json:"error"`
}

type runtimeLogIndexRecord struct {
	Enabled       bool       `json:"enabled"`
	Started       bool       `json:"started"`
	Received      int64      `json:"received"`
	Indexed       int64      `json:"indexed"`
	Failed        int64      `json:"failed"`
	LastError     string     `json:"lastError"`
	LastIndexedAt *time.Time `json:"lastIndexedAt"`
}

type runtimeLogExportRecord struct {
	Enabled            bool       `json:"enabled"`
	Started            bool       `json:"started"`
	Format             string     `json:"format"`
	EndpointConfigured bool       `json:"endpointConfigured"`
	Received           int64      `json:"received"`
	Exported           int64      `json:"exported"`
	Failed             int64      `json:"failed"`
	LastError          string     `json:"lastError"`
	LastExportedAt     *time.Time `json:"lastExportedAt"`
}

type runtimeLogOverviewRecord struct {
	WindowMinutes int `json:"windowMinutes"`
	LevelCounts   []struct {
		Level string `json:"level"`
		Count int64  `json:"count"`
	} `json:"levelCounts"`
	ModuleCounts []struct {
		Module string `json:"module"`
		Count  int64  `json:"count"`
	} `json:"moduleCounts"`
	TopErrors []struct {
		Fingerprint string     `json:"fingerprint"`
		Level       string     `json:"level"`
		Module      string     `json:"module"`
		Message     string     `json:"message"`
		Error       string     `json:"error"`
		Count       int64      `json:"count"`
		LastSeenAt  *time.Time `json:"lastSeenAt"`
	} `json:"topErrors"`
	GeneratedAt *time.Time             `json:"generatedAt"`
	Index       runtimeLogIndexRecord  `json:"index"`
	Export      runtimeLogExportRecord `json:"export"`
}

func (c *Client) QueryLogs(ctx context.Context, options LogsQueryOptions) (LogQueryResult, error) {
	source := strings.ToLower(strings.TrimSpace(options.Source))
	if source == "" {
		source = LogQuerySourceServer
	}
	if source != LogQuerySourceServer && source != LogQuerySourceAgent && source != LogQuerySourceOverview {
		return LogQueryResult{}, newInputError("log source must be server, agent, or overview")
	}
	if source == LogQuerySourceOverview {
		return c.queryLogOverview(ctx, options.WindowMinutes)
	}
	level := strings.ToLower(strings.TrimSpace(options.Level))
	if !isAllowedReadValue(level, logQueryLevels) {
		return LogQueryResult{}, newInputError("log level must be debug, info, warn, or error")
	}
	module, err := validateReadFilter(options.Module, "log module", 120)
	if err != nil {
		return LogQueryResult{}, err
	}
	search, err := validateReadFilter(options.Search, "log search", maxReadSearchLength)
	if err != nil {
		return LogQueryResult{}, err
	}
	limit := options.Limit
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxLogQueryItems {
		return LogQueryResult{}, newInputError("log limit must be between 1 and 100")
	}
	if options.SinceMinutes < 0 || options.SinceMinutes > 10080 {
		return LogQueryResult{}, newInputError("since minutes must be between 1 and 10080, or omitted")
	}
	if options.SinceTimestampMS < 0 {
		return LogQueryResult{}, newInputError("since timestamp must not be negative")
	}
	if source == LogQuerySourceAgent {
		return c.queryAgentLogs(ctx, options, level, search, limit)
	}
	return c.queryServerLogs(ctx, options, level, module, search, limit)
}

func (c *Client) queryServerLogs(ctx context.Context, options LogsQueryOptions, level, module, search string, limit int) (LogQueryResult, error) {
	query := url.Values{"limit": {fmt.Sprintf("%d", limit)}}
	setOptionalQuery(query, "level", level)
	setOptionalQuery(query, "module", module)
	setOptionalQuery(query, "search", search)
	if options.AgentID != "" {
		agentID, err := validateUUID(options.AgentID, "agent ID")
		if err != nil {
			return LogQueryResult{}, err
		}
		query.Set("agent_id", agentID)
	}
	if options.TaskID != "" {
		taskID, err := validateUUID(options.TaskID, "task ID")
		if err != nil {
			return LogQueryResult{}, err
		}
		query.Set("task_id", taskID)
	}
	if options.SinceMinutes > 0 {
		query.Set("since_minutes", fmt.Sprintf("%d", options.SinceMinutes))
	}
	if options.SinceTimestampMS > 0 {
		query.Set("since_timestamp_ms", fmt.Sprintf("%d", options.SinceTimestampMS))
	}
	var data struct {
		Source string                 `json:"source"`
		Index  runtimeLogIndexRecord  `json:"index"`
		Items  []runtimeLogLineRecord `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"observability", "server-logs"}, query, nil, &data, true); err != nil {
		return LogQueryResult{}, err
	}
	return summarizeLogLines(LogQuerySourceServer, "", data.Items, &data.Index), nil
}

func (c *Client) queryAgentLogs(ctx context.Context, options LogsQueryOptions, level, search string, limit int) (LogQueryResult, error) {
	agentID, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return LogQueryResult{}, err
	}
	payload := struct {
		Level          string `json:"level,omitempty"`
		Search         string `json:"search,omitempty"`
		Limit          int    `json:"limit"`
		SinceMinutes   int    `json:"sinceMinutes,omitempty"`
		SinceTimestamp int64  `json:"sinceTimestampMs,omitempty"`
	}{level, search, limit, options.SinceMinutes, options.SinceTimestampMS}
	var data struct {
		RequestID string                 `json:"requestId"`
		Status    string                 `json:"status"`
		Error     string                 `json:"error"`
		Items     []runtimeLogLineRecord `json:"items"`
	}
	if err := c.do(ctx, http.MethodPost, []string{"observability", "agents", agentID, "logs", "query"}, nil, payload, &data, true); err != nil {
		return LogQueryResult{}, err
	}
	result := summarizeLogLines(LogQuerySourceAgent, data.Status, data.Items, nil)
	if data.Error != "" {
		message, redacted, truncated := redactBoundedReadText(data.Error, maxReadTextLength)
		result.RedactionApplied = result.RedactionApplied || redacted
		result.Truncated = result.Truncated || truncated
		result.Notice = trimPublicText(result.Notice+" Agent query status: "+message, 1000)
	}
	return result, nil
}

func (c *Client) queryLogOverview(ctx context.Context, windowMinutes int) (LogQueryResult, error) {
	if windowMinutes == 0 {
		windowMinutes = 60
	}
	if windowMinutes < 1 || windowMinutes > 10080 {
		return LogQueryResult{}, newInputError("log overview window must be between 1 and 10080 minutes")
	}
	query := url.Values{"window_minutes": {fmt.Sprintf("%d", windowMinutes)}}
	var data runtimeLogOverviewRecord
	if err := c.do(ctx, http.MethodGet, []string{"observability", "server-logs", "overview"}, query, nil, &data, true); err != nil {
		return LogQueryResult{}, err
	}
	boundary := newReadResultBoundary("This is a bounded log overview. Endpoint addresses, raw identifiers, credentials, and unbounded log content are excluded.")
	overview := &LogOverviewSummary{WindowMinutes: data.WindowMinutes, GeneratedAt: data.GeneratedAt}
	for index, item := range data.LevelCounts {
		if index >= maxLogOverviewItems {
			boundary.Truncated = true
			break
		}
		overview.LevelCounts = append(overview.LevelCounts, LogDimensionCount{Name: trimPublicText(item.Level, 40), Count: item.Count})
	}
	for index, item := range data.ModuleCounts {
		if index >= maxLogOverviewItems {
			boundary.Truncated = true
			break
		}
		overview.ModuleCounts = append(overview.ModuleCounts, LogDimensionCount{Name: trimPublicText(item.Module, 120), Count: item.Count})
	}
	for index, item := range data.TopErrors {
		if index >= maxLogOverviewItems {
			boundary.Truncated = true
			break
		}
		message, messageChanged, messageTruncated := redactBoundedReadText(item.Message, maxReadTextLength)
		errorText, errorChanged, errorTruncated := redactBoundedReadText(item.Error, maxReadTextLength)
		boundary.RedactionApplied = boundary.RedactionApplied || messageChanged || errorChanged
		boundary.Truncated = boundary.Truncated || messageTruncated || errorTruncated
		overview.TopErrors = append(overview.TopErrors, LogErrorSummary{Level: trimPublicText(item.Level, 40), Module: trimPublicText(item.Module, 120), Message: message, Error: errorText, Count: item.Count, LastSeenAt: item.LastSeenAt})
	}
	overview.Export = &LogExportSummary{Enabled: data.Export.Enabled, Started: data.Export.Started, Format: trimPublicText(data.Export.Format, 40), EndpointConfigured: data.Export.EndpointConfigured, Received: data.Export.Received, Exported: data.Export.Exported, Failed: data.Export.Failed, LastExportedAt: data.Export.LastExportedAt}
	return LogQueryResult{ReadResultBoundary: boundary, Source: LogQuerySourceOverview, Overview: overview, Index: summarizeLogIndex(data.Index)}, nil
}

func summarizeLogLines(source, status string, items []runtimeLogLineRecord, index *runtimeLogIndexRecord) LogQueryResult {
	boundary := newReadResultBoundary("This is bounded log output. Request IDs, task correlation fields, fingerprints, raw source paths and credentials are excluded. If redactionApplied is true, content was conservatively replaced; do not repeat the same query expecting the original secret-bearing text.")
	result := LogQueryResult{ReadResultBoundary: boundary, Source: source, Status: trimPublicText(status, 40)}
	usedBytes := 0
	for itemIndex, item := range items {
		if itemIndex >= maxLogQueryItems {
			result.Truncated = true
			break
		}
		message, messageChanged, messageTruncated := redactBoundedReadText(item.Message, maxReadTextLength)
		errorText, errorChanged, errorTruncated := redactBoundedReadText(item.Error, maxReadTextLength)
		lineBytes := len(message) + len(errorText) + len(item.Service) + len(item.Module) + 64
		if usedBytes+lineBytes > maxLogQueryBytes {
			result.Truncated = true
			break
		}
		usedBytes += lineBytes
		result.RedactionApplied = result.RedactionApplied || messageChanged || errorChanged
		result.Truncated = result.Truncated || messageTruncated || errorTruncated
		result.Items = append(result.Items, LogLineSummary{TimestampMS: item.TimestampMS, Time: trimPublicText(item.Time, 64), Level: trimPublicText(item.Level, 40), Service: trimPublicText(item.Service, 120), Module: trimPublicText(item.Module, 120), Message: message, AgentID: trimPublicText(item.AgentID, 64), Error: errorText})
	}
	if index != nil {
		result.Index = summarizeLogIndex(*index)
	}
	return result
}

func summarizeLogIndex(data runtimeLogIndexRecord) *LogIndexSummary {
	return &LogIndexSummary{Enabled: data.Enabled, Started: data.Started, Received: data.Received, Indexed: data.Indexed, Failed: data.Failed, LastIndexedAt: data.LastIndexedAt}
}

func setOptionalQuery(query url.Values, key, value string) {
	if value != "" {
		query.Set(key, value)
	}
}
