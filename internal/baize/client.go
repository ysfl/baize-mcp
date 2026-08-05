package baize

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

type Client struct {
	baseURL   *url.URL
	http      *http.Client
	token     string
	userAgent string
}

type Session struct {
	Token    string
	Username string
	Role     string
}

type CurrentUser struct {
	Username string   `json:"username"`
	Role     string   `json:"role"`
	Roles    []string `json:"roles"`
}

type APIError struct {
	StatusCode int
	TraceID    string
}

func (e *APIError) Error() string {
	if e.TraceID != "" {
		return fmt.Sprintf("Baize returned HTTP %d (traceId=%s)", e.StatusCode, e.TraceID)
	}
	return fmt.Sprintf("Baize returned HTTP %d", e.StatusCode)
}

func NewClient(apiURL, token string, allowHTTP bool, userAgent string) (*Client, error) {
	normalized, err := ValidateAPIURL(apiURL, allowHTTP)
	if err != nil {
		return nil, err
	}
	parsed, err := url.Parse(normalized)
	if err != nil {
		return nil, errors.New("invalid Baize API URL")
	}
	if strings.TrimSpace(userAgent) == "" {
		userAgent = "baize-mcp/dev"
	}
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("too many redirects")
			}
			if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
				return errors.New("cross-origin redirect blocked")
			}
			return nil
		},
	}
	return &Client{baseURL: parsed, http: client, token: token, userAgent: userAgent}, nil
}

func ValidateAPIURL(raw string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("API URL must be an absolute http or https URL")
	}
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", errors.New("API URL must use http or https")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("API URL must not contain credentials, query parameters, or fragments")
	}
	if strings.TrimRight(parsed.EscapedPath(), "/") != "/api/v1" {
		return "", errors.New("API URL must end with /api/v1")
	}
	if parsed.Scheme == "http" && !allowHTTP && !isLoopbackHost(parsed.Hostname()) {
		return "", errors.New("non-loopback HTTP requires explicit --allow-http confirmation")
	}
	parsed.Path = "/api/v1"
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (c *Client) Login(ctx context.Context, username, password string) (Session, error) {
	payload := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password}
	var data struct {
		Token    string `json:"token"`
		Username string `json:"username"`
		Role     string `json:"role"`
	}
	if err := c.do(ctx, http.MethodPost, []string{"auth", "login"}, payload, &data, false); err != nil {
		return Session{}, err
	}
	if strings.TrimSpace(data.Token) == "" {
		return Session{}, errors.New("Baize login response did not include a session credential")
	}
	return Session{Token: data.Token, Username: data.Username, Role: data.Role}, nil
}

func (c *Client) CurrentUser(ctx context.Context) (CurrentUser, error) {
	var data CurrentUser
	if err := c.do(ctx, http.MethodGet, []string{"auth", "profile"}, nil, &data, true); err != nil {
		return CurrentUser{}, err
	}
	return data, nil
}

func (c *Client) Logout(ctx context.Context) error {
	var data struct {
		Revoked bool `json:"revoked"`
	}
	return c.do(ctx, http.MethodPost, []string{"auth", "logout"}, nil, &data, true)
}

func (c *Client) do(ctx context.Context, method string, segments []string, payload any, output any, authenticated bool) error {
	endpoint := c.baseURL.JoinPath(segments...)
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			return errors.New("encode Baize request")
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return errors.New("create Baize request")
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if authenticated {
		if c.token == "" {
			return errors.New("Baize session credential is unavailable")
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return errors.New("Baize request timed out or was cancelled")
		}
		return errors.New("Baize request failed before a response was received")
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return errors.New("read Baize response")
	}
	if len(raw) > maxResponseBytes {
		return errors.New("Baize response exceeded the allowed size")
	}
	var envelope struct {
		Data    json.RawMessage `json:"data"`
		TraceID string          `json:"traceId"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) != nil {
		return errors.New("Baize response was not valid JSON")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{StatusCode: resp.StatusCode, TraceID: safeTraceID(envelope.TraceID)}
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("Baize response did not match the expected format")
	}
	return nil
}

func sameOrigin(a, b *url.URL) bool {
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Host, b.Host)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func safeTraceID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || strings.ContainsRune("._:-", r) {
			continue
		}
		return ""
	}
	return value
}
