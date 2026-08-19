package baize

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeDiagnosisUsesFixedEndpointsAndBoundedFields(t *testing.T) {
	const (
		agentID     = "11111111-2222-3333-4444-555555555555"
		diagnosisID = "dddddddd-eeee-ffff-0000-111111111111"
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("missing authorization header")
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.Method + " " + r.URL.Path {
		case http.MethodPost + " /api/v1/runtime-diagnoses/query":
			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode start body: %v", err)
			}
			if body["agentId"] != agentID || body["targetType"] != "process_name" || body["targetValue"] != "nginx" {
				t.Fatalf("unexpected start body: %#v", body)
			}
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + diagnosisID + `","agentId":"` + agentID + `","targetType":"process_name","targetValue":"nginx","status":"running","summary":"probe running","pushed":true,"createdAt":"2026-08-20T01:02:03Z"}}`))
		case http.MethodGet + " /api/v1/runtime-diagnoses/" + diagnosisID:
			_, _ = w.Write([]byte(`{"code":0,"data":{"id":"` + diagnosisID + `","agentId":"` + agentID + `","targetType":"pid","targetValue":"1234","status":"resolved","summary":"PID 1234 resolved","processes":[{"pid":1234,"command":"curl -H Authorization: Bearer secret","cwd":"/srv/private","executable":"/usr/bin/curl"}],"ports":[{"localAddress":"0.0.0.0","localPort":443}],"riskFindings":[{"code":"public_listener","message":"sensitive finding"}],"evidences":[{"key":"command","value":"secret-evidence"}],"recommendedTemplateIds":["aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"],"truncated":false}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	started, err := client.StartRuntimeDiagnosis(context.Background(), RuntimeDiagnosisStartOptions{
		AgentID: agentID, TargetType: " process_name ", TargetValue: " nginx ", TimeHint: "incident", SourceModule: "test", TimeoutSec: 3, MaxResults: 5,
	})
	if err != nil {
		t.Fatalf("StartRuntimeDiagnosis() error = %v", err)
	}
	if started.ID != diagnosisID || started.Status != "running" || !started.Pushed {
		t.Fatalf("unexpected start result: %#v", started)
	}

	detail, err := client.GetRuntimeDiagnosis(context.Background(), diagnosisID)
	if err != nil {
		t.Fatalf("GetRuntimeDiagnosis() error = %v", err)
	}
	if detail.ProcessCount != 1 || detail.PortCount != 1 || detail.RiskFindingCount != 1 || detail.EvidenceCount != 1 || !detail.DetailAvailable {
		t.Fatalf("unexpected detail counts: %#v", detail)
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		t.Fatalf("marshal detail: %v", err)
	}
	for _, forbidden := range []string{"curl -H", "Authorization", "/srv/private", "/usr/bin/curl", "localAddress", "secret-evidence", "sensitive finding"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("bounded diagnosis contains %q: %s", forbidden, raw)
		}
	}
}

func TestRuntimeDiagnosisRejectsUnboundedOrUnsupportedInput(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	cases := []RuntimeDiagnosisStartOptions{
		{AgentID: "not-a-uuid", TargetType: "pid", TargetValue: "1"},
		{AgentID: "11111111-2222-3333-4444-555555555555", TargetType: "shell", TargetValue: "ps aux"},
		{AgentID: "11111111-2222-3333-4444-555555555555", TargetType: "pid", TargetValue: ""},
		{AgentID: "11111111-2222-3333-4444-555555555555", TargetType: "pid", TargetValue: "1", TimeoutSec: 11},
		{AgentID: "11111111-2222-3333-4444-555555555555", TargetType: "pid", TargetValue: "1", MaxResults: 51},
	}
	for index, options := range cases {
		if _, err := client.StartRuntimeDiagnosis(context.Background(), options); err == nil {
			t.Errorf("case %d accepted invalid input", index)
		}
	}
	if _, err := client.GetRuntimeDiagnosis(context.Background(), "../auth/profile"); err == nil {
		t.Fatal("GetRuntimeDiagnosis accepted a path traversal value")
	}
}
