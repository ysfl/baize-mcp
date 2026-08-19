package baize

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	ObserveViewHealth       = "health"
	ObserveViewMetrics      = "metrics"
	ObserveViewProcesses    = "processes"
	ObserveViewStorage      = "storage"
	ObserveViewDocker       = "docker"
	ObserveViewNginx        = "nginx"
	ObserveViewHostProfile  = "host_profile"
	ObserveViewControlPlane = "control_plane"

	maxObserveMetricFamilies = 12
	maxObserveMetricPoints   = 8
	maxObserveStorageItems   = 50
	maxObserveReasons        = 8
	maxObserveProcessItems   = 20
	maxObserveTextLength     = 512
	maxObserveNoticeLength   = 1000
)

var observeViews = map[string]struct{}{
	ObserveViewHealth:       {},
	ObserveViewMetrics:      {},
	ObserveViewProcesses:    {},
	ObserveViewStorage:      {},
	ObserveViewDocker:       {},
	ObserveViewNginx:        {},
	ObserveViewHostProfile:  {},
	ObserveViewControlPlane: {},
}

var observeProcessMetrics = map[string]struct{}{
	"cpu": {}, "memory": {}, "read_rate": {}, "write_rate": {}, "rx_rate": {}, "tx_rate": {},
}

// AgentObserveOptions 是单节点观察的受控参数。
// 视图只映射到固定的已发布 API，不接受路径、方法或任意请求体。
type AgentObserveOptions struct {
	AgentID string
	View    string
	Metric  string
	From    *time.Time
	To      *time.Time
	Limit   int
}

// AgentObserveResult 是面向 AI 的有界观察结果。
// ResultMode、Notice 和 RedactionApplied 明确说明结果边界，避免 AI 把缺失字段误判为执行失败。
type AgentObserveResult struct {
	AgentID                          string                        `json:"agentId"`
	View                             string                        `json:"view"`
	ResultMode                       string                        `json:"resultMode"`
	SensitiveContentExcluded         bool                          `json:"sensitiveContentExcluded"`
	UnknownSensitiveContentMayRemain bool                          `json:"unknownSensitiveContentMayRemain"`
	RedactionApplied                 bool                          `json:"redactionApplied"`
	RedactionPolicy                  string                        `json:"redactionPolicy"`
	Truncated                        bool                          `json:"truncated"`
	Notice                           string                        `json:"notice"`
	Health                           *AgentHealthObservation       `json:"health,omitempty"`
	Metrics                          *AgentMetricsObservation      `json:"metrics,omitempty"`
	Processes                        *AgentProcessesObservation    `json:"processes,omitempty"`
	Storage                          *AgentStorageObservation      `json:"storage,omitempty"`
	Docker                           *AgentDockerObservation       `json:"docker,omitempty"`
	Nginx                            *AgentNginxObservation        `json:"nginx,omitempty"`
	HostProfile                      *AgentHostProfileObservation  `json:"hostProfile,omitempty"`
	ControlPlane                     *AgentControlPlaneObservation `json:"controlPlane,omitempty"`
}

