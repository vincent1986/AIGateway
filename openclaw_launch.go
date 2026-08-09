package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

const (
	defaultOpenClawGatewayPort = 18789
	openClawInstallHint        = "未找到 openclaw 命令。请先安装：npm install -g openclaw@latest && openclaw onboard --install-daemon"
)

// OpenClawLaunchResult is returned by LaunchOpenClaw.
type OpenClawLaunchResult struct {
	Binary         string `json:"binary"`
	ConfigPath     string `json:"configPath"`
	AlreadyRunning bool   `json:"alreadyRunning"`
	Started        bool   `json:"started"`
	Ready          bool   `json:"ready"`
	DashboardURL   string `json:"dashboardUrl"`
	Message        string `json:"message"`
	Managed        bool   `json:"managed"`
}

var (
	lookPathFn             = exec.LookPath
	runOpenClawCommandFn   = runOpenClawCommand
	startDetachedFn        = startDetachedProcess
	dialOpenClawFn         = dialOpenClawPort
	openClawReadyCheckFn   = defaultOpenClawReadyCheck
	openClawHTTPClient     = &http.Client{Timeout: 900 * time.Millisecond}
)

// LaunchOpenClaw ensures AIGateway proxy is up, prepares OpenClaw config
// (gateway.mode=local), starts the OpenClaw gateway if needed, and returns
// the Control UI URL for the frontend to open.
func (a *App) LaunchOpenClaw() (OpenClawLaunchResult, error) {
	res := OpenClawLaunchResult{}

	if a.proxy == nil {
		a.proxy = newProxyServer()
	}
	if !a.proxy.status().Running {
		if err := a.proxy.start(); err != nil {
			return res, fmt.Errorf("启动 AIGateway 网关失败: %w", err)
		}
	}

	bin, err := resolveOpenClawBinary()
	if err != nil {
		return res, err
	}
	res.Binary = bin

	st := a.resolveTool(ToolOpenClaw)
	res.ConfigPath = st.Path
	res.Managed = st.Managed
	if res.ConfigPath == "" {
		if d := driverByID("openclaw"); d != nil {
			res.ConfigPath = d.PreferredPath()
		}
	}
	if res.ConfigPath == "" {
		return res, fmt.Errorf("未找到 OpenClaw 配置路径，请先在应用管理中选择配置文件")
	}

	if _, err := ensureOpenClawGatewayModeLocal(res.ConfigPath); err != nil {
		return res, err
	}

	port := readOpenClawGatewayPort(res.ConfigPath)

	if openClawReadyCheckFn(bin, port) {
		return finishOpenClawLaunch(&res, bin, true, false, port)
	}

	// Prefer supervised service start when available.
	if _, startErr := runOpenClawCommandFn(bin, 12*time.Second, "gateway", "start"); startErr == nil {
		if waitOpenClawGatewayReady(bin, port, 12*time.Second) {
			return finishOpenClawLaunch(&res, bin, false, true, port)
		}
	}

	if err := startDetachedFn(bin, "gateway", "run"); err != nil {
		return res, fmt.Errorf("启动 OpenClaw Gateway 失败: %w（也可在终端执行: openclaw gateway）", err)
	}

	if waitOpenClawGatewayReady(bin, port, 20*time.Second) {
		return finishOpenClawLaunch(&res, bin, false, true, port)
	}

	return res, fmt.Errorf("已发起启动，但 OpenClaw Gateway 尚未就绪（端口 %d）。请执行 openclaw gateway status，确认完成后再打开控制台", port)
}

func finishOpenClawLaunch(res *OpenClawLaunchResult, bin string, already, started bool, port int) (OpenClawLaunchResult, error) {
	res.AlreadyRunning = already
	res.Started = started
	res.Ready = true
	dashboard, withToken := resolveOpenClawDashboardURL(bin, res.ConfigPath, port)
	res.DashboardURL = dashboard
	if already {
		res.Message = fmt.Sprintf("OpenClaw Gateway 已就绪（端口 %d），正在打开控制台", port)
	} else {
		res.Message = fmt.Sprintf("OpenClaw Gateway 已启动并就绪（端口 %d），正在打开控制台", port)
	}
	if withToken {
		res.Message += "（已附带默认 token）"
	}
	if !res.Managed {
		res.Message += "；建议先「一键接管」以便走 AIGateway 路由"
	}
	return *res, nil
}

