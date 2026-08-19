package baize

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestChangeAlertUsesFixedActionPathAndBoundedResult(t *testing.T) {
	const incidentID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	seen := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer session-token" {
			t.Fatalf("missing authorization header")
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		seen = append(seen, r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":null}`))
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	for _, action := range []string{" acknowledge ", "resolve"} {
		result, err := client.ChangeAlert(context.Background(), AlertChangeOptions{IncidentID: incidentID, Action: action})
		if err != nil {
			t.Fatalf("ChangeAlert(%q) error = %v", action, err)
		}
		if result.IncidentID != incidentID || !result.Accepted || !result.StatusQueryNeeded || !strings.Contains(result.Notice, "query") {
			t.Fatalf("unexpected result: %#v", result)
		}
	}
	want := []string{"/api/v1/alerts/incidents/" + incidentID + "/acknowledge", "/api/v1/alerts/incidents/" + incidentID + "/resolve"}
	if strings.Join(seen, "\n") != strings.Join(want, "\n") {
		t.Fatalf("paths = %#v, want %#v", seen, want)
	}
}

func TestChangeAlertRejectsUnsupportedInput(t *testing.T) {
	client, err := NewClient("https://baize.example.com/api/v1", "session-token", false, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	checks := []AlertChangeOptions{
		{IncidentID: "not-a-uuid", Action: "resolve"},
		{IncidentID: "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", Action: "delete"},
	}
	for _, options := range checks {
		if _, err := client.ChangeAlert(context.Background(), options); err == nil {
			t.Fatalf("ChangeAlert(%#v) accepted unsupported input", options)
		}
	}
}

func TestChangeAlertPreservesPermissionError(t *testing.T) {
	const incidentID = "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/alerts/incidents/"+incidentID+"/resolve" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client, err := NewClient(server.URL+"/api/v1", "session-token", true, "test")
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	_, err = client.ChangeAlert(context.Background(), AlertChangeOptions{IncidentID: incidentID, Action: "resolve"})
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusForbidden {
		t.Fatalf("error = %v, want HTTP 403 APIError", err)
	}
}
