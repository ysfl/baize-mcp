package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ysfl/baize-mcp/internal/baize"
)

type fakeClient struct {
	listOptions         baize.AgentListOptions
	agentID             string
	templateListOptions baize.CommandTemplateListOptions
	planID              string
	cancelReason        string
	taskID              string
	approvalID          string
	approvalOptions     baize.CommandPlanApprovalListOptions
	approvalDecision    baize.CommandPlanApprovalDecisionOptions
	directOptions       baize.DirectExecTaskOptions
	overviewOptions     baize.OverviewOptions
	observeOptions      baize.AgentObserveOptions
	outputOptions       baize.ExecTaskOutputOptions
	runtimeStartOptions baize.RuntimeDiagnosisStartOptions
	runtimeDiagnosisID  string
	checkErr            error
	listErr             error
	getErr              error
	overviewErr         error
	policyErr           error
	writeErr            error
}

type lifecycleClient struct {
	*fakeClient
	events []string
}

func (f *lifecycleClient) record(event string) {
	f.events = append(f.events, event)
}

func (f *lifecycleClient) ListCommandTemplates(ctx context.Context, options baize.CommandTemplateListOptions) (baize.CommandTemplatePage, error) {
	f.record("templates.list")
	return f.fakeClient.ListCommandTemplates(ctx, options)
}

func (f *lifecycleClient) PreviewCommandTemplate(ctx context.Context, options baize.CommandTemplateRenderOptions) (baize.CommandTemplateRenderResult, error) {
	f.record("template.preview")
	return f.fakeClient.PreviewCommandTemplate(ctx, options)
}

func (f *lifecycleClient) CreateCommandPlan(ctx context.Context, options baize.CommandPlanCreateOptions) (baize.PlanSummary, error) {
	f.record("plan.create")
	return f.fakeClient.CreateCommandPlan(ctx, options)
}

func (f *lifecycleClient) CancelCommandPlan(ctx context.Context, id, reason string) (baize.PlanSummary, error) {
	f.record("plan.cancel")
	return f.fakeClient.CancelCommandPlan(ctx, id, reason)
}

func (f *lifecycleClient) RequestCommandPlanApproval(ctx context.Context, options baize.CommandPlanApprovalCreateOptions) (baize.CommandPlanApproval, error) {
	f.record("approval.create")
	return f.fakeClient.RequestCommandPlanApproval(ctx, options)
}

func (f *lifecycleClient) DecideCommandPlanApproval(ctx context.Context, id string, options baize.CommandPlanApprovalDecisionOptions) (baize.CommandPlanApproval, error) {
	f.record("approval.decide")
	return f.fakeClient.DecideCommandPlanApproval(ctx, id, options)
}

func (f *lifecycleClient) ExecuteCommandPlan(ctx context.Context, id string, options baize.CommandPlanExecuteOptions) (baize.PlanExecutionSummary, error) {
	f.record("plan.execute")
	return f.fakeClient.ExecuteCommandPlan(ctx, id, options)
}

func (f *lifecycleClient) DirectExecTask(ctx context.Context, options baize.DirectExecTaskOptions) (baize.TaskSummary, error) {
	f.record("task.direct")
	return f.fakeClient.DirectExecTask(ctx, options)
}

func (f *lifecycleClient) GetExecTask(ctx context.Context, id string) (baize.TaskSummary, error) {
	f.record("task.get")
	return f.fakeClient.GetExecTask(ctx, id)
}

func (f *lifecycleClient) GetExecTaskOutput(ctx context.Context, options baize.ExecTaskOutputOptions) (baize.ExecTaskOutputSummary, error) {
	f.record("task.output")
	return f.fakeClient.GetExecTaskOutput(ctx, options)
}

func (f *lifecycleClient) CancelExecTask(ctx context.Context, id string) error {
	f.record("task.cancel")
	return f.fakeClient.CancelExecTask(ctx, id)
}

func (f *fakeClient) CheckSession(context.Context) error {
	return f.checkErr
}

func (f *fakeClient) GetOverview(_ context.Context, options baize.OverviewOptions) (baize.OverviewSummary, error) {
	if f.overviewErr != nil {
		return baize.OverviewSummary{}, f.overviewErr
	}
	f.overviewOptions = options
	return baize.OverviewSummary{
		ServerStatus: &baize.ServerStatusSummary{Total: 2, Online: 1, Offline: 1},
		AbnormalServers: []baize.AbnormalServerSummary{{
			AgentID: "11111111-2222-3333-4444-555555555555", Hostname: "web-01", Status: "offline",
			Reasons: []baize.OverviewAbnormalReason{{Metric: "offline"}},
		}},
		ResourceDataAvailable: true, AbnormalDataAvailable: true,
	}, nil
}

