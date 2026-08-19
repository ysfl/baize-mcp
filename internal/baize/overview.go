package baize

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"strings"
)

const (
	// OverviewDefaultLimit 是概览默认返回的重点异常节点数量。
	OverviewDefaultLimit = 10
	// OverviewMaxLimit 是 MCP 对异常节点数量设置的上限，避免一次调用放大上下文。
	OverviewMaxLimit = 20

	maxOverviewAbnormalReasons = 8
	maxOverviewHostnameLength  = 255
	maxOverviewStatusLength    = 64
	maxOverviewMetricLength    = 64
)

var overviewAbnormalMetrics = map[string]struct{}{
	"cpu": {}, "memory": {}, "disk": {}, "offline": {},
}

// OverviewOptions 是平台概览查询的受控参数。
type OverviewOptions struct {
	GroupID string
	Limit   int
}

// ServerStatusSummary 是平台范围内 Agent 状态的实时摘要。
type ServerStatusSummary struct {
	Total       int64   `json:"total"`
	Online      int64   `json:"online"`
	Offline     int64   `json:"offline"`
	Degraded    int64   `json:"degraded"`
	Maintenance int64   `json:"maintenance"`
	Registering int64   `json:"registering"`
	OnlineRate  float64 `json:"onlineRate"`
}

// DistributionBuckets 是资源使用率分布的低、中、高计数。
type DistributionBuckets struct {
	Low    int64 `json:"low"`
	Medium int64 `json:"medium"`
	High   int64 `json:"high"`
}

// ResourceDistribution 是 Dashboard 返回的资源分布摘要。
type ResourceDistribution struct {
	CPU    DistributionBuckets `json:"cpu"`
	Memory DistributionBuckets `json:"memory"`
	Disk   DistributionBuckets `json:"disk"`
}

// NginxOverview 是 Dashboard 返回的全局 Nginx 指标摘要。
type NginxOverview struct {
	TotalQPS      float64 `json:"totalQps"`
	Global4xxRate float64 `json:"global4xxRate"`
	Global5xxRate float64 `json:"global5xxRate"`
	GlobalP99Ms   float64 `json:"globalP99Ms"`
}

// OverviewAbnormalReason 是异常节点的有限原因摘要。
type OverviewAbnormalReason struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

// AbnormalServerSummary 是概览中需要优先关注的节点摘要。
// 后端排序权重属于实现细节，不进入 MCP 结果。
type AbnormalServerSummary struct {
	AgentID  string                   `json:"agentId"`
	Hostname string                   `json:"hostname"`
	Status   string                   `json:"status"`
	Reasons  []OverviewAbnormalReason `json:"reasons,omitempty"`
}

// OverviewSummary 是 baize_overview_get 的稳定、有界结果。
// 部分 API 失败时保留成功分区，并用 Partial/MissingSections 标记，不伪造健康结论。
type OverviewSummary struct {
	ServerStatus             *ServerStatusSummary    `json:"serverStatus,omitempty"`
	ResourceDistribution     *ResourceDistribution   `json:"resourceDistribution,omitempty"`
	NginxOverview            *NginxOverview          `json:"nginxOverview,omitempty"`
	AbnormalServers          []AbnormalServerSummary `json:"abnormalServers"`
	AbnormalServersTruncated bool                    `json:"abnormalServersTruncated,omitempty"`
	Partial                  bool                    `json:"partial"`
	MissingSections          []string                `json:"missingSections,omitempty"`
	ResourceDataAvailable    bool                    `json:"resourceDataAvailable"`
	NginxDataAvailable       bool                    `json:"nginxDataAvailable"`
	AbnormalDataAvailable    bool                    `json:"abnormalDataAvailable"`
	AbnormalDataMayBeStale   bool                    `json:"abnormalDataMayBeStale"`
}

type dashboardSummaryRecord struct {
	ServerStatus         *ServerStatusSummary  `json:"serverStatus"`
	ResourceDistribution *ResourceDistribution `json:"resourceDistribution"`
	NginxOverview        *NginxOverview        `json:"nginxOverview"`
}

type abnormalServerListRecord struct {
	Items []abnormalServerRecord `json:"items"`
}

type abnormalServerRecord struct {
	AgentID  string                 `json:"agentId"`
	Hostname string                 `json:"hostname"`
	Status   string                 `json:"status"`
	Reasons  []abnormalReasonRecord `json:"reasons"`
}

type abnormalReasonRecord struct {
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Threshold float64 `json:"threshold"`
}

