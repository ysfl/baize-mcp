package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/ysfl/baize-mcp/internal/baize"
)

// browserLoginTimeout 覆盖打开浏览器、完成登录与授权、回跳兑换的完整窗口。
const browserLoginTimeout = 5 * time.Minute

// runBrowserLogin 执行 CLI 浏览器登录流：
// 1. 在 127.0.0.1 随机端口监听一次性回调；
// 2. 生成 state 与 PKCE S256 challenge，拼装授权页地址；
// 3. 尽力打开系统浏览器（地址始终打印到 stderr，便于手动复制）；
// 4. 校验回跳 state 后用授权码 + verifier 兑换会话凭据。
// 返回会话凭据明文，由调用方负责写入凭据存储；进度信息只写 stderr，stdout 留给最终 JSON。
func runBrowserLogin(ctx context.Context, stderr io.Writer, client *baize.Client, apiURL, clientName string) (string, error) {
	verifier, err := randomURLSafe(32)
	if err != nil {
		return "", errors.New("generate PKCE verifier")
	}
	state, err := randomURLSafe(16)
	if err != nil {
		return "", errors.New("generate login state")
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", errors.New("bind loopback callback listener; if this device has no local browser, retry with --password")
	}
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", listener.Addr().(*net.TCPAddr).Port)

	authorizeURL, err := buildAuthorizeURL(apiURL, redirectURI, state, pkceS256Challenge(verifier), clientName)
	if err != nil {
		_ = listener.Close()
		return "", err
	}

	callback := make(chan browserCallback, 1)
	server := &http.Server{Handler: newCallbackHandler(state, callback)}
	serveErr := make(chan error, 1)
	go func() { serveErr <- server.Serve(listener) }()

	// 信号与超时统一收口：任一触发都关闭回调服务并返回可读错误。
	watchCtx, stopWatch := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopWatch()
	timeoutCtx, cancelTimeout := context.WithTimeout(watchCtx, browserLoginTimeout)
	defer cancelTimeout()

	_, _ = fmt.Fprintln(stderr, "Opening the Baize authorize page in your browser…")
	_, _ = fmt.Fprintf(stderr, "If the browser did not open, visit this URL to continue: %s\n", authorizeURL)
	_, _ = fmt.Fprintln(stderr, "Waiting for authorization (this expires in 5 minutes)…")
	openBrowserPage(authorizeURL)

	var result browserCallback
	select {
	case result = <-callback:
	case err := <-serveErr:
		return "", fmt.Errorf("loopback callback server stopped: %v", err)
	case <-timeoutCtx.Done():
		_ = server.Close()
		if errors.Is(timeoutCtx.Err(), context.Canceled) {
			return "", errors.New("browser login canceled")
		}
		return "", errors.New("timed out waiting for browser authorization; restart login or retry with --password")
	}
	_ = server.Close()

	if result.err != "" {
		if result.err == "access_denied" {
			return "", errors.New("authorization was denied in the browser")
		}
		return "", fmt.Errorf("authorization failed in the browser: %s", result.err)
	}

	return client.ExchangeCliToken(ctx, result.code, verifier, redirectURI)
}

// browserCallback 承载一次浏览器回跳的解析结果。
type browserCallback struct {
	code string
	err  string
}

// newCallbackHandler 返回只处理一次回跳的处理器：
// 校验 state 防 CSRF，返回简短双语结果页，并通过通道把结果交给登录主流程。
func newCallbackHandler(expectedState string, callback chan<- browserCallback) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if !strings.HasSuffix(r.URL.Path, "/callback") {
			http.NotFound(w, r)
			return
		}
		state := query.Get("state")
		if state == "" || subtleStateMismatch(state, expectedState) {
			// state 不匹配视为无关回跳：直接拒绝但不终止等待，避免恶意回调打断正常登录。
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(cliCallbackHTML("state-mismatch", "登录状态校验失败", "State check failed")))
			return
		}
		if errCode := query.Get("error"); errCode != "" {
			_, _ = w.Write([]byte(cliCallbackHTML(errCode, "已取消授权", "Authorization canceled")))
			callback <- browserCallback{err: errCode}
			return
		}
		code := query.Get("code")
		if code == "" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(cliCallbackHTML("missing-code", "回跳缺少授权码", "Missing authorization code")))
			callback <- browserCallback{err: "missing_code"}
			return
		}
		_, _ = w.Write([]byte(cliCallbackHTML("", "登录成功", "Authorization complete")))
		callback <- browserCallback{code: code}
	})
}

// subtleStateMismatch 以常量时间比较回跳 state 与期望值，避免时序侧信道。
func subtleStateMismatch(got, expected string) bool {
	gotBytes := []byte(got)
	wantBytes := []byte(expected)
	if len(gotBytes) != len(wantBytes) {
		return true
	}
	var diff byte
	for i := range gotBytes {
		diff |= gotBytes[i] ^ wantBytes[i]
	}
	return diff != 0
}

// cliCallbackHTML 输出回跳结果页；不加载外部资源，仅内联最小样式。
func cliCallbackHTML(status, titleZh, titleEn string) string {
	detail := "可以关闭此页面并回到终端。You can close this page and return to the terminal."
	if status != "" {
		detail = "请回到终端查看后续说明。Return to the terminal for details."
	}
	return fmt.Sprintf(`<!doctype html>
<html lang="zh-CN">
<head><meta charset="utf-8"><title>Baize CLI</title></head>
<body style="font-family:system-ui,sans-serif;text-align:center;padding-top:4rem;color:#1f2328">
<h2>%s</h2>
<p>%s</p>
<p style="opacity:.6">%s</p>
</body>
</html>`, titleZh, titleEn, detail)
}

// buildAuthorizeURL 由基础地址拼装授权页地址：
// 授权页属于 Web 应用（部署在站点根路径），因此去掉 API 前缀再拼接 /cli-authorize。
func buildAuthorizeURL(apiURL, redirectURI, state, challenge, clientName string) (string, error) {
	parsed, err := url.Parse(apiURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return "", errors.New("invalid Baize API URL")
	}
	origin := parsed.Scheme + "://" + parsed.Host
	target, err := url.Parse(origin + "/cli-authorize")
	if err != nil {
		return "", errors.New("build authorize URL")
	}
	query := target.Query()
	query.Set("redirect_uri", redirectURI)
	query.Set("state", state)
	query.Set("code_challenge", challenge)
	if strings.TrimSpace(clientName) != "" {
		query.Set("client", clientName)
	}
	target.RawQuery = query.Encode()
	return target.String(), nil
}

// pkceS256Challenge 按 PKCE S256 口径计算 base64url(sha256(verifier))。
func pkceS256Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// randomURLSafe 生成 URL 安全的随机串，用于 state 与 PKCE verifier。
func randomURLSafe(size int) (string, error) {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

// openBrowserPage 尽力打开系统浏览器；失败不打断流程，URL 已打印到 stderr 可手动打开。
// 独立成包级变量是为了在测试中替换，避免用例真的拉起浏览器。
var openBrowserPage = func(target string) {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("cmd", "/c", "start", "", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	_ = command.Start()
}
