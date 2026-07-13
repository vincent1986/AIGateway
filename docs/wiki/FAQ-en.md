# FAQ

**Languages:** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [Home](Home)

High-frequency questions for AIGateway users. Also see [Troubleshooting](Troubleshooting-en) and [Getting Started](Getting-Started-en).

---

## 1. Product

### What is AIGateway?

A local **AI model manager + traffic gateway**. It connects multi-provider LLM APIs with tools such as ChatGPT/Codex, Claude Code, OpenClaw, and Harness:

- **Save tokens** — usage by provider / model  
- **Cheaper vendors** — manage many APIs in one place  
- **One setup, many tools** — tools only point at the local gateway  
- **Hot routing & resilience** — virtual model groups + priority failover (e.g. 429 / quota)

### How is this different from editing each tool’s config?

V2 is **take over once, route forever**: tools’ `base_url` points at the gateway once; switch models/providers inside AIGateway—less file editing, restarts, and env conflicts.

### Does data leave my machine?

AIGateway runs locally. Requests go to the upstream APIs you configure. Config and usage default to `~/.codex-manager/`. Protect your API keys.

---

## 2. Install & platforms

### Supported OS?

macOS (Apple Silicon / Intel), Windows, Linux. Packages: [Releases](https://github.com/vincent1986/AIGateway/releases).

### Do I need Docker / Node?

Desktop releases: no. Building from source needs Go and a frontend toolchain.

---

## 3. Gateway

### Default gateway URL?

```
http://127.0.0.1:18080/v1
```

Check status under **Gateway**.

### How should tools point here?

Prefer **Apps → one-click takeover**. Manual:

- Base URL: `http://127.0.0.1:18080/v1`  
- API key: placeholder `aigateway` is fine (local gateway)  
- `model`: **model group name** (same as in Models)

### Must AIGateway stay open?

Yes. Traffic hits the local proxy first; quitting AIGateway stops the gateway.

---

## 4. Providers

### Quick add?

**Providers** → **preset** (DeepSeek, SiliconFlow, Ollama, OpenAI, Qwen, …) → paste key → fetch/enable models. Most cloud vendors only need a key.

### OpenAI format vs Passthrough?

| Mode | Meaning | When |
|------|---------|------|
| **Standard OpenAI** | Normalize request/response | Compatible cloud APIs, unified shape |
| **Passthrough** | Forward body with minimal rewrite | Special protocols / debugging |

Prefer **Standard OpenAI** first; try Passthrough if something breaks.

### Model toggles on a provider?

Control whether models participate in routing. Disabled models are not failover candidates.

---

## 5. Models & failover

### Virtual model group?

Group same/equivalent models across providers. Clients use one model name; the gateway picks a channel by priority.

### When does failover run?

Upstream errors such as **429**, **quota**, **account issues (e.g. 401)**. If all fail → `model_group_all_exhausted` (OpenAI-style JSON).

### Mid-stream switch?

Limited: first-byte / HTTP errors can switch; **after streaming starts**, seamless mid-stream switch is not available. See [Troubleshooting](Troubleshooting-en).

### Change primary/backup order?

In **Models**, drag providers in a group—higher = preferred.

---

## 6. App takeover

### Supported one-click tools?

**ChatGPT (Codex)**, **Claude Code**, **OpenClaw**, **Harness**. Other OpenAI-compatible clients: set base URL manually.

### What does takeover do?

Locate tool config → **backup** → point `base_url` (and needed provider fields) at the local gateway.

### Restore original config?

**Uninstall / restore** on the app card, or use `~/.codex-manager/backups/`.

### OpenClaw notes?

Prefer `models.providers.aigateway`. Wrong root-only fields may skip the gateway. One-click writes the recommended structure.

### Codex reserved IDs / missing aigateway_api_key?

See [Troubleshooting](Troubleshooting-en): rename reserved `openai`/`ollama`; use inline `api_key = "aigateway"`.

---

## 7. Tokens & data

### Where is usage?

**Usage** tab; stored in SQLite (`aigateway.db`). Paths: [Data Paths](Data-Paths-en).

### Are API keys uploaded to GitHub?

No. Keys stay local. Redact keys when filing issues.

---

## 8. UI languages

Popup languages: Simplified Chinese, Traditional Chinese, English, Japanese, Korean, German, Vietnamese, Thai.

### Five main tabs

1. **Providers** — channels & keys  
2. **Models** — virtual groups & failover  
3. **Apps** — tool takeover  
4. **Gateway** — status  
5. **Usage** — tokens  

---

## 9. Upgrade

- First v2 start: JSON → SQLite migration  
- Re-check gateway + takeover status  
- Codex ID / api_key rules as above  

Known limits: mid-stream failover; some UI fields still evolving—see `docs/PRD_V2_CHECKLIST.md` and release notes.

---

## 10. Feedback

- Issues: https://github.com/vincent1986/AIGateway/issues/new/choose  
- License: [MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller  

### Quick reference

| Item | Value |
|------|--------|
| Default base URL | `http://127.0.0.1:18080/v1` |
| Local API key placeholder | `aigateway` |
| Data dir | `~/.codex-manager/` |
| Main DB | `aigateway.db` |
| All-channels-exhausted code | `model_group_all_exhausted` |
