package baize

import (
	"context"
	"net/http"
	"time"
)

const (
	maxAIContextRiskFindings = 20
	maxAIContextTemplates    = 30
	maxAIContextCapabilities = 32
	maxAIContextText         = 512
)

// RuntimeDiagnosisAIContext 是按需读取的脱敏诊断上下文摘要。
// 它不携带请求追踪 ID、操作者、审计引用、命令正文、路径或证据值。
type RuntimeDiagnosisAIContext struct {
	Status                           string                         `json:"status"`
	Message                          string                         `json:"message"`
	ModelProvider                    *AIModelProviderStatus         `json:"modelProvider,omitempty"`
	Context                          *RuntimeDiagnosisAIContextView `json:"context,omitempty"`
	ResultMode                       string                         `json:"resultMode"`
	SensitiveContentExcluded         bool                           `json:"sensitiveContentExcluded"`
	UnknownSensitiveContentMayRemain bool                           `json:"unknownSensitiveContentMayRemain"`
	RedactionApplied                 bool                           `json:"redactionApplied"`
	RedactionPolicy                  string                         `json:"redactionPolicy"`
	Truncated                        bool                           `json:"truncated"`
	Notice                           string                         `json:"notice"`
}

// AIModelProviderStatus 只暴露模型 provider 的非敏感运行状态。
// endpoint 和 API key 本身永远不进入 MCP 结果。
type AIModelProviderStatus struct {
	Provider           string `json:"provider"`
	Model              string `json:"model,omitempty"`
	Status             string `json:"status"`
	EndpointConfigured bool   `json:"endpointConfigured"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSec         int    `json:"timeoutSec,omitempty"`
	MaxTokens          int    `json:"maxTokens,omitempty"`
}

type RuntimeDiagnosisAIContextView struct {
	ContextVersion     string                             `json:"contextVersion"`
	AgentScope         RuntimeDiagnosisAIAgentScope       `json:"agentScope"`
	Diagnosis          RuntimeDiagnosisAIContextDiagnosis `json:"diagnosis"`
	RiskFindings       []RuntimeDiagnosisAIRiskFinding    `json:"riskFindings,omitempty"`
	AvailableTemplates []RuntimeDiagnosisAITemplate       `json:"availableTemplates,omitempty"`
	ExecutionPolicy    RuntimeDiagnosisAIExecutionPolicy  `json:"executionPolicy"`
	RedactionPolicy    RuntimeDiagnosisAIRedactionPolicy  `json:"redactionPolicy"`
}

type RuntimeDiagnosisAIAgentScope struct {
	AgentID         string   `json:"agentId"`
	Hostname        string   `json:"hostname,omitempty"`
	Platform        string   `json:"platform,omitempty"`
	ManagementLevel string   `json:"managementLevel,omitempty"`
	Capabilities    []string `json:"capabilities,omitempty"`
}

type RuntimeDiagnosisAIContextDiagnosis struct {
	ID                     string     `json:"id"`
	AgentID                string     `json:"agentId"`
	TargetType             string     `json:"targetType"`
	Status                 string     `json:"status"`
	Summary                string     `json:"summary,omitempty"`
	ErrorMessage           string     `json:"errorMessage,omitempty"`
	TimeoutSec             int        `json:"timeoutSec"`
	MaxResults             int        `json:"maxResults"`
	ProcessCount           int        `json:"processCount"`
	PortCount              int        `json:"portCount"`
	RiskFindingCount       int        `json:"riskFindingCount"`
	EvidenceCount          int        `json:"evidenceCount"`
	RecommendedTemplateIDs []string   `json:"recommendedTemplateIds,omitempty"`
	Truncated              bool       `json:"truncated"`
	Pushed                 bool       `json:"pushed"`
	CreatedAt              *time.Time `json:"createdAt,omitempty"`
	StartedAt              *time.Time `json:"startedAt,omitempty"`
	CollectedAt            *time.Time `json:"collectedAt,omitempty"`
	FinishedAt             *time.Time `json:"finishedAt,omitempty"`
}

type RuntimeDiagnosisAIRiskFinding struct {
	Code     string `json:"code"`
	Severity string `json:"severity,omitempty"`
	Message  string `json:"message,omitempty"`
}

type RuntimeDiagnosisAITemplate struct {
	TemplateID           string                     `json:"templateId"`
	TemplateName         string                     `json:"templateName"`
	Description          string                     `json:"description,omitempty"`
	Category             string                     `json:"category,omitempty"`
	Version              int                        `json:"version"`
	Platform             string                     `json:"platform,omitempty"`
	RiskLevel            string                     `json:"riskLevel"`
	RenderMode           string                     `json:"renderMode"`
	RequiredCapabilities []string                   `json:"requiredCapabilities,omitempty"`
	ParameterSchema      []CommandTemplateParameter `json:"parameterSchema,omitempty"`
	RequiresHumanReview  bool                       `json:"requiresHumanReview"`
}

type RuntimeDiagnosisAIExecutionPolicy struct {
	CanCreateCommandPlan        bool `json:"canCreateCommandPlan"`
	CanDispatchExecTask         bool `json:"canDispatchExecTask"`
	CanApproveHighRisk          bool `json:"canApproveHighRisk"`
	RequiresApprovalForCritical bool `json:"requiresApprovalForCritical"`
}

type RuntimeDiagnosisAIRedactionPolicy struct {
	Version string `json:"version"`
	Summary string `json:"summary,omitempty"`
}

type runtimeDiagnosisAIContextRecord struct {
	Status        string                             `json:"status"`
	Message       string                             `json:"message"`
	ModelProvider *aiModelProviderRecord             `json:"modelProvider"`
	Context       *runtimeDiagnosisAIContextEnvelope `json:"context"`
}

type aiModelProviderRecord struct {
	Provider           string `json:"provider"`
	Model              string `json:"model"`
	Status             string `json:"status"`
	Message            string `json:"message"`
	EndpointConfigured bool   `json:"endpointConfigured"`
	APIKeyConfigured   bool   `json:"apiKeyConfigured"`
	TimeoutSec         int    `json:"timeoutSec"`
	MaxTokens          int    `json:"maxTokens"`
}

type runtimeDiagnosisAIContextEnvelope struct {
	ContextVersion     string                             `json:"contextVersion"`
	AgentScope         runtimeDiagnosisAIAgentScope       `json:"agentScope"`
	Diagnosis          runtimeDiagnosisAIContextDiagnosis `json:"diagnosis"`
	RiskFindings       []runtimeDiagnosisAIRiskFinding    `json:"riskFindings"`
	AvailableTemplates []runtimeDiagnosisAITemplate       `json:"availableTemplates"`
	ExecutionPolicy    RuntimeDiagnosisAIExecutionPolicy  `json:"executionPolicy"`
	RedactionPolicy    RuntimeDiagnosisAIRedactionPolicy  `json:"redactionPolicy"`
}

type runtimeDiagnosisAIAgentScope struct {
	AgentID         string   `json:"agentId"`
	Hostname        string   `json:"hostname"`
	Platform        string   `json:"platform"`
	ManagementLevel string   `json:"managementLevel"`
	Capabilities    []string `json:"capabilities"`
}

type runtimeDiagnosisAIContextDiagnosis struct {
	ID                     string                          `json:"id"`
	AgentID                string                          `json:"agentId"`
	TargetType             string                          `json:"targetType"`
	Status                 string                          `json:"status"`
	Summary                string                          `json:"summary"`
	ErrorMessage           string                          `json:"errorMessage"`
	TimeoutSec             int                             `json:"timeoutSec"`
	MaxResults             int                             `json:"maxResults"`
	Process                *jsonProcessIdentity            `json:"process"`
	Processes              []jsonProcessIdentity           `json:"processes"`
	Ports                  []jsonRuntimePort               `json:"ports"`
	RiskFindings           []runtimeDiagnosisAIRiskFinding `json:"riskFindings"`
	Evidences              []jsonRuntimeEvidence           `json:"evidences"`
	RecommendedTemplateIDs []string                        `json:"recommendedTemplateIds"`
	Truncated              bool                            `json:"truncated"`
	Pushed                 bool                            `json:"pushed"`
	CreatedAt              *time.Time                      `json:"createdAt"`
	StartedAt              *time.Time                      `json:"startedAt"`
	CollectedAt            *time.Time                      `json:"collectedAt"`
	FinishedAt             *time.Time                      `json:"finishedAt"`
}

type runtimeDiagnosisAIRiskFinding struct {
	Code         string   `json:"code"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	EvidenceRefs []string `json:"evidenceRefs"`
}

