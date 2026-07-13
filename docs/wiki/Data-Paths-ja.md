# データとパス

**言語：** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [ホーム](Home)

AIGateway はユーザーホームの `.codex-manager` に状態を保存します（クロスプラットフォーム）。

## 主なパス

| 内容 | パス |
|------|------|
| SQLite（プロバイダー、モデルグループ、ルート、使用量） | `~/.codex-manager/aigateway.db` |
| プロバイダー JSON ミラー | `~/.codex-manager/providers.json` |
| ゲートウェイ設定 | `~/.codex-manager/proxy.json` |
| ツール設定のバックアップ | `~/.codex-manager/backups/` |
| 環境変数関連 | `~/.codex-manager/env/` |

> Windows では `~` はユーザープロファイル（例：`C:\Users\<あなた>`）です。

## v1 からのアップグレード

- v2 初回起動時に旧 `providers.json` / `usage.json` を SQLite へ移行します。
- 業務状態の正は **SQLite**；一部の待受設定は `proxy.json` に残る場合があります。

## バックアップ

- アップグレードや再インストール前に `~/.codex-manager/` 全体をバックアップしてください。
- ワンクリック接続前にツール設定のスナップショットが取られ、カードからロールバックできます。

関連：[はじめに](Getting-Started-ja) · [トラブルシューティング](Troubleshooting-ja)
