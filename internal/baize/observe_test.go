package baize

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

const observeTestAgentID = "11111111-2222-3333-4444-555555555555"

func TestObserveAgentUsesPublishedViewEndpointsAndBoundsResults(t *testing.T) {
	var seenPath string
	var seenQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		data := any(map[string]any{})
		switch r.URL.Path {
		case "/api/v1/agents/" + observeTestAgentID:
			data = map[string]any{"id": observeTestAgentID, "hostname": "node-01", "alias": "Node 01", "status": "online", "osType": "linux", "osVersion": "6.8", "arch": "amd64", "agentVersion": "1.2.3"}
		case "/api/v1/agents/" + observeTestAgentID + "/metrics/latest":
			families := map[string]any{}
			for i := 0; i < maxObserveMetricFamilies+2; i++ {
				families["family"+string(rune('a'+i))] = []any{map[string]any{"metricName": "cpu", "minVal": 1, "maxVal": 2, "avgVal": 1.5}}
			}
			data = map[string]any{"agentId": observeTestAgentID, "families": families, "isStale": true}
		case "/api/v1/agents/" + observeTestAgentID + "/processes/top":
			data = map[string]any{"agentId": observeTestAgentID, "metric": "cpu", "from": time.Now().Add(-time.Minute), "to": time.Now(), "items": []any{map[string]any{"pid": 42, "userName": "root", "command": "curl password=super-secret", "avgValue": 80, "peakValue": 95}}}
		case "/api/v1/agents/" + observeTestAgentID + "/storage/filesystems":
			items := make([]any, 0, maxObserveStorageItems+1)
			for i := 0; i < maxObserveStorageItems+1; i++ {
				items = append(items, map[string]any{"device": "/dev/vda1", "mount": "/", "filesystemType": "ext4", "riskLevel": "normal"})
			}
			data = map[string]any{"agentId": observeTestAgentID, "items": items}
		case "/api/v1/agents/" + observeTestAgentID + "/docker/summary":
			data = map[string]any{"dockerStatus": "running", "containerTotal": 3, "runningCount": 2, "exitedCount": 1}
		case "/api/v1/agents/" + observeTestAgentID + "/analysis/nginx/overview":
			data = map[string]any{"agentId": observeTestAgentID, "traffic": map[string]any{"qps": 12.5}, "latency": map[string]any{"p50Ms": 4}, "upstream": map[string]any{"total": 2, "healthy": 2}, "topSlow": []any{map[string]any{"url": "/secret"}}}
		case "/api/v1/agents/" + observeTestAgentID + "/host-profile":
			data = map[string]any{"snapshot": map[string]any{"id": "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "status": "success", "refreshScope": "system", "source": "agent", "truncated": true, "errors": []any{map[string]any{"range": "users"}}}, "users": []any{map[string]any{"username": "admin"}}, "shellHistory": []any{map[string]any{"commandPlaintext": "secret"}}}
		case "/api/v1/agents/" + observeTestAgentID + "/control-plane":
			data = map[string]any{"agentId": observeTestAgentID, "controlOnline": true, "controlHealth": "healthy", "telemetryStatus": "ready", "spoolDepth": 2}
		default:
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": data})
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	cases := []struct {
		view  string
		check func(t *testing.T, result AgentObserveResult)
	}{
		{ObserveViewHealth, func(t *testing.T, result AgentObserveResult) {
			if result.Health == nil || result.Health.DisplayName != "Node 01" {
				t.Fatalf("health result = %#v", result.Health)
			}
		}},
		{ObserveViewMetrics, func(t *testing.T, result AgentObserveResult) {
			if result.Metrics == nil || len(result.Metrics.Families) != maxObserveMetricFamilies || !result.Truncated {
				t.Fatalf("metrics result = %#v", result)
			}
		}},
		{ObserveViewProcesses, func(t *testing.T, result AgentObserveResult) {
			if result.Processes == nil || !result.RedactionApplied || !strings.Contains(result.Processes.Items[0].Command, "******") || strings.Contains(result.Processes.Items[0].Command, "super-secret") {
				t.Fatalf("process result = %#v", result.Processes)
			}
			if seenQuery.Get("metric") != "cpu" || seenQuery.Get("limit") != "10" || seenQuery.Get("from") == "" || seenQuery.Get("to") == "" {
				t.Fatalf("process query = %v", seenQuery)
			}
		}},
		{ObserveViewStorage, func(t *testing.T, result AgentObserveResult) {
			if result.Storage == nil || len(result.Storage.Items) != maxObserveStorageItems || !result.Truncated {
				t.Fatalf("storage result = %#v", result.Storage)
			}
		}},
		{ObserveViewDocker, func(t *testing.T, result AgentObserveResult) {
			if result.Docker == nil || result.Docker.ContainerTotal != 3 {
				t.Fatalf("docker result = %#v", result.Docker)
			}
		}},
		{ObserveViewNginx, func(t *testing.T, result AgentObserveResult) {
			if result.Nginx == nil || !result.Nginx.TopSlowExcluded || result.Nginx.Traffic == nil || result.Nginx.Traffic.QPS != 12.5 {
				t.Fatalf("nginx result = %#v", result.Nginx)
			}
		}},
		{ObserveViewHostProfile, func(t *testing.T, result AgentObserveResult) {
			if result.HostProfile == nil || !result.HostProfile.ContentExcluded || result.HostProfile.ErrorCount != 1 || result.HostProfile.SnapshotID == "" {
				t.Fatalf("host profile result = %#v", result.HostProfile)
			}
		}},
		{ObserveViewControlPlane, func(t *testing.T, result AgentObserveResult) {
			if result.ControlPlane == nil || !result.ControlPlane.ControlOnline || result.ControlPlane.SpoolDepth != 2 {
				t.Fatalf("control plane result = %#v", result.ControlPlane)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.view, func(t *testing.T) {
			result, err := client.ObserveAgent(t.Context(), AgentObserveOptions{AgentID: observeTestAgentID, View: tc.view})
			if err != nil {
				t.Fatalf("ObserveAgent() error = %v", err)
			}
			if result.AgentID != observeTestAgentID || result.View != tc.view || result.ResultMode != "bounded_summary" || !result.SensitiveContentExcluded || !result.UnknownSensitiveContentMayRemain {
				t.Fatalf("common result fields = %#v", result)
			}
			if !strings.Contains(result.Notice, "当前返回为有界摘要") || !strings.Contains(result.Notice, "未返回内容不代表任务失败") || !strings.Contains(result.Notice, "不要因缺少输出重复提交") {
				t.Fatalf("notice = %q", result.Notice)
			}
			if tc.view == ObserveViewProcesses && (!strings.Contains(result.Notice, "发生了保守替换") || !strings.Contains(result.Notice, "未知敏感内容仍可能存在")) {
				t.Fatalf("redaction notice = %q", result.Notice)
			}
			tc.check(t, result)
		})
	}
	if seenPath == "" {
		t.Fatal("server did not receive an observation request")
	}
}

func TestObserveAgentRejectsInvalidWindowsAndViews(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	if _, err := client.ObserveAgent(t.Context(), AgentObserveOptions{AgentID: observeTestAgentID, View: "unknown"}); err == nil {
		t.Fatal("ObserveAgent() accepted unknown view")
	}
	from := time.Now()
	to := from.Add(-time.Minute)
	if _, err := client.ObserveAgent(t.Context(), AgentObserveOptions{AgentID: observeTestAgentID, View: ObserveViewProcesses, From: &from, To: &to}); err == nil {
		t.Fatal("ObserveAgent() accepted reversed window")
	}
}