type runtimeDiagnosisAITemplate struct {
	TemplateID           string                           `json:"templateId"`
	TemplateName         string                           `json:"templateName"`
	Description          string                           `json:"description"`
	Category             string                           `json:"category"`
	Version              int                              `json:"version"`
	Platform             string                           `json:"platform"`
	RiskLevel            string                           `json:"riskLevel"`
	RenderMode           string                           `json:"renderMode"`
	RequiredCapabilities []string                         `json:"requiredCapabilities"`
	ParameterSchema      []commandTemplateParameterRecord `json:"parameterSchema"`
	RequiresHumanReview  bool                             `json:"requiresHumanReview"`
}

// GetRuntimeDiagnosisAIContext 按需读取固定的 AI 上下文接口。
// 服务端负责权限、资源范围、AI 开关和审计；MCP 只保留有限字段。
func (c *Client) GetRuntimeDiagnosisAIContext(ctx context.Context, id string) (RuntimeDiagnosisAIContext, error) {
	diagnosisID, err := validateUUID(id, "diagnosis ID")
	if err != nil {
		return RuntimeDiagnosisAIContext{}, err
	}
	var data runtimeDiagnosisAIContextRecord
	if err := c.do(ctx, http.MethodGet, []string{"runtime-diagnoses", diagnosisID, "ai-context"}, nil, nil, &data, true); err != nil {
		return RuntimeDiagnosisAIContext{}, err
	}
	return summarizeRuntimeDiagnosisAIContext(data), nil
}

