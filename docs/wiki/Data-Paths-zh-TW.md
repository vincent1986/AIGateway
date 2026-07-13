# 資料與路徑

**語言：** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [首頁](Home)

AIGateway 將狀態寫在使用者目錄下的 `.codex-manager` 中（跨平台）。

## 主要路徑

| 內容 | 路徑 |
|------|------|
| SQLite 主庫（廠商、模型組、路由、用量） | `~/.codex-manager/aigateway.db` |
| 廠商 JSON 鏡像 | `~/.codex-manager/providers.json` |
| 閘道設定 | `~/.codex-manager/proxy.json` |
| 工具設定備份 | `~/.codex-manager/backups/` |
| 環境變數相關 | `~/.codex-manager/env/` |

> Windows 下 `~` 對應使用者主目錄（如 `C:\Users\<你>`）。

## 從 v1 升級

- 首次啟動 v2 會將舊的 `providers.json` / `usage.json` 遷移進 SQLite。
- 業務主狀態以 **SQLite** 為準；部分閘道監聽設定仍可能在 `proxy.json`。

## 備份建議

- 升級或重裝前，可備份整個 `~/.codex-manager/` 目錄。
- 應用管理「一鍵接管」前會對工具原設定做快照，可在卡片內回滾。

相關：[快速開始](Getting-Started-zh-TW) · [故障排除](Troubleshooting-zh-TW)
