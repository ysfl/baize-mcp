package baize

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	alertStatuses   = map[string]struct{}{"open": {}, "acknowledged": {}, "resolved": {}}
	alertSeverities = map[string]struct{}{"critical": {}, "error": {}, "warning": {}, "info": {}}
	assetViews      = map[string]struct{}{"list": {}, "summary": {}, "expiring": {}, "detail": {}}
)

type AlertsListOptions struct {
	Page     int
	PageSize int
	Status   string
	Severity string
}

type AlertIncidentPage struct {
	ReadResultBoundary
	ReadPageMeta
	Items []AlertIncidentSummary `json:"items"`
}

type AlertIncidentSummary struct {
	ID                  string     `json:"id"`
	Source              string     `json:"source,omitempty"`
	EventType           string     `json:"eventType,omitempty"`
	DiagnosisAgentID    string     `json:"diagnosisAgentId,omitempty"`
	DiagnosisTargetType string     `json:"diagnosisTargetType,omitempty"`
	Title               string     `json:"title"`
	Message             string     `json:"message,omitempty"`
	Severity            string     `json:"severity"`
	Status              string     `json:"status"`
	CurrentStep         int        `json:"currentStep"`
	TriggeredAt         *time.Time `json:"triggeredAt,omitempty"`
	AcknowledgedAt      *time.Time `json:"acknowledgedAt,omitempty"`
	NextEscalationAt    *time.Time `json:"nextEscalationAt,omitempty"`
	ResolvedAt          *time.Time `json:"resolvedAt,omitempty"`
	CreatedAt           *time.Time `json:"createdAt,omitempty"`
}

type alertIncidentRecord struct {
	ID                   string     `json:"id"`
	RuleID               string     `json:"ruleId"`
	EscalationPolicyID   string     `json:"escalationPolicyId"`
	Source               string     `json:"source"`
	EventType            string     `json:"eventType"`
	ResourceID           string     `json:"resourceId"`
	DiagnosisAgentID     string     `json:"diagnosisAgentId"`
	DiagnosisTargetType  string     `json:"diagnosisTargetType"`
	DiagnosisTargetValue string     `json:"diagnosisTargetValue"`
	Title                string     `json:"title"`
	Message              string     `json:"message"`
	Severity             string     `json:"severity"`
	Status               string     `json:"status"`
	CurrentStep          int        `json:"currentStep"`
	TriggeredAt          *time.Time `json:"triggeredAt"`
	AcknowledgedAt       *time.Time `json:"acknowledgedAt"`
	AcknowledgedBy       string     `json:"acknowledgedBy"`
	NextEscalationAt     *time.Time `json:"nextEscalationAt"`
	ResolvedAt           *time.Time `json:"resolvedAt"`
	CreatedAt            *time.Time `json:"createdAt"`
}

