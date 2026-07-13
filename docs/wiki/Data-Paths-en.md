# Data Paths

**Languages:** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [Home](Home)

AIGateway stores state under `.codex-manager` in your user home directory (cross-platform).

## Main paths

| Content | Path |
|---------|------|
| SQLite DB (providers, model groups, routing, usage) | `~/.codex-manager/aigateway.db` |
| Providers JSON mirror | `~/.codex-manager/providers.json` |
| Gateway config | `~/.codex-manager/proxy.json` |
| Tool config backups | `~/.codex-manager/backups/` |
| Environment-related files | `~/.codex-manager/env/` |

> On Windows, `~` is your user profile (e.g. `C:\Users\<you>`).

## Upgrading from v1

- First launch of v2 migrates old `providers.json` / `usage.json` into SQLite.
- **SQLite** is the source of truth for business state; some listen settings may still live in `proxy.json`.

## Backup tips

- Before upgrade or reinstall, back up the whole `~/.codex-manager/` folder.
- One-click takeover snapshots tool configs; you can roll back from the app card.

See also: [Getting Started](Getting-Started-en) · [Troubleshooting](Troubleshooting-en)
