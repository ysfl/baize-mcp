package app

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ysfl/baize-mcp/internal/credential"
	"github.com/ysfl/baize-mcp/internal/profile"
)

type memoryCredentialStore struct {
	values map[string]string
}

func (s *memoryCredentialStore) Get(name string) (string, error) {
	value, ok := s.values[name]
	if !ok {
		return "", credential.ErrNotFound
	}
	return value, nil
}

func (s *memoryCredentialStore) Set(name, value string) error {
	s.values[name] = value
	return nil
}

func (s *memoryCredentialStore) Delete(name string) error {
	delete(s.values, name)
	return nil
}

func TestRunStatusReturnsAuthenticationStateOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer session-token" {
			t.Fatalf("Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"username":"private-user","role":"private-role"}}`))
	}))
	defer server.Close()

	profiles := profile.NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err := profiles.Put("default", profile.Profile{APIURL: server.URL + "/api/v1", AllowHTTP: true}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	credentials := &memoryCredentialStore{values: map[string]string{"default": "session-token"}}
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	if err := runStatus(context.Background(), nil, &stdout, &stderr, profiles, credentials); err != nil {
		t.Fatalf("runStatus() error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "{\"authenticated\":true}\n" {
		t.Fatalf("runStatus() output = %q", got)
	}
	for _, forbidden := range []string{"private-user", "private-role", "default", "api/v1", "session-token"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("runStatus() output contains %q: %s", forbidden, stdout.String())
		}
	}
}

func TestRunConfigChangesWorkflowModeWithoutExposingProfileSecrets(t *testing.T) {
	profiles := profile.NewStore(filepath.Join(t.TempDir(), "profiles.json"))
	if err := profiles.Put("default", profile.Profile{APIURL: "https://private.example.com/api/v1", WorkflowMode: profile.WorkflowModeMulti}); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if err := runConfig([]string{"set", "--profile", "default", "--workflow-mode", "single"}, &stdout, &stderr, profiles); err != nil {
		t.Fatalf("runConfig(set) error = %v, stderr = %s", err, stderr.String())
	}
	if got := stdout.String(); got != "{\"workflowMode\":\"single\"}\n" {
		t.Fatalf("runConfig(set) output = %q", got)
	}
	stdout.Reset()
	if err := runConfig([]string{"get", "--profile", "default"}, &stdout, &stderr, profiles); err != nil {
		t.Fatalf("runConfig(get) error = %v", err)
	}
	if got := stdout.String(); got != "{\"workflowMode\":\"single\"}\n" {
		t.Fatalf("runConfig(get) output = %q", got)
	}
	for _, forbidden := range []string{"private.example.com", "api/v1", "default"} {
		if strings.Contains(stdout.String(), forbidden) {
			t.Fatalf("runConfig() output contains %q: %s", forbidden, stdout.String())
		}
	}
}