func (c *Client) ListAlerts(ctx context.Context, options AlertsListOptions) (AlertIncidentPage, error) {
	page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
	if err != nil {
		return AlertIncidentPage{}, err
	}
	status := strings.ToLower(strings.TrimSpace(options.Status))
	severity := strings.ToLower(strings.TrimSpace(options.Severity))
	if !isAllowedReadValue(status, alertStatuses) {
		return AlertIncidentPage{}, newInputError("alert status must be open, acknowledged, or resolved")
	}
	if !isAllowedReadValue(severity, alertSeverities) {
		return AlertIncidentPage{}, newInputError("alert severity must be critical, error, warning, or info")
	}
	query := url.Values{"page": {fmt.Sprintf("%d", page)}, "page_size": {fmt.Sprintf("%d", pageSize)}}
	setOptionalQuery(query, "status", status)
	setOptionalQuery(query, "severity", severity)
	var data struct {
		Items    []alertIncidentRecord `json:"items"`
		Total    int                   `json:"total"`
		Page     int                   `json:"page"`
		PageSize int                   `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"alerts", "incidents"}, query, nil, &data, true); err != nil {
		return AlertIncidentPage{}, err
	}
	boundary := newReadResultBoundary("Alert messages are bounded and conservatively redacted. Acknowledging user identity, resource identifiers and diagnosis target values are excluded.")
	items := make([]AlertIncidentSummary, 0, minInt(len(data.Items), maxReadPageSize))
	for index, item := range data.Items {
		if index >= maxReadPageSize {
			boundary.Truncated = true
			break
		}
		title, titleChanged, titleTruncated := redactBoundedReadText(item.Title, maxReadTextLength)
		message, messageChanged, messageTruncated := redactBoundedReadText(item.Message, maxReadTextLength)
		boundary.RedactionApplied = boundary.RedactionApplied || titleChanged || messageChanged
		boundary.Truncated = boundary.Truncated || titleTruncated || messageTruncated
		items = append(items, AlertIncidentSummary{ID: item.ID, Source: trimPublicText(item.Source, 80), EventType: trimPublicText(item.EventType, 100), DiagnosisAgentID: item.DiagnosisAgentID, DiagnosisTargetType: trimPublicText(item.DiagnosisTargetType, 40), Title: title, Message: message, Severity: trimPublicText(item.Severity, 40), Status: trimPublicText(item.Status, 40), CurrentStep: item.CurrentStep, TriggeredAt: item.TriggeredAt, AcknowledgedAt: item.AcknowledgedAt, NextEscalationAt: item.NextEscalationAt, ResolvedAt: item.ResolvedAt, CreatedAt: item.CreatedAt})
	}
	actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
	return AlertIncidentPage{ReadResultBoundary: boundary, ReadPageMeta: makeReadPageMeta(data.Total, actualPage, actualPageSize, boundary.Truncated), Items: items}, nil
}

type CertificatesListOptions struct {
	Page     int
	PageSize int
	Search   string
}

type CertificateTargetPage struct {
	ReadResultBoundary
	ReadPageMeta
	Items []CertificateTargetSummary `json:"items"`
}

type CertificateTargetSummary struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Host             string               `json:"host"`
	Port             int                  `json:"port"`
	AssetID          string               `json:"assetId,omitempty"`
	Enabled          bool                 `json:"enabled"`
	Source           string               `json:"source"`
	AgentID          string               `json:"agentId,omitempty"`
	NginxSiteID      string               `json:"nginxSiteId,omitempty"`
	LastDiscoveredAt *time.Time           `json:"lastDiscoveredAt,omitempty"`
	LatestSnapshot   *CertificateSnapshot `json:"latestSnapshot,omitempty"`
}

type CertificateSnapshot struct {
	ScannedAt     *time.Time `json:"scannedAt,omitempty"`
	NotBefore     *time.Time `json:"notBefore,omitempty"`
	NotAfter      *time.Time `json:"notAfter,omitempty"`
	DaysRemaining *int       `json:"daysRemaining,omitempty"`
	Status        string     `json:"status"`
	ErrorMessage  string     `json:"errorMessage,omitempty"`
}

type certificateTargetRecord struct {
	ID               string     `json:"id"`
	Name             string     `json:"name"`
	Host             string     `json:"host"`
	Port             int        `json:"port"`
	AssetID          string     `json:"assetId"`
	Enabled          bool       `json:"enabled"`
	Source           string     `json:"source"`
	AgentID          string     `json:"agentId"`
	NginxSiteID      string     `json:"nginxSiteId"`
	CertificatePath  string     `json:"certificatePath"`
	LastDiscoveredAt *time.Time `json:"lastDiscoveredAt"`
	LatestSnapshot   *struct {
		ScannedAt     *time.Time `json:"scannedAt"`
		Subject       string     `json:"subject"`
		Issuer        string     `json:"issuer"`
		NotBefore     *time.Time `json:"notBefore"`
		NotAfter      *time.Time `json:"notAfter"`
		DaysRemaining *int       `json:"daysRemaining"`
		Status        string     `json:"status"`
		ErrorMessage  string     `json:"errorMessage"`
	} `json:"latestSnapshot"`
}

func (c *Client) ListCertificates(ctx context.Context, options CertificatesListOptions) (CertificateTargetPage, error) {
	page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
	if err != nil {
		return CertificateTargetPage{}, err
	}
	search, err := validateReadFilter(options.Search, "certificate search", maxReadSearchLength)
	if err != nil {
		return CertificateTargetPage{}, err
	}
	query := url.Values{"page": {fmt.Sprintf("%d", page)}, "page_size": {fmt.Sprintf("%d", pageSize)}}
	setOptionalQuery(query, "search", search)
	var data struct {
		Items    []certificateTargetRecord `json:"items"`
		Total    int                       `json:"total"`
		Page     int                       `json:"page"`
		PageSize int                       `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"certificates"}, query, nil, &data, true); err != nil {
		return CertificateTargetPage{}, err
	}
	boundary := newReadResultBoundary("Certificate file paths, certificate subjects and issuers are excluded. Scan errors are bounded and conservatively redacted.")
	items := make([]CertificateTargetSummary, 0, minInt(len(data.Items), maxReadPageSize))
	for index, item := range data.Items {
		if index >= maxReadPageSize {
			boundary.Truncated = true
			break
		}
		summary := CertificateTargetSummary{ID: item.ID, Name: trimPublicText(item.Name, 120), Host: trimPublicText(item.Host, 255), Port: item.Port, AssetID: item.AssetID, Enabled: item.Enabled, Source: trimPublicText(item.Source, 40), AgentID: item.AgentID, NginxSiteID: item.NginxSiteID, LastDiscoveredAt: item.LastDiscoveredAt}
		if item.LatestSnapshot != nil {
			errorMessage, changed, truncated := redactBoundedReadText(item.LatestSnapshot.ErrorMessage, maxReadTextLength)
			boundary.RedactionApplied = boundary.RedactionApplied || changed
			boundary.Truncated = boundary.Truncated || truncated
			summary.LatestSnapshot = &CertificateSnapshot{ScannedAt: item.LatestSnapshot.ScannedAt, NotBefore: item.LatestSnapshot.NotBefore, NotAfter: item.LatestSnapshot.NotAfter, DaysRemaining: item.LatestSnapshot.DaysRemaining, Status: trimPublicText(item.LatestSnapshot.Status, 40), ErrorMessage: errorMessage}
		}
		items = append(items, summary)
	}
	actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
	return CertificateTargetPage{ReadResultBoundary: boundary, ReadPageMeta: makeReadPageMeta(data.Total, actualPage, actualPageSize, boundary.Truncated), Items: items}, nil
}

