# AI Switch · 模型管理

[![Release](https://img.shields.io/github/v/release/vincent1986/ai-switch)](https://github.com/vincent1986/ai-switch/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-blue)](https://github.com/vincent1986/ai-switch/releases)

多厂家多模型管理桌面应用（Wails + Go），面向 **Codex** / **Claude Code**。

**English / 日本語 / Deutsch** release notes: [docs/RELEASE_NOTES_v1.0.0.md](docs/RELEASE_NOTES_v1.0.0.md)

## Features

- **厂家模型**：名称 / API Base URL / API Key，测试连接，自动拉取 `/models`
- **配置文件**：Codex / Claude Code 自动搜索、手动选择、备份还原、切换模型
- **代理服务**：本地 OpenAI 兼容网关，按 model 路由到各厂家（标准 OpenAI 协议出入）
- **Token 套餐 / 统计**：额度管理与代理链路用量
- **多语言 UI**：中文 / English
- **跨平台**：macOS / Windows / Linux

## Download v1.0.0

| Platform | Asset |
|----------|--------|
| macOS Apple Silicon | [AI-Switch-v1.0.0-macos-arm64.zip](https://github.com/vincent1986/ai-switch/releases/download/v1.0.0/AI-Switch-v1.0.0-macos-arm64.zip) |
| macOS Intel | [AI-Switch-v1.0.0-macos-amd64.zip](https://github.com/vincent1986/ai-switch/releases/download/v1.0.0/AI-Switch-v1.0.0-macos-amd64.zip) |
| Windows x64 Setup | [AI-Switch-v1.0.0-windows-amd64-setup.exe](https://github.com/vincent1986/ai-switch/releases/download/v1.0.0/AI-Switch-v1.0.0-windows-amd64-setup.exe) |
| Windows x64 Portable | [AI-Switch-v1.0.0-windows-amd64-portable.zip](https://github.com/vincent1986/ai-switch/releases/download/v1.0.0/AI-Switch-v1.0.0-windows-amd64-portable.zip) |
| Linux x64 | [AI-Switch-v1.0.0-linux-amd64.tar.gz](https://github.com/vincent1986/ai-switch/releases/download/v1.0.0/AI-Switch-v1.0.0-linux-amd64.tar.gz) |

Full install guide (zh / en / ja / de): **[Release Notes](docs/RELEASE_NOTES_v1.0.0.md)** · **[GitHub Release](https://github.com/vincent1986/ai-switch/releases/tag/v1.0.0)**

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
# 当前平台
wails build

# macOS
wails build -platform darwin/arm64
wails build -platform darwin/amd64

# Windows（可生成 NSIS 安装包）
wails build -platform windows/amd64 -nsis

# Linux（请在 Linux 主机、Docker 或 GitHub Actions 中构建）
wails build -platform linux/amd64
```

打包产物建议输出到 `dist/release/vX.Y.Z/`。详细说明见 [docs/RELEASE_NOTES_v1.0.0.md](docs/RELEASE_NOTES_v1.0.0.md)。

## License

See repository for license terms.
