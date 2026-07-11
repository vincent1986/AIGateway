# AI Switch v1.0.0 Release Notes

| | |
|---|---|
| **Version** | 1.0.0 |
| **Tag** | `v1.0.0` |
| **Repository** | https://github.com/vincent1986/ai-switch |
| **Release** | https://github.com/vincent1986/ai-switch/releases/tag/v1.0.0 |

---

## 简体中文

### 简介

**AI Switch** 是面向 **Codex** 与 **Claude Code** 的多厂家 AI 模型管理桌面应用（Wails + Go）。统一管理 API 厂家、模型列表、本地配置文件与 OpenAI 兼容代理，并支持 Token 套餐与用量统计。

### 主要功能

- **厂家模型**：名称 / API Base URL / API Key；测试连接；自动拉取 `/models`；启用/禁用/默认模型
- **配置文件**：Codex / Claude Code 自动搜索、手动选择、备份与还原、一键切换模型
- **代理服务**：本地 OpenAI 兼容网关（默认 `http://127.0.0.1:18080/v1`），按 model 路由到各厂家并自动附带对应 Key
- **Token 套餐**：按厂家管理购买额度（总量、偏移已用、有效期），结合代理统计估算剩余
- **Token 统计**：经本地代理转发的请求用量（按模型 / 厂家 / 日 / 最近记录）
- **多语言**：界面支持中文 / English，顶栏切换，偏好保存在本机
- **跨平台**：macOS / Windows（Linux 见下方下载说明）

### 系统要求

| 平台 | 架构 | 说明 |
|------|------|------|
| macOS | Apple Silicon (arm64) | macOS 12+ 推荐 |
| macOS | Intel (amd64) | macOS 12+ 推荐 |
| Windows | x64 (amd64) | Windows 10/11，需 WebView2（安装包可自动处理） |
| Linux | x64 (amd64) | 见发布资源；需 WebKit/GTK 运行时 |

### 下载与安装

| 文件 | 平台 | 说明 |
|------|------|------|
| `AI-Switch-v1.0.0-macos-arm64.zip` | macOS Apple Silicon | 解压后得到 `AI Switch.app` |
| `AI-Switch-v1.0.0-macos-amd64.zip` | macOS Intel | 解压后得到 `AI Switch.app` |
| `AI-Switch-v1.0.0-windows-amd64-setup.exe` | Windows x64 | NSIS 安装程序（推荐） |
| `AI-Switch-v1.0.0-windows-amd64-portable.zip` | Windows x64 | 绿色免安装可执行文件 |
| `AI-Switch-v1.0.0-linux-amd64.tar.gz` | Linux x64 | 解压后运行二进制（由 CI 构建上传） |
| `SHA256SUMS.txt` | 全部 | SHA-256 校验和 |

#### macOS

1. 下载对应芯片架构的 zip 并解压  
2. 将 `AI Switch.app` 拖入「应用程序」  
3. 首次打开若提示未验证开发者：系统设置 → 隐私与安全性 → 仍要打开  
4. 或在终端：`xattr -cr "/Applications/AI Switch.app"`

#### Windows

1. **推荐**：运行 `AI-Switch-v1.0.0-windows-amd64-setup.exe` 按向导安装  
2. 或解压 portable zip，直接运行 `AI-Switch-v1.0.0-windows-amd64.exe`  
3. 若无法启动，请安装 [Microsoft Edge WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)

#### Linux

1. 下载 `AI-Switch-v1.0.0-linux-amd64.tar.gz` 并解压  
2. `chmod +x AI-Switch && ./AI-Switch`  
3. 依赖示例（Debian/Ubuntu）：`libgtk-3-0`、`libwebkit2gtk-4.0-37` 或发行版对应的 WebKitGTK 包  

### 快速开始