type AssetsQueryOptions struct {
	View        string
	ID          string
	Page        int
	PageSize    int
	Status      string
	Environment string
	Provider    string
	Search      string
	Days        int
}

type AssetQueryResult struct {
	ReadResultBoundary
	View    string        `json:"view"`
	Page    *ReadPageMeta `json:"page,omitempty"`
	Items   []AssetItem   `json:"items,omitempty"`
	Detail  *AssetItem    `json:"detail,omitempty"`
	Summary *AssetSummary `json:"summary,omitempty"`
}

type AssetItem struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	Hostname              string     `json:"hostname,omitempty"`
	Provider              string     `json:"provider,omitempty"`
	Region                string     `json:"region,omitempty"`
	Purpose               string     `json:"purpose,omitempty"`
	Environment           string     `json:"environment,omitempty"`
	Status                string     `json:"status"`
	PurchasedAt           *time.Time `json:"purchasedAt,omitempty"`
	ExpiresAt             *time.Time `json:"expiresAt,omitempty"`
	BillingCycle          string     `json:"billingCycle,omitempty"`
	Amount                *float64   `json:"amount,omitempty"`
	Currency              string     `json:"currency,omitempty"`
	BandwidthLimitMbps    *float64   `json:"bandwidthLimitMbps,omitempty"`
	MonthlyTrafficQuotaGB *float64   `json:"monthlyTrafficQuotaGb,omitempty"`
	AutoRenew             bool       `json:"autoRenew"`
	DueLevel              string     `json:"dueLevel,omitempty"`
	DaysLeft              *int       `json:"daysLeft,omitempty"`
	AgentID               string     `json:"agentId,omitempty"`
	AgentName             string     `json:"agentName,omitempty"`
	Abnormal              bool       `json:"abnormal,omitempty"`
}

