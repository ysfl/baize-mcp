package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
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
	taskID              string
	checkErr            error
	listErr             error
	getErr              error
	writeErr            error
}

func (f *fakeClient) CheckSession(context.Context) error {
	return f.checkErr
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

func (f *fakeClient) ExecuteCommandPlan(_ context.Context, id string, _ baize.CommandPlanExecuteOptions) (baize.PlanExecutionSummary, error) {
	if f.writeErr != nil {
		return baize.PlanExecutionSummary{}, f.writeErr
	}
	f.planID = id
	f.taskID = "cccccccc-dddd-eeee-ffff-000000000000"
	return baize.PlanExecutionSummary{Plan: baize.PlanSummary{ID: id, Status: "executed"}, Task: baize.TaskSummary{ID: f.taskID, Status: "pending"}}, nil
}

func (f *fakeClient) GetExecTask(_ context.Context, id string) (baize.TaskSummary, error) {
	if f.writeErr != nil {
		return baize.TaskSummary{}, f.writeErr
	}
	return baize.TaskSummary{ID: id, Status: "pending"}, nil
}

func (f *fakeClient) CancelExecTask(_ context.Context, id string) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	f.taskID = id
	return nil
}

func TestServerExposesReadAndWriteTools(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{}
	clientSession := connectClient(t, ctx, fake)

	wantNames := map[string]bool{
		"baize_connection_status":        false,
		"baize_agents_list":              false,
		"baize_agent_get":                false,
		"baize_command_templates_list":   false,
		"baize_command_template_preview": false,
		"baize_command_plan_create":      false,
		"baize_command_plan_get":         false,
		"baize_command_plan_execute":     false,
		"baize_exec_task_get":            false,
		"baize_exec_task_cancel":         false,
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
		readOnly := strings.HasSuffix(tool.Name, "status") || tool.Name == "baize_agents_list" || tool.Name == "baize_agent_get" || tool.Name == "baize_command_templates_list" || tool.Name == "baize_command_template_preview" || tool.Name == "baize_command_plan_get" || tool.Name == "baize_exec_task_get"
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
	executed := callTool(t, ctx, clientSession, "baize_command_plan_execute", map[string]any{"id": "bbbbbbbb-cccc-dddd-eeee-ffffffffffff", "confirmRisk": true})
	if !strings.Contains(executed, "pending") {
		t.Fatalf("unexpected execute result: %s", executed)
	}
	_ = callTool(t, ctx, clientSession, "baize_exec_task_get", map[string]any{"id": "cccccccc-dddd-eeee-ffff-000000000000"})
	_ = callTool(t, ctx, clientSession, "baize_exec_task_cancel", map[string]any{"id": "cccccccc-dddd-eeee-ffff-000000000000"})
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

func connectClient(t *testing.T, ctx context.Context, backend Client) *mcp.ClientSession {
	t.Helper()
	server := New(backend)
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
