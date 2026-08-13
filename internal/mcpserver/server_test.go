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
	listOptions baize.AgentListOptions
	agentID     string
	checkErr    error
	listErr     error
	getErr      error
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

func TestServerExposesOnlyReadOnlyTools(t *testing.T) {
	ctx := context.Background()
	fake := &fakeClient{}
	clientSession := connectClient(t, ctx, fake)

	wantNames := map[string]bool{
		"baize_connection_status": false,
		"baize_agents_list":       false,
		"baize_agent_get":         false,
	}
	for tool, err := range clientSession.Tools(ctx, nil) {
		if err != nil {
			t.Fatalf("Tools() error = %v", err)
		}
		if _, ok := wantNames[tool.Name]; !ok {
			t.Fatalf("unexpected tool %q", tool.Name)
		}
		wantNames[tool.Name] = true
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q does not declare read-only idempotent behavior", tool.Name)
		}
		if tool.Annotations.DestructiveHint == nil || *tool.Annotations.DestructiveHint {
			t.Fatalf("tool %q is not marked non-destructive", tool.Name)
		}
		if tool.Name == "baize_agents_list" {
			assertToolSchemaProperties(t, tool.InputSchema, []string{
				"page", "pageSize", "search", "alias", "system", "region", "agentVersion",
				"architecture", "status", "groupId", "tagKey", "tagValue", "sortBy", "sortOrder",
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
		"tagKey": " role ", "tagValue": " api ", "sortBy": " lastHeartbeatAt ", "sortOrder": " desc ",
	})
	wantOptions := baize.AgentListOptions{
		Page: 1, PageSize: 20, Search: "web", Alias: "frontend", System: "linux", Region: "shanghai",
		AgentVersion: "0.2", Architecture: "arm64", Status: "online", GroupID: groupID,
		TagKey: "role", TagValue: "api", SortBy: "lastHeartbeatAt", SortOrder: "desc",
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
