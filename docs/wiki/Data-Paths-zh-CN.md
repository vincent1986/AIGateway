# 数据与路径

**语言：** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [首页](Home)

AIGateway 将状态写在用户目录下的 `.codex-manager` 中（跨平台）。

## 主要路径

| 内容 | 路径 |
|------|------|
| SQLite 主库（厂家、模型组、路由、用量） | `~/.codex-manager/aigateway.db` |
| 厂家 JSON 镜像 | `~/.codex-manager/providers.json` |
| 网关配置 | `~/.codex-manager/proxy.json` |
| 工具配置备份 | `~/.codex-manager/backups/` |
| 环境变量相关 | `~/.codex-manager/env/` |

> Windows 下 `~` 对应用户主目录（如 `C:\Users\<你>`）。

## 从 v1 升级

- 首次启动 v2 会将旧的 `providers.json` / `usage.json` 迁移进 SQLite。
- 业务主状态以 **SQLite** 为准；部分网关监听配置仍可能在 `proxy.json`。

## 备份建议

- 升级或重装前，可备份整个 `~/.codex-manager/` 目录。
- 应用管理「一键接管」前会对工具原配置做快照，可在卡片内回滚。

相关：[快速开始](Getting-Started-zh-CN) · [故障排除](Troubleshooting-zh-CN)