func (f *fakeClient) ListAgents(_ context.Context, options baize.AgentListOptions) (baize.AgentPage, error) {
	if f.listErr != nil {
		return baize.AgentPage{}, f.listErr
	}
	f.listOptions = options
	return baize.AgentPage{
		Items: []baize.AgentSummary{{
			ID: "11111111-2222-3333-4444-555555555555", DisplayName: "Web 01", Status: "online",
			OperatingSystem: "linux Ubuntu 24.04", Architecture: "amd64", AgentVersion: "0.2.1",
		}},
		Total: 1, Page: options.Page, PageSize: options.PageSize,
	}, nil
}

func (f *fakeClient) GetAgent(_ context.Context, id string) (baize.AgentSummary, error) {
	if f.getErr != nil {
		return baize.AgentSummary{}, f.getErr
	}
	f.agentID = id
	heartbeat := time.Date(2026, time.August, 6, 1, 2, 3, 0, time.UTC)
	return baize.AgentSummary{
		ID: id, DisplayName: "Web 01", Status: "online", OperatingSystem: "linux Ubuntu 24.04",
		Architecture: "amd64", AgentVersion: "0.2.1", LastHeartbeatAt: &heartbeat,
	}, nil
}

func (f *fakeClient) ObserveAgent(_ context.Context, options baize.AgentObserveOptions) (baize.AgentObserveResult, error) {
	if f.getErr != nil {
		return baize.AgentObserveResult{}, f.getErr
	}
	f.observeOptions = options
	return baize.AgentObserveResult{
		AgentID: options.AgentID, View: options.View, ResultMode: "bounded_summary", SensitiveContentExcluded: true,
		RedactionPolicy: "conservative_patterns_only", Notice: "bounded summary; do not retry the same request",
		Health: &baize.AgentHealthObservation{DisplayName: "Web 01", Status: "online"},
	}, nil
}

func (f *fakeClient) ListCommandTemplates(_ context.Context, options baize.CommandTemplateListOptions) (baize.CommandTemplatePage, error) {
	if f.listErr != nil {
		return baize.CommandTemplatePage{}, f.listErr
	}
	f.templateListOptions = options
	return baize.CommandTemplatePage{Items: []baize.CommandTemplateSummary{{ID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Name: "Restart service", Status: "enabled", RiskLevel: "low"}}, Total: 1, Page: options.Page, PageSize: options.PageSize}, nil
}

func (f *fakeClient) PreviewCommandTemplate(_ context.Context, options baize.CommandTemplateRenderOptions) (baize.CommandTemplateRenderResult, error) {
	if f.writeErr != nil {
		return baize.CommandTemplateRenderResult{}, f.writeErr
	}
	return baize.CommandTemplateRenderResult{TemplateID: options.TemplateID, TemplateName: "Restart service", PrecheckPassed: true, DryRun: true}, nil
}

func (f *fakeClient) CreateCommandPlan(_ context.Context, options baize.CommandPlanCreateOptions) (baize.PlanSummary, error) {
	if f.writeErr != nil {
		return baize.PlanSummary{}, f.writeErr
	}
	f.planID = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
	return baize.PlanSummary{ID: f.planID, TemplateID: options.TemplateID, TargetAgentIDs: options.TargetAgentIDs, Status: "ready"}, nil
}

func (f *fakeClient) GetCommandPlan(_ context.Context, id string) (baize.PlanSummary, error) {
	if f.writeErr != nil {
		return baize.PlanSummary{}, f.writeErr
	}
	return baize.PlanSummary{ID: id, Status: "ready"}, nil
}

func (f *fakeClient) CancelCommandPlan(_ context.Context, id, reason string) (baize.PlanSummary, error) {
	if f.writeErr != nil {
		return baize.PlanSummary{}, f.writeErr
	}
	f.planID = id
	f.cancelReason = reason
	return baize.PlanSummary{ID: id, Status: "cancelled"}, nil
}

