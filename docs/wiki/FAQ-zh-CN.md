# 常见问题 FAQ

**语言：** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [首页](Home)

面向 AIGateway 用户的高频问答。更多排错见 [故障排除](Troubleshooting-zh-CN)；上手见 [快速开始](Getting-Started-zh-CN)。

---

## 目录

1. [产品与定位](#1-产品与定位)
2. [安装与平台](#2-安装与平台)
3. [统一入口 / 网关](#3-统一入口--网关)
4. [厂家管理](#4-厂家管理)
5. [模型管理与 Failover](#5-模型管理与-failover)
6. [应用接管](#6-应用接管)
7. [Token 与数据](#7-token-与数据)
8. [多语言与界面](#8-多语言与界面)
9. [升级与兼容](#9-升级与兼容)
10. [反馈与贡献](#10-反馈与贡献)

---

## 1. 产品与定位

### Q1. AIGateway 是什么？

本地 **AI 模型管理 + 流量网关**。把多家大模型 API 与常见 AI 工具（ChatGPT/Codex、Claude Code、OpenClaw、Harness 等）接在一起：

- **省 Token**：按厂家 / 模型看用量  
- **换便宜服务商**：多厂家一处管理，随时切换  
- **一次配置，多工具共用**：工具只指向本地网关  
- **热路由与容灾**：虚拟模型组 + 优先级 Failover（如 429 / 额度耗尽）

### Q2. 和直接改各工具配置有什么区别？

V2 采用 **一次接管、终身调度**：工具的 `base_url` 只改一次指向网关；之后换模型 / 换厂家在网关内完成，减少反复改文件、重启终端和环境变量冲突。

### Q3. 数据会离开本机吗？

AIGateway 在本机运行。请求按你配置的厂家转发到对应上游 API；配置与用量默认保存在本机 `~/.codex-manager/`。请自行保护 API Key。

---

## 2. 安装与平台

### Q4. 支持哪些系统？

macOS（Apple Silicon / Intel）、Windows、Linux。安装包见 [Releases](https://github.com/vincent1986/AIGateway/releases)。

### Q5. 从哪里下载？

官方渠道：https://github.com/vincent1986/AIGateway/releases

### Q6. 需要单独安装 Docker / Node 吗？

普通用户使用发布版桌面应用即可，无需自行搭运行时。从源码构建才需要 Go、前端构建环境等。

---

## 3. 统一入口 / 网关

### Q7. 默认网关地址是什么？

```
http://127.0.0.1:18080/v1
```

在 **统一入口** 查看运行状态与监听配置。

### Q8. 工具应如何指向网关？

推荐在 **应用管理** 使用 **一键接管**。手动配置时：

- Base URL：`http://127.0.0.1:18080/v1`  
- API Key：可用占位值 `aigateway`（本地网关识别，无需真实云 Key）  
- `model`：填 **模型组名称**（与模型管理中一致）

### Q9. 必须始终开着 AIGateway 吗？

是。工具请求先到本机网关再转发上游；退出 AIGateway 后本地代理不可用。

---

## 4. 厂家管理

### Q10. 如何快速添加厂家？

**厂家管理** → 选 **预设**（DeepSeek、硅基流动、Ollama、OpenAI、通义等）→ 粘贴 API Key → 拉取 / 启用模型。多数云厂家只需 Key。

### Q11. 「标准 OpenAI」和「原样转发 (Passthrough)」怎么选？

| 模式 | 含义 | 适用 |
|------|------|------|
| **标准 OpenAI** | 网关规整请求/响应，抹平部分非标差异 | 兼容 OpenAI 协议的云 API、需统一格式时 |
| **原样转发 Passthrough** | 尽量透传 body，少做协议改写 | 特殊协议或调试上游原样行为时 |

不确定时优先试 **标准 OpenAI**；异常再改 Passthrough 对比。

### Q12. 厂家里的模型开关有什么用？

控制该厂家下模型是否参与调度。停用后不会作为 Failover 候选。

---

## 5. 模型管理与 Failover

### Q13. 什么是虚拟模型组？

把 **不同厂家上的同名/等价模型** 聚合成一组。客户端只写一个模型名，网关按优先级选通道。

### Q14. Failover 何时触发？

上游返回如 **429 限流**、**额度/配额类错误**、**账户异常（如 401）** 等时，网关会按队列尝试下一顺位厂家。全部失败则返回 `model_group_all_exhausted`。

### Q15. 流式对话中途会不会切换厂家？

**有限制**：首包 / HTTP 层错误可切换；**已经开始向客户端推流后**无法无感中途换通道。详见 [故障排除](Troubleshooting-zh-CN)。

### Q16. 如何调整主备顺序？

在 **模型管理** 中拖拽组内厂家顺序：越靠上优先级越高。

---

## 6. 应用接管

### Q17. 支持哪些工具一键接管？

**ChatGPT（Codex）**、**Claude Code**、**OpenClaw**、**Harness**。其它 OpenAI 兼容客户端可手动填网关 Base URL。

### Q18. 一键接管会做什么？

定位工具配置文件 → **备份** → 将 `base_url`（及必要 provider 字段）指向本地网关。

### Q19. 如何恢复原来的工具配置？

在对应应用卡片使用 **一键卸载 / 恢复**（或从 `~/.codex-manager/backups/` 取备份）。

### Q20. OpenClaw 要注意什么？

应通过 `models.providers.aigateway` 等方式接入；仅改根级错误字段可能导致未真正走 AIGateway。

### Q21. Codex 报保留 ID 或缺少 aigateway_api_key？

见 [故障排除](Troubleshooting-zh-CN)：

- 保留 ID：`openai` / `ollama` 改为 `openai-custom` / `ollama-local`  
- Key：使用 `api_key = "aigateway"`

---

## 7. Token 与数据

### Q22. Token 统计在哪里看？

应用内 **Token 统计** 页；底层写入 SQLite（`aigateway.db`）。路径见 [数据与路径](Data-Paths-zh-CN)。

### Q23. 会把 API Key 上传到 GitHub 吗？

不会。Key 仅保存在本机。提 Issue 时请自行打码。

---

## 8. 多语言与界面

### Q24. 支持哪些界面语言？

弹窗可选：简体中文、繁體中文、English、日本語、한국어、Deutsch、Tiếng Việt、ไทย。

### Q25. 五个主功能区是什么？

1. **厂家管理** — 通路与 Key  
2. **模型管理** — 虚拟组与 Failover  
3. **应用管理** — 工具接管  
4. **统一入口** — 网关状态  
5. **Token 统计** — 用量  

---

## 9. 升级与兼容

### Q26. v1 升 v2 要注意什么？

- 首次启动自动 JSON → SQLite 迁移  
- 推荐重新确认 **统一入口** 与各工具接管状态  
- Codex 保留 ID / api_key 规则见上  

### Q27. 已知限制有哪些？

- 流式 **中途** Failover 有限  
- 模型看板部分字段可能仍在完善  
- 细节见 `docs/PRD_V2_CHECKLIST.md` 与 Release Notes  

---

## 10. 反馈与贡献

### Q28. 如何反馈？

https://github.com/vincent1986/AIGateway/issues/new/choose

### Q29. 开源协议？

[MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller

---

## 快速对照表

| 项目 | 值 |
|------|------|
| 默认 Base URL | `http://127.0.0.1:18080/v1` |
| 本地 API Key 占位 | `aigateway` |
| 数据目录 | `~/.codex-manager/` |
| 主库 | `aigateway.db` |
| 全通道耗尽错误码 | `model_group_all_exhausted` |
