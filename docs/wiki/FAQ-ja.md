# FAQ

**言語：** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [ホーム](Home)

よくある質問。[トラブルシューティング](Troubleshooting-ja)・[はじめに](Getting-Started-ja) も参照してください。

---

## 1. 製品

### AIGateway とは？

ローカルの **AI モデル管理 + トラフィックゲートウェイ**。複数プロバイダーの LLM API と ChatGPT/Codex、Claude Code、OpenClaw、Harness などをつなぎます。

- **トークン節約** — プロバイダー / モデル別使用量  
- **安いベンダー** — 複数 API を一括管理  
- **一度の設定で多ツール** — ツールはローカルゲートウェイだけを指す  
- **ホットルーティング** — 仮想モデルグループ + 優先 Failover（429 / 枠など）

### 各ツール設定を直接書くのと何が違う？

V2 は **一度接続、以降はゲートウェイで调度**：`base_url` は一度だけ。モデル／プロバイダー切替は AIGateway 内。

### データは外部に出る？

AIGateway はローカル実行。設定した上流 API へリクエストを転送。設定・使用量は既定で `~/.codex-manager/`。API Key は自己管理。

---

## 2. インストール

### 対応 OS

macOS（Apple Silicon / Intel）、Windows、Linux。[Releases](https://github.com/vincent1986/AIGateway/releases)

### Docker / Node は必要？

配布デスクトップ版は不要。ソースビルド時は Go とフロントエンド環境が必要。

---

## 3. ゲートウェイ

### 既定 URL

```
http://127.0.0.1:18080/v1
```

### ツールの向け方

**アプリ → ワンクリック接続** を推奨。手動時：

- Base URL：`http://127.0.0.1:18080/v1`  
- API Key：`aigateway` で可  
- `model`：**モデルグループ名**

### AIGateway は常時起動？

はい。終了するとローカルプロキシは使えません。

---

## 4. プロバイダー

### 追加の流れ

**プロバイダー** → プリセット → API Key → モデル取得。クラウドは Key だけで足りることが多い。

### OpenAI 形式とパススルー

| モード | 内容 |
|--------|------|
| **標準 OpenAI** | リクエスト/レスポンスを整える |
| **パススルー** | body をできるだけ透過 |

迷ったらまず標準 OpenAI。

---

## 5. モデルと Failover

### 仮想モデルグループ

複数プロバイダーの同等モデルを 1 グループに。クライアントは 1 つのモデル名だけ指定。

### Failover のタイミング

429、枠、401 系など。全滅時は `model_group_all_exhausted`。

### ストリーム途中の切替

先頭バイト／HTTP 失敗は可。**ストリーム開始後**は制限あり → [トラブルシューティング](Troubleshooting-ja)

---

## 6. アプリ接続

対応ワンクリック：**ChatGPT (Codex)**、**Claude Code**、**OpenClaw**、**Harness**。

接続時：設定をバックアップし `base_url` 等をローカルゲートウェイへ。復元はカードの解除/復元または `~/.codex-manager/backups/`。

Codex の予約 ID / `aigateway_api_key` は [トラブルシューティング](Troubleshooting-ja)。

---

## 7. トークンとデータ

**使用量** タブ、SQLite `aigateway.db`。パスは [データとパス](Data-Paths-ja)。API Key は GitHub に上がりません。

---

## 8. UI 言語

ポップアップ：簡体中文、繁體中文、English、日本語、한국어、Deutsch、Tiếng Việt、ไทย。

5 タブ：プロバイダー / モデル / アプリ / ゲートウェイ / 使用量。

---

## 9. アップグレード

v2 初回：JSON → SQLite 移行。ゲートウェイと接続状態を再確認。

---

## 10. フィードバック

Issue：https://github.com/vincent1986/AIGateway/issues/new/choose  
ライセンス：[MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller

| 項目 | 値 |
|------|-----|
| 既定 Base URL | `http://127.0.0.1:18080/v1` |
| ローカル API Key | `aigateway` |
| データ | `~/.codex-manager/` |
| 全滅エラー | `model_group_all_exhausted` |
