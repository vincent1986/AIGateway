# 快速开始 / Getting Started

## 5 步上手

1. **安装并启动 AIGateway**  
   从 [Releases](https://github.com/vincent1986/AIGateway/releases) 下载对应平台安装包并启动。

2. **确认统一入口在跑**  
   默认网关地址：`http://127.0.0.1:18080/v1`  
   在应用内打开 **统一入口**，确认服务为运行状态。

3. **添加厂家**  
   **厂家管理** → 选预设（DeepSeek、硅基流动、Ollama、通义等）→ 粘贴 API Key → 拉取模型。

4. **配置模型组**  
   **模型管理** → 查看虚拟模型组 → 拖拽调整 Failover 优先级（主通道在上，备用在下）。

5. **一键接管工具**  
   **应用管理** → 选择 ChatGPT / Claude Code / OpenClaw / Harness → **一键接管**。  
   之后换模型、换厂家只在 AIGateway 内操作，无需反复改工具配置、重启终端。

## 客户端怎么用

| 场景 | 建议 |
|------|------|
| 已一键接管 | 工具已指向本地网关；`model` 填 **模型组名**（如 `deepseek-chat`） |
| 手动配置 | `base_url` = `http://127.0.0.1:18080/v1`，API Key 可用 `aigateway` |
| Codex / ChatGPT | `model_provider = "aigateway"`，model 填模型组名 |

## 架构一览

```
工具 (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway（路由 / Failover / Token 统计）
        │
        ▼
上游厂家 (DeepSeek / 硅基流动 / Ollama / …)
```

## 下一步

- 完整问答见 [FAQ](FAQ)
- 路径与备份见 [数据与路径](Data-Paths)
- 报错处理见 [故障排除](Troubleshooting)
