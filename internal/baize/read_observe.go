package baize

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	maxObserveItems = 50
	maxObserveText  = 512
)

// NginxObserveOptions 选择一个固定的 Nginx 只读视图。
type NginxObserveOptions struct {
	View     string
	AgentID  string
	SiteID   string
	From     *time.Time
	To       *time.Time
	Page     int
	PageSize int
}

// NginxObserveResult 是面向 AI 的有界 Nginx 观察结果。
type NginxObserveResult struct {
	ReadResultBoundary
	View         string                     `json:"view"`
	Sites        []NginxSiteObservation     `json:"sites,omitempty"`
	Site         *NginxSiteObservation      `json:"site,omitempty"`
	Metrics      *NginxMetricsObservation   `json:"metrics,omitempty"`
	Overview     *NginxOverviewObservation  `json:"overview,omitempty"`
	Upstreams    []NginxUpstreamObservation `json:"upstreams,omitempty"`
	SlowRequests []NginxSlowRequestSummary  `json:"slowRequests,omitempty"`
	ResponseTime *NginxResponseTimeSummary  `json:"responseTime,omitempty"`
	Page         *ReadPageMeta              `json:"page,omitempty"`
}

type NginxSiteObservation struct {
	ID                string     `json:"id"`
	AgentID           string     `json:"agentId"`
	Name              string     `json:"name"`
	PrimaryHost       string     `json:"primaryHost,omitempty"`
	Status            string     `json:"status"`
	DiscoverySource   string     `json:"discoverySource,omitempty"`
	CertificateStatus string     `json:"certificateStatus,omitempty"`
	Enabled           bool       `json:"enabled"`
	DefaultServer     bool       `json:"defaultServer"`
	TodayRequests     int64      `json:"todayRequests"`
	TodayBlocked      int64      `json:"todayBlocked"`
	LastDiscoveredAt  *time.Time `json:"lastDiscoveredAt,omitempty"`
	LastSeenAt        *time.Time `json:"lastSeenAt,omitempty"`
}

type NginxMetricsObservation struct {
	Timestamp         *time.Time `json:"timestamp,omitempty"`
	Stale             bool       `json:"stale"`
	NotDetected       bool       `json:"notDetected"`
	TotalRequests     int64      `json:"totalRequests"`
	QPS               float64    `json:"qps"`
	Status2xx         int64      `json:"status2xx"`
	Status3xx         int64      `json:"status3xx"`
	Status4xx         int64      `json:"status4xx"`
	Status5xx         int64      `json:"status5xx"`
	ActiveConnections int        `json:"activeConnections"`
	P50Ms             *float64   `json:"p50Ms,omitempty"`
	P90Ms             *float64   `json:"p90Ms,omitempty"`
	P99Ms             *float64   `json:"p99Ms,omitempty"`
}

type NginxOverviewObservation struct {
	TrafficAvailable  bool    `json:"trafficAvailable"`
	LatencyAvailable  bool    `json:"latencyAvailable"`
	UpstreamAvailable bool    `json:"upstreamAvailable"`
	TotalRequests     int64   `json:"totalRequests,omitempty"`
	QPS               float64 `json:"qps,omitempty"`
	ErrorRate         float64 `json:"errorRate,omitempty"`
	P50Ms             float64 `json:"p50Ms,omitempty"`
	P90Ms             float64 `json:"p90Ms,omitempty"`
	P99Ms             float64 `json:"p99Ms,omitempty"`
	UpstreamTotal     int     `json:"upstreamTotal,omitempty"`
	UpstreamHealthy   int     `json:"upstreamHealthy,omitempty"`
	UpstreamDegraded  int     `json:"upstreamDegraded,omitempty"`
	UpstreamUnhealthy int     `json:"upstreamUnhealthy,omitempty"`
}

type NginxUpstreamObservation struct {
	Group           string  `json:"group"`
	RequestCount    int64   `json:"requestCount"`
	AvgResponseTime float64 `json:"avgResponseTime"`
	ErrorCount      int64   `json:"errorCount"`
	ErrorRate       float64 `json:"errorRate"`
	Status          string  `json:"status"`
}

type NginxSlowRequestSummary struct {
	Time           *time.Time `json:"time,omitempty"`
	Method         string     `json:"method,omitempty"`
	Status         int        `json:"status"`
	ResponseTimeMS float64    `json:"responseTimeMs"`
	UpstreamTimeMS float64    `json:"upstreamTimeMs,omitempty"`
	UpstreamStatus int        `json:"upstreamStatus,omitempty"`
}

type NginxResponseTimeSummary struct {
	P50Ms   float64              `json:"p50Ms"`
	P90Ms   float64              `json:"p90Ms"`
	P99Ms   float64              `json:"p99Ms"`
	Buckets []NginxBucketSummary `json:"buckets,omitempty"`
}

type NginxBucketSummary struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

var nginxObserveViews = map[string]struct{}{
	"sites": {}, "site": {}, "overview": {}, "latest": {}, "upstream": {}, "slow_requests": {}, "response_time": {},
}