func (f *fakeClient) RequestCommandPlanApproval(_ context.Context, options baize.CommandPlanApprovalCreateOptions) (baize.CommandPlanApproval, error) {
	if f.writeErr != nil {
		return baize.CommandPlanApproval{}, f.writeErr
	}
	f.approvalID = "eeeeeeee-ffff-0000-1111-222222222222"
	return baize.CommandPlanApproval{ID: f.approvalID, PlanID: options.PlanID, RiskLevel: "critical", Status: "pending", Reason: options.Reason}, nil
}

func (f *fakeClient) ListCommandPlanApprovals(_ context.Context, options baize.CommandPlanApprovalListOptions) (baize.CommandPlanApprovalPage, error) {
	if f.writeErr != nil {
		return baize.CommandPlanApprovalPage{}, f.writeErr
	}
	f.approvalOptions = options
	return baize.CommandPlanApprovalPage{Items: []baize.CommandPlanApproval{{ID: "eeeeeeee-ffff-0000-1111-222222222222", PlanID: options.PlanID, RiskLevel: "critical", Status: "pending", Reason: "maintenance"}}, Total: 1, Page: options.Page, PageSize: options.PageSize}, nil
}

func (f *fakeClient) GetCommandPlanApproval(_ context.Context, id string) (baize.CommandPlanApproval, error) {
	if f.writeErr != nil {
		return baize.CommandPlanApproval{}, f.writeErr
	}
	f.approvalID = id
	return baize.CommandPlanApproval{ID: id, PlanID: "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", RiskLevel: "critical", Status: "pending", Reason: "maintenance"}, nil
}

func (f *fakeClient) DecideCommandPlanApproval(_ context.Context, id string, options baize.CommandPlanApprovalDecisionOptions) (baize.CommandPlanApproval, error) {
	if f.writeErr != nil {
		return baize.CommandPlanApproval{}, f.writeErr
	}
	f.approvalID = id
	f.approvalDecision = options
	status := "rejected"
	if options.Approved {
		status = "approved"
	}
	return baize.CommandPlanApproval{ID: id, PlanID: "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", RiskLevel: "critical", Status: status, DecisionMessage: options.DecisionMessage}, nil
}

func (f *fakeClient) ListCommandPlanApprovalPolicies(_ context.Context) ([]baize.CommandPlanApprovalPolicySummary, error) {
	if f.policyErr != nil {
		return nil, f.policyErr
	}
	return []baize.CommandPlanApprovalPolicySummary{
		{RiskLevel: "high", Enabled: true, AllowSelfApproval: false},
		{RiskLevel: "critical", Enabled: true, AllowSelfApproval: false},
	}, nil
}

func (f *fakeClient) ExecuteCommandPlan(_ context.Context, id string, _ baize.CommandPlanExecuteOptions) (baize.PlanExecutionSummary, error) {
	if f.writeErr != nil {
		return baize.PlanExecutionSummary{}, f.writeErr
	}
	f.planID = id
	f.taskID = "cccccccc-dddd-eeee-ffff-000000000000"
	return baize.PlanExecutionSummary{Plan: baize.PlanSummary{ID: id, Status: "executed"}, Task: baize.TaskSummary{ID: f.taskID, Status: "pending"}}, nil
}

func (f *fakeClient) DirectExecTask(_ context.Context, options baize.DirectExecTaskOptions) (baize.TaskSummary, error) {
	if f.writeErr != nil {
		return baize.TaskSummary{}, f.writeErr
	}
	f.directOptions = options
	f.taskID = "cccccccc-dddd-eeee-ffff-000000000000"
	return baize.TaskSummary{ID: f.taskID, TaskType: "command", Title: options.Title, Status: "pending"}, nil
}

func (f *fakeClient) GetExecTask(_ context.Context, id string) (baize.TaskSummary, error) {
	if f.writeErr != nil {
		return baize.TaskSummary{}, f.writeErr
	}
	return baize.TaskSummary{ID: id, Status: "pending"}, nil
}

func (f *fakeClient) GetExecTaskOutput(_ context.Context, options baize.ExecTaskOutputOptions) (baize.ExecTaskOutputSummary, error) {
	if f.writeErr != nil {
		return baize.ExecTaskOutputSummary{}, f.writeErr
	}
	f.outputOptions = options
	return baize.ExecTaskOutputSummary{TaskID: options.TaskID, ResultMode: "on_demand_bounded_output", Notice: "bounded output; do not retry the same request"}, nil
}

func (f *fakeClient) CancelExecTask(_ context.Context, id string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.taskID = id
	return nil
}