func summarizeRuntimeDiagnosisAIContext(data runtimeDiagnosisAIContextRecord) RuntimeDiagnosisAIContext {
	result := RuntimeDiagnosisAIContext{
		Status:                           trimPublicText(data.Status, 40),
		Message:                          trimPublicText(data.Message, maxAIContextText),
		ResultMode:                       "on_demand_bounded_ai_context",
		SensitiveContentExcluded:         true,
		UnknownSensitiveContentMayRemain: true,
		RedactionApplied:                 data.Context != nil,
		RedactionPolicy:                  "server_declared_plus_conservative_patterns",
		Notice:                           "这是按需读取的脱敏 AI 上下文摘要；命令、路径、证据值、请求追踪信息、操作者、审计引用、凭据和模型地址不会返回。未返回的详细信息不代表诊断失败。",
	}
	if data.ModelProvider != nil {
		result.ModelProvider = &AIModelProviderStatus{
			Provider:           trimPublicText(data.ModelProvider.Provider, 64),
			Model:              trimPublicText(data.ModelProvider.Model, 128),
			Status:             trimPublicText(data.ModelProvider.Status, 40),
			EndpointConfigured: data.ModelProvider.EndpointConfigured,
			APIKeyConfigured:   data.ModelProvider.APIKeyConfigured,
			TimeoutSec:         data.ModelProvider.TimeoutSec,
			MaxTokens:          data.ModelProvider.MaxTokens,
		}
	}
	if data.Context != nil {
		contextView, truncated := summarizeRuntimeDiagnosisAIContextEnvelope(*data.Context)
		result.Context = &contextView
		result.Truncated = truncated
	}
	return result
}

func summarizeRuntimeDiagnosisAIContextEnvelope(data runtimeDiagnosisAIContextEnvelope) (RuntimeDiagnosisAIContextView, bool) {
	diagnosis, diagnosisTruncated := summarizeRuntimeDiagnosisAIContextDiagnosis(data.Diagnosis)
	riskFindings := make([]RuntimeDiagnosisAIRiskFinding, 0, minInt(len(data.RiskFindings), maxAIContextRiskFindings))
	truncated := diagnosisTruncated || len(data.RiskFindings) > maxAIContextRiskFindings
	for index, item := range data.RiskFindings {
		if index >= maxAIContextRiskFindings {
			break
		}
		message, _, messageTruncated := redactAIContextText(item.Message)
		riskFindings = append(riskFindings, RuntimeDiagnosisAIRiskFinding{
			Code: trimPublicText(item.Code, 100), Severity: trimPublicText(item.Severity, 40), Message: message,
		})
		truncated = truncated || messageTruncated
	}
	templates, templatesTruncated := summarizeRuntimeDiagnosisAITemplates(data.AvailableTemplates)
	truncated = truncated || templatesTruncated
	return RuntimeDiagnosisAIContextView{
		ContextVersion: trimPublicText(data.ContextVersion, 100),
		AgentScope: RuntimeDiagnosisAIAgentScope{
			AgentID: trimPublicText(data.AgentScope.AgentID, 64), Hostname: trimPublicText(data.AgentScope.Hostname, 255),
			Platform: trimPublicText(data.AgentScope.Platform, 64), ManagementLevel: trimPublicText(data.AgentScope.ManagementLevel, 64),
			Capabilities: trimContextCapabilities(data.AgentScope.Capabilities, &truncated),
		},
		Diagnosis:          diagnosis,
		RiskFindings:       riskFindings,
		AvailableTemplates: templates,
		ExecutionPolicy:    data.ExecutionPolicy,
		RedactionPolicy: RuntimeDiagnosisAIRedactionPolicy{
			Version: trimPublicText(data.RedactionPolicy.Version, 100), Summary: trimPublicText(data.RedactionPolicy.Summary, maxAIContextText),
		},
	}, truncated
}

