# はじめに

**言語：** [EN](Getting-Started-en) · [中文](Getting-Started-zh-CN) · [日本語](Getting-Started-ja) · [Deutsch](Getting-Started-de) · [Tiếng Việt](Getting-Started-vi) · [繁體中文](Getting-Started-zh-TW) · [Wiki ホーム](Home)

## 5 ステップ

1. **AIGateway をインストールして起動**  
   [Releases](https://github.com/vincent1986/AIGateway/releases) から対応パッケージを入手して起動します。

2. **ゲートウェイが動作していることを確認**  
   既定アドレス：`http://127.0.0.1:18080/v1`  
   アプリの **ゲートウェイ / 統一入口** で稼働状態を確認します。

3. **プロバイダーを追加**  
   **プロバイダー** → プリセット（DeepSeek、SiliconFlow、Ollama、Qwen など）→ API Key を貼付 → モデル取得。

4. **モデルグループを設定**  
   **モデル** → 仮想モデルグループを確認 → ドラッグで Failover 優先度を調整（上が主、下が予備）。

5. **ツールをワンクリック接続**  
   **アプリ** → ChatGPT / Claude Code / OpenClaw / Harness を選択 → **ワンクリック接続**。  
   以降のモデル／プロバイダー切替は AIGateway 内だけで完結します。

## クライアント設定

| シナリオ | 推奨 |
|----------|------|
| ワンクリック接続後 | ツールはローカルゲートウェイ向き；`model` に **モデルグループ名** |
| 手動設定 | `base_url` = `http://127.0.0.1:18080/v1`、API Key は `aigateway` 可 |
| Codex / ChatGPT | `model_provider = "aigateway"`、model はグループ名 |

## アーキテクチャ

```
ツール (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway（ルーティング / Failover / 使用量）
        │
        ▼
上流プロバイダー (DeepSeek / SiliconFlow / Ollama / …)
```

## 次のステップ

- [FAQ](FAQ-ja)
- [データとパス](Data-Paths-ja)
- [トラブルシューティング](Troubleshooting-ja)