type AssetSummary struct {
	Total                   int64               `json:"total"`
	Active                  int64               `json:"active"`
	Archived                int64               `json:"archived"`
	Bound                   int64               `json:"bound"`
	Unbound                 int64               `json:"unbound"`
	Abnormal                int64               `json:"abnormal"`
	Expiring30d             int                 `json:"expiring30d"`
	Expiring7d              int                 `json:"expiring7d"`
	Expiring1d              int                 `json:"expiring1d"`
	ProviderDistribution    []AssetDistribution `json:"providerDistribution,omitempty"`
	StatusDistribution      []AssetDistribution `json:"statusDistribution,omitempty"`
	EnvironmentDistribution []AssetDistribution `json:"environmentDistribution,omitempty"`
	Cost                    AssetCostSummary    `json:"cost"`
}

type AssetDistribution struct {
	Key   string  `json:"key"`
	Count int64   `json:"count"`
	Ratio float64 `json:"ratio"`
}

type AssetCostSummary struct {
	PrimaryCurrency string  `json:"primaryCurrency,omitempty"`
	Monthly         float64 `json:"monthly"`
	Yearly          float64 `json:"yearly"`
}

type assetItemRecord struct {
	ID                    string     `json:"id"`
	Name                  string     `json:"name"`
	PrimaryIP             string     `json:"primaryIp"`
	SecondaryIPs          any        `json:"secondaryIps"`
	Hostname              string     `json:"hostname"`
	Provider              string     `json:"provider"`
	Region                string     `json:"region"`
	Purpose               string     `json:"purpose"`
	Environment           string     `json:"environment"`
	Status                string     `json:"status"`
	PurchasedAt           *time.Time `json:"purchasedAt"`
	ExpiresAt             *time.Time `json:"expiresAt"`
	BillingCycle          string     `json:"billingCycle"`
	Amount                *float64   `json:"amount"`
	Currency              string     `json:"currency"`
	BandwidthLimitMbps    *float64   `json:"bandwidthLimitMbps"`
	MonthlyTrafficQuotaGB *float64   `json:"monthlyTrafficQuotaGb"`
	AutoRenew             bool       `json:"autoRenew"`
	Notes                 string     `json:"notes"`
	DueLevel              string     `json:"dueLevel"`
	DaysLeft              *int       `json:"daysLeft"`
	AgentID               string     `json:"agentId"`
	AgentName             string     `json:"agentName"`
	Summary               struct {
		Abnormal bool `json:"abnormal"`
	} `json:"summary"`
}