1. 打开应用 → **厂家模型** → 添加厂家（可用预设：OpenAI、DeepSeek、通义千问等）  
2. 填写 Base URL 与 API Key → **测试连接** → **自动获取模型**  
3. 需要统一入口时：厂家选择「走本地代理」并保存（会自动启动代理）  
4. 在 **配置文件** 页将模型写入 Codex / Claude Code，或在模型行点击 → Codex / → Claude  
5. 客户端 Codex 示例：

```toml
model = "deepseek-chat"
model_provider = "codex_proxy"

[model_providers.codex_proxy]
name = "OpenAI Proxy"
base_url = "http://127.0.0.1:18080/v1"
env_key = "codex_proxy_api_key"
```

### 数据位置

| 内容 | 路径 |
|------|------|
| 厂家与密钥 | `~/.codex-manager/providers.json` |
| 配置路径覆盖 | `~/.codex-manager/paths.json` |
| 默认备份 | `~/.codex-manager/backups/{codex,claude}/` |
| 代理配置 | `~/.codex-manager/proxy.json` |
| 用量统计 | `~/.codex-manager/`（usage 相关文件） |

### 从源码构建

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
# 需要：Go、Node.js、Wails CLI v2
wails build
# 指定平台示例
wails build -platform darwin/arm64
wails build -platform windows/amd64 -nsis
wails build -platform linux/amd64   # 请在 Linux 主机或 CI 中构建
```

### 安全说明

- API Key 仅保存在本机 `~/.codex-manager/`  
- 本地代理默认监听 `127.0.0.1`，不对外网开放  
- 请勿将 `providers.json` 提交到公开仓库  

### 已知限制

- Linux 二进制需在 Linux 环境构建；本仓库通过 CI/发布资源提供时可用  
- 界面 i18n 为中英；本说明额外提供日文、德文  
- 未经过公证的 macOS 应用可能需手动允许运行  

---

## English

### Overview

**AI Switch** is a desktop multi-provider AI model manager for **Codex** and **Claude Code** (Wails + Go). Manage vendors, model lists, local tool configs, and an OpenAI-compatible local proxy, with token packages and usage stats.

### Features

- **Providers**: name / API Base URL / API Key; connection test; fetch `/models`; enable/disable/default models  
- **Configs**: auto-scan Codex / Claude Code, pick files, backup/restore, one-click model switch  
- **Proxy**: local OpenAI-compatible gateway (default `http://127.0.0.1:18080/v1`), route by model ID with the matching key  
- **Token packages**: per-vendor quota plans; remaining estimate = offset + proxy-tracked usage  
- **Usage stats**: requests via the local proxy (by model / provider / day / recent)  
- **i18n**: Chinese / English UI; preference stored locally  
- **Platforms**: macOS / Windows (Linux: see downloads)

### System requirements

| Platform | Arch | Notes |
|----------|------|--------|
| macOS | Apple Silicon (arm64) | macOS 12+ recommended |
| macOS | Intel (amd64) | macOS 12+ recommended |
| Windows | x64 (amd64) | Windows 10/11 + WebView2 |
| Linux | x64 (amd64) | WebKit/GTK runtime required |

### Downloads

| Asset | Platform | Notes |
|-------|----------|--------|
| `AI-Switch-v1.0.0-macos-arm64.zip` | macOS Apple Silicon | Unzip → `AI Switch.app` |
| `AI-Switch-v1.0.0-macos-amd64.zip` | macOS Intel | Unzip → `AI Switch.app` |
| `AI-Switch-v1.0.0-windows-amd64-setup.exe` | Windows x64 | NSIS installer (recommended) |
| `AI-Switch-v1.0.0-windows-amd64-portable.zip` | Windows x64 | Portable executable |
| `AI-Switch-v1.0.0-linux-amd64.tar.gz` | Linux x64 | Unpack and run (when published) |

#### macOS

1. Download the zip for your chip and extract  
2. Move `AI Switch.app` to Applications  
3. If Gatekeeper blocks it: System Settings → Privacy & Security → Open Anyway  
4. Or: `xattr -cr "/Applications/AI Switch.app"`

