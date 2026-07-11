# Codex · 模型管理

多厂家多模型管理工具（Wails + Go），支持：

- **厂家模型**：名称 / API Base URL / API Key，测试连接，自动拉取 `/models`
- **配置文件**：Codex / Claude Code 自动搜索、手动选择、备份还原、切换模型
- **代理服务**：本地 OpenAI 兼容网关，按 model 路由到各厂家（标准 OpenAI 协议出入）
- **跨平台**：macOS / Windows 路径与文件管理器适配

## 开发

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
export GOMODCACHE="${GOMODCACHE:-$HOME/.cache/go-mod}"
wails dev
```

## 数据位置

| 内容 | 路径 |
|------|------|
| 厂家与密钥 | `~/.codex-manager/providers.json` |
| 配置路径覆盖 | `~/.codex-manager/paths.json` |
| 默认备份 | `~/.codex-manager/backups/{codex,claude}/` |
| 代理配置 | `~/.codex-manager/proxy.json`（默认 `http://127.0.0.1:18080/v1`） |

### 代理用法

1. 在「厂家模型」配置好各厂家 API 与模型  
2. 「代理服务」→ 启动  
3. 客户端 / Codex 使用：

```toml
model = "deepseek-chat"          # 必须是已启用模型 ID
model_provider = "codex_proxy"

[model_providers.codex_proxy]
name = "OpenAI Proxy"
base_url = "http://127.0.0.1:18080/v1"
env_key = "codex_proxy_api_key"
```

或在代理页点击「写入 Codex 配置」。

## 构建

```bash
wails build
```