func (c *Client) QueryAssets(ctx context.Context, options AssetsQueryOptions) (AssetQueryResult, error) {
	view := strings.ToLower(strings.TrimSpace(options.View))
	if view == "" {
		view = "list"
	}
	if !isAllowedReadValue(view, assetViews) {
		return AssetQueryResult{}, newInputError("asset view must be list, summary, expiring, or detail")
	}
	boundary := newReadResultBoundary("Asset IP addresses, notes, links and credentials are excluded. Results only contain inventory and lifecycle summaries visible to the signed-in account.")
	result := AssetQueryResult{ReadResultBoundary: boundary, View: view}
	switch view {
	case "summary":
		var data AssetSummary
		if err := c.do(ctx, http.MethodGet, []string{"assets", "summary"}, nil, nil, &data, true); err != nil {
			return AssetQueryResult{}, err
		}
		data.ProviderDistribution, result.Truncated = trimAssetDistributions(data.ProviderDistribution, result.Truncated)
		data.StatusDistribution, result.Truncated = trimAssetDistributions(data.StatusDistribution, result.Truncated)
		data.EnvironmentDistribution, result.Truncated = trimAssetDistributions(data.EnvironmentDistribution, result.Truncated)
		result.Summary = &data
	case "detail":
		id, err := validateUUID(options.ID, "asset ID")
		if err != nil {
			return AssetQueryResult{}, err
		}
		var data assetItemRecord
		if err := c.do(ctx, http.MethodGet, []string{"assets", id}, nil, nil, &data, true); err != nil {
			return AssetQueryResult{}, err
		}
		item := summarizeAssetItem(data)
		result.Detail = &item
	case "expiring":
		days := options.Days
		if days == 0 {
			days = 30
		}
		if days < 1 || days > 365 {
			return AssetQueryResult{}, newInputError("asset expiry window must be between 1 and 365 days")
		}
		query := url.Values{"days": {fmt.Sprintf("%d", days)}}
		var data []assetItemRecord
		if err := c.do(ctx, http.MethodGet, []string{"assets", "expiring"}, query, nil, &data, true); err != nil {
			return AssetQueryResult{}, err
		}
		for index, item := range data {
			if index >= maxReadPageSize {
				result.Truncated = true
				break
			}
			result.Items = append(result.Items, summarizeAssetItem(item))
		}
	case "list":
		page, pageSize, err := normalizeReadPage(options.Page, options.PageSize)
		if err != nil {
			return AssetQueryResult{}, err
		}
		query := url.Values{"page": {fmt.Sprintf("%d", page)}, "pageSize": {fmt.Sprintf("%d", pageSize)}}
		for key, raw := range map[string]string{"status": options.Status, "environment": options.Environment, "provider": options.Provider, "search": options.Search} {
			value, err := validateReadFilter(raw, "asset "+key, maxReadSearchLength)
			if err != nil {
				return AssetQueryResult{}, err
			}
			setOptionalQuery(query, key, value)
		}
		var data struct {
			Items    []assetItemRecord `json:"items"`
			Total    int               `json:"total"`
			Page     int               `json:"page"`
			PageSize int               `json:"pageSize"`
		}
		if err := c.do(ctx, http.MethodGet, []string{"assets"}, query, nil, &data, true); err != nil {
			return AssetQueryResult{}, err
		}
		for index, item := range data.Items {
			if index >= maxReadPageSize {
				result.Truncated = true
				break
			}
			result.Items = append(result.Items, summarizeAssetItem(item))
		}
		actualPage, actualPageSize := normalizedResponsePage(data.Page, data.PageSize, page, pageSize)
		pageMeta := makeReadPageMeta(data.Total, actualPage, actualPageSize, result.Truncated)
		result.Page = &pageMeta
	}
	return result, nil
}

func summarizeAssetItem(data assetItemRecord) AssetItem {
	return AssetItem{ID: data.ID, Name: trimPublicText(data.Name, 120), Hostname: trimPublicText(data.Hostname, 255), Provider: trimPublicText(data.Provider, 80), Region: trimPublicText(data.Region, 80), Purpose: trimPublicText(data.Purpose, 120), Environment: trimPublicText(data.Environment, 40), Status: trimPublicText(data.Status, 40), PurchasedAt: data.PurchasedAt, ExpiresAt: data.ExpiresAt, BillingCycle: trimPublicText(data.BillingCycle, 40), Amount: data.Amount, Currency: trimPublicText(data.Currency, 10), BandwidthLimitMbps: data.BandwidthLimitMbps, MonthlyTrafficQuotaGB: data.MonthlyTrafficQuotaGB, AutoRenew: data.AutoRenew, DueLevel: trimPublicText(data.DueLevel, 40), DaysLeft: data.DaysLeft, AgentID: data.AgentID, AgentName: trimPublicText(data.AgentName, 255), Abnormal: data.Summary.Abnormal}
}

func trimAssetDistributions(items []AssetDistribution, alreadyTruncated bool) ([]AssetDistribution, bool) {
	if len(items) > maxReadPageSize {
		alreadyTruncated = true
		items = items[:maxReadPageSize]
	}
	for index := range items {
		items[index].Key = trimPublicText(items[index].Key, 120)
	}
	return items, alreadyTruncated
}

func normalizedResponsePage(actualPage, actualPageSize, requestedPage, requestedPageSize int) (int, int) {
	if actualPage < 1 {
		actualPage = requestedPage
	}
	if actualPageSize < 1 {
		actualPageSize = requestedPageSize
	}
	return actualPage, actualPageSize
}
