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
			if got := r.URL.Query().Get("status"); got != "online" {
				t.Fatalf("status = %q", got)
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
		Page: 2, PageSize: 5, Search: " web ", Status: " online ",
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
	if _, err := client.GetAgent(context.Background(), "../auth/profile"); err == nil {
		t.Fatal("GetAgent() accepted an invalid ID")
	}
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