func (f *fakeClient) StartRuntimeDiagnosis(_ context.Context, options baize.RuntimeDiagnosisStartOptions) (baize.RuntimeDiagnosisSummary, error) {
	if f.writeErr != nil {
		return baize.RuntimeDiagnosisSummary{}, f.writeErr
	}
	f.runtimeStartOptions = options
	f.runtimeDiagnosisID = "dddddddd-eeee-ffff-0000-111111111111"
	return baize.RuntimeDiagnosisSummary{ID: f.runtimeDiagnosisID, AgentID: options.AgentID, TargetType: options.TargetType, TargetValue: options.TargetValue, Status: "running", Pushed: true, Summary: "probe running"}, nil
}

func (f *fakeClient) GetRuntimeDiagnosis(_ context.Context, id string) (baize.RuntimeDiagnosisDetail, error) {
	if f.writeErr != nil {
		return baize.RuntimeDiagnosisDetail{}, f.writeErr
	}
	f.runtimeDiagnosisID = id
	return baize.RuntimeDiagnosisDetail{
		RuntimeDiagnosisSummary: baize.RuntimeDiagnosisSummary{ID: id, AgentID: "11111111-2222-3333-4444-555555555555", Status: "resolved", Summary: "probe resolved", ResultMode: "bounded_summary", Notice: "missing detail does not mean the diagnosis failed"},
		ProcessCount:            1, PortCount: 1, EvidenceCount: 2, DetailAvailable: true,
		RecommendedTemplateIDs: []string{"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"},
	}, nil
}

