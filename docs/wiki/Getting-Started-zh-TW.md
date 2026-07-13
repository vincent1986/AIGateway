# 快速開始

**語言：** [EN](Getting-Started-en) · [中文](Getting-Started-zh-CN) · [日本語](Getting-Started-ja) · [Deutsch](Getting-Started-de) · [Tiếng Việt](Getting-Started-vi) · [繁體中文](Getting-Started-zh-TW) · [Wiki 首頁](Home)

## 5 步上手

1. **安裝並啟動 AIGateway**  
   從 [Releases](https://github.com/vincent1986/AIGateway/releases) 下載對應平台安裝包並啟動。

2. **確認統一入口在跑**  
   預設閘道位址：`http://127.0.0.1:18080/v1`  
   在應用內開啟 **統一入口**，確認服務為執行狀態。

3. **新增廠商**  
   **廠商管理** → 選預設（DeepSeek、SiliconFlow、Ollama、通義等）→ 貼上 API Key → 拉取模型。

4. **設定模型組**  
   **模型管理** → 檢視虛擬模型組 → 拖曳調整 Failover 優先順序（主通道在上，備援在下）。

5. **一鍵接管工具**  
   **應用管理** → 選擇 ChatGPT / Claude Code / OpenClaw / Harness → **一鍵接管**。  
   之後換模型、換廠商只在 AIGateway 內操作，無需反覆改工具設定、重啟終端。

## 用戶端怎麼用

| 情境 | 建議 |
|------|------|
| 已一鍵接管 | 工具已指向本機閘道；`model` 填 **模型組名**（如 `deepseek-chat`） |
| 手動設定 | `base_url` = `http://127.0.0.1:18080/v1`，API Key 可用 `aigateway` |
| Codex / ChatGPT | `model_provider = "aigateway"`，model 填模型組名 |

## 架構一覽

```
工具 (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway（路由 / Failover / Token 統計）
        │
        ▼
上游廠商 (DeepSeek / SiliconFlow / Ollama / …)
```

## 下一步

- [常見問題 FAQ](FAQ-zh-TW)
- [資料與路徑](Data-Paths-zh-TW)
- [故障排除](Troubleshooting-zh-TW)