#### Windows

1. Run the setup installer, or  
2. Unzip the portable package and run the `.exe`  
3. Install [WebView2 Runtime](https://developer.microsoft.com/microsoft-edge/webview2/) if needed  

#### Linux

1. Extract the tarball  
2. `chmod +x AI-Switch && ./AI-Switch`  
3. Install WebKitGTK / GTK packages for your distro  

### Quick start

1. **Providers** → add a vendor (presets available)  
2. Set Base URL + API Key → **Test** → **Fetch models**  
3. Enable **Via local proxy** and save when you want a unified OpenAI base URL  
4. **Configs** page or row actions write models into Codex / Claude Code  
5. Point Codex at the proxy base URL as shown in the Chinese section above  

### Data locations

Same paths under `~/.codex-manager/` as listed in the Chinese section.

### Build from source

```bash
export PATH="$PATH:$(go env GOPATH)/bin"
wails build
wails build -platform windows/amd64 -nsis
wails build -platform linux/amd64   # on Linux host or CI
```

### Security

- API keys stay on-device under `~/.codex-manager/`  
- Proxy binds to `127.0.0.1` by default  
- Do not commit `providers.json`  

### Known limitations

- Linux builds require a Linux environment (or CI)  
- App UI i18n is zh/en; these notes also include ja/de  
- Unsigned macOS builds may need manual approval  

---

## 日本語

### 概要

**AI Switch** は **Codex** / **Claude Code** 向けのマルチプロバイダー AI モデル管理デスクトップアプリ（Wails + Go）です。ベンダー設定、モデル一覧、ローカル設定ファイル、OpenAI 互換プロキシ、トークン枠と利用量を一括管理します。

### 主な機能

- **プロバイダー**：名称 / API Base URL / API Key、接続テスト、`/models` 取得、有効/無効/デフォルト  
- **設定ファイル**：Codex / Claude Code の自動検索・手動選択・バックアップ/復元・モデル切替  
- **プロキシ**：ローカル OpenAI 互換ゲートウェイ（既定 `http://127.0.0.1:18080/v1`）、model でルーティング  
- **トークンパッケージ**：ベンダーごとの枠管理（オフセット＋プロキシ計測で残量概算）  
- **利用統計**：プロキシ経由のリクエスト集計  
- **多言語 UI**：中国語 / 英語（本リリースノートは日・独も記載）  
- **対応 OS**：macOS / Windows（Linux はアセット参照）

### 必要環境

| OS | アーキテクチャ | 備考 |
|----|----------------|------|
| macOS | arm64 / amd64 | macOS 12 以降推奨 |
| Windows | amd64 | Windows 10/11 + WebView2 |
| Linux | amd64 | WebKit/GTK が必要 |

### ダウンロード

| ファイル | 用途 |
|----------|------|
| `AI-Switch-v1.0.0-macos-arm64.zip` | Apple Silicon 用 |
| `AI-Switch-v1.0.0-macos-amd64.zip` | Intel Mac 用 |
| `AI-Switch-v1.0.0-windows-amd64-setup.exe` | Windows インストーラ（推奨） |
| `AI-Switch-v1.0.0-windows-amd64-portable.zip` | Windows ポータブル |
| `AI-Switch-v1.0.0-linux-amd64.tar.gz` | Linux バイナリ（公開時） |

### インストール（要約）

- **macOS**：zip を展開 → `AI Switch.app` をアプリケーションへ。初回は「プライバシーとセキュリティ」で許可  
- **Windows**：setup を実行、または portable の exe を起動。必要なら WebView2 を導入  
- **Linux**：tar.gz を展開 → 実行権限を付与 → WebKitGTK 等を導入  

### 使い方（要約）

1. プロバイダーを追加し Key を設定 → 接続テスト → モデル取得  
2. 必要なら「ローカルプロキシ経由」を有効化して保存  
3. 設定ページまたはモデル行から Codex / Claude Code へ書き込み  
4. Codex の `base_url` をプロキシ（既定 `http://127.0.0.1:18080/v1`）に設定  

### データ保存場所

`~/.codex-manager/` 配下（`providers.json`、`proxy.json`、バックアップ等）。

### 注意

- API キーは端末内のみに保存  
- プロキシは既定で localhost のみ  
- 未公証の macOS アプリは手動許可が必要な場合があります  

---

## Deutsch

### Überblick

**AI Switch** ist eine Desktop-Anwendung zur Verwaltung mehrerer AI-Anbieter für **Codex** und **Claude Code** (Wails + Go). Sie verwaltet Anbieter, Modelllisten, lokale Konfigurationsdateien und einen OpenAI-kompatiblen lokalen Proxy sowie Token-Pakete und Nutzungsstatistiken.

### Funktionen

- **Anbieter**: Name / API Base URL / API Key; Verbindungstest; Abruf von `/models`; Aktivieren/Deaktivieren/Standardmodell  
- **Konfigurationen**: Auto-Suche für Codex / Claude Code, manuelle Auswahl, Backup/Wiederherstellung, Modellwechsel  
- **Proxy**: lokales OpenAI-kompatibles Gateway (Standard `http://127.0.0.1:18080/v1`), Routing nach Model-ID  
- **Token-Pakete**: Kontingente pro Anbieter; Rest = manueller Offset + Proxy-Nutzung  
- **Statistik**: Anfragen über den lokalen Proxy (nach Modell / Anbieter / Tag)  
- **UI-Sprachen**: Chinesisch / Englisch (diese Notes zusätzlich ja/de)  
- **Plattformen**: macOS / Windows (Linux siehe Downloads)

### Systemvoraussetzungen

| Plattform | Architektur | Hinweise |
|-----------|-------------|----------|
| macOS | arm64 / amd64 | macOS 12+ empfohlen |
| Windows | amd64 | Windows 10/11 + WebView2 |
| Linux | amd64 | WebKit/GTK erforderlich |

### Downloads

| Datei | Plattform |
|-------|-----------|
| `AI-Switch-v1.0.0-macos-arm64.zip` | macOS Apple Silicon |
| `AI-Switch-v1.0.0-macos-amd64.zip` | macOS Intel |
| `AI-Switch-v1.0.0-windows-amd64-setup.exe` | Windows-Installer (empfohlen) |
| `AI-Switch-v1.0.0-windows-amd64-portable.zip` | Windows portable |
| `AI-Switch-v1.0.0-linux-amd64.tar.gz` | Linux (falls veröffentlicht) |

### Installation (kurz)

- **macOS**: ZIP entpacken → App in Programme legen → ggf. unter Datenschutz & Sicherheit freigeben  
- **Windows**: Setup ausführen oder portable EXE starten; WebView2 bei Bedarf installieren  
- **Linux**: Archiv entpacken, ausführbar machen, WebKitGTK installieren  

### Schnellstart

1. Anbieter anlegen, Base URL + Key setzen → testen → Modelle laden  
2. Optional „über lokalen Proxy“ aktivieren und speichern  
3. Modelle in Codex / Claude Code schreiben  
4. Codex auf Proxy-URL `http://127.0.0.1:18080/v1` zeigen  

### Daten

Alles unter `~/.codex-manager/` (Windows: Benutzerprofil-Home analog).

### Sicherheit & Hinweise

- API-Keys nur lokal  
- Proxy standardmäßig nur auf `127.0.0.1`  
- Linux-Builds benötigen eine Linux-Umgebung bzw. CI  
- Nicht notarisiertes macOS-Build ggf. manuell erlauben  

---

## Checksums

Generate after packaging:

```bash
cd dist/release/v1.0.0
shasum -a 256 * > SHA256SUMS.txt
```

Verify:

```bash
shasum -a 256 -c SHA256SUMS.txt
```
