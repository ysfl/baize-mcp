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
		{name: "embedded credentials", raw: "https://user:pass@baize.example.com/api/v1", wantErr: true},
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
	session, err := loginClient.Login(context.Background(), "operator", "secret")
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if session.Token != "session-token" {
		t.Fatalf("Login() token = %q", session.Token)
	}

	client, err := NewClient(server.URL+"/api/v1", session.Token, true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	user, err := client.CurrentUser(context.Background())
	if err != nil {
		t.Fatalf("CurrentUser() error = %v", err)
	}
	if user.Username != "operator" || user.Role != "viewer" {
		t.Fatalf("CurrentUser() = %#v", user)
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
	if _, err := client.CurrentUser(context.Background()); err == nil {
		t.Fatal("CurrentUser() followed a cross-origin redirect")
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
	_, err = client.CurrentUser(context.Background())
	if err == nil {
		t.Fatal("CurrentUser() error = nil")
	}
	if strings.Contains(err.Error(), "do-not-return") {
		t.Fatalf("error exposed response body: %v", err)
	}
}

func TestClientRejectsUnsafeTraceID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("{\"traceId\":\"trace-1\\nsecret\"}"))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.CurrentUser(context.Background())
	var apiErr *APIError
	if !errors.As(err, &apiErr) {
		t.Fatalf("CurrentUser() error = %v, want APIError", err)
	}
	if apiErr.TraceID != "" {
		t.Fatalf("unsafe trace ID was retained: %q", apiErr.TraceID)
	}
}
