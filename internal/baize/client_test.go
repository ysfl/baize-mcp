package baize

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode/utf8"
)

func TestValidateAPIURL(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		allowHTTP bool
		wantErr   bool
	}{
		{name: "https", raw: "https://baize.example.com/api/v1"},
		{name: "loopback http", raw: "http://127.0.0.1:22501/api/v1"},
		{name: "confirmed http", raw: "http://10.0.0.10:22501/api/v1", allowHTTP: true},
		{name: "unconfirmed http", raw: "http://10.0.0.10:22501/api/v1", wantErr: true},
		{name: "wrong path", raw: "https://baize.example.com", wantErr: true},
		{name: "embedded credentials", raw: "https://" + "user:pass@" + "baize.example.com/api/v1", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ValidateAPIURL(tt.raw, tt.allowHTTP)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateAPIURL() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestClientReadsPrivacyReducedAgents(t *testing.T) {
	const agentID = "11111111-2222-3333-4444-555555555555"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/agents":
			if got := r.URL.Query().Get("page"); got != "2" {
				t.Fatalf("page = %q", got)
			}
			if got := r.URL.Query().Get("page_size"); got != "5" {
				t.Fatalf("page_size = %q", got)
			}
			if got := r.URL.Query().Get("search"); got != "web" {
				t.Fatalf("search = %q", got)
			}
			wantQuery := map[string]string{
				"search": "web", "alias": "frontend", "system": "linux", "region": "shanghai",
				"agent_version": "0.2", "arch": "arm64", "status": "online",
				"group_id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
				"sort_by":  "lastHeartbeatAt", "sort_order": "desc",
			}
			for name, want := range wantQuery {
				if got := r.URL.Query().Get(name); got != want {
					t.Fatalf("%s = %q, want %q", name, got, want)
				}
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + agentID + `","hostname":"web-01.internal","alias":"Web 01","status":"online","osType":"linux","osVersion":"Ubuntu 24.04","arch":"amd64","agentVersion":"0.2.1","lastHeartbeatAt":"2026-08-06T01:02:03Z","ipInternal":"10.0.0.1","ipExternal":"203.0.113.10","fingerprint":"sensitive-fingerprint","capabilities":["sensitive-capability"],"tokenId":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}],"total":1,"page":2,"pageSize":5}}`))
		case "/api/v1/agents/" + agentID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + agentID + `","hostname":"web-01.internal","alias":"Web 01","status":"online","osType":"linux","osVersion":"Ubuntu 24.04","arch":"amd64","agentVersion":"0.2.1","lastHeartbeatAt":"2026-08-06T01:02:03Z","ipInternal":"10.0.0.1","fingerprint":"sensitive-fingerprint","capabilities":["sensitive-capability"]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	page, err := client.ListAgents(context.Background(), AgentListOptions{
		Page: 2, PageSize: 5, Search: " web ", Alias: " frontend ", System: " linux ", Region: " shanghai ",
		AgentVersion: " 0.2 ", Architecture: " arm64 ", Status: " online ",
		GroupID: " AAAAAAAA-BBBB-CCCC-DDDD-EEEEEEEEEEEE ",
		SortBy:  " lastHeartbeatAt ", SortOrder: " DESC ",
	})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if page.Total != 1 || len(page.Items) != 1 {
		t.Fatalf("ListAgents() = %#v", page)
	}
	wantTime := time.Date(2026, time.August, 6, 1, 2, 3, 0, time.UTC)
	want := AgentSummary{
		ID: agentID, DisplayName: "Web 01", Status: "online", OperatingSystem: "linux Ubuntu 24.04",
		Architecture: "amd64", AgentVersion: "0.2.1", LastHeartbeatAt: &wantTime,
	}
	if got := page.Items[0]; !equalAgentSummary(got, want) {
		t.Fatalf("ListAgents() item = %#v, want %#v", got, want)
	}

	detail, err := client.GetAgent(context.Background(), strings.ToUpper(agentID))
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if !equalAgentSummary(detail, want) {
		t.Fatalf("GetAgent() = %#v, want %#v", detail, want)
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"ipInternal", "ipExternal", "fingerprint", "capabilities", "tokenId"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("privacy-reduced result contains %q: %s", forbidden, raw)
		}
	}
}

func TestClientRejectsInvalidAgentQuery(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.ListAgents(context.Background(), AgentListOptions{Page: 1, PageSize: 101}); err == nil {
		t.Fatal("ListAgents() accepted an oversized page")
	}
	if _, err := client.ListAgents(context.Background(), AgentListOptions{Page: 1, PageSize: 20, GroupID: "not-a-uuid"}); err == nil {
		t.Fatal("ListAgents() accepted an invalid group ID")
	}
	if _, err := client.ListAgents(context.Background(), AgentListOptions{Page: 1, PageSize: 20, SortOrder: "sideways"}); err == nil {
		t.Fatal("ListAgents() accepted an invalid sort order")
	}
	if _, err := client.ListAgents(context.Background(), AgentListOptions{Page: 1, PageSize: 20, SortBy: "private_field"}); err == nil {
		t.Fatal("ListAgents() accepted an unsupported sort field")
	}
	if _, err := client.GetAgent(context.Background(), "../auth/profile"); err == nil {
		t.Fatal("GetAgent() accepted an invalid ID")
	}
}

func TestClientCommandWorkflowUsesPublishedEndpointsAndReducesFields(t *testing.T) {
	const (
		templateID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		agentID    = "11111111-2222-3333-4444-555555555555"
		planID     = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
		taskID     = "cccccccc-dddd-eeee-ffff-000000000000"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		longPreview := strings.Repeat("x", maxPreviewLength+1)
		switch r.Method + " " + r.URL.Path {
		case http.MethodGet + " /api/v1/ops/command-templates":
			if got := r.URL.Query().Get("status"); got != "enabled" {
				t.Fatalf("template status = %q", got)
			}
			if got := r.URL.Query().Get("risk_level"); got != "low" {
				t.Fatalf("template risk level = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + templateID + `","name":"Restart service","description":"restart","command":"systemctl restart {{service}}","workDir":"/srv","timeoutSec":30,"status":"enabled","riskLevel":"low","renderMode":"shell","version":2,"platform":"linux","parameters":[{"name":"service","type":"service_name","required":true,"default":"nginx","secret":false}],"requiredCapabilities":["exec.task"]}],"total":1,"page":1,"pageSize":20}}`))
		case http.MethodPost + " /api/v1/ops/command-templates/" + templateID + "/render":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode render body: %v", err)
			}
			if body["dryRun"] != true {
				t.Fatalf("render dryRun = %#v", body["dryRun"])
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"templateId":"` + templateID + `","templateName":"Restart service","templateVersion":2,"renderMode":"shell","riskLevel":"low","renderedPreview":"` + longPreview + `","commandHash":"hash","precheckPassed":true,"dryRun":true,"parameterSnapshot":{"service":"nginx"}}}`))
		case http.MethodPost + " /api/v1/ops/command-plans":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode plan body: %v", err)
			}
			if body["templateId"] != templateID || body["targetAgentIds"].([]any)[0] != agentID {
				t.Fatalf("unexpected plan body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + planID + `","templateId":"` + templateID + `","templateName":"Restart service","templateVersion":2,"title":"Restart","riskLevel":"low","renderMode":"shell","renderedPreview":"systemctl restart nginx","commandHash":"hash","workDir":"/srv","parameters":{"service":"nginx"},"timeoutSec":30,"targetAgentIds":["` + agentID + `"],"precheck":{"precheckPassed":true},"approvalRequired":false,"operatorId":"private-user","operatorName":"Operator","status":"ready"}}`))
		case http.MethodGet + " /api/v1/ops/command-plans/" + planID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + planID + `","templateId":"` + templateID + `","templateName":"Restart service","templateVersion":2,"title":"Restart","riskLevel":"low","renderMode":"shell","renderedPreview":"systemctl restart nginx","commandHash":"hash","workDir":"/srv","parameters":{"service":"nginx"},"timeoutSec":30,"targetAgentIds":["` + agentID + `"],"precheck":{"precheckPassed":true},"approvalRequired":false,"operatorId":"private-user","operatorName":"Operator","status":"ready"}}`))
		case http.MethodPost + " /api/v1/ops/command-plans/" + planID + "/execute":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode execute body: %v", err)
			}
			if body["confirmRisk"] != true || body["autoDispatch"] != false || body["confirmMessage"] != "maintenance" {
				t.Fatalf("unexpected execute body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"plan":{"id":"` + planID + `","templateId":"` + templateID + `","title":"Restart","riskLevel":"low","renderMode":"shell","renderedPreview":"secret-command","commandHash":"hash","workDir":"/srv","parameters":{"service":"nginx"},"targetAgentIds":["` + agentID + `"],"precheck":{"precheckPassed":true},"approvalRequired":false,"status":"executed","operatorName":"Operator"},"task":{"id":"` + taskID + `","taskType":"command","title":"Restart","command":"secret-command","workDir":"/srv","envVars":{"TOKEN":"secret"},"operatorName":"Operator","status":"pending","targets":[{"id":"dddddddd-eeee-ffff-0000-111111111111","agentId":"` + agentID + `","status":"pending","outputSize":0}]}}}`))
		case http.MethodGet + " /api/v1/ops/tasks/" + taskID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + taskID + `","taskType":"command","title":"Restart","command":"secret-command","workDir":"/srv","envVars":{"TOKEN":"secret"},"operatorName":"Operator","status":"pending","targets":[{"id":"dddddddd-eeee-ffff-0000-111111111111","agentId":"` + agentID + `","status":"pending","outputSize":0}]}}`))
		case http.MethodPost + " /api/v1/ops/tasks/" + taskID + "/cancel":
			_, _ = w.Write([]byte(`{"code":0,"data":null}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	page, err := client.ListCommandTemplates(context.Background(), CommandTemplateListOptions{Page: 1, PageSize: 20, RiskLevel: "LOW"})
	if err != nil || len(page.Items) != 1 || page.Items[0].Parameters[0].Name != "service" {
		t.Fatalf("ListCommandTemplates() = %#v, error = %v", page, err)
	}
	preview, err := client.PreviewCommandTemplate(context.Background(), CommandTemplateRenderOptions{TemplateID: templateID, AgentIDs: []string{agentID}, Parameters: map[string]any{"service": "nginx"}})
	if err != nil {
		t.Fatalf("PreviewCommandTemplate() error = %v", err)
	}
	if len(preview.RenderedPreview) != maxPreviewLength || !preview.PreviewTruncated {
		t.Fatalf("preview truncation = %#v", preview)
	}
	plan, err := client.CreateCommandPlan(context.Background(), CommandPlanCreateOptions{TemplateID: templateID, TargetAgentIDs: []string{agentID}, Parameters: map[string]any{"service": "nginx"}})
	if err != nil {
		t.Fatalf("CreateCommandPlan() error = %v", err)
	}
	if plan.ID != planID || plan.CommandHash != "hash" {
		t.Fatalf("plan = %#v", plan)
	}
	if raw, marshalErr := json.Marshal(plan); marshalErr != nil {
		t.Fatalf("marshal plan: %v", marshalErr)
	} else if strings.Contains(string(raw), "renderedPreview") || strings.Contains(string(raw), "parameters") || strings.Contains(string(raw), "workDir") || strings.Contains(string(raw), "operatorName") {
		t.Fatalf("plan exposed private fields: %s", raw)
	}
	if _, err := client.GetCommandPlan(context.Background(), planID); err != nil {
		t.Fatalf("GetCommandPlan() error = %v", err)
	}
	execution, err := client.ExecuteCommandPlan(context.Background(), planID, CommandPlanExecuteOptions{AutoDispatch: boolPtr(false), ConfirmRisk: true, ConfirmMessage: "maintenance"})
	if err != nil || execution.Task.ID != taskID {
		t.Fatalf("ExecuteCommandPlan() = %#v, error = %v", execution, err)
	}
	if raw, marshalErr := json.Marshal(execution); marshalErr != nil {
		t.Fatalf("marshal execution: %v", marshalErr)
	} else if strings.Contains(string(raw), "secret-command") || strings.Contains(string(raw), "envVars") || strings.Contains(string(raw), "operatorName") {
		t.Fatalf("execution exposed private fields: %s", raw)
	}
	if _, err := client.GetExecTask(context.Background(), taskID); err != nil {
		t.Fatalf("GetExecTask() error = %v", err)
	}
	if err := client.CancelExecTask(context.Background(), taskID); err != nil {
		t.Fatalf("CancelExecTask() error = %v", err)
	}
}

func TestClientRejectsCommandWorkflowInput(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.ListCommandTemplates(context.Background(), CommandTemplateListOptions{Page: 1, PageSize: 20, RiskLevel: "dangerous"}); err == nil {
		t.Fatal("ListCommandTemplates() accepted an invalid risk level")
	}
	if _, err := client.PreviewCommandTemplate(context.Background(), CommandTemplateRenderOptions{TemplateID: "bad", AgentIDs: []string{"bad"}}); err == nil {
		t.Fatal("PreviewCommandTemplate() accepted invalid IDs")
	}
	if _, err := client.CreateCommandPlan(context.Background(), CommandPlanCreateOptions{TemplateID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", TargetAgentIDs: []string{"11111111-2222-3333-4444-555555555555"}, Parameters: map[string]any{"nested": map[string]any{"value": "no"}}}); err == nil {
		t.Fatal("CreateCommandPlan() accepted a non-scalar parameter")
	}
	if _, err := client.ExecuteCommandPlan(context.Background(), "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", CommandPlanExecuteOptions{ConfirmMessage: strings.Repeat("x", maxReasonLength+1)}); err == nil {
		t.Fatal("ExecuteCommandPlan() accepted an oversized confirmation message")
	}
}

func TestClientCommandPlanApprovalWorkflowUsesPublishedEndpointsAndRedactsSnapshot(t *testing.T) {
	const (
		planID     = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
		approvalID = "eeeeeeee-ffff-0000-1111-222222222222"
		targetID   = "11111111-2222-3333-4444-555555555555"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /api/v1/ops/command-plans/" + planID + "/approvals":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode approval request: %v", err)
			}
			if body["reason"] != "critical maintenance" || body["expiresAt"] == nil {
				t.Fatalf("unexpected approval request: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + approvalID + `","planId":"` + planID + `","riskLevel":"critical","status":"pending","reason":"critical maintenance","requesterId":"private-user","requesterName":"Operator","planSnapshot":{"templateId":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","templateName":"Restart service","templateVersion":2,"title":"Restart","riskLevel":"critical","commandHash":"hash","renderedPreview":"do-not-expose","workDir":"/srv","parameters":{"secret":"hidden"},"targetAgentIds":["` + targetID + `"],"precheck":{"precheckPassed":true},"warnings":[]},"policySnapshot":{"requiredApproverPermission":"private.permission"}}}`))
		case http.MethodPost + " /api/v1/ops/command-plans/" + planID + "/cancel":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode cancel request: %v", err)
			}
			if body["reason"] != "no longer needed" {
				t.Fatalf("unexpected cancel request: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + planID + `","templateId":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee","templateName":"Restart service","riskLevel":"critical","status":"cancelled","targetAgentIds":["` + targetID + `"]}}`))
		case http.MethodGet + " /api/v1/ops/command-plan-approvals":
			if got := r.URL.Query().Get("page_size"); got != "10" {
				t.Fatalf("page_size = %q", got)
			}
			if got := r.URL.Query().Get("status"); got != "pending" {
				t.Fatalf("status = %q", got)
			}
			if got := r.URL.Query().Get("riskLevel"); got != "critical" {
				t.Fatalf("riskLevel = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"id":"` + approvalID + `","planId":"` + planID + `","riskLevel":"critical","status":"pending","reason":"critical maintenance","planSnapshot":{"templateName":"Restart service","renderedPreview":"do-not-expose","parameters":{"secret":"hidden"},"targetAgentIds":["` + targetID + `"]}}],"total":20,"page":1,"pageSize":10}}`))
		case http.MethodGet + " /api/v1/ops/command-plan-approvals/" + approvalID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + approvalID + `","planId":"` + planID + `","riskLevel":"critical","status":"approved","reason":"critical maintenance","decisionMessage":"approved","requesterName":"Operator","approverName":"Approver","planSnapshot":{"templateName":"Restart service","commandHash":"hash","renderedPreview":"do-not-expose","parameters":{"secret":"hidden"},"targetAgentIds":["` + targetID + `"],"precheck":{"precheckPassed":true}}}}`))
		case http.MethodPost + " /api/v1/ops/command-plan-approvals/" + approvalID + "/decision":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode approval decision: %v", err)
			}
			if body["approved"] != true || body["decisionMessage"] != "approved" {
				t.Fatalf("unexpected approval decision: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + approvalID + `","planId":"` + planID + `","riskLevel":"critical","status":"approved","decisionMessage":"approved"}}`))
		case http.MethodGet + " /api/v1/ops/command-plan-approval-policies":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"riskLevel":"high","enabled":true,"allowSelfApproval":false,"requiredApproverPermission":"private.permission","notificationChannelIds":["secret-channel"]},{"riskLevel":"critical","enabled":true,"allowSelfApproval":true},{"riskLevel":"internal-policy-name","enabled":true,"allowSelfApproval":true}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	expiresAt := time.Now().Add(10 * time.Minute)
	approval, err := client.RequestCommandPlanApproval(context.Background(), CommandPlanApprovalCreateOptions{PlanID: planID, Reason: "critical maintenance", ExpiresAt: &expiresAt})
	if err != nil {
		t.Fatalf("RequestCommandPlanApproval() error: %v", err)
	}
	if approval.ID != approvalID || approval.PlanSnapshot == nil || approval.PlanSnapshot.TemplateName != "Restart service" {
		t.Fatalf("approval = %#v", approval)
	}
	raw, err := json.Marshal(approval)
	if err != nil {
		t.Fatalf("marshal approval: %v", err)
	}
	for _, forbidden := range []string{"renderedPreview", "do-not-expose", "workDir", "parameters", "secret", "policySnapshot", "private.permission", "requesterName", "approverName"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("approval exposed %q: %s", forbidden, raw)
		}
	}
	page, err := client.ListCommandPlanApprovals(context.Background(), CommandPlanApprovalListOptions{Page: 1, PageSize: 10, Status: "pending", RiskLevel: "critical"})
	if err != nil || !page.HasMore || page.NextPage != 2 || page.Total != 20 {
		t.Fatalf("approval page = %#v, error = %v", page, err)
	}
	detail, err := client.GetCommandPlanApproval(context.Background(), approvalID)
	if err != nil || detail.Status != "approved" {
		t.Fatalf("approval detail = %#v, error = %v", detail, err)
	}
	if _, err := client.DecideCommandPlanApproval(context.Background(), approvalID, CommandPlanApprovalDecisionOptions{Approved: true, DecisionMessage: "approved"}); err != nil {
		t.Fatalf("DecideCommandPlanApproval() error: %v", err)
	}
	plan, err := client.CancelCommandPlan(context.Background(), planID, "no longer needed")
	if err != nil || plan.Status != "cancelled" {
		t.Fatalf("CancelCommandPlan() = %#v, error: %v", plan, err)
	}
	policies, err := client.ListCommandPlanApprovalPolicies(context.Background())
	if err != nil || len(policies) != 2 || policies[1].AllowSelfApproval != true {
		t.Fatalf("ListCommandPlanApprovalPolicies() = %#v, error: %v", policies, err)
	}
	rawPolicies, err := json.Marshal(policies)
	if err != nil {
		t.Fatalf("marshal policies: %v", err)
	}
	for _, forbidden := range []string{"private.permission", "secret-channel", "notificationChannelIds", "internal-policy-name"} {
		if strings.Contains(string(rawPolicies), forbidden) {
			t.Fatalf("policy summary exposed %q: %s", forbidden, rawPolicies)
		}
	}
}

func TestClientRejectsInvalidCommandPlanApprovalInput(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error: %v", err)
	}
	if _, err := client.RequestCommandPlanApproval(context.Background(), CommandPlanApprovalCreateOptions{PlanID: "bad", Reason: "reason"}); err == nil {
		t.Fatal("RequestCommandPlanApproval() accepted invalid plan ID")
	}
	if _, err := client.RequestCommandPlanApproval(context.Background(), CommandPlanApprovalCreateOptions{PlanID: "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"}); err == nil {
		t.Fatal("RequestCommandPlanApproval() accepted an empty reason")
	}
	if _, err := client.ListCommandPlanApprovals(context.Background(), CommandPlanApprovalListOptions{Page: 1, PageSize: maxApprovalPageSize + 1}); err == nil {
		t.Fatal("ListCommandPlanApprovals() accepted an oversized page")
	}
	if _, err := client.ListCommandPlanApprovals(context.Background(), CommandPlanApprovalListOptions{Page: 1, PageSize: 10, Status: "unknown"}); err == nil {
		t.Fatal("ListCommandPlanApprovals() accepted an invalid status")
	}
	if _, err := client.DecideCommandPlanApproval(context.Background(), "eeeeeeee-ffff-0000-1111-222222222222", CommandPlanApprovalDecisionOptions{}); err == nil {
		t.Fatal("DecideCommandPlanApproval() accepted an empty rejection message")
	}
	if _, err := client.CancelCommandPlan(context.Background(), "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", strings.Repeat("x", maxReasonLength+1)); err == nil {
		t.Fatal("CancelCommandPlan() accepted an oversized reason")
	}
}

func boolPtr(value bool) *bool {
	return &value
}

func TestTrimPublicTextPreservesUTF8Boundaries(t *testing.T) {
	got, truncated := trimPublicTextWithFlag("中文中文a", 7)
	if got != "中文" || !truncated || !utf8.ValidString(got) {
		t.Fatalf("trimPublicTextWithFlag() = %q, %v", got, truncated)
	}
}

func TestSummariesBoundTemplateAndPrecheckCollections(t *testing.T) {
	parameters := make([]commandTemplateParameterRecord, maxTemplateParameters+1)
	for index := range parameters {
		parameters[index] = commandTemplateParameterRecord{Name: "parameter", Type: "string", EnumValues: []string{"one", "two"}}
	}
	capabilities := make([]string, maxTemplateCapabilities+1)
	for index := range capabilities {
		capabilities[index] = "capability"
	}
	template := summarizeCommandTemplate(commandTemplateRecord{
		CommandTemplateSummary: CommandTemplateSummary{Name: strings.Repeat("名", 300), Description: strings.Repeat("描述", 300)},
		Parameters:             parameters,
		RequiredCapabilities:   capabilities,
	})
	if len(template.Parameters) != maxTemplateParameters || len(template.RequiredCapabilities) != maxTemplateCapabilities || !template.ParametersTruncated {
		t.Fatalf("template bounds = %#v", template)
	}
	if !utf8.ValidString(template.Name) || len(template.Name) > maxTemplateFieldLength || len(template.Description) > maxTemplateDescription {
		t.Fatalf("template text bounds = %#v", template)
	}

	precheckItems := make([]PrecheckItem, maxPrecheckItems+1)
	for index := range precheckItems {
		precheckItems[index] = PrecheckItem{Code: "code", Level: "warning", Message: "message"}
	}
	plan := summarizeCommandPlan(commandPlanRecord{
		ID:       "bbbbbbbb-cccc-dddd-eeee-ffffffffffff",
		Precheck: mustJSONRaw(t, PrecheckSummary{PrecheckPassed: false, BlockedReasons: precheckItems}),
		Warnings: precheckItems,
	})
	if len(plan.Precheck.BlockedReasons) != maxPrecheckItems || len(plan.Warnings) != maxPrecheckItems || !plan.PrecheckTruncated {
		t.Fatalf("plan bounds = %#v", plan)
	}

	targets := make([]execTargetRecord, maxTaskTargets+1)
	for index := range targets {
		targets[index] = execTargetRecord{ID: "dddddddd-eeee-ffff-0000-111111111111", AgentID: "11111111-2222-3333-4444-555555555555", Status: strings.Repeat("状态", 200)}
	}
	task := summarizeExecTask(execTaskRecord{TaskType: strings.Repeat("任务", 200), Title: strings.Repeat("标题", 300), Status: strings.Repeat("状态", 200), Targets: targets})
	if len(task.Targets) != maxTaskTargets || !task.TargetsTruncated {
		t.Fatalf("task bounds = %#v", task)
	}
	if len(task.TaskType) > maxTemplateFieldLength || len(task.Title) > maxReasonLength || len(task.Status) > maxTemplateFieldLength {
		t.Fatalf("task text bounds = %#v", task)
	}
}

func mustJSONRaw(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal test JSON: %v", err)
	}
	return raw
}

func equalAgentSummary(a, b AgentSummary) bool {
	if a.ID != b.ID || a.DisplayName != b.DisplayName || a.Status != b.Status ||
		a.OperatingSystem != b.OperatingSystem || a.Architecture != b.Architecture || a.AgentVersion != b.AgentVersion {
		return false
	}
	if a.LastHeartbeatAt == nil || b.LastHeartbeatAt == nil {
		return a.LastHeartbeatAt == nil && b.LastHeartbeatAt == nil
	}
	return a.LastHeartbeatAt.Equal(*b.LastHeartbeatAt)
}

func TestClientAuthenticationFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/auth/login":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode login body: %v", err)
			}
			if body["username"] != "operator" || body["password"] != "secret" {
				t.Fatalf("unexpected login body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"token":"session-token","username":"operator","role":"viewer"}}`))
		case "/api/v1/auth/profile":
			if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
				t.Fatalf("Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"username":"operator","role":"viewer","roles":["viewer"]}}`))
		case "/api/v1/auth/logout":
			if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
				t.Fatalf("Authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"revoked":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	loginClient, err := NewClient(server.URL+"/api/v1", "", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	token, err := loginClient.Login(context.Background(), "operator", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if token != "session-token" {
		t.Fatalf("Login() token = %q", token)
	}

	client, err := NewClient(server.URL+"/api/v1", token, true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.CheckSession(context.Background()); err != nil {
		t.Fatalf("CheckSession() error = %v", err)
	}
	if err := client.Logout(context.Background()); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
}

func TestClientBlocksCrossOriginRedirect(t *testing.T) {
	var receivedAuthorization atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			receivedAuthorization.Store(true)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{}}`))
	}))
	defer target.Close()

	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer source.Close()

	client, err := NewClient(source.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if err := client.CheckSession(context.Background()); err == nil {
		t.Fatal("CheckSession() followed a cross-origin redirect")
	}
	if receivedAuthorization.Load() {
		t.Fatal("cross-origin target received Authorization header")
	}
}

func TestClientDoesNotExposeErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"traceId":"trace-1","details":{"secret":"do-not-return"}}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	err = client.CheckSession(context.Background())
	if err == nil {
		t.Fatal("CheckSession() error = nil")
	}
	if strings.Contains(err.Error(), "do-not-return") {
		t.Fatalf("error exposed response body: %v", err)
	}
}

func TestClientDoesNotExposeTraceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("{\"traceId\":\"trace-1\\nsecret\"}"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	err = client.CheckSession(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CheckSession() error = %v, want APIError", err)
	}
	if strings.Contains(err.Error(), "trace-1") || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error exposed trace data: %v", err)
	}
}
