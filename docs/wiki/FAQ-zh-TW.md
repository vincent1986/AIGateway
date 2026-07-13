# 常見問題 FAQ

**語言：** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [首頁](Home)

面向 AIGateway 使用者的高頻問答。更多排錯見 [故障排除](Troubleshooting-zh-TW)；上手見 [快速開始](Getting-Started-zh-TW)。

---

## 1. 產品與定位

### AIGateway 是什麼？

本機 **AI 模型管理 + 流量閘道**。把多家大模型 API 與常見 AI 工具（ChatGPT/Codex、Claude Code、OpenClaw、Harness 等）接在一起：

- **省 Token**：按廠商 / 模型看用量  
- **換便宜服務商**：多廠商一處管理，隨時切換  
- **一次設定，多工具共用**：工具只指向本機閘道  
- **熱路由與容災**：虛擬模型組 + 優先順序 Failover（如 429 / 額度耗盡）

### 和直接改各工具設定有什麼差別？

V2 採用 **一次接管、終身調度**：工具的 `base_url` 只改一次指向閘道；之後換模型 / 換廠商在閘道內完成。

### 資料會離開本機嗎？

AIGateway 在本機執行。請求依你設定的廠商轉發到對應上游 API；設定與用量預設保存在 `~/.codex-manager/`。請自行保護 API Key。

---

## 2. 安裝與平台

### 支援哪些系統？

macOS（Apple Silicon / Intel）、Windows、Linux。安裝包見 [Releases](https://github.com/vincent1986/AIGateway/releases)。

### 需要單獨安裝 Docker / Node 嗎？

一般使用者使用發行版桌面應用即可。從原始碼建置才需要 Go、前端建置環境等。

---

## 3. 統一入口 / 閘道

### 預設閘道位址？

```
http://127.0.0.1:18080/v1
```

### 工具應如何指向閘道？

建議在 **應用管理** 使用 **一鍵接管**。手動設定時：

- Base URL：`http://127.0.0.1:18080/v1`  
- API Key：可用佔位值 `aigateway`  
- `model`：填 **模型組名稱**

### 必須始終開著 AIGateway 嗎？

是。退出後本機代理不可用。

---

## 4. 廠商管理

### 如何快速新增廠商？

**廠商管理** → 選 **預設** → 貼上 API Key → 拉取 / 啟用模型。多數雲廠商只需 Key。

### 「標準 OpenAI」和「原樣轉發 (Passthrough)」怎麼選？

| 模式 | 含義 |
|------|------|
| **標準 OpenAI** | 閘道規整請求/回應 |
| **原樣轉發** | 盡量透傳 body |

不確定時優先試標準 OpenAI。

---

## 5. 模型管理與 Failover

### 什麼是虛擬模型組？

把不同廠商上的同名/等價模型聚合成一組。用戶端只寫一個模型名，閘道按優先順序選通道。

### Failover 何時觸發？

429 限流、額度/配額、帳戶異常（如 401）等。全部失敗則 `model_group_all_exhausted`。

### 串流中途會不會切換廠商？

有限制：首包 / HTTP 錯誤可切換；**已開始向用戶端推流後**無法無感中途換通道。見 [故障排除](Troubleshooting-zh-TW)。

---

## 6. 應用接管

### 支援哪些工具一鍵接管？

**ChatGPT（Codex）**、**Claude Code**、**OpenClaw**、**Harness**。

### 一鍵接管會做什麼？

定位工具設定檔 → **備份** → 將 `base_url`（及必要 provider 欄位）指向本機閘道。

### 如何還原？

卡片內 **一鍵解除 / 還原**，或 `~/.codex-manager/backups/`。

Codex 保留 ID / `aigateway_api_key`：見 [故障排除](Troubleshooting-zh-TW)。

---

## 7. Token 與資料

**Token 統計** 頁；SQLite `aigateway.db`。路徑見 [資料與路徑](Data-Paths-zh-TW)。API Key 不會上傳到 GitHub。

---

## 8. 多語言與介面

彈窗：簡體中文、繁體中文、English、日本語、한국어、Deutsch、Tiếng Việt、ไทย。

五個主功能區：廠商管理 / 模型管理 / 應用管理 / 統一入口 / Token 統計。

---

## 9. 升級

v1 → v2：首次啟動 JSON → SQLite 遷移。請重新確認統一入口與接管狀態。

---

## 10. 回饋

Issue：https://github.com/vincent1986/AIGateway/issues/new/choose  
授權：[MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller

| 項目 | 值 |
|------|------|
| 預設 Base URL | `http://127.0.0.1:18080/v1` |
| 本機 API Key 佔位 | `aigateway` |
| 資料目錄 | `~/.codex-manager/` |
| 全通道耗盡錯誤碼 | `model_group_all_exhausted` |
