# 故障排除

**語言：** [EN](Troubleshooting-en) · [中文](Troubleshooting-zh-CN) · [日本語](Troubleshooting-ja) · [Deutsch](Troubleshooting-de) · [Tiếng Việt](Troubleshooting-vi) · [繁體中文](Troubleshooting-zh-TW) · [首頁](Home)

## 閘道連不上

1. 確認 AIGateway 已啟動，**統一入口**顯示執行中。  
2. 瀏覽器或 curl 存取：`http://127.0.0.1:18080/v1`（埠是否被占用）。  
3. 工具是否仍指向舊 Base URL；重新執行 **一鍵接管**。  
4. 本機防火牆 / 代理是否攔截 `127.0.0.1`。

## Codex：Missing environment variable: aigateway_api_key

在 provider 設定中使用 **內聯** `api_key`，不要只寫未設定的 `env_key`：

```toml
api_key = "aigateway"
```

不要依賴未定義的環境變數 `aigateway_api_key`。

## Codex：保留 provider ID 衝突

不要使用內建保留 ID `openai` / `ollama` 作為自訂 provider 名。應改為例如：

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## 回傳 model_group_all_exhausted

表示該 **虛擬模型組** 下所有廠商通道都失敗或額度耗盡（常見 429 / 配額）。

處理：

1. 檢查廠商 API Key、餘額與限流。  
2. 在 **模型管理** 增加備援通道並調整優先順序。  
3. 確認廠商與模型均已 **啟用**。

## 串流請求中途不切換通道

目前限制：HTTP / **首包**錯誤可 Failover；**已向用戶端推流後**無法無感中途切換。  
若首包成功後上游中斷，需在用戶端重試。

## 一鍵接管後工具仍走官方

1. 確認接管狀態為「已接管」。  
2. 重啟對應 CLI / IDE 工作階段使設定生效。  
3. OpenClaw：應使用 `models.providers.aigateway`。  
4. 用 **一鍵解除/還原** 後再重新接管。

## 用量統計不準

- 以 SQLite 中的統計為準。  
- 部分上游不回傳 usage 時，計數可能為 0 或不完整。

## 仍無法解決

1. 查看 [FAQ](FAQ-zh-TW)  
2. 升級到 [最新 Release](https://github.com/vincent1986/AIGateway/releases)  
3. [提交 Issue](https://github.com/vincent1986/AIGateway/issues/new/choose)（附系統、版本、重現步驟；**打碼 API Key**）
