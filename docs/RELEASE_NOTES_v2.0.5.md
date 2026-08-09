# AIGateway v2.0.5 Release Notes

## Highlights

- Added **one-click OpenClaw launch** on the Apps card: start the local OpenClaw Gateway and open the Control UI.
- Tightened “managed” detection so local Ollama / other `127.0.0.1` services are no longer mistaken for AIGateway takeover.
- Provider model rows now use a compact **Apply to…** menu for ChatGPT, Claude Code, OpenClaw, Harness, and Grok CLI.
- Added Grok CLI rollback coverage in takeover artifact cleanup tests.

## OpenClaw Launch

From **Apps → OpenClaw → 一键启动 OpenClaw**:

1. Ensures the AIGateway local proxy is running.
2. Resolves the `openclaw` CLI (`PATH`, Homebrew, npm, nvm/fnm/volta common paths).
3. Writes `gateway.mode=local` only when the key is missing (does **not** overwrite `remote`).
4. Prefers `openclaw gateway start`, then falls back to a detached `openclaw gateway run`.
5. Opens `http://127.0.0.1:<port>/` (default `18789`) when the port becomes ready.

If the CLI is missing, the UI shows the install hint:
`npm install -g openclaw@latest && openclaw onboard --install-daemon`.

## Managed-state Fix

Claude / OpenClaw / Harness / Grok `IsManaged` now keys off AIGateway markers (`aigateway`, `aiSwitchModel`, `AIGateway`) instead of any localhost URL, avoiding false positives on Ollama and similar local APIs.
