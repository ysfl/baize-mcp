package baize

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	maxRuntimeDiagnosisTargetValue = 512
	maxRuntimeDiagnosisSource      = 120
	maxRuntimeDiagnosisSummary     = 500
	maxRuntimeDiagnosisTemplates   = 30
)

var runtimeDiagnosisTargetTypes = map[string]struct{}{
	"pid": {}, "port": {}, "process_name": {}, "file": {}, "service": {}, "container": {},
}

// RuntimeDiagnosisStartOptions 描述一次服务端白名单化的只读诊断探针。
type RuntimeDiagnosisStartOptions struct {
	AgentID      string
	TargetType   string
	TargetValue  string
	TimeHint     string
	SourceModule string
	TimeoutSec   int
	MaxResults   int
}

// RuntimeDiagnosisSummary 是诊断任务的稳定摘要，不包含原始 Agent 回包。
type RuntimeDiagnosisSummary struct {
	ID                               string     `json:"id"`
	AgentID                          string     `json:"agentId"`
	TargetType                       string     `json:"targetType"`
	TargetValue                      string     `json:"targetValue"`
	Status                           string     `json:"status"`
	Summary                          string     `json:"summary"`
	TimeoutSec                       int        `json:"timeoutSec"`
	MaxResults                       int        `json:"maxResults"`
	Truncated                        bool       `json:"truncated"`
	Pushed                           bool       `json:"pushed"`
	SensitiveContentExcluded         bool       `json:"sensitiveContentExcluded"`
	UnknownSensitiveContentMayRemain bool       `json:"unknownSensitiveContentMayRemain"`
	ResultMode                       string     `json:"resultMode"`
	Notice                           string     `json:"notice"`
	CreatedAt                        *time.Time `json:"createdAt,omitempty"`
	StartedAt                        *time.Time `json:"startedAt,omitempty"`
	CollectedAt                      *time.Time `json:"collectedAt,omitempty"`
	FinishedAt                       *time.Time `json:"finishedAt,omitempty"`
	ErrorMessage                     string     `json:"errorMessage,omitempty"`
}

// RuntimeDiagnosisDetail 是按需读取的有限结果。计数和推荐 ID 足够组织后续动作，
// 诊断正文不会因普通状态查询自动进入 AI 上下文。
type RuntimeDiagnosisDetail struct {
	RuntimeDiagnosisSummary
	ProcessCount           int      `json:"processCount"`
	PortCount              int      `json:"portCount"`
	RiskFindingCount       int      `json:"riskFindingCount"`
	EvidenceCount          int      `json:"evidenceCount"`
	RecommendedTemplateIDs []string `json:"recommendedTemplateIds,omitempty"`
	DetailAvailable        bool     `json:"detailAvailable"`
}

type runtimeDiagnosisRecord struct {
	ID                     string                `json:"id"`
	AgentID                string                `json:"agentId"`
	TargetType             string                `json:"targetType"`
	TargetValue            string                `json:"targetValue"`
	Status                 string                `json:"status"`
	Summary                string                `json:"summary"`
	TimeoutSec             int                   `json:"timeoutSec"`
	MaxResults             int                   `json:"maxResults"`
	ErrorMessage           *string               `json:"errorMessage"`
	Process                *jsonProcessIdentity  `json:"process"`
	Processes              []jsonProcessIdentity `json:"processes"`
	Ports                  []jsonRuntimePort     `json:"ports"`
	RiskFindings           []jsonRiskFinding     `json:"riskFindings"`
	Evidences              []jsonRuntimeEvidence `json:"evidences"`
	RecommendedTemplateIDs []string              `json:"recommendedTemplateIds"`
	Truncated              bool                  `json:"truncated"`
	Pushed                 bool                  `json:"pushed"`
	CreatedAt              *time.Time            `json:"createdAt"`
	StartedAt              *time.Time            `json:"startedAt"`
	CollectedAt            *time.Time            `json:"collectedAt"`
	FinishedAt             *time.Time            `json:"finishedAt"`
}

// 仅声明需要计数的字段，避免把路径、命令、端口绑定地址或证据值带入 MCP 结果。
type jsonProcessIdentity struct {
	PID int `json:"pid"`
}

type jsonRuntimePort struct {
	LocalPort uint32 `json:"localPort"`
}

type jsonRiskFinding struct {
	Code string `json:"code"`
}

type jsonRuntimeEvidence struct {
	Key string `json:"key"`
}