func (c *Client) ObserveNginx(ctx context.Context, options NginxObserveOptions) (NginxObserveResult, error) {
	view := strings.ToLower(strings.TrimSpace(options.View))
	if view == "" {
		view = "overview"
	}
	if !isAllowedReadValue(view, nginxObserveViews) {
		return NginxObserveResult{}, newInputError("nginx view is not supported")
	}
	result := NginxObserveResult{ReadResultBoundary: newReadResultBoundary("Nginx configuration contents, client addresses, complete URLs, credentials and raw slow-request details are excluded. Missing detail is not evidence of failure."), View: view}
	switch view {
	case "sites":
		return c.observeNginxSites(ctx, options, result)
	case "site":
		return c.observeNginxSite(ctx, options, result)
	case "overview":
		return c.observeNginxOverview(ctx, options, result)
	case "latest":
		return c.observeNginxLatest(ctx, options, result)
	case "upstream":
		return c.observeNginxUpstream(ctx, options, result)
	case "slow_requests":
		return c.observeNginxSlowRequests(ctx, options, result)
	default:
		return c.observeNginxResponseTime(ctx, options, result)
	}
}

type nginxSiteRecord struct {
	ID                string     `json:"id"`
	AgentID           string     `json:"agentId"`
	Name              string     `json:"name"`
	PrimaryHost       string     `json:"primaryHost"`
	Status            string     `json:"status"`
	DiscoverySource   string     `json:"discoverySource"`
	CertificateStatus string     `json:"certificateStatus"`
	Enabled           bool       `json:"enabled"`
	DefaultServer     bool       `json:"defaultServer"`
	TodayRequestCount int64      `json:"todayRequestCount"`
	TodayBlockCount   int64      `json:"todayBlockCount"`
	LastDiscoveredAt  time.Time  `json:"lastDiscoveredAt"`
	LastSeenAt        *time.Time `json:"lastSeenAt"`
}

func summarizeNginxSite(item nginxSiteRecord) NginxSiteObservation {
	var discoveredAt *time.Time
	if !item.LastDiscoveredAt.IsZero() {
		value := item.LastDiscoveredAt
		discoveredAt = &value
	}
	return NginxSiteObservation{ID: item.ID, AgentID: item.AgentID, Name: trimPublicText(item.Name, 160), PrimaryHost: trimPublicText(item.PrimaryHost, 255), Status: trimPublicText(item.Status, 40), DiscoverySource: trimPublicText(item.DiscoverySource, 80), CertificateStatus: trimPublicText(item.CertificateStatus, 40), Enabled: item.Enabled, DefaultServer: item.DefaultServer, TodayRequests: item.TodayRequestCount, TodayBlocked: item.TodayBlockCount, LastDiscoveredAt: discoveredAt, LastSeenAt: item.LastSeenAt}
}

