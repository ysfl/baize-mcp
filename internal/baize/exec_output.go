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
	TaskOutputDefaultLimit = 50
	TaskOutputMaxLimit     = 200
	TaskOutputMaxBytes     = 16 << 10
	TaskOutputMaxLine      = 4096
	TaskOutputMaxTargets   = 20
)

// ExecTaskOutputOptions 是按需读取远程任务输出的受控参数。
// 默认使用服务端尾读；游标和目标窗口用于继续读取，不自动遍历全部输出。
type ExecTaskOutputOptions struct {
	TaskID       string
	TargetID     string
	Limit        int
	Mode         string
	AfterSeq     *int
	BeforeSeq    *int
	TargetLimit  int
	TargetOffset int
}

type ExecTaskOutputSummary struct {
	TaskID                           string                        `json:"taskId"`
	ResultMode                       string                        `json:"resultMode"`
	SensitiveContentExcluded         bool                          `json:"sensitiveContentExcluded"`
	UnknownSensitiveContentMayRemain bool                          `json:"unknownSensitiveContentMayRemain"`
	RedactionApplied                 bool                          `json:"redactionApplied"`
	RedactionPolicy                  string                        `json:"redactionPolicy"`
	Truncated                        bool                          `json:"truncated"`
	Notice                           string                        `json:"notice"`
	Targets                          []ExecTaskOutputTargetSummary `json:"targets"`
}

type ExecTaskOutputTargetSummary struct {
	TargetID     string               `json:"targetId"`
	AgentID      string               `json:"agentId"`
	Status       string               `json:"status"`
	ExitCode     *int                 `json:"exitCode,omitempty"`
	Total        int64                `json:"total"`
	Mode         string               `json:"mode"`
	Outputs      []ExecTaskOutputLine `json:"outputs"`
	HasMore      bool                 `json:"hasMore"`
	NextAfterSeq int                  `json:"nextAfterSeq,omitempty"`
}

type ExecTaskOutputLine struct {
	Seq       int        `json:"seq"`
	Stream    string     `json:"stream"`
	Data      string     `json:"data"`
	CreatedAt *time.Time `json:"createdAt,omitempty"`
}

type execTaskOutputRecord struct {
	TargetID string                `json:"targetId"`
	AgentID  string                `json:"agentId"`
	Status   string                `json:"status"`
	ExitCode *int                  `json:"exitCode"`
	Outputs  []execOutputRecord    `json:"outputs"`
	Output   *execOutputPageRecord `json:"output"`
}

type execOutputRecord struct {
	Seq       int        `json:"seq"`
	Stream    string     `json:"stream"`
	Data      string     `json:"data"`
	CreatedAt *time.Time `json:"createdAt"`
}

type execOutputPageRecord struct {
	Outputs []execOutputRecord `json:"outputs"`
	Total   int64              `json:"total"`
	Limit   int                `json:"limit"`
	Mode    string             `json:"mode"`
}

// GetExecTaskOutput 按需读取远程任务输出，并明确返回裁剪与保守替换状态。
func (c *Client) GetExecTaskOutput(ctx context.Context, options ExecTaskOutputOptions) (ExecTaskOutputSummary, error) {
	taskID, err := validateUUID(options.TaskID, "execution task ID")
	if err != nil {
		return ExecTaskOutputSummary{}, err
	}
	limit := options.Limit
	if limit == 0 {
		limit = TaskOutputDefaultLimit
	}
	if limit < 1 || limit > TaskOutputMaxLimit {
		return ExecTaskOutputSummary{}, newInputError(fmt.Sprintf("output limit must be between 1 and %d", TaskOutputMaxLimit))
	}
	mode := strings.ToLower(strings.TrimSpace(options.Mode))
	if mode == "" {
		mode = "tail"
	}
	if mode != "tail" && mode != "page" {
		return ExecTaskOutputSummary{}, newInputError("output mode must be tail or page")
	}
	if options.AfterSeq != nil && options.BeforeSeq != nil {
		return ExecTaskOutputSummary{}, newInputError("afterSeq and beforeSeq cannot be used together")
	}
	targetID := strings.TrimSpace(options.TargetID)
	if targetID != "" {
		if targetID, err = validateUUID(targetID, "target ID"); err != nil {
			return ExecTaskOutputSummary{}, err
		}
	}
	if options.TargetLimit < 0 || options.TargetLimit > TaskOutputMaxTargets {
		return ExecTaskOutputSummary{}, newInputError(fmt.Sprintf("target limit must be between 0 and %d", TaskOutputMaxTargets))
	}
	if options.TargetOffset < 0 {
		return ExecTaskOutputSummary{}, newInputError("target offset must be non-negative")
	}
	query := url.Values{"mode": {mode}, "limit": {strconv.Itoa(limit)}}
	if targetID != "" {
		query.Set("target_id", targetID)
	}
	if options.AfterSeq != nil {
		if *options.AfterSeq < 0 {
			return ExecTaskOutputSummary{}, newInputError("afterSeq must be non-negative")
		}
		query.Set("after_seq", strconv.Itoa(*options.AfterSeq))
	}
	if options.BeforeSeq != nil {
		if *options.BeforeSeq < 0 {
			return ExecTaskOutputSummary{}, newInputError("beforeSeq must be non-negative")
		}
		query.Set("before_seq", strconv.Itoa(*options.BeforeSeq))
	}
	if options.TargetLimit > 0 {
		query.Set("target_limit", strconv.Itoa(options.TargetLimit))
	}
	if options.TargetOffset > 0 {
		query.Set("target_offset", strconv.Itoa(options.TargetOffset))
	}
	var data []execTaskOutputRecord
	if err := c.do(ctx, http.MethodGet, []string{"ops", "tasks", taskID, "output"}, query, nil, &data, true); err != nil {
		return ExecTaskOutputSummary{}, err
	}
	return summarizeExecTaskOutput(taskID, data), nil
}

