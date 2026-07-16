# AIGateway v2.0.4 Release Notes

## Highlights

- Added Grok CLI application support for the official `xai-org/grok-build` CLI/TUI.
- Application management can now discover, take over, restore, and switch the Grok CLI model through AIGateway.
- Model shortcuts in provider details now target the registered application list, including ChatGPT, Claude Code, OpenClaw, Harness, and Grok CLI.

## Grok CLI Integration

The Grok CLI adapter writes the official Grok Build TOML model schema:

- `[model."aiSwitchModel-grok"]`
- `[models]`
- `default = "aiSwitchModel-grok"`
- `model`
- `base_url`
- `name`
- `env_key`
- optional inline `api_key` fallback

Default config search paths include:

- `GROK_CONFIG_DIR/config.toml`
- `~/.grok/config.toml`
- `~/.config/grok/config.toml`
- `~/.xai/grok/config.toml`
- `.grok/config.toml`
- macOS: `~/Library/Application Support/Grok/config.toml`

After one-click takeover, Grok CLI uses the stable alias `aiSwitchModel-grok`; AIGateway routes that alias to the selected model group.
Use `grok inspect` to verify the generated config, then run `grok -m aiSwitchModel-grok "..."` or `/model aiSwitchModel-grok` in the TUI.