func TestServerExposesReadAndWriteTools(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{}
	clientSession := connectClient(t, ctx, fake)

	wantNames := map[string]bool{
		"baize_overview_get":                 false,
		"baize_workflow_status":              false,
		"baize_connection_status":            false,
		"baize_agents_list":                  false,
		"baize_agent_get":                    false,
		"baize_agent_observe":                false,
		"baize_runtime_diagnosis_start":      false,
		"baize_runtime_diagnosis_get":        false,
		"baize_command_templates_list":       false,
		"baize_command_template_preview":     false,
		"baize_command_plan_create":          false,
		"baize_command_plan_get":             false,
		"baize_command_plan_cancel":          false,
		"baize_command_plan_approval_create": false,
		"baize_command_plan_approvals_list":  false,
		"baize_command_plan_approval_get":    false,
		"baize_command_plan_approval_decide": false,
		"baize_command_plan_execute":         false,
		"baize_exec_task_direct":             false,
		"baize_exec_task_get":                false,
		"baize_exec_task_output_get":         false,
		"baize_exec_task_cancel":             false,
	}
	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		if _, ok := wantNames[tool.Name]; !ok {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		wantNames[tool.Name] = true
		if tool.Annotations == nil {
			t.Fatalf("tool %q has no annotations", tool.Name)
		}
		readOnly := strings.HasSuffix(tool.Name, "status") || tool.Name == "baize_workflow_status" || tool.Name == "baize_overview_get" || tool.Name == "baize_agents_list" || tool.Name == "baize_agent_get" || tool.Name == "baize_agent_observe" || tool.Name == "baize_runtime_diagnosis_get" || tool.Name == "baize_command_templates_list" || tool.Name == "baize_command_template_preview" || tool.Name == "baize_command_plan_get" || tool.Name == "baize_command_plan_approvals_list" || tool.Name == "baize_command_plan_approval_get" || tool.Name == "baize_exec_task_get" || tool.Name == "baize_exec_task_output_get"
		if readOnly {
			if !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
				t.Fatalf("tool %q does not declare read-only idempotent behavior", tool.Name)
			}
			if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
				t.Fatalf("tool %q is not marked non-destructive", tool.Name)
			}
		} else if tool.Annotations.ReadOnlyHint || tool.Annotations.IdempotentHint {
			t.Fatalf("write tool %q has read-only or idempotent annotations", tool.Name)
		}
		if tool.Name == "baize_agents_list" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{
				"page", "pageSize", "search", "alias", "system", "region", "agentVersion",
				"architecture", "status", "groupId", "sortBy", "sortOrder",
			})
		}
		if tool.Name == "baize_overview_get" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{"groupId", "limit"})
		}
		if tool.Name == "baize_agent_observe" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{"agentId", "view", "metric", "from", "to", "limit"})
		}
		if tool.Name == "baize_runtime_diagnosis_start" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{"agentId", "targetType", "targetValue", "timeHint", "sourceModule", "timeoutSec", "maxResults"})
		}
		if tool.Name == "baize_exec_task_output_get" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{"taskId", "targetId", "limit", "mode", "afterSeq", "beforeSeq", "targetLimit", "targetOffset"})
		}
	}
	for name, found := range wantNames {
		if !found {
			t.Fatalf("tool %q was not advertised", name)
		}
	}

	status := callTool(t, ctx, clientSession, "baize_connection_status", map[string]any{})
	if strings.Contains(status, "private-user") || strings.Contains(status, "viewer") || strings.Contains(status, "apiUrl") {
		t.Fatalf("connection result exposed private connection data: %s", status)
	}
	if !strings.Contains(status, `"connected":true`) || strings.Contains(status, "profile") {
		t.Fatalf("unexpected connection result: %s", status)
	}
	workflow := callTool(t, ctx, clientSession, "baize_workflow_status", map[string]any{})
	if !strings.Contains(workflow, `"workflowMode":"multi"`) || !strings.Contains(workflow, `"approvalPolicyAccess":"available"`) || !strings.Contains(workflow, `"auditControl":"server_managed"`) {
		t.Fatalf("unexpected workflow status: %s", workflow)
	}
	overview := callTool(t, ctx, clientSession, "baize_overview_get", map[string]any{"groupId": " aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee ", "limit": 5})
	if fake.overviewOptions != (baize.OverviewOptions{GroupID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Limit: 5}) {
		t.Fatalf("GetOverview() options = %#v", fake.overviewOptions)
	}
	if !strings.Contains(overview, "abnormalServers") || strings.Contains(overview, "weight") {
		t.Fatalf("unexpected overview result: %s", overview)
	}
	_ = callTool(t, ctx, clientSession, "baize_overview_get", map[string]any{})
	if fake.overviewOptions.Limit != baize.OverviewDefaultLimit {
		t.Fatalf("GetOverview() default limit = %d", fake.overviewOptions.Limit)
	}

	const groupID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	list := callTool(t, ctx, clientSession, "baize_agents_list", map[string]any{
		"search": " web ", "alias": " frontend ", "system": " linux ", "region": " shanghai ",
		"agentVersion": " 0.2 ", "architecture": " arm64 ", "status": " online ", "groupId": " " + groupID + " ",
		"sortBy": " lastHeartbeatAt ", "sortOrder": " desc ",
	})
	wantOptions := baize.AgentListOptions{
		Page: 1, PageSize: 20, Search: "web", Alias: "frontend", System: "linux", Region: "shanghai",
		AgentVersion: "0.2", Architecture: "arm64", Status: "online", GroupID: groupID,
		SortBy: "lastHeartbeatAt", SortOrder: "desc",
	}
	if fake.listOptions != wantOptions {
		t.Fatalf("ListAgents() options = %#v", fake.listOptions)
	}
	assertNoPrivateFields(t, list)

	const agentID = "11111111-2222-3333-4444-555555555555"
	detail := callTool(t, ctx, clientSession, "baize_agent_get", map[string]any{"id": agentID})
	if fake.agentID != agentID {
		t.Fatalf("GetAgent() id = %q", fake.agentID)
	}
	assertNoPrivateFields(t, detail)
	observed := callTool(t, ctx, clientSession, "baize_agent_observe", map[string]any{"agentId": agentID, "view": "health"})
	if fake.observeOptions != (baize.AgentObserveOptions{AgentID: agentID, View: "health"}) || !strings.Contains(observed, "bounded_summary") || !strings.Contains(observed, "do not retry") {
		t.Fatalf("unexpected agent observation: options=%#v result=%s", fake.observeOptions, observed)
	}
	diagnosis := callTool(t, ctx, clientSession, "baize_runtime_diagnosis_start", map[string]any{
		"agentId": agentID, "targetType": "process_name", "targetValue": " nginx ", "timeHint": "incident-1", "sourceModule": "test", "timeoutSec": 3, "maxResults": 5,
	})
	if fake.runtimeStartOptions.TargetValue != "nginx" || fake.runtimeStartOptions.TargetType != "process_name" || !strings.Contains(diagnosis, "probe running") {
		t.Fatalf("unexpected runtime diagnosis start: options=%#v result=%s", fake.runtimeStartOptions, diagnosis)
	}
	diagnosisDetail := callTool(t, ctx, clientSession, "baize_runtime_diagnosis_get", map[string]any{"id": fake.runtimeDiagnosisID})
	if !strings.Contains(diagnosisDetail, "bounded_summary") || !strings.Contains(diagnosisDetail, "processCount") {
		t.Fatalf("unexpected runtime diagnosis detail: %s", diagnosisDetail)
	}
	if !strings.Contains(diagnosisDetail, "missing detail does not mean") {
		t.Fatalf("runtime diagnosis result did not explain bounded output: %s", diagnosisDetail)
	}
	assertStructuredFieldsAbsent(t, diagnosisDetail, "command", "cwd", "executable", "localAddress", "value", "environment")

	templates := callTool(t, ctx, clientSession, "baize_command_templates_list", map[string]any{"pageSize": 10, "riskLevel": "low"})
	if !strings.Contains(templates, "Restart service") {
		t.Fatalf("unexpected templates result: %s", templates)
	}
	plan := callTool(t, ctx, clientSession, "baize_command_plan_create", map[string]any{
		"templateId":     "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee",
		"targetAgentIds": []string{agentID},
		"parameters":     map[string]any{"service": "nginx"},
	})
	assertStructuredFieldsAbsent(t, plan, "renderedPreview", "parameters", "workDir", "operatorId", "operatorName", "command")
	if !strings.Contains(plan, "ready") {
		t.Fatalf("unexpected plan result: %s", plan)
	}
	cancelled := callTool(t, ctx, clientSession, "baize_command_plan_cancel", map[string]any{
		"id": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", "reason": "no longer needed",
	})
	if !strings.Contains(cancelled, "cancelled") || fake.cancelReason != "no longer needed" {
		t.Fatalf("unexpected cancelled plan result: %s", cancelled)
	}
	approval := callTool(t, ctx, clientSession, "baize_command_plan_approval_create", map[string]any{
		"planId": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", "reason": "critical maintenance approval",
	})
	if !strings.Contains(approval, "pending") || strings.Contains(approval, "policySnapshot") {
		t.Fatalf("unexpected approval result: %s", approval)
	}
	approvalList := callTool(t, ctx, clientSession, "baize_command_plan_approvals_list", map[string]any{"status": "pending"})
	if !strings.Contains(approvalList, "pending") {
		t.Fatalf("unexpected approval list result: %s", approvalList)
	}
	approvalDetail := callTool(t, ctx, clientSession, "baize_command_plan_approval_get", map[string]any{"id": "eeeeeeee-ffff-0000-1111-222222222222"})
	if !strings.Contains(approvalDetail, "critical") {
		t.Fatalf("unexpected approval detail result: %s", approvalDetail)
	}
	approved := callTool(t, ctx, clientSession, "baize_command_plan_approval_decide", map[string]any{"id": "eeeeeeee-ffff-0000-1111-222222222222", "approved": true, "decisionMessage": "approved for maintenance"})
	if !strings.Contains(approved, "approved") || !fake.approvalDecision.Approved {
		t.Fatalf("unexpected approval decision result: %s", approved)
	}
	executed := callTool(t, ctx, clientSession, "baize_command_plan_execute", map[string]any{"id": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", "confirmRisk": true})
	if !strings.Contains(executed, "pending") {
		t.Fatalf("unexpected execute result: %s", executed)
	}
	_ = callTool(t, ctx, clientSession, "baize_exec_task_get", map[string]any{"id": "cccccccc-dddd-eeee-ffff-000000000000"})
	output := callTool(t, ctx, clientSession, "baize_exec_task_output_get", map[string]any{"taskId": "cccccccc-dddd-eeee-ffff-000000000000", "limit": 10, "mode": "tail"})
	if fake.outputOptions.TaskID != "cccccccc-dddd-eeee-ffff-000000000000" || fake.outputOptions.Limit != 10 || fake.outputOptions.Mode != "tail" || !strings.Contains(output, "on_demand_bounded_output") {
		t.Fatalf("unexpected task output request/result: options=%#v result=%s", fake.outputOptions, output)
	}
	_ = callTool(t, ctx, clientSession, "baize_exec_task_cancel", map[string]any{"id": "cccccccc-dddd-eeee-ffff-000000000000"})
	_ = callTool(t, ctx, clientSession, "baize_exec_task_direct", map[string]any{
		"command": "systemctl restart nginx", "title": "Restart nginx", "targetAgentIds": []string{"11111111-2222-3333-4444-555555555555"}, "confirmRisk": true,
	})
}

func TestServerWriteLifecycleRequiresPreviewApprovalBeforeExecution(t *testing.T) {
	ctx := context.Background()
	backend := &lifecycleClient{fakeClient: &fakeClient{}}
	clientSession := connectClient(t, ctx, backend)
	const (
		templateID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
		agentID    = "11111111-2222-3333-4444-555555555555"
		planID     = "bbbbbbbb-cccc-dddd-eeee-ffffffffffff"
		approvalID = "eeeeeeee-ffff-0000-1111-222222222222"
		taskID     = "cccccccc-dddd-eeee-ffff-000000000000"
	)

	_ = callTool(t, ctx, clientSession, "baize_command_templates_list", map[string]any{})
	_ = callTool(t, ctx, clientSession, "baize_command_template_preview", map[string]any{
		"templateId": templateID, "agentIds": []string{agentID}, "parameters": map[string]any{"service": "nginx"},
	})
	_ = callTool(t, ctx, clientSession, "baize_command_plan_create", map[string]any{
		"templateId": templateID, "targetAgentIds": []string{agentID}, "parameters": map[string]any{"service": "nginx"},
	})
	_ = callTool(t, ctx, clientSession, "baize_command_plan_approval_create", map[string]any{
		"planId": planID, "reason": "approved maintenance window",
	})
	_ = callTool(t, ctx, clientSession, "baize_command_plan_approval_decide", map[string]any{
		"id": approvalID, "approved": true, "decisionMessage": "approved",
	})
	_ = callTool(t, ctx, clientSession, "baize_command_plan_execute", map[string]any{"id": planID, "confirmRisk": true})
	_ = callTool(t, ctx, clientSession, "baize_exec_task_get", map[string]any{"id": taskID})
	_ = callTool(t, ctx, clientSession, "baize_exec_task_output_get", map[string]any{"taskId": taskID})
	_ = callTool(t, ctx, clientSession, "baize_exec_task_cancel", map[string]any{"id": taskID})

	want := []string{"templates.list", "template.preview", "plan.create", "approval.create", "approval.decide", "plan.execute", "task.get", "task.output", "task.cancel"}
	if !reflect.DeepEqual(backend.events, want) {
		t.Fatalf("write lifecycle events = %#v, want %#v", backend.events, want)
	}
}

func TestServerToolListStaysWithinContextBudget(t *testing.T) {
	ctx := context.Background()
	clientSession := connectClient(t, ctx, &fakeClient{})
	var toolsList []any
	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		toolsList = append(toolsList, tool)
	}
	raw, err := json.Marshal(toolsList)
	if err != nil {
		t.Fatalf("Marshal tools list: %v", err)
	}
	if len(raw) > 64<<10 {
		t.Fatalf("tools/list is %d bytes, exceeds 64 KiB budget", len(raw))
	}
}