func summarizeExecTaskOutput(taskID string, records []execTaskOutputRecord) ExecTaskOutputSummary {
	result := ExecTaskOutputSummary{
		TaskID:                           taskID,
		ResultMode:                       "on_demand_bounded_output",
		SensitiveContentExcluded:         true,
		UnknownSensitiveContentMayRemain: true,
		RedactionPolicy:                  "conservative_patterns_only",
		Notice:                           "这是用户明确请求后的有界任务输出，不是完整原始输出。MCP 只对明显的密码、令牌、Authorization 等模式做保守替换，不能识别所有敏感信息；未返回的内容不代表任务失败。若摘要不足，请先根据 nextAfterSeq 或目标窗口继续读取，不要重复提交任务。",
		Targets:                          make([]ExecTaskOutputTargetSummary, 0, minInt(len(records), TaskOutputMaxTargets)),
	}
	bytesUsed := 0
	for index, record := range records {
		if index >= TaskOutputMaxTargets {
			result.Truncated = true
			break
		}
		lines := record.Outputs
		total := int64(len(lines))
		mode := "tail"
		if record.Output != nil {
			if len(record.Output.Outputs) > 0 || record.Output.Total > 0 {
				lines = record.Output.Outputs
			}
			total = record.Output.Total
			mode = trimPublicText(record.Output.Mode, 32)
			if mode == "" {
				mode = "tail"
			}
		}
		target := ExecTaskOutputTargetSummary{TargetID: record.TargetID, AgentID: record.AgentID, Status: trimPublicText(record.Status, 64), ExitCode: record.ExitCode, Total: total, Mode: mode, Outputs: make([]ExecTaskOutputLine, 0, minInt(len(lines), TaskOutputMaxLimit))}
		for lineIndex, line := range lines {
			if lineIndex >= TaskOutputMaxLimit {
				result.Truncated = true
				target.HasMore = true
				break
			}
			data, redacted := redactSensitiveTextUnbounded(line.Data)
			result.RedactionApplied = result.RedactionApplied || redacted
			data, lineTruncated := trimPublicTextWithFlag(data, TaskOutputMaxLine)
			if lineTruncated {
				result.Truncated = true
				target.HasMore = true
			}
			lineBytes := len(data)
			if bytesUsed+lineBytes > TaskOutputMaxBytes {
				result.Truncated = true
				target.HasMore = true
				break
			}
			target.Outputs = append(target.Outputs, ExecTaskOutputLine{Seq: line.Seq, Stream: trimPublicText(line.Stream, 32), Data: data, CreatedAt: line.CreatedAt})
			bytesUsed += lineBytes
		}
		if total > int64(len(target.Outputs)) {
			target.HasMore = true
			result.Truncated = true
		}
		if target.HasMore && len(target.Outputs) > 0 {
			target.NextAfterSeq = target.Outputs[len(target.Outputs)-1].Seq
		}
		result.Targets = append(result.Targets, target)
	}
	result.Notice = execTaskOutputNotice(result)
	return result
}

func execTaskOutputNotice(result ExecTaskOutputSummary) string {
	truncation := "本次结果未截断"
	if result.Truncated {
		truncation = "本次结果发生截断"
	}
	redaction := "本次未发生保守替换"
	if result.RedactionApplied {
		redaction = "本次发生了保守替换"
	}
	return trimPublicText(fmt.Sprintf("当前返回为用户明确请求后的有界任务输出；%s；%s。MCP 只对明显的密码、令牌、Authorization 等模式做保守替换，未知敏感内容仍可能存在。未返回内容不代表任务失败；若需要继续读取，请根据 nextAfterSeq 或目标窗口分页，不要重复提交或重复执行任务。命令、环境变量、凭据和完整原始终端流不会返回。", truncation, redaction), 1200)
}