func summarizeRuntimeDiagnosisAIContextDiagnosis(data runtimeDiagnosisAIContextDiagnosis) (RuntimeDiagnosisAIContextDiagnosis, bool) {
	summary, _, summaryTruncated := redactAIContextText(data.Summary)
	errorMessage, _, errorTruncated := redactAIContextText(data.ErrorMessage)
	templates, templatesTruncated := trimPublicStringList(data.RecommendedTemplateIDs, 64, maxAIContextTemplates)
	return RuntimeDiagnosisAIContextDiagnosis{
		ID: data.ID, AgentID: data.AgentID, TargetType: trimPublicText(data.TargetType, 64), Status: trimPublicText(data.Status, 40),
		Summary: summary, ErrorMessage: errorMessage, TimeoutSec: data.TimeoutSec, MaxResults: data.MaxResults,
		ProcessCount: len(data.Processes), PortCount: len(data.Ports), RiskFindingCount: len(data.RiskFindings), EvidenceCount: len(data.Evidences),
		RecommendedTemplateIDs: templates, Truncated: data.Truncated, Pushed: data.Pushed,
		CreatedAt: data.CreatedAt, StartedAt: data.StartedAt, CollectedAt: data.CollectedAt, FinishedAt: data.FinishedAt,
	}, summaryTruncated || errorTruncated || templatesTruncated
}

func summarizeRuntimeDiagnosisAITemplates(items []runtimeDiagnosisAITemplate) ([]RuntimeDiagnosisAITemplate, bool) {
	result := make([]RuntimeDiagnosisAITemplate, 0, minInt(len(items), maxAIContextTemplates))
	truncated := len(items) > maxAIContextTemplates
	for index, item := range items {
		if index >= maxAIContextTemplates {
			break
		}
		parameters := summarizeCommandTemplate(commandTemplateRecord{Parameters: item.ParameterSchema}).Parameters
		if len(item.ParameterSchema) > maxTemplateParameters {
			truncated = true
		}
		capabilities := trimContextCapabilities(item.RequiredCapabilities, &truncated)
		description, _, descriptionTruncated := redactAIContextText(item.Description)
		result = append(result, RuntimeDiagnosisAITemplate{
			TemplateID: item.TemplateID, TemplateName: trimPublicText(item.TemplateName, maxTemplateFieldLength),
			Description: description, Category: trimPublicText(item.Category, maxTemplateFieldLength), Version: item.Version,
			Platform: trimPublicText(item.Platform, maxTemplateFieldLength), RiskLevel: trimPublicText(item.RiskLevel, maxTemplateFieldLength),
			RenderMode: trimPublicText(item.RenderMode, maxTemplateFieldLength), RequiredCapabilities: capabilities,
			ParameterSchema: parameters, RequiresHumanReview: item.RequiresHumanReview,
		})
		truncated = truncated || descriptionTruncated
	}
	return result, truncated
}

func redactAIContextText(value string) (string, bool, bool) {
	value, truncated := trimPublicTextWithFlag(value, maxAIContextText)
	value, redacted := redactSensitiveTextUnbounded(value)
	return value, redacted, truncated
}

func trimContextCapabilities(values []string, truncated *bool) []string {
	result := make([]string, 0, minInt(len(values), maxAIContextCapabilities))
	for index, value := range values {
		if index >= maxAIContextCapabilities {
			if truncated != nil {
				*truncated = true
			}
			break
		}
		result = append(result, trimPublicText(value, maxTemplateFieldLength))
	}
	return result
}