// resolveOpenClawDashboardURL prefers an authenticated Control UI link so the
// browser can enter without pasting gateway.auth.token manually.
func resolveOpenClawDashboardURL(bin, configPath string, port int) (dashboard string, withToken bool) {
	base := fmt.Sprintf("http://127.0.0.1:%d/", port)
	if port <= 0 {
		base = fmt.Sprintf("http://127.0.0.1:%d/", defaultOpenClawGatewayPort)
	}
	if u := openClawDashboardCLIURL(bin); u != "" {
		return u, strings.Contains(u, "#token=") || strings.Contains(u, "token=")
	}
	if token := readOpenClawGatewayAuthToken(configPath); token != "" {
		return openClawDashboardURLWithToken(base, token), true
	}
	return base, false
}

func openClawDashboardURLWithToken(base, token string) string {
	token = strings.TrimSpace(token)
	base = strings.TrimSpace(base)
	if token == "" {
		return base
	}
	if base == "" {
		base = fmt.Sprintf("http://127.0.0.1:%d/", defaultOpenClawGatewayPort)
	}
	// Control UI reads auth from the URL fragment and strips it after bootstrap.
	u := strings.TrimRight(base, "/") + "/"
	return u + "#token=" + url.QueryEscape(token)
}

func openClawDashboardCLIURL(bin string) string {
	if strings.TrimSpace(bin) == "" {
		return ""
	}
	// Newer CLIs: machine-readable browserUrl with one-time/bootstrap auth.
	for _, args := range [][]string{
		{"dashboard", "--json"},
		{"dashboard", "--no-open", "--json"},
	} {
		out, err := runOpenClawCommandFn(bin, 10*time.Second, args...)
		if err != nil && strings.TrimSpace(out) == "" {
			continue
		}
		if u := parseOpenClawDashboardJSONURL(out); u != "" {
			return u
		}
	}
	return ""
}

func parseOpenClawDashboardJSONURL(out string) string {
	start := strings.IndexByte(out, '{')
	if start < 0 {
		return ""
	}
	var root map[string]any
	if json.Unmarshal([]byte(out[start:]), &root) != nil {
		return ""
	}
	if ok, _ := root["ok"].(bool); ok == false && root["ok"] != nil {
		return ""
	}
	for _, key := range []string{"browserUrl", "url", "httpUrl"} {
		if v, _ := root[key].(string); strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func readOpenClawGatewayAuthToken(configPath string) string {
	if v := strings.TrimSpace(os.Getenv("OPENCLAW_GATEWAY_TOKEN")); v != "" {
		return v
	}
	path := expandPath(configPath)
	if path == "" || !fileExists(path) {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var root map[string]any
	if unmarshalOpenClawJSON5(b, &root) != nil {
		return ""
	}
	gateway, _ := root["gateway"].(map[string]any)
	if gateway == nil {
		return ""
	}
	auth, _ := gateway["auth"].(map[string]any)
	if auth == nil {
		return ""
	}
	switch v := auth["token"].(type) {
	case string:
		return strings.TrimSpace(v)
	default:
		// SecretRef / non-string tokens cannot be safely inlined into a URL.
		return ""
	}
}

func waitOpenClawGatewayReady(bin string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if openClawReadyCheckFn(bin, port) {
			return true
		}
		time.Sleep(400 * time.Millisecond)
	}
	return openClawReadyCheckFn(bin, port)
}

// defaultOpenClawReadyCheck requires the TCP port and either a serving Control UI
// HTTP response or a healthy `openclaw gateway status --json` probe.
func defaultOpenClawReadyCheck(bin string, port int) bool {
	if port <= 0 {
		port = defaultOpenClawGatewayPort
	}
	if !dialOpenClawFn(port) {
		return false
	}
	if httpOpenClawControlUIReady(port) {
		return true
	}
	return openClawCLIStatusReady(bin)
}

func resolveOpenClawBinary() (string, error) {
	if p, err := lookPathFn("openclaw"); err == nil && strings.TrimSpace(p) != "" {
		return p, nil
	}
	home := userHome()
	candidates := []string{
		"/usr/local/bin/openclaw",
		"/opt/homebrew/bin/openclaw",
	}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local", "bin", "openclaw"),
			filepath.Join(home, ".npm-global", "bin", "openclaw"),
			filepath.Join(home, "AppData", "Roaming", "npm", "openclaw.cmd"),
			filepath.Join(home, "AppData", "Roaming", "npm", "openclaw"),
		)
		for _, pattern := range []string{
			filepath.Join(home, ".nvm", "versions", "node", "*", "bin", "openclaw"),
			filepath.Join(home, ".local", "share", "fnm", "node-versions", "*", "installation", "bin", "openclaw"),
			filepath.Join(home, ".volta", "bin", "openclaw"),
		} {
			if matches, err := filepath.Glob(pattern); err == nil {
				candidates = append(candidates, matches...)
			}
		}
	}
	if runtime.GOOS == "windows" {
		candidates = append(candidates, `C:\Program Files\nodejs\openclaw.cmd`)
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c, nil
		}
	}
	return "", fmt.Errorf("%s", openClawInstallHint)
}