// GetOverview 组合同域的 Dashboard 摘要和异常节点接口。
// 认证/授权错误不会降级为 partial；网络或服务端 5xx 只在另一分区成功时部分返回。
func (c *Client) GetOverview(ctx context.Context, options OverviewOptions) (OverviewSummary, error) {
	groupID := strings.TrimSpace(options.GroupID)
	if groupID != "" {
		validated, err := validateUUID(groupID, "group ID")
		if err != nil {
			return OverviewSummary{}, err
		}
		groupID = validated
	}
	limit := options.Limit
	if limit == 0 {
		limit = OverviewDefaultLimit
	}
	if limit < 1 || limit > OverviewMaxLimit {
		return OverviewSummary{}, newInputError(fmt.Sprintf("overview limit must be between 1 and %d", OverviewMaxLimit))
	}

	result := OverviewSummary{AbnormalServers: make([]AbnormalServerSummary, 0)}
	var firstDegradableErr error
	var summary dashboardSummaryRecord
	summaryErr := c.do(ctx, http.MethodGet, []string{"dashboard", "summary"}, overviewGroupQuery(groupID), nil, &summary, true)
	summarySucceeded := summaryErr == nil
	if summaryErr != nil {
		if !isOverviewDegradableError(summaryErr) {
			return OverviewSummary{}, summaryErr
		}
		firstDegradableErr = summaryErr
	} else {
		result.ServerStatus = summary.ServerStatus
		result.ResourceDistribution = summary.ResourceDistribution
		result.NginxOverview = summary.NginxOverview
		result.ResourceDataAvailable = summary.ResourceDistribution != nil
		result.NginxDataAvailable = summary.NginxOverview != nil
	}

	query := overviewGroupQuery(groupID)
	query.Set("limit", fmt.Sprintf("%d", limit))
	var abnormal abnormalServerListRecord
	abnormalErr := c.do(ctx, http.MethodGet, []string{"dashboard", "abnormal-servers"}, query, nil, &abnormal, true)
	abnormalSucceeded := abnormalErr == nil
	if abnormalErr != nil {
		if !isOverviewDegradableError(abnormalErr) {
			return OverviewSummary{}, abnormalErr
		}
		if firstDegradableErr == nil {
			firstDegradableErr = abnormalErr
		}
	} else {
		result.AbnormalDataAvailable = true
		// Server 当前接口在缓存缺失时也返回空列表，空结果不能单独证明“没有异常”。
		result.AbnormalDataMayBeStale = len(abnormal.Items) == 0
		result.AbnormalServers, result.AbnormalServersTruncated = summarizeAbnormalServers(abnormal.Items, limit)
	}

	if !summarySucceeded {
		result.MissingSections = append(result.MissingSections, "summary")
	}
	if !abnormalSucceeded {
		result.MissingSections = append(result.MissingSections, "abnormalServers")
	}
	result.Partial = len(result.MissingSections) > 0

	// 两个分区都失败时没有可供 AI 判断的事实，保留原始分类错误让 MCP 层统一脱敏。
	if firstDegradableErr != nil && !summarySucceeded && !abnormalSucceeded {
		return OverviewSummary{}, firstDegradableErr
	}
	return result, nil
}

func overviewGroupQuery(groupID string) url.Values {
	query := url.Values{}
	if groupID != "" {
		query.Set("group_id", groupID)
	}
	return query
}

func isOverviewDegradableError(err error) bool {
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		return true
	}
	return apiErr.StatusCode >= http.StatusInternalServerError && apiErr.StatusCode < 600
}

func summarizeAbnormalServers(items []abnormalServerRecord, limit int) ([]AbnormalServerSummary, bool) {
	limit = minInt(limit, OverviewMaxLimit)
	result := make([]AbnormalServerSummary, 0, minInt(len(items), limit))
	truncated := false
	for index, item := range items {
		if index >= limit {
			truncated = true
			break
		}
		reasons := summarizeOverviewReasons(item.Reasons)
		result = append(result, AbnormalServerSummary{
			AgentID:  trimPublicText(item.AgentID, maxOverviewHostnameLength),
			Hostname: trimPublicText(item.Hostname, maxOverviewHostnameLength),
			Status:   trimPublicText(item.Status, maxOverviewStatusLength),
			Reasons:  reasons,
		})
	}
	return result, truncated
}

func summarizeOverviewReasons(items []abnormalReasonRecord) []OverviewAbnormalReason {
	if len(items) == 0 {
		return nil
	}
	result := make([]OverviewAbnormalReason, 0, minInt(len(items), maxOverviewAbnormalReasons))
	for _, item := range items {
		metric := strings.ToLower(trimPublicText(item.Metric, maxOverviewMetricLength))
		if _, ok := overviewAbnormalMetrics[metric]; !ok {
			continue
		}
		if math.IsNaN(item.Value) || math.IsInf(item.Value, 0) || math.IsNaN(item.Threshold) || math.IsInf(item.Threshold, 0) {
			continue
		}
		result = append(result, OverviewAbnormalReason{Metric: metric, Value: item.Value, Threshold: item.Threshold})
		if len(result) >= maxOverviewAbnormalReasons {
			break
		}
	}
	return result
}
