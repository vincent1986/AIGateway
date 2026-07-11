# AIGateway · 模型管理

[![Release](https://img.shields.io/github/v/release/vincent1986/AIGateway)](https://github.com/vincent1986/AIGateway/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-blue)](https://github.com/vincent1986/AIGateway/releases)

**AIGateway** 是一款 **AI 模型管理软件**：把市场上常见 AI 工具用到的模型、厂家与额度统一管起来，帮你 **尽量少花 Token、多发现更便宜的服务商**。

适用场景包括（不限于）：

- **Codex** / 各类 CLI / IDE 助手  
- **ChatGPT** 生态与 OpenAI 兼容接口客户端  
- **Claude** / **Claude Code**  
- **OpenClaw**、**Harness** 等工具链与编排场景  
- 以及其它走 **OpenAI 兼容 API** 的本地或云端工具  

**English / 日本語 / Deutsch** release notes: [docs/RELEASE_NOTES_v1.0.0.md](docs/RELEASE_NOTES_v1.0.0.md)  
**V2 PRD**: [docs/PRD_V2.md](docs/PRD_V2.md) · **验收清单**: [docs/PRD_V2_CHECKLIST.md](docs/PRD_V2_CHECKLIST.md)

## 你能得到什么

| 目标 | 怎么做到 |
|------|----------|
| **省 Token** | 统一看清各厂家用量与套餐剩余；按模型/厂家统计，避免「黑盒烧额度」 |
| **找更便宜的服务商** | 一处添加多家 API（DeepSeek、通义、智谱、Moonshot、自定义等），随时对比与切换 |
| **一套模型管多工具** | 把模型写入 Codex / Claude Code 等配置，或经本地兼容网关给任意客户端用 |
| **随时换模型** | 厂家 → 模型列表 → 一键应用到工具，不用到处改配置 |

## 功能概览

- **厂家 / 模型 / 应用 / 统一入口 / Token 统计** 五栏解耦  
- **SQLite 网关**：虚拟模型组、优先级 Failover、用量统计  
- **一键接管**：ChatGPT / Claude Code / OpenClaw / Harness  
- **预设库**：点选厂家，云端多数只需 API Key  
- **多语言（弹窗）**：简中 / 繁中 / 英 / 日 / 韩 / 德 / 越 / 泰  
- **跨平台**：macOS / Windows / Linux  

## Download

### v2.0.0（当前）

| Platform | Asset |
|----------|--------|
| macOS Apple Silicon | [AIGateway-v2.0.0-macos-arm64.zip](https://github.com/vincent1986/AIGateway/releases/download/v2.0.0/AIGateway-v2.0.0-macos-arm64.zip) |
| macOS Intel | [AIGateway-v2.0.0-macos-amd64.zip](https://github.com/vincent1986/AIGateway/releases/download/v2.0.0/AIGateway-v2.0.0-macos-amd64.zip) |
| Windows x64 Setup | [AIGateway-v2.0.0-windows-amd64-setup.exe](https://github.com/vincent1986/AIGateway/releases/download/v2.0.0/AIGateway-v2.0.0-windows-amd64-setup.exe) |
| Windows x64 Portable | [AIGateway-v2.0.0-windows-amd64-portable.zip](https://github.com/vincent1986/AIGateway/releases/download/v2.0.0/AIGateway-v2.0.0-windows-amd64-portable.zip) |
| Linux x64 | [AIGateway-v2.0.0-linux-amd64.tar.gz](https://github.com/vincent1986/AIGateway/releases/download/v2.0.0/AIGateway-v2.0.0-linux-amd64.tar.gz) |

**说明**：[RELEASE_NOTES_v2.0.0.md](docs/RELEASE_NOTES_v2.0.0.md) · **[GitHub Release](https://github.com/vincent1986/AIGateway/releases/tag/v2.0.0)**  
**V2 PRD**: [PRD_V2.md](docs/PRD_V2.md) · **验收清单**: [PRD_V2_CHECKLIST.md](docs/PRD_V2_CHECKLIST.md)  
**v1.0.0 归档**: [releases/tag/v1.0.0](https://github.com/vincent1986/AIGateway/releases/tag/v1.0.0)

## 开发

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
export GOMODCACHE="${GOMODCACHE:-$HOME/.cache/go-mod}"
wails dev
```

## 数据位置

| 内容 | 路径 |
|------|------|
| SQLite 主库 | `~/.codex-manager/aigateway.db` |
| 厂家 JSON 镜像 | `~/.codex-manager/providers.json` |
| 配置路径覆盖 | `~/.codex-manager/paths.json` |
| 默认备份 | `~/.codex-manager/backups/` |
| 网关配置 | `~/.codex-manager/proxy.json`（默认 `http://127.0.0.1:18080/v1`） |

### 给 Codex / 兼容客户端用的统一入口

1. 在「厂家模型」添加多家服务商与模型  
2. 需要统一 Base URL 时开启「走本地代理」并保存  
3. 在 Codex / 其它工具里使用：

```toml
model = "deepseek-chat"          # 必须是已启用模型 ID
model_provider = "codex_proxy"

[model_providers.codex_proxy]
name = "OpenAI Proxy"
base_url = "http://127.0.0.1:18080/v1"
env_key = "codex_proxy_api_key"
```

这样即可在 **不改工具协议** 的前提下，把流量切到更便宜的厂家模型。

## 构建

```bash
wails build
wails build -platform darwin/arm64
wails build -platform windows/amd64 -nsis
wails build -platform linux/amd64   # Linux 主机 / CI
```

详见 [docs/RELEASE_NOTES_v1.0.0.md](docs/RELEASE_NOTES_v1.0.0.md)。

## License

See repository for license terms.