func TestServerWorkflowStatusHonorsSingleProfilePreference(t *testing.T) {
	ctx := context.Background()
	clientSession := connectClientWithOptions(t, ctx, &fakeClient{}, Options{WorkflowMode: "single"})
	status := callTool(t, ctx, clientSession, "baize_workflow_status", map[string]any{})
	if !strings.Contains(status, `"workflowMode":"single"`) {
		t.Fatalf("workflow status did not preserve single mode: %s", status)
	}
	if !strings.Contains(status, `"allowSelfApproval":false`) {
		t.Fatalf("workflow status omitted server policy: %s", status)
	}
}

func TestServerWorkflowStatusKeepsLocalModeWhenPolicyIsNotVisible(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{policyErr: &baize.APIError{StatusCode: 403}}
	clientSession := connectClientWithOptions(t, ctx, fake, Options{WorkflowMode: "single"})
	status := callTool(t, ctx, clientSession, "baize_workflow_status", map[string]any{})
	if !strings.Contains(status, `"workflowMode":"single"`) {
		t.Fatalf("workflow status lost local mode: %s", status)
	}
	if !strings.Contains(status, `"approvalPolicyAccess":"not_visible"`) || !strings.Contains(status, `"approvalPolicies":[]`) {
		t.Fatalf("workflow status did not mark hidden policy: %s", status)
	}
}

