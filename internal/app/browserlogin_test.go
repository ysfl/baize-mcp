package app

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ysfl/baize-mcp/internal/baize"
)

// silenceBrowserOpen 用空实现替换浏览器拉起，避免测试真的打开浏览器窗口。
func silenceBrowserOpen(t *testing.T) *[]string {
	t.Helper()
	opened := &[]string{}
	previous := openBrowserPage
	openBrowserPage = func(target string) { *opened = append(*opened, target) }
	t.Cleanup(func() { openBrowserPage = previous })
	return opened
}

// stubTokenExchange 起 httptest 服务模拟 /auth/cli/token 兑换端点，并记录请求体供断言。
func stubTokenExchange(t *testing.T, token string) (*httptest.Server, *[]map[string]string) {
	t.Helper()
	requests := &[]map[string]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/auth/cli/token" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		var payload struct {
			Code         string `json:"code"`
			CodeVerifier string `json:"codeVerifier"`
			RedirectURI  string `json:"redirectUri"`
		}
		_ = json.NewDecoder(r.Body).Decode(&payload)
		*requests = append(*requests, map[string]string{
			"code": payload.Code, "codeVerifier": payload.CodeVerifier, "redirectUri": payload.RedirectURI,
		})
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"data":{"token":"` + token + `"}}`))
	}))
	t.Cleanup(server.Close)
	return server, requests
}

// syncBuffer 是并发安全的 stderr 缓冲：登录主流程在后台 goroutine 写入，
// 测试主 goroutine 轮询读取，直接用 strings.Builder 会触发数据竞争。
type syncBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitForAuthorizeURL 从 stderr 轮询授权页地址，并解析出其中的回环回调地址与 state。
func waitForAuthorizeURL(t *testing.T, stderr *syncBuffer) (authorize, redirect, state string) {
	t.Helper()
	pattern := regexp.MustCompile(`visit this URL to continue: (\S+)`)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		match := pattern.FindStringSubmatch(stderr.String())
		if match != nil {
			parsed, err := url.Parse(match[1])
			if err != nil {
				t.Fatalf("parse authorize URL: %v", err)
			}
			return match[1], parsed.Query().Get("redirect_uri"), parsed.Query().Get("state")
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("authorize URL never appeared in stderr: %s", stderr.String())
	return "", "", ""
}

func TestRunBrowserLoginFlow(t *testing.T) {
	silenceBrowserOpen(t)
	server, requests := stubTokenExchange(t, "browser-session-token")
	client, err := baize.NewClient(server.URL+"/api/v1", "", true, "test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	stderr := &syncBuffer{}
	resultCh := make(chan struct {
		token string
		err   error
	}, 1)
	go func() {
		token, err := runBrowserLogin(context.Background(), stderr, client, server.URL+"/api/v1", "baize-mcp/test")
		resultCh <- struct {
			token string
			err   error
		}{token, err}
	}()

	_, redirectURI, state := waitForAuthorizeURL(t, stderr)
	if !strings.HasPrefix(redirectURI, "http://127.0.0.1:") {
		t.Fatalf("redirect URI is not loopback: %q", redirectURI)
	}
	if state == "" {
		t.Fatal("authorize URL should carry a non-empty state")
	}

	// 模拟浏览器回跳：先发一次错误 state（服务端拒绝但不终止等待），再发正确回跳。
	parsed, _ := url.Parse(redirectURI)
	mismatchTarget := *parsed
	mismatchTarget.RawQuery = url.Values{"code": {"first-code"}, "state": {"wrong"}}.Encode()
	resp, err := http.Get(mismatchTarget.String())
	if err != nil {
		t.Fatalf("state mismatch callback: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("state mismatch status: %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "State check failed") {
		t.Fatalf("state mismatch page should explain the failure, got: %s", string(body))
	}

	successTarget := *parsed
	successTarget.RawQuery = url.Values{"code": {"one-time-code"}, "state": {state}}.Encode()
	resp, err = http.Get(successTarget.String())
	if err != nil {
		t.Fatalf("success callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("success callback status: %d", resp.StatusCode)
	}

	select {
	case result := <-resultCh:
		if result.err != nil {
			t.Fatalf("runBrowserLogin: %v", result.err)
		}
		if result.token != "browser-session-token" {
			t.Fatalf("unexpected token: %q", result.token)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runBrowserLogin did not finish")
	}

	if len(*requests) != 1 {
		t.Fatalf("expected exactly one token exchange, got %d", len(*requests))
	}
	exchange := (*requests)[0]
	if exchange["code"] != "one-time-code" {
		t.Fatalf("exchange should use the valid-state code, got %q", exchange["code"])
	}
	if exchange["redirectUri"] != redirectURI {
		t.Fatalf("exchange redirect URI mismatch: %q vs %q", exchange["redirectUri"], redirectURI)
	}
	if len(exchange["codeVerifier"]) < 43 {
		t.Fatalf("PKCE verifier too short: %q", exchange["codeVerifier"])
	}
}

func TestRunBrowserLoginDenied(t *testing.T) {
	silenceBrowserOpen(t)
	server, requests := stubTokenExchange(t, "should-not-issue")
	client, err := baize.NewClient(server.URL+"/api/v1", "", true, "test")
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	stderr := &syncBuffer{}
	resultCh := make(chan error, 1)
	go func() {
		_, err := runBrowserLogin(context.Background(), stderr, client, server.URL+"/api/v1", "baize-mcp/test")
		resultCh <- err
	}()

	_, redirectURI, state := waitForAuthorizeURL(t, stderr)
	parsed, _ := url.Parse(redirectURI)
	deniedTarget := *parsed
	deniedTarget.RawQuery = url.Values{"error": {"access_denied"}, "state": {state}}.Encode()
	resp, err := http.Get(deniedTarget.String())
	if err != nil {
		t.Fatalf("denied callback: %v", err)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()

	select {
	case err := <-resultCh:
		if err == nil || !strings.Contains(err.Error(), "denied") {
			t.Fatalf("expected denial error, got %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("runBrowserLogin did not finish")
	}
	if len(*requests) != 0 {
		t.Fatalf("denied flow must not exchange tokens, got %d exchanges", len(*requests))
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	verifier := strings.Repeat("v", 43)
	target, err := buildAuthorizeURL("https://baize.example.com/api/v1", "http://127.0.0.1:53712/callback", "state-1", pkceS256Challenge(verifier), "baize-mcp/0.1.7")
	if err != nil {
		t.Fatalf("buildAuthorizeURL: %v", err)
	}
	parsed, err := url.Parse(target)
	if err != nil {
		t.Fatalf("parse authorize URL: %v", err)
	}
	if parsed.Scheme != "https" || parsed.Host != "baize.example.com" || parsed.Path != "/cli-authorize" {
		t.Fatalf("unexpected authorize URL: %q", target)
	}
	query := parsed.Query()
	if query.Get("redirect_uri") != "http://127.0.0.1:53712/callback" {
		t.Fatalf("redirect_uri: %q", query.Get("redirect_uri"))
	}
	if query.Get("state") != "state-1" {
		t.Fatalf("state: %q", query.Get("state"))
	}
	if query.Get("client") != "baize-mcp/0.1.7" {
		t.Fatalf("client: %q", query.Get("client"))
	}
	sum := sha256.Sum256([]byte(verifier))
	if query.Get("code_challenge") != base64.RawURLEncoding.EncodeToString(sum[:]) {
		t.Fatalf("code_challenge should be base64url(sha256(verifier))")
	}
}

func TestCallbackHandlerStateMismatch(t *testing.T) {
	callback := make(chan browserCallback, 1)
	handler := newCallbackHandler("expected-state", callback)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/callback?code=x&state=evil", nil))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("state mismatch status: %d", recorder.Code)
	}
	// state 不匹配不通知等待流程：无关回跳不应打断正常登录。
	select {
	case result := <-callback:
		t.Fatalf("state mismatch must not terminate the flow, got %q", result.err)
	default:
	}
}
