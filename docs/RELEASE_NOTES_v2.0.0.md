# AIGateway v2.0.0 Release Notes

| | |
|---|---|
| **Version** | 2.0.0 |
| **Tag** | `v2.0.0` |
| **Repository** | https://github.com/vincent1986/AIGateway |
| **Release** | https://github.com/vincent1986/AIGateway/releases/tag/v2.0.0 |
| **PRD** | [PRD_V2.md](./PRD_V2.md) · [验收清单](./PRD_V2_CHECKLIST.md) |

---

## 简体中文

### 这是什么

**AIGateway** 是本地 **AI 流量底座 / 模型管理网关**：把 ChatGPT（Codex）、Claude Code、OpenClaw、Harness 等工具的请求统一接入，按虚拟模型组做多厂家路由与 Failover，帮助 **省 Token、发现更便宜的服务商**。

### v2.0.0 相对 v1.0.0 的核心变化

1. **一次接管，终身调度**  
   工具配置只需把 `base_url` 指到本地网关（`http://127.0.0.1:18080/v1`），之后换模型 / 换厂家在网关内完成，无需反复改配置、重启终端。

2. **SQLite 数据底座**  
   主状态落在 `~/.codex-manager/aigateway.db`（厂家、模型组、路由、用量）。兼容迁移原有 `providers.json` / `usage.json`。

3. **虚拟模型组 + Failover**  
   同名模型可聚合多家供应商，支持拖拽优先级；上游 429 / 额度类错误时自动切换备用通道；全部失败返回标准 OpenAI 错误 JSON（`model_group_all_exhausted`）。

4. **五栏产品结构**  
   厂家管理 · 模型管理 · 应用管理 · 统一入口 · Token 统计。

5. **应用市场卡片**  
   ChatGPT / Claude Code / OpenClaw / Harness：一键接管、备份还原。

6. **厂家预设库**  
   点选 DeepSeek、硅基流动、Ollama 等，云厂家多数只需粘贴 API Key。

7. **OpenAI / Passthrough 格式**  
   按厂家选择报文规整或原样转发。

8. **多语言（弹出选择）**  
   简体中文、繁體中文、English、日本語、한국어、Deutsch、Tiếng Việt、ไทย。

9. **Codex 兼容修复**  
   - 避免覆盖内置 provider ID（`openai` → `openai-custom`，`ollama` → `ollama-local`）  
   - 本地 `aigateway` 使用内联 `api_key`，避免 `Missing environment variable: aigateway_api_key`

### 快速开始

1. 启动 AIGateway，确认 **统一入口** 运行（默认 `http://127.0.0.1:18080/v1`）。  
2. **厂家管理** → 添加预设 → 填 Key → 拉取模型。  
3. **模型管理** → 查看聚合组，拖拽调整 Failover 顺序。  
4. **应用管理** → ChatGPT 等 → **一键接管**。  
5. 客户端使用 `model_provider = "aigateway"`（或工具指向网关 Base URL），`model` 填模型组名。

### 数据位置

| 内容 | 路径 |
|------|------|
| SQLite 主库 | `~/.codex-manager/aigateway.db` |
| 厂家 JSON 镜像 | `~/.codex-manager/providers.json` |
| 网关配置 | `~/.codex-manager/proxy.json` |
| 工具备份 | `~/.codex-manager/backups/` |

### 已知限制

- 流式响应：HTTP 错误 / **首包**错误可切通道；**已向客户端吐流后**无法无感中途切换。  
- 全渠道耗尽时返回标准错误 JSON；模型组自动永久停用仍可增强。  
- 模型看板尚未展示「套餐金额 / 已应用工具」全字段。  
- `proxy.json` 仍为独立 JSON（业务状态以 SQLite 为主）。  
- 详见 [PRD_V2_CHECKLIST.md](./PRD_V2_CHECKLIST.md)（约 80%+ 对齐，持续迭代）。

### 升级说明

- 从 v1 升级：首次启动会迁移 JSON → SQLite。  
- 若 Codex 报保留 provider ID：将 `[model_providers.openai]` / `ollama` 改为 `openai-custom` / `ollama-local`。  
- 若报缺失 `aigateway_api_key`：在 provider 块使用 `api_key = "aigateway"`，不要只写未设置的 `env_key`。

---

## English

### Highlights

**AIGateway v2** is a local multi-provider **model gateway**: one-time tool takeover, SQLite-backed virtual model groups, priority failover, token usage, and an 8-language UI.

### What's new vs 1.0.0

- Gateway-first architecture (tools point once to `127.0.0.1:18080/v1`)  
- SQLite store + JSON migration  
- Virtual model groups, drag-and-drop priority, failover on 429/quota  
- Five-tab UI: Providers / Models / Apps / Gateway / Usage  
- App cards: ChatGPT, Claude Code, OpenClaw, Harness  
- Provider presets (key-only for most clouds)  
- OpenAI vs Passthrough format  
- Languages: zh-CN, zh-TW, en, ja, ko, de, vi, th  
- Codex compatibility fixes for reserved provider IDs and local gateway key  

### Known limitations

Streaming mid-response failover is limited; full PRD gap list in [PRD_V2_CHECKLIST.md](./PRD_V2_CHECKLIST.md).

---

## 日本語（要約）

ローカル AI ゲートウェイ。一度の設定で ChatGPT / Claude / OpenClaw / Harness を統合。SQLite・仮想モデルグループ・フェイルオーバー・8 言語 UI。詳細は英語 / 中国語セクション参照。

---

## 한국어（요약）

로컬 AI 게이트웨이. 도구 base_url을 한 번만 게이트웨이로 지정. SQLite, 가상 모델 그룹, Failover, 8개 언어 UI. 자세한 내용은 중/영 섹션 참고.

---

## Deutsch (Kurz)

Lokales AI-Gateway. Einmalige Tool-Übernahme, SQLite, virtuelle Modellgruppen, Failover, 8 UI-Sprachen. Details siehe EN/ZH.

---

## Tiếng Việt (tóm tắt)

Cổng AI cục bộ: kết nối một lần, SQLite, nhóm mô hình ảo, failover, giao diện 8 ngôn ngữ. Xem mục tiếng Trung/Anh để biết chi tiết.

---

## ไทย (สรุป)

เกตเวย์ AI ในเครื่อง ตั้งค่าครั้งเดียว SQLite กลุ่มโมเดลเสมือน failover และ UI 8 ภาษา รายละเอียดดูส่วนจีน/อังกฤษ

---

## Changelog (git)

Major commits on the v2 line (selected):

- SQLite store, model groups, gateway failover  
- Five-tab UI, model board, app takeover  
- Format standard, tool drivers, stream peek failover  
- Provider preset library  
- Drag-reorder routes + SQLite usage  
- PRD acceptance checklist  
- 8-language popup  
- Codex reserved IDs + aigateway inline api_key  

Full history: `git log v1.0.0..v2.0.0`
