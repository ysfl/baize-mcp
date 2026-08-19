package baize

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClientGetOverviewCombinesDashboardAndReducesFields(t *testing.T) {
	const groupID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodGet + " /api/v1/dashboard/summary":
			if got := r.URL.Query().Get("group_id"); got != groupID {
				t.Fatalf("group_id = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"serverStatus":{"total":3,"online":2,"offline":1,"onlineRate":66.6},"resourceDistribution":{"cpu":{"low":1,"medium":1,"high":1},"memory":{"low":2,"medium":1,"high":0},"disk":{"low":3,"medium":0,"high":0}},"nginxOverview":{"totalQps":12.5,"global4xxRate":0.1,"global5xxRate":0.2,"globalP99Ms":20},"privateField":"do-not-expose"}}`))
		case http.MethodGet + " /api/v1/dashboard/abnormal-servers":
			if got := r.URL.Query().Get("group_id"); got != groupID {
				t.Fatalf("abnormal group_id = %q", got)
			}
			if got := r.URL.Query().Get("limit"); got != "1" {
				t.Fatalf("limit = %q", got)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"agentId":"11111111-2222-3333-4444-555555555555","hostname":"web-01","status":"offline","weight":100,"reasons":[{"metric":"offline","value":0,"threshold":0},{"metric":"private_metric","value":99,"threshold":1}]},{"agentId":"22222222-3333-4444-5555-666666666666","hostname":"web-02","status":"abnormal","reasons":[{"metric":"cpu","value":91,"threshold":80}]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	overview, err := client.GetOverview(context.Background(), OverviewOptions{GroupID: " " + strings.ToUpper(groupID) + " ", Limit: 1})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if overview.ServerStatus == nil || overview.ServerStatus.Total != 3 || len(overview.AbnormalServers) != 1 || !overview.AbnormalServersTruncated {
		t.Fatalf("GetOverview() = %#v", overview)
	}
	if !overview.ResourceDataAvailable || !overview.NginxDataAvailable || !overview.AbnormalDataAvailable || overview.AbnormalDataMayBeStale {
		t.Fatalf("unexpected data availability flags: %#v", overview)
	}
	if overview.Partial || len(overview.MissingSections) != 0 {
		t.Fatalf("unexpected partial result: %#v", overview)
	}
	if len(overview.AbnormalServers[0].Reasons) != 1 || overview.AbnormalServers[0].Reasons[0].Metric != "offline" {
		t.Fatalf("unexpected reasons: %#v", overview.AbnormalServers[0].Reasons)
	}
	raw, err := json.Marshal(overview)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"weight", "private_metric", "privateField"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("overview exposed %q: %s", forbidden, raw)
		}
	}
}

func TestClientGetOverviewMarksDegradedSectionsPartial(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/dashboard/summary":
			w.WriteHeader(http.StatusInternalServerError)
		case "/api/v1/dashboard/abnormal-servers":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[{"agentId":"11111111-2222-3333-4444-555555555555","hostname":"web-01","status":"offline","reasons":[]}]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	overview, err := client.GetOverview(context.Background(), OverviewOptions{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if !overview.Partial || !overview.AbnormalDataAvailable || overview.ServerStatus != nil {
		t.Fatalf("unexpected partial overview: %#v", overview)
	}
	if len(overview.MissingSections) != 1 || overview.MissingSections[0] != "summary" {
		t.Fatalf("missing sections = %#v", overview.MissingSections)
	}
}

func TestClientGetOverviewMarksEmptyAbnormalCacheAsUncertain(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/dashboard/summary":
			_, _ = w.Write([]byte(`{"code":0,"data":{"serverStatus":{"total":1,"online":1}}}`))
		case "/api/v1/dashboard/abnormal-servers":
			_, _ = w.Write([]byte(`{"code":0,"data":{"items":[]}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	overview, err := client.GetOverview(context.Background(), OverviewOptions{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if !overview.AbnormalDataAvailable || !overview.AbnormalDataMayBeStale || len(overview.AbnormalServers) != 0 {
		t.Fatalf("empty abnormal result was not marked uncertain: %#v", overview)
	}
}

func TestClientGetOverviewKeepsSummaryWhenAbnormalSectionDegrades(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v1/dashboard/summary" {
			_, _ = w.Write([]byte(`{"code":0,"data":{"serverStatus":{"total":2,"online":2},"resourceDistribution":{"cpu":{"low":2,"medium":0,"high":0}}}}`))
			return
		}
		if r.URL.Path == "/api/v1/dashboard/abnormal-servers" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	overview, err := client.GetOverview(context.Background(), OverviewOptions{})
	if err != nil {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if !overview.Partial || overview.ServerStatus == nil || overview.AbnormalDataAvailable || len(overview.MissingSections) != 1 || overview.MissingSections[0] != "abnormalServers" {
		t.Fatalf("unexpected degraded abnormal result: %#v", overview)
	}
}

func TestClientGetOverviewDoesNotHideAuthorizationErrors(t *testing.T) {
	abnormalCalled := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/dashboard/abnormal-servers" {
			abnormalCalled = true
			t.Fatalf("abnormal endpoint called after authorization failure")
		}
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"code":40300,"details":{"secret":"do-not-expose"}}`))
	}))
	defer server.Close()
	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.GetOverview(context.Background(), OverviewOptions{})
	if err == nil {
		t.Fatal("GetOverview() error = nil")
	}
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("GetOverview() error = %v", err)
	}
	if abnormalCalled {
		t.Fatal("abnormal endpoint was called")
	}
}

func TestClientGetOverviewRejectsInputBounds(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, options := range []OverviewOptions{
		{GroupID: "not-a-uuid"},
		{Limit: -1},
		{Limit: OverviewMaxLimit + 1},
	} {
		if _, err := client.GetOverview(context.Background(), options); err == nil {
			t.Fatalf("GetOverview() accepted %#v", options)
		}
	}
}