func TestServerWorkflowStatusStillReportsUnexpectedPolicyErrors(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{policyErr: &baize.APIError{StatusCode: 500}}
	clientSession := connectClientWithOptions(t, ctx, fake, Options{WorkflowMode: "multi"})
	statusErr := callToolError(t, ctx, clientSession, "baize_workflow_status", map[string]any{})
	if !strings.Contains(statusErr, "could not complete") {
		t.Fatalf("unexpected workflow status error: %s", statusErr)
	}
}

func assertToolSchemaProperties(t *testing.T, schema any, names []string) {
	t.Helper()
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("Marshal input schema: %v", err)
	}
	var value struct {
		Properties map[string]json.RawMessage `json:"properties"`
	}
	if err := json.Unmarshal(raw, &value); err != nil {
		t.Fatalf("Unmarshal input schema: %v", err)
	}
	for _, name := range names {
		if _, ok := value.Properties[name]; !ok {
			t.Fatalf("input schema is missing property %q: %s", name, raw)
		}
	}
}

func TestServerSanitizesToolErrors(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{checkErr: errors.New("private endpoint https://private.example/api/v1 for private-user")}
	clientSession := connectClient(t, ctx, fake)

	statusErr := callToolError(t, ctx, clientSession, "baize_connection_status", map[string]any{})
	for _, forbidden := range []string{"private.example", "private-user", "api/v1"} {
		if strings.Contains(statusErr, forbidden) {
			t.Fatalf("connection error contains %q: %s", forbidden, statusErr)
		}
	}
	if !strings.Contains(statusErr, "could not be completed") {
		t.Fatalf("unexpected sanitized connection error: %s", statusErr)
	}

	fake.checkErr = nil
	fake.listErr = &baize.APIError{StatusCode: 403}
	listErr := callToolError(t, ctx, clientSession, "baize_agents_list", map[string]any{})
	if !strings.Contains(listErr, "denied this read request") {
		t.Fatalf("unexpected sanitized API error: %s", listErr)
	}
}

