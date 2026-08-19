package baize

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const (
	outputTestTaskID   = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	outputTestTargetID = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	outputTestAgentID  = "11111111-2222-3333-4444-555555555555"
)

func TestGetExecTaskOutputUsesBoundedQueryAndWarnsAboutResultLimits(t *testing.T) {
	var gotPath string
	var gotQuery = make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery <- r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{
			map[string]any{
				"targetId": outputTestTargetID,
				"agentId":  outputTestAgentID,
				"status":   "completed",
				"outputs": []any{
					map[string]any{"seq": 7, "stream": "stdout", "data": "password=top-secret"},
					map[string]any{"seq": 8, "stream": "stderr", "data": "done"},
				},
				"output": map[string]any{"total": 5, "limit": 2, "mode": "page", "outputs": []any{
					map[string]any{"seq": 7, "stream": "stdout", "data": "password=top-secret"},
					map[string]any{"seq": 8, "stream": "stderr", "data": "done"},
				}},
			},
		}})
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	after := 6
	result, err := client.GetExecTaskOutput(t.Context(), ExecTaskOutputOptions{TaskID: outputTestTaskID, TargetID: outputTestTargetID, Limit: 2, Mode: "page", AfterSeq: &after, TargetLimit: 1, TargetOffset: 3})
	if err != nil {
		t.Fatalf("GetExecTaskOutput() error = %v", err)
	}
	if gotPath != "/api/v1/ops/tasks/"+outputTestTaskID+"/output" {
		t.Fatalf("path = %q", gotPath)
	}
	query := <-gotQuery
	for _, want := range []string{"mode=page", "limit=2", "target_id=" + outputTestTargetID, "after_seq=6", "target_limit=1", "target_offset=3"} {
		if !strings.Contains(query, want) {
			t.Fatalf("query %q does not contain %q", query, want)
		}
	}
	if len(result.Targets) != 1 || result.Targets[0].Total != 5 || !result.Targets[0].HasMore || result.Targets[0].NextAfterSeq != 8 {
		t.Fatalf("output pagination = %#v", result.Targets)
	}
	line := result.Targets[0].Outputs[0]
	if line.Data != "password=******" || !result.RedactionApplied || !result.UnknownSensitiveContentMayRemain {
		t.Fatalf("output redaction = %#v", result)
	}
	for _, phrase := range []string{"当前返回为用户明确请求后的有界任务输出", "本次结果发生截断", "发生了保守替换", "未知敏感内容仍可能存在", "未返回内容不代表任务失败", "不要重复提交或重复执行任务"} {
		if !strings.Contains(result.Notice, phrase) {
			t.Fatalf("notice %q does not contain %q", result.Notice, phrase)
		}
	}
}

func TestGetExecTaskOutputEnforcesInputBoundsAndOutputCaps(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	before := 5
	for name, options := range map[string]ExecTaskOutputOptions{
		"both cursors":         {TaskID: outputTestTaskID, AfterSeq: new(int), BeforeSeq: &before},
		"negative cursor":      {TaskID: outputTestTaskID, AfterSeq: func() *int { value := -1; return &value }()},
		"invalid limit":        {TaskID: outputTestTaskID, Limit: TaskOutputMaxLimit + 1},
		"invalid mode":         {TaskID: outputTestTaskID, Mode: "all"},
		"invalid target limit": {TaskID: outputTestTaskID, TargetLimit: TaskOutputMaxTargets + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := client.GetExecTaskOutput(t.Context(), options); err == nil {
				t.Fatal("GetExecTaskOutput() accepted invalid input")
			}
		})
	}

	longLine := strings.Repeat("x", TaskOutputMaxLine+100)
	records := make([]execTaskOutputRecord, 0, 2)
	for i := 0; i < 2; i++ {
		records = append(records, execTaskOutputRecord{TargetID: outputTestTargetID, AgentID: outputTestAgentID, Status: "completed", Outputs: []execOutputRecord{{Seq: i + 1, Stream: "stdout", Data: longLine}}})
	}
	result := summarizeExecTaskOutput(outputTestTaskID, records)
	if !result.Truncated || !result.Targets[0].HasMore || len(result.Targets[0].Outputs[0].Data) != TaskOutputMaxLine || !strings.Contains(result.Notice, "发生截断") {
		t.Fatalf("bounded output result = %#v", result)
	}
}
