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
	"regexp"
	"strings"
	"time"
)

const maxResponseBytes = 2 << 20

var agentIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

type Client struct {
	baseURL   *url.URL
	http      *http.Client
	token     string
	userAgent string
}

type AgentSummary struct {
	ID              string     `json:"id"`
	DisplayName     string     `json:"displayName"`
	Status          string     `json:"status"`
	OperatingSystem string     `json:"operatingSystem"`
	Architecture    string     `json:"architecture"`
	AgentVersion    string     `json:"agentVersion,omitempty"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt,omitempty"`
}

type AgentPage struct {
	Items    []AgentSummary `json:"items"`
	Total    int            `json:"total"`
	Page     int            `json:"page"`
	PageSize int            `json:"pageSize"`
}

type AgentListOptions struct {
	Page     int
	PageSize int
	Search   string
	Status   string
}

type agentRecord struct {
	ID              string     `json:"id"`
	Hostname        string     `json:"hostname"`
	Alias           string     `json:"alias"`
	Status          string     `json:"status"`
	OSType          string     `json:"osType"`
	OSVersion       string     `json:"osVersion"`
	Arch            string     `json:"arch"`
	AgentVersion    string     `json:"agentVersion"`
	LastHeartbeatAt *time.Time `json:"lastHeartbeatAt"`
}

type APIError struct {
	StatusCode int
}

func (e *APIError) Error() string {
	return fmt.Sprintf("Baize returned HTTP %d", e.StatusCode)
}

type InputError struct {
	message string
}

func (e *InputError) Error() string {
	return e.message
}

func newInputError(message string) error {
	return &InputError{message: message}
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

func (c *Client) Login(ctx context.Context, username, password string) (string, error) {
	payload := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{Username: username, Password: password}
	var data struct {
		Token string `json:"token"`
	}
	if err := c.do(ctx, http.MethodPost, []string{"auth", "login"}, nil, payload, &data, false); err != nil {
		return "", err
	}
	if strings.TrimSpace(data.Token) == "" {
		return "", errors.New("Baize login response did not include a session credential")
	}
	return data.Token, nil
}

func (c *Client) CheckSession(ctx context.Context) error {
	return c.do(ctx, http.MethodGet, []string{"auth", "profile"}, nil, nil, nil, true)
}

func (c *Client) Logout(ctx context.Context) error {
	var data struct {
		Revoked bool `json:"revoked"`
	}
	return c.do(ctx, http.MethodPost, []string{"auth", "logout"}, nil, nil, &data, true)
}

func (c *Client) ListAgents(ctx context.Context, options AgentListOptions) (AgentPage, error) {
	if options.Page < 1 {
		return AgentPage{}, newInputError("page must be at least 1")
	}
	if options.PageSize < 1 || options.PageSize > 100 {
		return AgentPage{}, newInputError("page size must be between 1 and 100")
	}
	search := strings.TrimSpace(options.Search)
	if len(search) > 200 {
		return AgentPage{}, newInputError("search must not exceed 200 characters")
	}
	status := strings.TrimSpace(options.Status)
	if len(status) > 64 {
		return AgentPage{}, newInputError("status must not exceed 64 characters")
	}
	query := url.Values{
		"page":      {fmt.Sprintf("%d", options.Page)},
		"page_size": {fmt.Sprintf("%d", options.PageSize)},
	}
	if search != "" {
		query.Set("search", search)
	}
	if status != "" {
		query.Set("status", status)
	}
	var data struct {
		Items    []agentRecord `json:"items"`
		Total    int           `json:"total"`
		Page     int           `json:"page"`
		PageSize int           `json:"pageSize"`
	}
	if err := c.do(ctx, http.MethodGet, []string{"agents"}, query, nil, &data, true); err != nil {
		return AgentPage{}, err
	}
	items := make([]AgentSummary, 0, len(data.Items))
	for _, item := range data.Items {
		items = append(items, summarizeAgent(item))
	}
	return AgentPage{Items: items, Total: data.Total, Page: data.Page, PageSize: data.PageSize}, nil
}

func (c *Client) GetAgent(ctx context.Context, id string) (AgentSummary, error) {
	id = strings.TrimSpace(id)
	if !agentIDPattern.MatchString(id) {
		return AgentSummary{}, newInputError("agent ID must be a UUID")
	}
	var data agentRecord
	if err := c.do(ctx, http.MethodGet, []string{"agents", strings.ToLower(id)}, nil, nil, &data, true); err != nil {
		return AgentSummary{}, err
	}
	return summarizeAgent(data), nil
}

func (c *Client) do(ctx context.Context, method string, segments []string, query url.Values, payload any, output any, authenticated bool) error {
	endpoint := c.baseURL.JoinPath(segments...)
	if len(query) > 0 {
		endpoint.RawQuery = query.Encode()
	}
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
		Data json.RawMessage `json:"data"`
	}
	if len(raw) > 0 && json.Unmarshal(raw, &envelope) != nil {
		return errors.New("Baize response was not valid JSON")
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return &APIError{StatusCode: resp.StatusCode}
	}
	if output == nil || len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return nil
	}
	if err := json.Unmarshal(envelope.Data, output); err != nil {
		return errors.New("Baize response did not match the expected format")
	}
	return nil
}

func summarizeAgent(item agentRecord) AgentSummary {
	displayName := strings.TrimSpace(item.Alias)
	if displayName == "" {
		displayName = strings.TrimSpace(item.Hostname)
	}
	if displayName == "" {
		displayName = item.ID
	}
	operatingSystem := strings.TrimSpace(strings.Join([]string{
		strings.TrimSpace(item.OSType),
		strings.TrimSpace(item.OSVersion),
	}, " "))
	return AgentSummary{
		ID:              item.ID,
		DisplayName:     displayName,
		Status:          item.Status,
		OperatingSystem: operatingSystem,
		Architecture:    item.Arch,
		AgentVersion:    item.AgentVersion,
		LastHeartbeatAt: item.LastHeartbeatAt,
	}
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