func ensureOpenClawGatewayModeLocal(path string) (bool, error) {
	path = expandPath(path)
	if path == "" {
		return false, fmt.Errorf("OpenClaw 配置路径为空")
	}
	var raw []byte
	var root map[string]any
	if fileExists(path) {
		b, err := os.ReadFile(path)
		if err != nil {
			return false, err
		}
		raw = b
		if len(bytes.TrimSpace(raw)) > 0 {
			if err := unmarshalOpenClawJSON5(raw, &root); err != nil {
				return false, fmt.Errorf("解析 OpenClaw 配置失败: %w", err)
			}
		}
	}
	if root == nil {
		root = map[string]any{}
	}
	gateway, _ := root["gateway"].(map[string]any)
	if gateway == nil {
		gateway = map[string]any{}
	}
	mode := ""
	if v, ok := gateway["mode"].(string); ok {
		mode = strings.TrimSpace(v)
	}
	if strings.EqualFold(mode, "local") {
		return false, nil
	}
	if mode != "" {
		return false, fmt.Errorf("OpenClaw gateway.mode=%s，不能一键启动本地 Gateway；请改为 local，或使用 openclaw dashboard 连接远程", mode)
	}
	gateway["mode"] = "local"
	root["gateway"] = gateway
	b, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return false, err
	}
	next := string(b) + "\n"
	if len(raw) > 0 {
		next = preserveLineEndings(string(raw), next)
	}
	if err := writeFileAtomic(path, next); err != nil {
		return false, err
	}
	return true, nil
}

func readOpenClawGatewayPort(path string) int {
	port := defaultOpenClawGatewayPort
	path = expandPath(path)
	if path == "" || !fileExists(path) {
		return port
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return port
	}
	var root map[string]any
	if unmarshalOpenClawJSON5(b, &root) != nil {
		return port
	}
	gateway, _ := root["gateway"].(map[string]any)
	if gateway == nil {
		return port
	}
	switch v := gateway["port"].(type) {
	case float64:
		if int(v) > 0 {
			return int(v)
		}
	case json.Number:
		if n, err := v.Int64(); err == nil && n > 0 {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
	case int:
		if v > 0 {
			return v
		}
	}
	return port
}

func dialOpenClawPort(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func httpOpenClawControlUIReady(port int) bool {
	url := fmt.Sprintf("http://127.0.0.1:%d/", port)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := openClawHTTPClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	// Any HTTP response means the Control UI listener is serving pages.
	return resp.StatusCode > 0 && resp.StatusCode < 600
}

func openClawCLIStatusReady(bin string) bool {
	if strings.TrimSpace(bin) == "" {
		return false
	}
	out, err := runOpenClawCommandFn(bin, 8*time.Second, "gateway", "status", "--json")
	if err != nil && strings.TrimSpace(out) == "" {
		return false
	}
	return openClawStatusJSONReady(out)
}

func openClawStatusJSONReady(out string) bool {
	start := strings.IndexByte(out, '{')
	if start < 0 {
		return false
	}
	out = out[start:]
	var root map[string]any
	if json.Unmarshal([]byte(out), &root) != nil {
		return false
	}
	if rpc, _ := root["rpc"].(map[string]any); rpc != nil {
		if ok, _ := rpc["ok"].(bool); ok {
			return true
		}
	}
	if svc, _ := root["service"].(map[string]any); svc != nil {
		if rt, _ := svc["runtime"].(map[string]any); rt != nil {
			status := strings.ToLower(strings.TrimSpace(fmt.Sprint(rt["status"])))
			state := strings.ToLower(strings.TrimSpace(fmt.Sprint(rt["state"])))
			if status == "running" || state == "active" {
				return true
			}
		}
	}
	return false
}

func runOpenClawCommand(bin string, timeout time.Duration, args ...string) (string, error) {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func startDetachedProcess(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	configureDetachedCmd(cmd)
	return cmd.Start()
}