func (c *Client) StartRuntimeDiagnosis(ctx context.Context, options RuntimeDiagnosisStartOptions) (RuntimeDiagnosisSummary, error) {
	agentID, err := validateUUID(options.AgentID, "agent ID")
	if err != nil {
		return RuntimeDiagnosisSummary{}, err
	}
	targetType := strings.ToLower(strings.TrimSpace(options.TargetType))
	if _, ok := runtimeDiagnosisTargetTypes[targetType]; !ok {
		return RuntimeDiagnosisSummary{}, newInputError("target type must be pid, port, process_name, file, service, or container")
	}
	targetValue := strings.TrimSpace(options.TargetValue)
	if targetValue == "" || len(targetValue) > maxRuntimeDiagnosisTargetValue {
		return RuntimeDiagnosisSummary{}, newInputError(fmt.Sprintf("target value must be non-empty and no longer than %d characters", maxRuntimeDiagnosisTargetValue))
	}
	sourceModule := strings.TrimSpace(options.SourceModule)
	if len(sourceModule) > maxRuntimeDiagnosisSource {
		return RuntimeDiagnosisSummary{}, newInputError(fmt.Sprintf("source module must not exceed %d characters", maxRuntimeDiagnosisSource))
	}
	timeHint := strings.TrimSpace(options.TimeHint)
	if len(timeHint) > maxRuntimeDiagnosisSource {
		return RuntimeDiagnosisSummary{}, newInputError(fmt.Sprintf("time hint must not exceed %d characters", maxRuntimeDiagnosisSource))
	}
	if options.TimeoutSec < 0 || options.TimeoutSec > 10 {
		return RuntimeDiagnosisSummary{}, newInputError("timeout must be between 1 and 10 seconds, or omitted")
	}
	if options.MaxResults < 0 || options.MaxResults > 50 {
		return RuntimeDiagnosisSummary{}, newInputError("max results must be between 1 and 50, or omitted")
	}
	payload := struct {
		AgentID      string `json:"agentId"`
		TargetType   string `json:"targetType"`
		TargetValue  string `json:"targetValue"`
		TimeHint     string `json:"timeHint,omitempty"`
		SourceModule string `json:"sourceModule,omitempty"`
		TimeoutSec   int    `json:"timeoutSec,omitempty"`
		MaxResults   int    `json:"maxResults,omitempty"`
	}{agentID, targetType, targetValue, timeHint, sourceModule, options.TimeoutSec, options.MaxResults}
	var data runtimeDiagnosisRecord
	if err := c.do(ctx, http.MethodPost, []string{"runtime-diagnoses", "query"}, nil, payload, &data, true); err != nil {
		return RuntimeDiagnosisSummary{}, err
	}
	return summarizeRuntimeDiagnosis(data), nil
}

func (c *Client) GetRuntimeDiagnosis(ctx context.Context, id string) (RuntimeDiagnosisDetail, error) {
	diagnosisID, err := validateUUID(id, "diagnosis ID")
	if err != nil {
		return RuntimeDiagnosisDetail{}, err
	}
	var data runtimeDiagnosisRecord
	if err := c.do(ctx, http.MethodGet, []string{"runtime-diagnoses", diagnosisID}, nil, nil, &data, true); err != nil {
		return RuntimeDiagnosisDetail{}, err
	}
	summary := summarizeRuntimeDiagnosis(data)
	templates, templatesTruncated := trimPublicStringList(data.RecommendedTemplateIDs, 0, maxRuntimeDiagnosisTemplates)
	summary.Truncated = summary.Truncated || templatesTruncated
	return RuntimeDiagnosisDetail{
		RuntimeDiagnosisSummary: summary,
		ProcessCount:            len(data.Processes),
		PortCount:               len(data.Ports),
		RiskFindingCount:        len(data.RiskFindings),
		EvidenceCount:           len(data.Evidences),
		RecommendedTemplateIDs:  templates,
		DetailAvailable:         len(data.Processes) > 0 || len(data.Ports) > 0 || len(data.Evidences) > 0,
	}, nil
}

func summarizeRuntimeDiagnosis(item runtimeDiagnosisRecord) RuntimeDiagnosisSummary {
	var errorMessage string
	if item.ErrorMessage != nil {
		errorMessage = trimPublicText(*item.ErrorMessage, maxRuntimeDiagnosisSummary)
	}
	return RuntimeDiagnosisSummary{
		ID: item.ID, AgentID: item.AgentID, TargetType: trimPublicText(item.TargetType, 40),
		TargetValue: trimPublicText(item.TargetValue, maxRuntimeDiagnosisTargetValue), Status: trimPublicText(item.Status, 40),
		Summary: trimPublicText(item.Summary, maxRuntimeDiagnosisSummary), TimeoutSec: item.TimeoutSec, MaxResults: item.MaxResults,
		Truncated: item.Truncated, Pushed: item.Pushed, CreatedAt: item.CreatedAt, StartedAt: item.StartedAt,
		CollectedAt: item.CollectedAt, FinishedAt: item.FinishedAt, ErrorMessage: errorMessage,
		SensitiveContentExcluded:         true,
		UnknownSensitiveContentMayRemain: true,
		ResultMode:                       "bounded_summary",
		Notice:                           "This is a bounded diagnosis summary. Process commands, paths, port addresses, evidence values, credentials, and raw Agent output are excluded; missing detail does not mean the diagnosis failed.",
	}
}
