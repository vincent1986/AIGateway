# トラブルシューティング

**言語：** [EN](Troubleshooting-en) · [中文](Troubleshooting-zh-CN) · [日本語](Troubleshooting-ja) · [Deutsch](Troubleshooting-de) · [Tiếng Việt](Troubleshooting-vi) · [繁體中文](Troubleshooting-zh-TW) · [ホーム](Home)

## ゲートウェイに接続できない

1. AIGateway が起動し、**ゲートウェイ**が稼働中であることを確認。  
2. `http://127.0.0.1:18080/v1` にアクセス（ポート競合を確認）。  
3. ツールが古い Base URL のままなら **ワンクリック接続** を再実行。  
4. ファイアウォール / プロキシが `127.0.0.1` を遮断していないか確認。

## Codex: Missing environment variable: aigateway_api_key

プロバイダー設定では **インライン** `api_key` を使い、未設定の `env_key` だけに頼らない：

```toml
api_key = "aigateway"
```

未定義の環境変数 `aigateway_api_key` に依存しないでください。

## Codex: 予約済み provider ID

組み込み予約 ID `openai` / `ollama` をカスタム名に使わないでください。例：

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## model_group_all_exhausted

その **仮想モデルグループ** の全プロバイダーが失敗または枠切れ（429 / クォータが多い）。

対処：

1. API Key・残高・レート制限を確認。  
2. **モデル** で予備チャネルを追加し優先度を調整。  
3. プロバイダーとモデルが **有効** か確認。

## ストリーム途中で切り替えない

HTTP / **先頭バイト** の失敗は Failover 可。**クライアントへのストリーム開始後** は途中切替できない場合があります。  
先頭成功後に上流が落ちた場合はクライアントから再試行してください。

## 接続後も公式 API に行く

1. 「接続済み」状態か確認。  
2. CLI / IDE を再起動して設定を再読込。  
3. OpenClaw は `models.providers.aigateway` を使う。  
4. **解除/復元** してから再接続。

## 使用量統計がおかしい

- SQLite の統計を正とする。  
- 上流が usage を返さない場合、0 や不完全になることがあります。

## 解決しない場合

1. [FAQ](FAQ-ja)  
2. [最新 Release](https://github.com/vincent1986/AIGateway/releases) へ更新  
3. [Issue を作成](https://github.com/vincent1986/AIGateway/issues/new/choose)（OS・版・再現手順；**API Key は伏せる**）