func (c *Client) observeNginxSites(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	query := url.Values{}
	if options.AgentID != "" {
		id, err := validateUUID(options.AgentID, "agent ID")
		if err != nil {
			return NginxObserveResult{}, err
		}
		query.Set("agent_id", id)
	}
	limit := options.PageSize
	if limit == 0 {
		limit = 50
	}
	if limit < 1 || limit > maxObserveItems {
		return NginxObserveResult{}, newInputError("nginx site limit must be between 1 and 50")
	}
	query.Set("limit", strconv.Itoa(limit))
	var data []nginxSiteRecord
	if err := c.do(ctx, http.MethodGet, []string{"nginx", "sites"}, query, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	for i, item := range data {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		result.Sites = append(result.Sites, summarizeNginxSite(item))
	}
	return result, nil
}

func (c *Client) observeNginxSite(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.SiteID, "Nginx site ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	var data nginxSiteRecord
	if err := c.do(ctx, http.MethodGet, []string{"nginx", "sites", id}, nil, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	item := summarizeNginxSite(data)
	result.Site = &item
	return result, nil
}

type nginxLatestRecord struct {
	AgentID           string     `json:"agentId"`
	Timestamp         *time.Time `json:"timestamp"`
	IsStale           bool       `json:"isStale"`
	NginxNotDetected  bool       `json:"nginxNotDetected"`
	TotalRequests     int64      `json:"totalRequests"`
	QPS               float64    `json:"qps"`
	Status2xx         int64      `json:"status2xx"`
	Status3xx         int64      `json:"status3xx"`
	Status4xx         int64      `json:"status4xx"`
	Status5xx         int64      `json:"status5xx"`
	ActiveConnections int        `json:"activeConnections"`
	P50Ms             *float64   `json:"p50Ms"`
	P90Ms             *float64   `json:"p90Ms"`
	P99Ms             *float64   `json:"p99Ms"`
}

func (c *Client) observeNginxLatest(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	var data nginxLatestRecord
	if err := c.do(ctx, http.MethodGet, []string{"agents", id, "nginx", "latest"}, nil, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	result.Metrics = &NginxMetricsObservation{Timestamp: data.Timestamp, Stale: data.IsStale, NotDetected: data.NginxNotDetected, TotalRequests: data.TotalRequests, QPS: data.QPS, Status2xx: data.Status2xx, Status3xx: data.Status3xx, Status4xx: data.Status4xx, Status5xx: data.Status5xx, ActiveConnections: data.ActiveConnections, P50Ms: data.P50Ms, P90Ms: data.P90Ms, P99Ms: data.P99Ms}
	return result, nil
}

type nginxOverviewRecord struct {
	AgentID string `json:"agentId"`
	Traffic *struct {
		Timestamp     *time.Time `json:"timestamp"`
		IsStale       bool       `json:"isStale"`
		TotalRequests int64      `json:"totalRequests"`
		QPS           float64    `json:"qps"`
		ErrorRate     float64    `json:"errorRate"`
	} `json:"traffic"`
	Latency *struct {
		P50Ms float64 `json:"p50Ms"`
		P90Ms float64 `json:"p90Ms"`
		P99Ms float64 `json:"p99Ms"`
	} `json:"latency"`
	Upstream *struct {
		Total     int `json:"total"`
		Healthy   int `json:"healthy"`
		Degraded  int `json:"degraded"`
		Unhealthy int `json:"unhealthy"`
	} `json:"upstream"`
}

func (c *Client) observeNginxOverview(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	var data nginxOverviewRecord
	if err := c.do(ctx, http.MethodGet, []string{"agents", id, "analysis", "nginx", "overview"}, nil, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	item := &NginxOverviewObservation{}
	if data.Traffic != nil {
		item.TrafficAvailable = true
		item.TotalRequests = data.Traffic.TotalRequests
		item.QPS = data.Traffic.QPS
		item.ErrorRate = data.Traffic.ErrorRate
	}
	if data.Latency != nil {
		item.LatencyAvailable = true
		item.P50Ms = data.Latency.P50Ms
		item.P90Ms = data.Latency.P90Ms
		item.P99Ms = data.Latency.P99Ms
	}
	if data.Upstream != nil {
		item.UpstreamAvailable = true
		item.UpstreamTotal = data.Upstream.Total
		item.UpstreamHealthy = data.Upstream.Healthy
		item.UpstreamDegraded = data.Upstream.Degraded
		item.UpstreamUnhealthy = data.Upstream.Unhealthy
	}
	result.Overview = item
	return result, nil
}

type nginxUpstreamRecord struct {
	UpstreamGroup   string  `json:"upstreamGroup"`
	RequestCount    int64   `json:"requestCount"`
	AvgResponseTime float64 `json:"avgResponseTime"`
	ErrorCount      int64   `json:"errorCount"`
	ErrorRate       float64 `json:"errorRate"`
	Status          string  `json:"status"`
}

func (c *Client) observeNginxUpstream(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	var data struct {
		Items []nginxUpstreamRecord `json:"items"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"agents", id, "nginx", "upstream"}, nil, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		result.Upstreams = append(result.Upstreams, NginxUpstreamObservation{Group: trimPublicText(item.UpstreamGroup, 160), RequestCount: item.RequestCount, AvgResponseTime: item.AvgResponseTime, ErrorCount: item.ErrorCount, ErrorRate: item.ErrorRate, Status: trimPublicText(item.Status, 40)})
	}
	return result, nil
}

func nginxWindow(options NginxObserveOptions) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	from := now.Add(-time.Hour)
	if options.From != nil {
		from = options.From.UTC()
	}
	to := now
	if options.To != nil {
		to = options.To.UTC()
	}
	if from.After(to) {
		return time.Time{}, time.Time{}, newInputError("nginx from must not be after to")
	}
	if to.Sub(from) > 7*24*time.Hour {
		return time.Time{}, time.Time{}, newInputError("nginx observation window must not exceed 7 days")
	}
	return from, to, nil
}
func setNginxWindow(query url.Values, from, to time.Time) {
	query.Set("from", from.Format(time.RFC3339))
	query.Set("to", to.Format(time.RFC3339))
}

type nginxSlowRecord struct {
	Time           time.Time `json:"time"`
	Method         string    `json:"method"`
	Status         int       `json:"status"`
	ResponseTime   float64   `json:"responseTime"`
	UpstreamTime   float64   `json:"upstreamTime"`
	UpstreamStatus int       `json:"upstreamStatus"`
}

func (c *Client) observeNginxSlowRequests(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	from, to, err := nginxWindow(options)
	if err != nil {
		return NginxObserveResult{}, err
	}
	page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
	if err != nil {
		return NginxObserveResult{}, err
	}
	query := url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(pageSize)}}
	setNginxWindow(query, from, to)
	var data struct {
		Items    []nginxSlowRecord `json:"items"`
		Total    int               `json:"total"`
		Page     int               `json:"page"`
		PageSize int               `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"agents", id, "nginx", "slow-requests"}, query, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		t := item.Time
		result.SlowRequests = append(result.SlowRequests, NginxSlowRequestSummary{Time: &t, Method: trimPublicText(item.Method, 12), Status: item.Status, ResponseTimeMS: item.ResponseTime * 1000, UpstreamTimeMS: item.UpstreamTime * 1000, UpstreamStatus: item.UpstreamStatus})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}

type nginxBucketRecord struct {
	Label string `json:"label"`
	Count int64  `json:"count"`
}

func (c *Client) observeNginxResponseTime(ctx context.Context, options NginxObserveOptions, result NginxObserveResult) (NginxObserveResult, error) {
	id, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return NginxObserveResult{}, err
	}
	from, to, err := nginxWindow(options)
	if err != nil {
		return NginxObserveResult{}, err
	}
	query := url.Values{}
	setNginxWindow(query, from, to)
	var data struct {
		P50Ms   float64             `json:"p50Ms"`
		P90Ms   float64             `json:"p90Ms"`
		P99Ms   float64             `json:"p99Ms"`
		Buckets []nginxBucketRecord `json:"buckets"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"agents", id, "nginx", "response-time-distribution"}, query, nil, &data, true); err != nil {
		return NginxObserveResult{}, err
	}
	result.ResponseTime = &NginxResponseTimeSummary{P50Ms: data.P50Ms, P90Ms: data.P90Ms, P99Ms: data.P99Ms}
	for i, item := range data.Buckets {
		if i >= 20 {
			result.Truncated = true
			break
		}
		result.ResponseTime.Buckets = append(result.ResponseTime.Buckets, NginxBucketSummary{Label: trimPublicText(item.Label, 40), Count: item.Count})
	}
	return result, nil
}

// SecurityObserveOptions 选择一个固定的安全治理观察视图。
type SecurityObserveOptions struct {
	View     string
	AgentID  string
	Page     int
	PageSize int
}

type SecurityObserveResult struct {
	ReadResultBoundary
	View             string                    `json:"view"`
	ExposureOverview *SecurityExposureOverview `json:"exposureOverview,omitempty"`
	NetworkOverview  *SecurityNetworkOverview  `json:"networkOverview,omitempty"`
	Items            []SecurityObservationItem `json:"items,omitempty"`
	Page             *ReadPageMeta             `json:"page,omitempty"`
}
type SecurityExposureOverview struct {
	TotalFindings    int `json:"totalFindings"`
	CriticalFindings int `json:"criticalFindings"`
	HighFindings     int `json:"highFindings"`
	MediumFindings   int `json:"mediumFindings"`
	AffectedAgents   int `json:"affectedAgents"`
	Governable       int `json:"governable"`
	NeedConfirmation int `json:"needConfirmation"`
}
type SecurityNetworkOverview struct {
	Agents        int `json:"agents"`
	Observations  int `json:"observations"`
	Paths         int `json:"paths"`
	Risks         int `json:"risks"`
	CriticalRisks int `json:"criticalRisks"`
	HighRisks     int `json:"highRisks"`
	StaleAgents   int `json:"staleAgents"`
	Warnings      int `json:"warnings"`
}
type SecurityObservationItem struct {
	ID          string     `json:"id,omitempty"`
	Code        string     `json:"code,omitempty"`
	Severity    string     `json:"severity,omitempty"`
	Status      string     `json:"status,omitempty"`
	Category    string     `json:"category,omitempty"`
	SourceType  string     `json:"sourceType,omitempty"`
	Protocol    string     `json:"protocol,omitempty"`
	Summary     string     `json:"summary,omitempty"`
	Count       int64      `json:"count,omitempty"`
	ObservedAt  *time.Time `json:"observedAt,omitempty"`
	FirstSeenAt *time.Time `json:"firstSeenAt,omitempty"`
	LastSeenAt  *time.Time `json:"lastSeenAt,omitempty"`
}

var securityObserveViews = map[string]struct{}{"exposure_overview": {}, "exposure_findings": {}, "exposure_scans": {}, "network_overview": {}, "network_observations": {}, "network_paths": {}, "network_risks": {}}

func (c *Client) ObserveSecurity(ctx context.Context, options SecurityObserveOptions) (SecurityObserveResult, error) {
	view := strings.ToLower(strings.TrimSpace(options.View))
	if view == "" {
		view = "exposure_overview"
	}
	if !isAllowedReadValue(view, securityObserveViews) {
		return SecurityObserveResult{}, newInputError("security view is not supported")
	}
	result := SecurityObserveResult{ReadResultBoundary: newReadResultBoundary("Security observations contain bounded risk summaries only. Addresses, paths, process details and raw evidence are excluded."), View: view}
	switch view {
	case "exposure_overview":
		return c.observeExposureOverview(ctx, result)
	case "exposure_findings":
		return c.observeExposureFindings(ctx, options, result)
	case "exposure_scans":
		return c.observeExposureScans(ctx, options, result)
	case "network_overview":
		return c.observeNetworkOverview(ctx, options, result)
	case "network_observations":
		return c.observeNetworkObservations(ctx, options, result)
	case "network_paths":
		return c.observeNetworkPaths(ctx, options, result)
	default:
		return c.observeNetworkRisks(ctx, options, result)
	}
}

func (c *Client) observeExposureOverview(ctx context.Context, result SecurityObserveResult) (SecurityObserveResult, error) {
	var data struct {
		Summary struct {
			TotalFindings         int `json:"totalFindings"`
			CriticalFindings      int `json:"criticalFindings"`
			HighFindings          int `json:"highFindings"`
			MediumFindings        int `json:"mediumFindings"`
			AffectedAgentCount    int `json:"affectedAgentCount"`
			GovernableCount       int `json:"governableCount"`
			NeedConfirmationCount int `json:"needConfirmationCount"`
		} `json:"summary"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "exposure", "overview"}, nil, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	result.ExposureOverview = &SecurityExposureOverview{TotalFindings: data.Summary.TotalFindings, CriticalFindings: data.Summary.CriticalFindings, HighFindings: data.Summary.HighFindings, MediumFindings: data.Summary.MediumFindings, AffectedAgents: data.Summary.AffectedAgentCount, Governable: data.Summary.GovernableCount, NeedConfirmation: data.Summary.NeedConfirmationCount}
	return result, nil
}

func securityPage(options SecurityObserveOptions) (int, int, error) {
	return normalizeReadPage(options.Page, options.PageSize)
}
func securityQuery(options SecurityObserveOptions) (url.Values, int, int, error) {
	page, size, err := securityPage(options)
	if err != nil {
		return nil, 0, 0, err
	}
	return url.Values{"page": {strconv.Itoa(page)}, "page_size": {strconv.Itoa(size)}}, page, size, nil
}
func (c *Client) observeExposureFindings(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query, page, size, err := securityQuery(options)
	if err != nil {
		return SecurityObserveResult{}, err
	}
	var data struct {
		Items []struct {
			ID              string    `json:"id"`
			Code            string    `json:"code"`
			Title           string    `json:"title"`
			Severity        string    `json:"severity"`
			Category        string    `json:"category"`
			Status          string    `json:"status"`
			EvidenceSummary string    `json:"evidenceSummary"`
			FirstSeenAt     time.Time `json:"firstSeenAt"`
			LastSeenAt      time.Time `json:"lastSeenAt"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "exposure", "findings"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		summary, redacted, truncated := redactBoundedReadText(item.EvidenceSummary, maxObserveText)
		result.RedactionApplied = result.RedactionApplied || redacted
		result.Truncated = result.Truncated || truncated
		result.Items = append(result.Items, SecurityObservationItem{ID: item.ID, Code: trimPublicText(item.Code, 100), Severity: trimPublicText(item.Severity, 40), Status: trimPublicText(item.Status, 40), Category: trimPublicText(item.Category, 80), Summary: summary, FirstSeenAt: &item.FirstSeenAt, LastSeenAt: &item.LastSeenAt})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, size)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}
func (c *Client) observeExposureScans(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query, page, size, err := securityQuery(options)
	if err != nil {
		return SecurityObserveResult{}, err
	}
	var data struct {
		Items []struct {
			ID            string     `json:"id"`
			Status        string     `json:"status"`
			FindingCount  int        `json:"findingCount"`
			CriticalCount int        `json:"criticalCount"`
			HighCount     int        `json:"highCount"`
			MediumCount   int        `json:"mediumCount"`
			StartedAt     time.Time  `json:"startedAt"`
			FinishedAt    *time.Time `json:"finishedAt"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "exposure", "scans"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		result.Items = append(result.Items, SecurityObservationItem{ID: item.ID, Status: trimPublicText(item.Status, 40), Count: int64(item.FindingCount), Summary: fmt.Sprintf("critical=%d high=%d medium=%d", item.CriticalCount, item.HighCount, item.MediumCount), ObservedAt: &item.StartedAt, LastSeenAt: item.FinishedAt})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, size)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}

func (c *Client) observeNetworkOverview(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query := url.Values{}
	if options.AgentID != "" {
		id, err := validateUUID(options.AgentID, "agent ID")
		if err != nil {
			return SecurityObserveResult{}, err
		}
		query.Set("agent_id", id)
	}
	var data struct {
		Summary struct {
			AgentCount       int `json:"agentCount"`
			ObservationCount int `json:"observationCount"`
			PathCount        int `json:"pathCount"`
			RiskCount        int `json:"riskCount"`
			CriticalRisks    int `json:"criticalRisks"`
			HighRisks        int `json:"highRisks"`
			StaleAgents      int `json:"staleAgents"`
			WarningCount     int `json:"warningCount"`
		} `json:"summary"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "network-entry", "overview"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	result.NetworkOverview = &SecurityNetworkOverview{Agents: data.Summary.AgentCount, Observations: data.Summary.ObservationCount, Paths: data.Summary.PathCount, Risks: data.Summary.RiskCount, CriticalRisks: data.Summary.CriticalRisks, HighRisks: data.Summary.HighRisks, StaleAgents: data.Summary.StaleAgents, Warnings: data.Summary.WarningCount}
	return result, nil
}
func (c *Client) networkQuery(options SecurityObserveOptions) (url.Values, int, int, error) {
	query, page, size, err := securityQuery(options)
	if err != nil {
		return nil, 0, 0, err
	}
	if options.AgentID != "" {
		id, err := validateUUID(options.AgentID, "agent ID")
		if err != nil {
			return nil, 0, 0, err
		}
		query.Set("agent_id", id)
	}
	return query, page, size, nil
}
func (c *Client) observeNetworkObservations(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query, page, size, err := c.networkQuery(options)
	if err != nil {
		return SecurityObserveResult{}, err
	}
	var data struct {
		Items []struct {
			ID         string    `json:"id"`
			SourceType string    `json:"sourceType"`
			Protocol   string    `json:"protocol"`
			Managed    bool      `json:"managed"`
			Summary    string    `json:"summary"`
			ObservedAt time.Time `json:"observedAt"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "network-entry", "observations"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		summary, redacted, truncated := redactBoundedReadText(item.Summary, maxObserveText)
		result.RedactionApplied = result.RedactionApplied || redacted
		result.Truncated = result.Truncated || truncated
		result.Items = append(result.Items, SecurityObservationItem{ID: item.ID, SourceType: trimPublicText(item.SourceType, 40), Protocol: trimPublicText(item.Protocol, 24), Status: map[bool]string{true: "managed", false: "unmanaged"}[item.Managed], Summary: summary, ObservedAt: &item.ObservedAt})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, size)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}
func (c *Client) observeNetworkPaths(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query, page, size, err := c.networkQuery(options)
	if err != nil {
		return SecurityObserveResult{}, err
	}
	var data struct {
		Items []struct {
			ID         string    `json:"id"`
			Protocol   string    `json:"protocol"`
			Confidence string    `json:"confidence"`
			RiskCodes  []string  `json:"riskCodes"`
			ObservedAt time.Time `json:"observedAt"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "network-entry", "paths"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		codes, _ := trimPublicStringList(item.RiskCodes, 80, 8)
		result.Items = append(result.Items, SecurityObservationItem{ID: item.ID, Protocol: trimPublicText(item.Protocol, 24), Status: trimPublicText(item.Confidence, 40), Code: strings.Join(codes, ","), Count: int64(len(codes)), ObservedAt: &item.ObservedAt})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, size)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}
func (c *Client) observeNetworkRisks(ctx context.Context, options SecurityObserveOptions, result SecurityObserveResult) (SecurityObserveResult, error) {
	query, page, size, err := c.networkQuery(options)
	if err != nil {
		return SecurityObserveResult{}, err
	}
	var data struct {
		Items []struct {
			ID              string    `json:"id"`
			Code            string    `json:"code"`
			Severity        string    `json:"severity"`
			Category        string    `json:"category"`
			Status          string    `json:"status"`
			SourceType      string    `json:"sourceType"`
			EvidenceSummary string    `json:"evidenceSummary"`
			FirstSeenAt     time.Time `json:"firstSeenAt"`
			LastSeenAt      time.Time `json:"lastSeenAt"`
		} `json:"items"`
		Total    int `json:"total"`
		Page     int `json:"page"`
		PageSize int `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"security", "network-entry", "risks"}, query, nil, &data, true); err != nil {
		return SecurityObserveResult{}, err
	}
	for i, item := range data.Items {
		if i >= maxObserveItems {
			result.Truncated = true
			break
		}
		summary, redacted, truncated := redactBoundedReadText(item.EvidenceSummary, maxObserveText)
		result.RedactionApplied = result.RedactionApplied || redacted
		result.Truncated = result.Truncated || truncated
		result.Items = append(result.Items, SecurityObservationItem{ID: item.ID, Code: trimPublicText(item.Code, 100), Severity: trimPublicText(item.Severity, 40), Status: trimPublicText(item.Status, 40), Category: trimPublicText(item.Category, 80), SourceType: trimPublicText(item.SourceType, 40), Summary: summary, FirstSeenAt: &item.FirstSeenAt, LastSeenAt: &item.LastSeenAt})
	}
	actualPage, actualSize := normalizedResponsePage(data.Page, data.PageSize, page, size)
	meta := makeReadPageMeta(data.Total, actualPage, actualSize, result.Truncated)
	result.Page = &meta
	return result, nil
}

// SystemReleaseOptions 保留固定的版本状态查询入口。
type SystemReleaseOptions struct{}
type SystemReleaseResult struct {
	ReadResultBoundary
	Status    SystemReleaseStatus  `json:"status"`
	Changelog []SystemReleaseEntry `json:"changelog,omitempty"`
}
type SystemReleaseStatus struct {
	GeneratedAt           string                   `json:"generatedAt,omitempty"`
	Components            []SystemReleaseComponent `json:"components"`
	UpdateSource          string                   `json:"updateSource"`
	UpdateSourceAvailable bool                     `json:"updateSourceAvailable"`
	UpdateSourceMessage   string                   `json:"updateSourceMessage,omitempty"`
}
type SystemReleaseComponent struct {
	Key                   string `json:"key"`
	Label                 string `json:"label,omitempty"`
	CurrentVersion        string `json:"currentVersion,omitempty"`
	LatestVersion         string `json:"latestVersion,omitempty"`
	UpdateAvailable       bool   `json:"updateAvailable"`
	UpgradeSupported      bool   `json:"upgradeSupported"`
	UpgradeDisabledReason string `json:"upgradeDisabledReason,omitempty"`
}
type SystemReleaseEntry struct {
	Component string `json:"component,omitempty"`
	Version   string `json:"version,omitempty"`
	Date      string `json:"date,omitempty"`
	Title     string `json:"title"`
	Summary   string `json:"summary,omitempty"`
	Risk      string `json:"risk,omitempty"`
}

func (c *Client) GetSystemRelease(ctx context.Context, _ SystemReleaseOptions) (SystemReleaseResult, error) {
	result := SystemReleaseResult{ReadResultBoundary: newReadResultBoundary("Release images, digests, internal commits, upgrade commands and source URLs are excluded.")}
	var status struct {
		GeneratedAt string `json:"generatedAt"`
		Components  []struct {
			Key                   string `json:"key"`
			Label                 string `json:"label"`
			CurrentVersion        string `json:"currentVersion"`
			LatestVersion         string `json:"latestVersion"`
			UpdateAvailable       bool   `json:"updateAvailable"`
			UpgradeSupported      bool   `json:"upgradeSupported"`
			UpgradeDisabledReason string `json:"upgradeDisabledReason"`
		} `json:"components"`
		UpdateSource struct {
			Status  string `json:"status"`
			Message string `json:"message"`
		} `json:"updateSource"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"system", "release", "status"}, nil, nil, &status, true); err != nil {
		return SystemReleaseResult{}, err
	}
	result.Status.GeneratedAt = trimPublicText(status.GeneratedAt, 64)
	result.Status.UpdateSource = trimPublicText(status.UpdateSource.Status, 40)
	result.Status.UpdateSourceAvailable = status.UpdateSource.Status == "ok"
	result.Status.UpdateSourceMessage = trimPublicText(status.UpdateSource.Message, maxObserveText)
	for _, item := range status.Components {
		result.Status.Components = append(result.Status.Components, SystemReleaseComponent{Key: trimPublicText(item.Key, 32), Label: trimPublicText(item.Label, 80), CurrentVersion: trimPublicText(item.CurrentVersion, 64), LatestVersion: trimPublicText(item.LatestVersion, 64), UpdateAvailable: item.UpdateAvailable, UpgradeSupported: item.UpgradeSupported, UpgradeDisabledReason: trimPublicText(item.UpgradeDisabledReason, 80)})
	}
	var changelog struct {
		Entries []struct {
			Component string `json:"component"`
			Version   string `json:"version"`
			Date      string `json:"date"`
			Title     string `json:"title"`
			Summary   string `json:"summary"`
			Risk      string `json:"risk"`
		} `json:"entries"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"system", "release", "changelog"}, nil, nil, &changelog, true); err != nil {
		return SystemReleaseResult{}, err
	}
	for i, item := range changelog.Entries {
		if i >= 20 {
			result.Truncated = true
			break
		}
		title, redacted, truncated := redactBoundedReadText(item.Title, maxObserveText)
		summary, redacted2, truncated2 := redactBoundedReadText(item.Summary, maxObserveText)
		risk, redacted3, truncated3 := redactBoundedReadText(item.Risk, 160)
		result.RedactionApplied = result.RedactionApplied || redacted || redacted2 || redacted3
		result.Truncated = result.Truncated || truncated || truncated2 || truncated3
		result.Changelog = append(result.Changelog, SystemReleaseEntry{Component: trimPublicText(item.Component, 32), Version: trimPublicText(item.Version, 64), Date: trimPublicText(item.Date, 32), Title: title, Summary: summary, Risk: risk})
	}
	return result, nil
}

// SubscriptionOptions 保留固定的订阅状态查询入口。
type SubscriptionOptions struct{}
type SubscriptionResult struct {
	ReadResultBoundary
	Status SubscriptionStatusSummary `json:"status"`
	Usage  *SubscriptionUsageSummary `json:"usage,omitempty"`
}
type SubscriptionStatusSummary struct {
	PlanCode        string                         `json:"planCode"`
	LicenseStatus   string                         `json:"licenseStatus"`
	Status          string                         `json:"status"`
	BillingCycle    string                         `json:"billingCycle"`
	ValidUntil      *time.Time                     `json:"validUntil,omitempty"`
	TelemetryPolicy string                         `json:"telemetryPolicy"`
	Features        []SubscriptionFeatureSummary   `json:"features,omitempty"`
	Limits          []SubscriptionLimitSummary     `json:"limits,omitempty"`
	Restriction     SubscriptionRestrictionSummary `json:"restriction"`
}
type SubscriptionFeatureSummary struct {
	FeatureKey   string `json:"featureKey"`
	Mode         string `json:"mode"`
	ReasonCode   string `json:"reasonCode,omitempty"`
	RequiredPlan string `json:"requiredPlan,omitempty"`
}
type SubscriptionLimitSummary struct {
	LimitKey   string     `json:"limitKey"`
	LimitValue *int64     `json:"limitValue,omitempty"`
	UsedValue  int64      `json:"usedValue"`
	ResetAt    *time.Time `json:"resetAt,omitempty"`
}
type SubscriptionRestrictionSummary struct {
	Active           bool   `json:"active"`
	ReasonCode       string `json:"reasonCode,omitempty"`
	Mode             string `json:"mode"`
	LimitedFeatures  int    `json:"limitedFeatures"`
	DisabledFeatures int    `json:"disabledFeatures"`
	BlockedLimits    int    `json:"blockedLimits"`
}
type SubscriptionUsageSummary struct {
	Counters   []SubscriptionCounterSummary `json:"counters"`
	WindowFrom time.Time                    `json:"windowFrom"`
	WindowTo   time.Time                    `json:"windowTo"`
}
type SubscriptionCounterSummary struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value int64  `json:"value"`
}

func (c *Client) GetSubscription(ctx context.Context, _ SubscriptionOptions) (SubscriptionResult, error) {
	result := SubscriptionResult{ReadResultBoundary: newReadResultBoundary("Installation identity, license material, recovery targets and upgrade URLs are excluded.")}
	var status struct {
		PlanCode        string     `json:"planCode"`
		LicenseStatus   string     `json:"licenseStatus"`
		Status          string     `json:"status"`
		BillingCycle    string     `json:"billingCycle"`
		ValidUntil      *time.Time `json:"validUntil"`
		TelemetryPolicy string     `json:"telemetryPolicy"`
		Features        []struct {
			FeatureKey   string `json:"featureKey"`
			Mode         string `json:"mode"`
			ReasonCode   string `json:"reasonCode"`
			RequiredPlan string `json:"requiredPlan"`
		} `json:"features"`
		Limits []struct {
			LimitKey   string     `json:"limitKey"`
			LimitValue *int64     `json:"limitValue"`
			UsedValue  int64      `json:"usedValue"`
			ResetAt    *time.Time `json:"resetAt"`
		} `json:"limits"`
		Restriction struct {
			Active           bool   `json:"active"`
			ReasonCode       string `json:"reasonCode"`
			Mode             string `json:"mode"`
			LimitedFeatures  int    `json:"limitedFeatures"`
			DisabledFeatures int    `json:"disabledFeatures"`
			BlockedLimits    int    `json:"blockedLimits"`
		} `json:"restriction"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"subscription", "status"}, nil, nil, &status, true); err != nil {
		return SubscriptionResult{}, err
	}
	result.Status = SubscriptionStatusSummary{PlanCode: trimPublicText(status.PlanCode, 40), LicenseStatus: trimPublicText(status.LicenseStatus, 40), Status: trimPublicText(status.Status, 40), BillingCycle: trimPublicText(status.BillingCycle, 40), ValidUntil: status.ValidUntil, TelemetryPolicy: trimPublicText(status.TelemetryPolicy, 80), Restriction: SubscriptionRestrictionSummary{Active: status.Restriction.Active, ReasonCode: trimPublicText(status.Restriction.ReasonCode, 100), Mode: trimPublicText(status.Restriction.Mode, 40), LimitedFeatures: status.Restriction.LimitedFeatures, DisabledFeatures: status.Restriction.DisabledFeatures, BlockedLimits: status.Restriction.BlockedLimits}}
	for _, item := range status.Features {
		result.Status.Features = append(result.Status.Features, SubscriptionFeatureSummary{FeatureKey: trimPublicText(item.FeatureKey, 120), Mode: trimPublicText(item.Mode, 40), ReasonCode: trimPublicText(item.ReasonCode, 100), RequiredPlan: trimPublicText(item.RequiredPlan, 40)})
	}
	for _, item := range status.Limits {
		result.Status.Limits = append(result.Status.Limits, SubscriptionLimitSummary{LimitKey: trimPublicText(item.LimitKey, 120), LimitValue: item.LimitValue, UsedValue: item.UsedValue, ResetAt: item.ResetAt})
	}
	var usage struct {
		Counters []struct {
			Key   string `json:"key"`
			Label string `json:"label"`
			Value int64  `json:"value"`
		} `json:"counters"`
		WindowFrom time.Time `json:"windowFrom"`
		WindowTo   time.Time `json:"windowTo"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"subscription", "usage"}, nil, nil, &usage, true); err != nil {
		return SubscriptionResult{}, err
	}
	result.Usage = &SubscriptionUsageSummary{WindowFrom: usage.WindowFrom, WindowTo: usage.WindowTo}
	for i, item := range usage.Counters {
		if i >= 50 {
			result.Truncated = true
			break
		}
		result.Usage.Counters = append(result.Usage.Counters, SubscriptionCounterSummary{Key: trimPublicText(item.Key, 120), Label: trimPublicText(item.Label, 160), Value: item.Value})
	}
	return result, nil
}
