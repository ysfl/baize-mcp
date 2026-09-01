package baize

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync"
)

// credentialState 保存当前进程使用的会话，并在登录或退出登录后按需更新。
// 凭据读取器只从本机安全存储取值，不把会话内容带入 MCP 结果或日志。
type credentialState struct {
	mu       sync.RWMutex
	reloadMu sync.Mutex
	token    string
	loader   func() (string, error)
}

func newCredentialState(token string) *credentialState {
	return &credentialState{token: strings.TrimSpace(token)}
}

// SetCredentialLoader 绑定本机凭据读取器。读取失败时保留现有会话，避免短暂的系统凭据存储故障打断正在进行的请求。
func (c *Client) SetCredentialLoader(loader func() (string, error)) {
	if c == nil {
		return
	}
	if c.credentials == nil {
		c.credentials = newCredentialState("")
	}
	c.credentials.mu.Lock()
	c.credentials.loader = loader
	c.credentials.mu.Unlock()
}

func (c *Client) currentCredential() string {
	if c == nil || c.credentials == nil {
		return ""
	}
	c.credentials.mu.RLock()
	defer c.credentials.mu.RUnlock()
	return c.credentials.token
}

// refreshCredential 重新读取本机凭据。空值表示用户已经移除本地会话，成功读取的新值才会替换内存中的旧值。
func (c *Client) refreshCredential(previous string) (string, bool) {
	if c == nil || c.credentials == nil {
		return "", false
	}
	state := c.credentials
	// 凭据读取可能跨越系统存储调用；串行化可避免较早请求的迟到结果覆盖较新的会话。
	state.reloadMu.Lock()
	defer state.reloadMu.Unlock()
	state.mu.RLock()
	current, loader := state.token, state.loader
	state.mu.RUnlock()
	if loader == nil {
		return current, false
	}

	loaded, err := loader()
	if err != nil {
		return current, false
	}
	loaded = strings.TrimSpace(loaded)

	state.mu.Lock()
	defer state.mu.Unlock()
	if loaded == "" {
		changed := state.token != ""
		state.token = ""
		return "", changed
	}
	// 并发请求可能已经装载了更新的会话，优先保留较新的内存值。
	if state.token != previous && state.token != "" {
		return state.token, state.token != previous
	}
	changed := state.token != loaded
	state.token = loaded
	return loaded, changed
}

func canRetryAfterAuthFailure(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	default:
		return false
	}
}

// cloneRequestForRetry 只为可安全重放的请求创建副本；无法重建请求体时直接放弃重试。
func cloneRequestForRetry(req *http.Request) (*http.Request, error) {
	if req == nil {
		return nil, errors.New("request is unavailable for retry")
	}
	retry := req.Clone(req.Context())
	if req.Body == nil || req.Body == http.NoBody {
		return retry, nil
	}
	if req.GetBody == nil {
		return nil, errors.New("request body cannot be replayed")
	}
	body, err := req.GetBody()
	if err != nil {
		return nil, errors.New("request body could not be replayed")
	}
	retry.Body = body
	return retry, nil
}

func normalizeRequestError(err error) error {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return errors.New("Baize request timed out or was cancelled")
	}
	return errors.New("Baize request failed before a response was received")
}
