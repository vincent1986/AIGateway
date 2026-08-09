package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	DashboardURL   string `json:"dashboardUrl"`
	Message        string `json:"message"`
	Managed        bool   `json:"managed"`
}

var (
	lookPathFn           = exec.LookPath
	runOpenClawCommandFn = runOpenClawCommand
	startDetachedFn      = startDetachedProcess
	dialOpenClawFn       = dialOpenClawPort
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
	res.DashboardURL = fmt.Sprintf("http://127.0.0.1:%d", port)

	if isOpenClawGatewayReachable(port) {
		res.AlreadyRunning = true
		res.Message = fmt.Sprintf("OpenClaw Gateway 已在运行（端口 %d）", port)
		if !res.Managed {
			res.Message += "；建议先「一键接管」以便走 AIGateway 路由"
		}
		return res, nil
	}

	// Prefer supervised service start when available.
	if _, startErr := runOpenClawCommandFn(bin, 12*time.Second, "gateway", "start"); startErr == nil {
		if waitOpenClawGateway(port, 5*time.Second) {
			res.Started = true
			res.Message = fmt.Sprintf("已通过服务启动 OpenClaw Gateway（端口 %d）", port)
			if !res.Managed {
				res.Message += "；建议先「一键接管」以便走 AIGateway 路由"
			}
			return res, nil
		}
	}

	if err := startDetachedFn(bin, "gateway", "run"); err != nil {
		return res, fmt.Errorf("启动 OpenClaw Gateway 失败: %w（也可在终端执行: openclaw gateway）", err)
	}

	if waitOpenClawGateway(port, 8*time.Second) {
		res.Started = true
		res.Message = fmt.Sprintf("已启动 OpenClaw Gateway（端口 %d）", port)
		if !res.Managed {
			res.Message += "；建议先「一键接管」以便走 AIGateway 路由"
		}
		return res, nil
	}

	return res, fmt.Errorf("已发起启动，但端口 %d 尚未就绪。请执行 openclaw gateway status，或在终端运行: openclaw gateway", port)
}

func waitOpenClawGateway(port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if isOpenClawGatewayReachable(port) {
			return true
		}
		time.Sleep(300 * time.Millisecond)
	}
	return isOpenClawGatewayReachable(port)
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

func isOpenClawGatewayReachable(port int) bool {
	if port <= 0 {
		port = defaultOpenClawGatewayPort
	}
	return dialOpenClawFn(port)
}

func dialOpenClawPort(port int) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 400*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
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

func openClawStatusLooksRunning(out string) bool {
	s := strings.ToLower(out)
	if strings.Contains(s, `"running":true`) || strings.Contains(s, `"ok":true`) {
		return true
	}
	if strings.Contains(s, "running") && !strings.Contains(s, "not running") {
		return true
	}
	return false
}

func startDetachedProcess(bin string, args ...string) error {
	cmd := exec.Command(bin, args...)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.Stdin = nil
	configureDetachedCmd(cmd)
	return cmd.Start()
}