type AgentHealthObservation struct {
	DisplayName     string     `json:"displayName"`
	Status          string     `json:"status"`
	OperatingSystem string     `json:"operatingSystem"`
	Architecture    string     `json:"architecture"`
	AgentVersion    string     `json:"agentVersion,omitempty"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
}

type AgentMetricPointObservation struct {
	MetricName string   `json:"metricName"`
	MinValue   float64  `json:"minValue"`
	MaxValue   float64  `json:"maxValue"`
	Average    float64  `json:"average"`
	P95        *float64 `json:"p95,omitempty"`
}

type AgentMetricFamilyObservation struct {
	Name      string                        `json:"name"`
	Points    []AgentMetricPointObservation `json:"points"`
	Truncated bool                          `json:"truncated,omitempty"`
}

type AgentMetricsObservation struct {
	AgentID     string                         `json:"agentId"`
	Timestamp   *time.Time                     `json:"timestamp,omitempty"`
	LastUpdated *time.Time                     `json:"lastUpdated,omitempty"`
	IsStale     bool                           `json:"isStale"`
	Families    []AgentMetricFamilyObservation `json:"families"`
}

type AgentProcessObservation struct {
	PID                int        `json:"pid"`
	UserName           string     `json:"userName,omitempty"`
	Command            string     `json:"command,omitempty"`
	AverageValue       float64    `json:"averageValue"`
	PeakValue          float64    `json:"peakValue"`
	LatestSnapshotTime *time.Time `json:"latestSnapshotTime,omitempty"`
	CaptureReason      string     `json:"captureReason,omitempty"`
}

type AgentProcessesObservation struct {
	AgentID string                    `json:"agentId"`
	Metric  string                    `json:"metric"`
	From    time.Time                 `json:"from"`
	To      time.Time                 `json:"to"`
	Items   []AgentProcessObservation `json:"items"`
}

type AgentStorageReasonObservation struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Message   string  `json:"message,omitempty"`
}

type AgentStorageFilesystemObservation struct {
	Device             string                          `json:"device"`
	Mount              string                          `json:"mount"`
	FilesystemType     string                          `json:"filesystemType"`
	Readonly           bool                            `json:"readonly"`
	TotalBytes         *float64                        `json:"totalBytes,omitempty"`
	AvailableBytes     *float64                        `json:"availableBytes,omitempty"`
	UsedBytes          *float64                        `json:"usedBytes,omitempty"`
	UsagePercent       *float64                        `json:"usagePercent,omitempty"`
	InodeUsedPercent   *float64                        `json:"inodeUsedPercent,omitempty"`
	GrowthBytesPerDay  *float64                        `json:"growthBytesPerDay,omitempty"`
	ExhaustionETA      *time.Time                      `json:"exhaustionEta,omitempty"`
	ExhaustionETAState string                          `json:"exhaustionEtaState,omitempty"`
	RiskLevel          string                          `json:"riskLevel"`
	Reasons            []AgentStorageReasonObservation `json:"reasons,omitempty"`
	LastUpdated        *time.Time                      `json:"lastUpdated,omitempty"`
}

type AgentStorageObservation struct {
	AgentID          string                              `json:"agentId"`
	Items            []AgentStorageFilesystemObservation `json:"items"`
	PeakReadRateBps  float64                             `json:"peakReadRateBps"`
	PeakWriteRateBps float64                             `json:"peakWriteRateBps"`
	PeakRxRateBps    float64                             `json:"peakRxRateBps"`
	PeakTxRateBps    float64                             `json:"peakTxRateBps"`
	SampleCount      int                                 `json:"sampleCount"`
}

type AgentDockerObservation struct {
	DockerStatus    string `json:"dockerStatus"`
	Message         string `json:"message,omitempty"`
	ContainerTotal  int    `json:"containerTotal"`
	RunningCount    int    `json:"runningCount"`
	ExitedCount     int    `json:"exitedCount"`
	RestartingCount int    `json:"restartingCount"`
	AbnormalCount   int    `json:"abnormalCount"`
}

type AgentNginxTrafficObservation struct {
	Timestamp         *time.Time `json:"timestamp,omitempty"`
	IsStale           bool       `json:"isStale"`
	TotalRequests     int64      `json:"totalRequests"`
	QPS               float64    `json:"qps"`
	Status2xx         int64      `json:"status2xx"`
	Status3xx         int64      `json:"status3xx"`
	Status4xx         int64      `json:"status4xx"`
	Status5xx         int64      `json:"status5xx"`
	ErrorRate         float64    `json:"errorRate"`
	BytesIn           int64      `json:"bytesIn"`
	BytesOut          int64      `json:"bytesOut"`
	ActiveConnections int64      `json:"activeConnections"`
}

type AgentNginxLatencyObservation struct {
	P50Ms float64 `json:"p50Ms"`
	P90Ms float64 `json:"p90Ms"`
	P99Ms float64 `json:"p99Ms"`
}

type AgentNginxUpstreamObservation struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Degraded  int `json:"degraded"`
	Unhealthy int `json:"unhealthy"`
}

type AgentNginxObservation struct {
	AgentID         string                         `json:"agentId"`
	Traffic         *AgentNginxTrafficObservation  `json:"traffic,omitempty"`
	Latency         *AgentNginxLatencyObservation  `json:"latency,omitempty"`
	Upstream        *AgentNginxUpstreamObservation `json:"upstream,omitempty"`
	TopSlowExcluded bool                           `json:"topSlowExcluded"`
}

type AgentHostProfileObservation struct {
	SnapshotID      string     `json:"snapshotId,omitempty"`
	Status          string     `json:"status"`
	RefreshScope    string     `json:"refreshScope,omitempty"`
	CollectedAt     *time.Time `json:"collectedAt,omitempty"`
	Source          string     `json:"source,omitempty"`
	Truncated       bool       `json:"truncated"`
	ErrorCount      int        `json:"errorCount"`
	ContentExcluded bool       `json:"contentExcluded"`
}

type AgentControlPlaneObservation struct {
	AgentID             string     `json:"agentId"`
	ControlOnline       bool       `json:"controlOnline"`
	ControlLastSeenAt   *time.Time `json:"controlLastSeenAt,omitempty"`
	ControlHealth       string     `json:"controlHealth"`
	TelemetryStatus     string     `json:"telemetryStatus"`
	TelemetryLastSeenAt *time.Time `json:"telemetryLastSeenAt,omitempty"`
	SpoolDepth          int        `json:"spoolDepth"`
	ActiveIncident      bool       `json:"activeIncident"`
	LastFaultReason     string     `json:"lastFaultReason,omitempty"`
}

type agentMetricsRecord struct {
	AgentID     string                              `json:"agentId"`
	Timestamp   *time.Time                          `json:"timestamp"`
	Families    map[string][]agentMetricPointRecord `json:"families"`
	IsStale     bool                                `json:"isStale"`
	LastUpdated *time.Time                          `json:"lastUpdated"`
}

type agentMetricPointRecord struct {
	MetricName string   `json:"metricName"`
	MinVal     float64  `json:"minVal"`
	MaxVal     float64  `json:"maxVal"`
	AvgVal     float64  `json:"avgVal"`
	P95Val     *float64 `json:"p95Val"`
}

type agentProcessRecord struct {
	PID                int        `json:"pid"`
	UserName           string     `json:"userName"`
	Command            string     `json:"command"`
	AvgValue           float64    `json:"avgValue"`
	PeakValue          float64    `json:"peakValue"`
	LatestSnapshotTime *time.Time `json:"latestSnapshotTime"`
	CaptureReason      string     `json:"captureReason"`
}

type agentProcessesRecord struct {
	AgentID string               `json:"agentId"`
	Metric  string               `json:"metric"`
	From    time.Time            `json:"from"`
	To      time.Time            `json:"to"`
	Items   []agentProcessRecord `json:"items"`
}

type agentStorageRecord struct {
	AgentID          string                   `json:"agentId"`
	Items            []agentStorageItemRecord `json:"items"`
	PeakReadRateBps  float64                  `json:"peakReadRateBps"`
	PeakWriteRateBps float64                  `json:"peakWriteRateBps"`
	PeakRxRateBps    float64                  `json:"peakRxRateBps"`
	PeakTxRateBps    float64                  `json:"peakTxRateBps"`
	SampleCount      int                      `json:"sampleCount"`
}

type agentStorageItemRecord struct {
	Device             string                     `json:"device"`
	Mount              string                     `json:"mount"`
	FilesystemType     string                     `json:"filesystemType"`
	Readonly           bool                       `json:"readonly"`
	TotalBytes         *float64                   `json:"totalBytes"`
	AvailableBytes     *float64                   `json:"availableBytes"`
	UsedBytes          *float64                   `json:"usedBytes"`
	UsagePercent       *float64                   `json:"usagePercent"`
	InodeUsedPercent   *float64                   `json:"inodeUsedPercent"`
	GrowthBytesPerDay  *float64                   `json:"growthBytesPerDay"`
	ExhaustionETA      *time.Time                 `json:"exhaustionEta"`
	ExhaustionETAState string                     `json:"exhaustionEtaState"`
	RiskLevel          string                     `json:"riskLevel"`
	Reasons            []agentStorageReasonRecord `json:"reasons"`
	LastUpdated        *time.Time                 `json:"lastUpdated"`
}

type agentStorageReasonRecord struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
	Message   string  `json:"message"`
}

type agentNginxRecord struct {
	AgentID  string                    `json:"agentId"`
	Traffic  *agentNginxTrafficRecord  `json:"traffic"`
	Latency  *agentNginxLatencyRecord  `json:"latency"`
	Upstream *agentNginxUpstreamRecord `json:"upstream"`
}

type agentNginxTrafficRecord struct {
	Timestamp         *time.Time `json:"timestamp"`
	IsStale           bool       `json:"isStale"`
	TotalRequests     int64      `json:"totalRequests"`
	QPS               float64    `json:"qps"`
	Status2xx         int64      `json:"status2xx"`
	Status3xx         int64      `json:"status3xx"`
	Status4xx         int64      `json:"status4xx"`
	Status5xx         int64      `json:"status5xx"`
	ErrorRate         float64    `json:"errorRate"`
	BytesIn           int64      `json:"bytesIn"`
	BytesOut          int64      `json:"bytesOut"`
	ActiveConnections int64      `json:"activeConnections"`
}

type agentNginxLatencyRecord struct {
	P50Ms float64 `json:"p50Ms"`
	P90Ms float64 `json:"p90Ms"`
	P99Ms float64 `json:"p99Ms"`
}
type agentNginxUpstreamRecord struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Degraded  int `json:"degraded"`
	Unhealthy int `json:"unhealthy"`
}

type agentHostProfileRecord struct {
	Snapshot agentHostProfileSnapshotRecord `json:"snapshot"`
}
type agentHostProfileSnapshotRecord struct {
	ID           string     `json:"id"`
	RefreshScope string     `json:"refreshScope"`
	Status       string     `json:"status"`
	CollectedAt  *time.Time `json:"collectedAt"`
	Source       string     `json:"source"`
	Errors       []struct {
		Range string `json:"range"`
	} `json:"errors"`
	Truncated bool `json:"truncated"`
}

type agentControlPlaneRecord struct {
	AgentID             string     `json:"agentId"`
	ControlOnline       bool       `json:"controlOnline"`
	ControlLastSeenAt   *time.Time `json:"controlLastSeenAt"`
	ControlHealth       string     `json:"controlHealth"`
	TelemetryStatus     string     `json:"telemetryStatus"`
	TelemetryLastSeenAt *time.Time `json:"telemetryLastSeenAt"`
	SpoolDepth          int        `json:"spoolDepth"`
	ActiveIncident      bool       `json:"activeIncident"`
	LastFaultReason     string     `json:"lastFaultReason"`
}

// ObserveAgent 读取单个 Agent 的一个明确观察视图。
func (c *Client) ObserveAgent(ctx context.Context, options AgentObserveOptions) (AgentObserveResult, error) {
	agentID, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return AgentObserveResult{}, err
	}
	view := strings.ToLower(strings.TrimSpace(options.View))
	if _, ok := observeViews[view]; !ok {
		return AgentObserveResult{}, newInputError("observe view is not supported")
	}
	result := newAgentObserveResult(agentID, view)
	switch view {
	case ObserveViewHealth:
		item, err := c.GetAgent(ctx, agentID)
		if err != nil {
			return AgentObserveResult{}, err
		}
		result.Health = &AgentHealthObservation{DisplayName: item.DisplayName, Status: item.Status, OperatingSystem: item.OperatingSystem, Architecture: item.Architecture, AgentVersion: item.AgentVersion, LastHeartbeatAt: item.LastHeartbeatAt}
	case ObserveViewMetrics:
		var data agentMetricsRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "metrics", "latest"}, nil, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.Metrics, result.Truncated = summarizeAgentMetrics(data)
	case ObserveViewProcesses:
		metric := strings.ToLower(strings.TrimSpace(options.Metric))
		if metric == "" {
			metric = "cpu"
		}
		if _, ok := observeProcessMetrics[metric]; !ok {
			return AgentObserveResult{}, newInputError("process metric is not supported")
		}
		from, to, err := normalizeObserveWindow(options.From, options.To)
		if err != nil {
			return AgentObserveResult{}, err
		}
		limit := options.Limit
		if limit == 0 {
			limit = 10
		}
		if limit < 1 || limit > maxObserveProcessItems {
			return AgentObserveResult{}, fmt.Errorf("process limit must be between 1 and %d", maxObserveProcessItems)
		}
		query := url.Values{"metric": {metric}, "from": {from.Format(time.RFC3339)}, "to": {to.Format(time.RFC3339)}, "limit": {fmt.Sprintf("%d", limit)}}
		var data agentProcessesRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "processes", "top"}, query, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.Processes, result.RedactionApplied = summarizeAgentProcesses(data)
		result.Truncated = len(data.Items) > maxObserveProcessItems
	case ObserveViewStorage:
		var data agentStorageRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "storage", "filesystems"}, nil, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.Storage, result.Truncated = summarizeAgentStorage(data)
	case ObserveViewDocker:
		var data struct {
			DockerStatus    string `json:"dockerStatus"`
			Message         string `json:"message"`
			ContainerTotal  int    `json:"containerTotal"`
			RunningCount    int    `json:"runningCount"`
			ExitedCount     int    `json:"exitedCount"`
			RestartingCount int    `json:"restartingCount"`
			AbnormalCount   int    `json:"abnormalCount"`
		}
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "docker", "summary"}, nil, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.Docker = &AgentDockerObservation{DockerStatus: trimPublicText(data.DockerStatus, maxObserveTextLength), Message: trimPublicText(data.Message, maxObserveTextLength), ContainerTotal: data.ContainerTotal, RunningCount: data.RunningCount, ExitedCount: data.ExitedCount, RestartingCount: data.RestartingCount, AbnormalCount: data.AbnormalCount}
	case ObserveViewNginx:
		var data agentNginxRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "analysis", "nginx", "overview"}, nil, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.Nginx = summarizeAgentNginx(data)
	case ObserveViewHostProfile:
		query := url.Values{"scope": {"system"}}
		var data agentHostProfileRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "host-profile"}, query, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.HostProfile = summarizeAgentHostProfile(data)
	case ObserveViewControlPlane:
		var data agentControlPlaneRecord
		if err := c.do(ctx, http.MethodGet, []string{"agents", agentID, "control-plane"}, nil, nil, &data, true); err != nil {
			return AgentObserveResult{}, err
		}
		result.ControlPlane = &AgentControlPlaneObservation{AgentID: data.AgentID, ControlOnline: data.ControlOnline, ControlLastSeenAt: data.ControlLastSeenAt, ControlHealth: trimPublicText(data.ControlHealth, maxObserveTextLength), TelemetryStatus: trimPublicText(data.TelemetryStatus, maxObserveTextLength), TelemetryLastSeenAt: data.TelemetryLastSeenAt, SpoolDepth: data.SpoolDepth, ActiveIncident: data.ActiveIncident, LastFaultReason: trimPublicText(data.LastFaultReason, maxObserveTextLength)}
	}
	result.Notice = agentObserveNotice(result)
	return result, nil
}

func newAgentObserveResult(agentID, view string) AgentObserveResult {
	return AgentObserveResult{AgentID: agentID, View: view, ResultMode: "bounded_summary", SensitiveContentExcluded: true, UnknownSensitiveContentMayRemain: true, RedactionPolicy: "conservative_patterns_only"}
}

func agentObserveNotice(result AgentObserveResult) string {
	truncation := "本次结果未截断"
	if result.Truncated {
		truncation = "本次结果发生截断"
	}
	redaction := "本次未发生保守替换"
	if result.RedactionApplied {
		redaction = "本次发生了保守替换"
	}
	return trimPublicText(fmt.Sprintf("当前返回为有界摘要；%s；%s。MCP 只对明显的密码、令牌、Authorization 等模式做保守处理，未知敏感内容仍可能存在。未返回内容不代表任务失败；若信息不足，请明确请求已支持的更详细视图，不要因缺少输出重复提交或重复执行任务。敏感字段、凭据、命令环境变量、主机画像正文、Nginx 慢请求明细和完整历史不会随本摘要返回。", truncation, redaction), maxObserveNoticeLength)
}

func normalizeObserveWindow(from, to *time.Time) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	end := now
	if to != nil {
		end = to.UTC()
	}
	start := end.Add(-15 * time.Minute)
	if from != nil {
		start = from.UTC()
	}
	if !start.Before(end) {
		return time.Time{}, time.Time{}, newInputError("process time window must have from before to")
	}
	if end.Sub(start) > 24*time.Hour {
		return time.Time{}, time.Time{}, newInputError("process time window must not exceed 24 hours")
	}
	return start, end, nil
}

func summarizeAgentMetrics(data agentMetricsRecord) (*AgentMetricsObservation, bool) {
	families := make([]AgentMetricFamilyObservation, 0, minInt(len(data.Families), maxObserveMetricFamilies))
	truncated := len(data.Families) > maxObserveMetricFamilies
	keys := make([]string, 0, len(data.Families))
	for key := range data.Families {
		keys = append(keys, key)
	}
	sortStrings(keys)
	for index, key := range keys {
		if index >= maxObserveMetricFamilies {
			break
		}
		points := data.Families[key]
		item := AgentMetricFamilyObservation{Name: trimPublicText(key, maxObserveTextLength)}
		if len(points) > maxObserveMetricPoints {
			item.Truncated = true
			truncated = true
		}
		for pointIndex, point := range points {
			if pointIndex >= maxObserveMetricPoints {
				break
			}
			item.Points = append(item.Points, AgentMetricPointObservation{MetricName: trimPublicText(point.MetricName, maxObserveTextLength), MinValue: point.MinVal, MaxValue: point.MaxVal, Average: point.AvgVal, P95: point.P95Val})
		}
		families = append(families, item)
	}
	return &AgentMetricsObservation{AgentID: data.AgentID, Timestamp: data.Timestamp, LastUpdated: data.LastUpdated, IsStale: data.IsStale, Families: families}, truncated
}

func summarizeAgentProcesses(data agentProcessesRecord) (*AgentProcessesObservation, bool) {
	items := make([]AgentProcessObservation, 0, minInt(len(data.Items), maxObserveProcessItems))
	redacted := false
	for index, item := range data.Items {
		if index >= maxObserveProcessItems {
			break
		}
		command, changed := redactSensitiveText(item.Command)
		redacted = redacted || changed
		items = append(items, AgentProcessObservation{PID: item.PID, UserName: trimPublicText(item.UserName, maxObserveTextLength), Command: command, AverageValue: item.AvgValue, PeakValue: item.PeakValue, LatestSnapshotTime: item.LatestSnapshotTime, CaptureReason: trimPublicText(item.CaptureReason, maxObserveTextLength)})
	}
	return &AgentProcessesObservation{AgentID: data.AgentID, Metric: trimPublicText(data.Metric, maxObserveTextLength), From: data.From, To: data.To, Items: items}, redacted
}

func summarizeAgentStorage(data agentStorageRecord) (*AgentStorageObservation, bool) {
	items := make([]AgentStorageFilesystemObservation, 0, minInt(len(data.Items), maxObserveStorageItems))
	truncated := len(data.Items) > maxObserveStorageItems
	for index, item := range data.Items {
		if index >= maxObserveStorageItems {
			break
		}
		reasons := make([]AgentStorageReasonObservation, 0, minInt(len(item.Reasons), maxObserveReasons))
		if len(item.Reasons) > maxObserveReasons {
			truncated = true
		}
		for reasonIndex, reason := range item.Reasons {
			if reasonIndex >= maxObserveReasons {
				break
			}
			reasons = append(reasons, AgentStorageReasonObservation{Metric: trimPublicText(reason.Metric, maxObserveTextLength), Value: reason.Value, Threshold: reason.Threshold, Message: trimPublicText(reason.Message, maxObserveTextLength)})
		}
		items = append(items, AgentStorageFilesystemObservation{Device: trimPublicText(item.Device, maxObserveTextLength), Mount: trimPublicText(item.Mount, maxObserveTextLength), FilesystemType: trimPublicText(item.FilesystemType, maxObserveTextLength), Readonly: item.Readonly, TotalBytes: item.TotalBytes, AvailableBytes: item.AvailableBytes, UsedBytes: item.UsedBytes, UsagePercent: item.UsagePercent, InodeUsedPercent: item.InodeUsedPercent, GrowthBytesPerDay: item.GrowthBytesPerDay, ExhaustionETA: item.ExhaustionETA, ExhaustionETAState: trimPublicText(item.ExhaustionETAState, maxObserveTextLength), RiskLevel: trimPublicText(item.RiskLevel, maxObserveTextLength), Reasons: reasons, LastUpdated: item.LastUpdated})
	}
	return &AgentStorageObservation{AgentID: data.AgentID, Items: items, PeakReadRateBps: data.PeakReadRateBps, PeakWriteRateBps: data.PeakWriteRateBps, PeakRxRateBps: data.PeakRxRateBps, PeakTxRateBps: data.PeakTxRateBps, SampleCount: data.SampleCount}, truncated
}

func summarizeAgentNginx(data agentNginxRecord) *AgentNginxObservation {
	result := &AgentNginxObservation{AgentID: data.AgentID, TopSlowExcluded: true}
	if data.Traffic != nil {
		result.Traffic = &AgentNginxTrafficObservation{Timestamp: data.Traffic.Timestamp, IsStale: data.Traffic.IsStale, TotalRequests: data.Traffic.TotalRequests, QPS: data.Traffic.QPS, Status2xx: data.Traffic.Status2xx, Status3xx: data.Traffic.Status3xx, Status4xx: data.Traffic.Status4xx, Status5xx: data.Traffic.Status5xx, ErrorRate: data.Traffic.ErrorRate, BytesIn: data.Traffic.BytesIn, BytesOut: data.Traffic.BytesOut, ActiveConnections: data.Traffic.ActiveConnections}
	}
	if data.Latency != nil {
		result.Latency = &AgentNginxLatencyObservation{P50Ms: data.Latency.P50Ms, P90Ms: data.Latency.P90Ms, P99Ms: data.Latency.P99Ms}
	}
	if data.Upstream != nil {
		result.Upstream = &AgentNginxUpstreamObservation{Total: data.Upstream.Total, Healthy: data.Upstream.Healthy, Degraded: data.Upstream.Degraded, Unhealthy: data.Upstream.Unhealthy}
	}
	return result
}

func summarizeAgentHostProfile(data agentHostProfileRecord) *AgentHostProfileObservation {
	return &AgentHostProfileObservation{SnapshotID: data.Snapshot.ID, Status: trimPublicText(data.Snapshot.Status, maxObserveTextLength), RefreshScope: trimPublicText(data.Snapshot.RefreshScope, maxObserveTextLength), CollectedAt: data.Snapshot.CollectedAt, Source: trimPublicText(data.Snapshot.Source, maxObserveTextLength), Truncated: data.Snapshot.Truncated, ErrorCount: len(data.Snapshot.Errors), ContentExcluded: true}
}

var observeSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)((?:password|passwd|token|secret|api[_-]?key|authorization|private[_-]?key)\s*[:=]\s*)([^\s,;]+)`),
	regexp.MustCompile(`(?i)(Bearer\s+)([A-Za-z0-9._~+/=-]+)`),
}

func redactSensitiveText(value string) (string, bool) {
	value = trimPublicText(value, maxObserveTextLength)
	return redactSensitiveTextUnbounded(value)
}

func redactSensitiveTextUnbounded(value string) (string, bool) {
	value = strings.TrimSpace(value)
	changed := false
	for _, pattern := range observeSecretPatterns {
		next := pattern.ReplaceAllString(value, `${1}******`)
		if next != value {
			changed = true
			value = next
		}
	}
	return value, changed
}

func sortStrings(values []string) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}