func TestToolOutputEnforcesContextBudget(t *testing.T) {
	_, _, err := toolOutput(strings.Repeat("x", maxToolOutputBytes+1), nil, "read")
	if err == nil || !strings.Contains(err.Error(), "context size") {
		t.Fatalf("toolOutput() error = %v", err)
	}

	_, _, err = toolOutput(struct{}{}, &baize.APIError{StatusCode: 403}, "write")
	if err == nil || !strings.Contains(err.Error(), "denied this write request") {
		t.Fatalf("toolOutput() API error = %v", err)
	}
}

func connectClient(t *testing.T, ctx context.Context, backend Client) *mcp.ClientSession {
	return connectClientWithOptions(t, ctx, backend, Options{WorkflowMode: "multi"})
}

func connectClientWithOptions(t *testing.T, ctx context.Context, backend Client, options Options) *mcp.ClientSession {
	t.Helper()
	server := NewWithOptions(backend, options)
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatalf("server.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatalf("client.Connect() error = %v", err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func callTool(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("CallTool(%q) error = %v", name, err)
	}
	if result.IsError {
		t.Fatalf("CallTool(%q) returned a tool error: %#v", name, result.Content)
	}
	raw, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatalf("Marshal structured content: %v", err)
	}
	return string(raw)
}

func callToolError(t *testing.T, ctx context.Context, session *mcp.ClientSession, name string, args map[string]any) string {
	t.Helper()
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		return err.Error()
	}
	if !result.IsError {
		t.Fatalf("CallTool(%q) did not return a tool error: %#v", name, result.Content)
	}
	raw, err := json.Marshal(result.Content)
	if err != nil {
		t.Fatalf("Marshal error content: %v", err)
	}
	return string(raw)
}

func assertNoPrivateFields(t *testing.T, value string) {
	t.Helper()
	for _, forbidden := range []string{"apiUrl", "ipInternal", "ipExternal", "fingerprint", "capabilities", "token", "private-user", "viewer"} {
		if strings.Contains(value, forbidden) {
			t.Fatalf("tool result contains %q: %s", forbidden, value)
		}
	}
}

func assertStructuredFieldsAbsent(t *testing.T, value string, names ...string) {
	t.Helper()
	var decoded any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		t.Fatalf("unmarshal structured content: %v", err)
	}
	for _, name := range names {
		if containsJSONField(decoded, name) {
			t.Fatalf("structured content contains forbidden field %q: %s", name, value)
		}
	}
}

func containsJSONField(value any, target string) bool {
	switch item := value.(type) {
	case map[string]any:
		if _, ok := item[target]; ok {
			return true
		}
		for _, child := range item {
			if containsJSONField(child, target) {
				return true
			}
		}
	case []any:
		for _, child := range item {
			if containsJSONField(child, target) {
				return true
			}
		}
	}
	return false
}
