# Troubleshooting

**Languages:** [EN](Troubleshooting-en) · [中文](Troubleshooting-zh-CN) · [日本語](Troubleshooting-ja) · [Deutsch](Troubleshooting-de) · [Tiếng Việt](Troubleshooting-vi) · [繁體中文](Troubleshooting-zh-TW) · [Home](Home)

## Cannot reach the gateway

1. Confirm AIGateway is running and **Gateway** shows active.  
2. Open or curl `http://127.0.0.1:18080/v1` (check port conflicts).  
3. Tools may still use an old base URL — run **one-click takeover** again.  
4. Check firewall / system proxy blocking `127.0.0.1`.

## Codex: Missing environment variable: aigateway_api_key

Use an **inline** `api_key` in the provider config; do not rely on an unset `env_key`:

```toml
api_key = "aigateway"
```

Do not depend on an undefined `aigateway_api_key` environment variable.

## Codex: reserved provider ID conflict

Do not use built-in reserved IDs `openai` / `ollama` as custom provider names. Prefer e.g.:

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## Error: model_group_all_exhausted

All provider channels in that **virtual model group** failed or hit quota (often 429 / quota).

What to do:

1. Check API keys, balance, and rate limits.  
2. In **Models**, add backup channels and adjust priority.  
3. Ensure providers and models are **enabled**.

Error body is standard OpenAI-style JSON:

```json
{
  "error": {
    "message": "AIGateway Error: All provider backups for this model group have been exhausted or rate-limited.",
    "type": "insufficient_quota",
    "param": "model_group",
    "code": "model_group_all_exhausted"
  }
}
```

## Stream does not fail over mid-response

Current limit: HTTP / **first-byte** errors can fail over; **after streaming to the client has started**, mid-stream switch is not seamless.  
If the upstream dies after a successful first byte, retry from the client (routing runs again by priority).

## Tool still hits the official API after takeover

1. Confirm status is “taken over”.  
2. Restart the CLI / IDE session so config reloads.  
3. OpenClaw: use `models.providers.aigateway`; do not only set wrong root `baseUrl` / `OPENAI_BASE_URL`.  
4. Use **uninstall/restore**, then take over again.

## Usage stats look wrong

- Prefer SQLite-backed stats; back up and restart if corrupted.  
- Some upstreams omit usage fields → counts may be 0 or incomplete.

## Still stuck?

1. Read [FAQ](FAQ-en)  
2. Upgrade to the [latest Release](https://github.com/vincent1986/AIGateway/releases)  
3. [Open an Issue](https://github.com/vincent1986/AIGateway/issues/new/choose) (OS, version, steps; **redact API keys**)
