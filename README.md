# AIGateway

[![Release](https://img.shields.io/github/v/release/vincent1986/AIGateway)](https://github.com/vincent1986/AIGateway/releases)
[![Platform](https://img.shields.io/badge/platform-macOS%20%7C%20Windows%20%7C%20Linux-blue)](https://github.com/vincent1986/AIGateway/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**English** · **中文** · **日本語** · **Deutsch** · **Tiếng Việt**

---

## English

**AIGateway** is a local **AI model manager and traffic gateway**. It unifies providers, models, and quotas used by popular tools—so you can **spend fewer tokens** and **switch to cheaper vendors** more easily.

### Works with

- **ChatGPT** / **Codex** and OpenAI-compatible clients  
- **Claude** / **Claude Code**  
- **OpenClaw**, **Harness**, **Grok CLI**, and similar toolchains  
- Any app that speaks **OpenAI-compatible APIs**

### What you get

| Goal | How |
|------|-----|
| **Save tokens** | Track usage by provider and model; avoid opaque burn |
| **Find cheaper vendors** | Add many APIs in one place (DeepSeek, SiliconFlow, Qwen, …) and switch anytime |
| **One setup, many tools** | Point tools once at the local gateway; route models inside AIGateway |
| **Hot model routing** | Virtual model groups with priority failover (e.g. 429 / quota) |

### Features

- **Providers / Models / Apps / Gateway / Usage** — clear five-tab workspace  
- **SQLite-backed gateway** — virtual model groups, priority failover, usage stats  
- **One-click takeover** — ChatGPT, Claude Code, OpenClaw, Harness, Grok CLI  
- **One-click OpenClaw launch** — start OpenClaw Gateway and open the Control UI  

- **Preset library** — pick a vendor; most cloud providers need only an API key  
- **OpenAI or Passthrough** format per provider  
- **Multi-language UI** (popup): Simplified Chinese, Traditional Chinese, English, Japanese, Korean, German, Vietnamese, Thai  
- **Cross-platform**: macOS, Windows, Linux  

[Download latest release](https://github.com/vincent1986/AIGateway/releases)

**Docs:** [FAQ](docs/wiki/FAQ.md) · [Getting Started](docs/wiki/Getting-Started.md) · [Troubleshooting](docs/wiki/Troubleshooting.md) · [Wiki](https://github.com/vincent1986/AIGateway/wiki)

---

## 中文

**AIGateway** 是本地 **AI 模型管理与流量网关**。把常见 AI 工具用到的厂家、模型与额度统一管理，帮你 **少花 Token、更容易换到更便宜的服务商**。

### 适用场景

- **ChatGPT** / **Codex** 与 OpenAI 兼容客户端  
- **Claude** / **Claude Code**  
- **OpenClaw**、**Harness**、**Grok CLI** 等工具链  
- 其它使用 **OpenAI 兼容 API** 的应用  

### 你能得到什么

| 目标 | 怎么做到 |
|------|----------|
| **省 Token** | 按厂家 / 模型统计用量，避免黑盒烧额度 |
| **找更便宜服务商** | 一处添加多家 API（DeepSeek、硅基流动、通义等），随时切换 |
| **一套配置管多工具** | 工具只指向本地网关一次，模型路由在 AIGateway 内完成 |
| **热切换与容灾** | 虚拟模型组 + 优先级 Failover（如 429 / 额度耗尽） |

### 功能概览

- **厂家 / 模型 / 应用 / 统一入口 / Token 统计** 五栏结构  
- **SQLite 网关**：虚拟模型组、优先级故障转移、用量统计  
- **一键接管**：ChatGPT、Claude Code、OpenClaw、Harness、Grok CLI  
- **一键启动 OpenClaw**：启动 OpenClaw Gateway 并打开控制台  

- **预设库**：点选厂家，云端多数只需 API Key  
- **OpenAI / 原样转发** 按厂家可选  
- **多语言界面（弹窗）**：简中、繁中、英、日、韩、德、越、泰  
- **跨平台**：macOS、Windows、Linux  

[下载最新版本](https://github.com/vincent1986/AIGateway/releases)

**文档：** [常见问题 FAQ](docs/wiki/FAQ.md) · [快速开始](docs/wiki/Getting-Started.md) · [故障排除](docs/wiki/Troubleshooting.md) · [Wiki](https://github.com/vincent1986/AIGateway/wiki)

---

## 日本語

**AIGateway** はローカルで動く **AI モデル管理・トラフィックゲートウェイ**です。よく使う AI ツールのプロバイダー・モデル・枠をまとめ、**トークンを節約**し、**より安いサービスへ切り替えやすく**します。

### 対応・連携

- **ChatGPT** / **Codex**、OpenAI 互換クライアント  
- **Claude** / **Claude Code**  
- **OpenClaw**・**Harness** など  
- **OpenAI 互換 API** を使うアプリ全般  

### できること

| 目的 | 方法 |
|------|------|
| **トークン節約** | プロバイダー / モデル別の使用量を可視化 |
| **安いベンダー探し** | DeepSeek・SiliconFlow・Qwen などを一括登録して切替 |
| **一度の設定で多ツール** | ツールはローカルゲートウェイを指すだけ |
| **ホットルーティング** | 仮想モデルグループと優先度付きフェイルオーバー |

### 機能

- **プロバイダー / モデル / アプリ / ゲートウェイ / 使用量** の 5 タブ  
- **SQLite ゲートウェイ**：仮想モデル、優先 Failover、統計  
- **ワンクリック接続**：ChatGPT、Claude Code、OpenClaw、Harness  
- **プリセット**：クラウドは API Key だけで追加できることが多い  
- **OpenAI / パススルー** 形式  
- **多言語 UI**（ポップアップ）  
- **macOS / Windows / Linux**  

[最新リリースをダウンロード](https://github.com/vincent1986/AIGateway/releases)

---

## Deutsch

**AIGateway** ist ein lokales **KI-Modellmanagement und Traffic-Gateway**. Es bündelt Anbieter, Modelle und Kontingente gängiger Tools—damit Sie **weniger Tokens verbrauchen** und **günstigere Anbieter** leichter nutzen können.

### Geeignet für

- **ChatGPT** / **Codex** und OpenAI-kompatible Clients  
- **Claude** / **Claude Code**  
- **OpenClaw**, **Harness** und ähnliche Toolchains  
- Apps mit **OpenAI-kompatibler API**  

### Nutzen

| Ziel | Umsetzung |
|------|-----------|
| **Tokens sparen** | Nutzung nach Anbieter/Modell sichtbar machen |
| **Günstigere Anbieter** | Viele APIs an einem Ort (DeepSeek, SiliconFlow, Qwen, …) |
| **Einmal einrichten** | Tools zeigen nur auf das lokale Gateway |
| **Hot Routing** | Virtuelle Modellgruppen mit Prioritäts-Failover |

### Funktionen

- Fünf Bereiche: **Anbieter / Modelle / Apps / Gateway / Nutzung**  
- **SQLite-Gateway** mit Failover und Statistiken  
- **Ein-Klick-Übernahme** für ChatGPT, Claude Code, OpenClaw, Harness  
- **Presets** — oft nur API-Key nötig  
- **OpenAI- oder Passthrough-Format**  
- **Mehrsprachige UI** (Popup)  
- **macOS, Windows, Linux**  

[Neueste Version herunterladen](https://github.com/vincent1986/AIGateway/releases)

---

## Tiếng Việt

**AIGateway** là **cổng quản lý mô hình AI và lưu lượng cục bộ**. Gom nhà cung cấp, mô hình và hạn mức cho các công cụ phổ biến—giúp **tiết kiệm token** và **chuyển sang dịch vụ rẻ hơn** dễ dàng hơn.

### Phù hợp với

- **ChatGPT** / **Codex** và client tương thích OpenAI  
- **Claude** / **Claude Code**  
- **OpenClaw**, **Harness**  
- Ứng dụng dùng **API tương thích OpenAI**  

### Lợi ích

| Mục tiêu | Cách làm |
|----------|----------|
| **Tiết kiệm token** | Theo dõi dùng theo nhà cung cấp / mô hình |
| **Tìm dịch vụ rẻ hơn** | Thêm nhiều API (DeepSeek, SiliconFlow, Qwen, …) và chuyển nhanh |
| **Cấu hình một lần** | Công cụ chỉ trỏ về gateway cục bộ |
| **Định tuyến linh hoạt** | Nhóm mô hình ảo + failover theo ưu tiên |

### Tính năng

- Năm tab: **Nhà cung cấp / Mô hình / Ứng dụng / Gateway / Thống kê**  
- **Gateway SQLite**, failover, thống kê token  
- **Kết nối một chạm**: ChatGPT, Claude Code, OpenClaw, Harness  
- **Thư viện preset** — cloud thường chỉ cần API Key  
- **Định dạng OpenAI hoặc Passthrough**  
- **Giao diện đa ngôn ngữ** (popup)  
- **macOS, Windows, Linux**  

[Tải bản phát hành mới nhất](https://github.com/vincent1986/AIGateway/releases)

---

## Team

**Mars Waller**

## License

[MIT](LICENSE) © 2026 Mars Waller
