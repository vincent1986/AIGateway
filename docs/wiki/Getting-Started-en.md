# Getting Started

**Languages:** [EN](Getting-Started-en) · [中文](Getting-Started-zh-CN) · [日本語](Getting-Started-ja) · [Deutsch](Getting-Started-de) · [Tiếng Việt](Getting-Started-vi) · [繁體中文](Getting-Started-zh-TW) · [Wiki Home](Home)

## 5 steps

1. **Install and launch AIGateway**  
   Download the package for your platform from [Releases](https://github.com/vincent1986/AIGateway/releases) and start the app.

2. **Confirm the gateway is running**  
   Default address: `http://127.0.0.1:18080/v1`  
   Open **Gateway** in the app and verify the service is running.

3. **Add providers**  
   **Providers** → pick a preset (DeepSeek, SiliconFlow, Ollama, Qwen, …) → paste API key → fetch models.

4. **Configure model groups**  
   **Models** → review virtual model groups → drag to set failover priority (primary on top, backups below).

5. **One-click tool takeover**  
   **Apps** → select ChatGPT / Claude Code / OpenClaw / Harness → **Take over**.  
   After that, switch models/providers only inside AIGateway—no repeated tool config edits or terminal restarts.

## Client setup

| Scenario | Recommendation |
|----------|----------------|
| After one-click takeover | Tools already point at the local gateway; set `model` to a **model group name** (e.g. `deepseek-chat`) |
| Manual config | `base_url` = `http://127.0.0.1:18080/v1`, API key may be `aigateway` |
| Codex / ChatGPT | `model_provider = "aigateway"`, model = group name |

## Architecture

```
Tools (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway (routing / failover / token stats)
        │
        ▼
Upstream providers (DeepSeek / SiliconFlow / Ollama / …)
```

## Next

- [FAQ](FAQ-en)
- [Data Paths](Data-Paths-en)
- [Troubleshooting](Troubleshooting-en)
