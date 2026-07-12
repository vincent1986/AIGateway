# AIGateway V2 · PRD 验收清单

| | |
|---|---|
| **PRD** | [PRD_V2.md](./PRD_V2.md) |
| **分支** | `v2` |
| **产品版本** | `2.0.0-dev`（`wails.json`） |
| **对照提交** | 含 `713a432` … `b2b97eb` 及后续 |
| **更新日期** | 2026-07-11 |

图例：✅ 已落地 · 🟡 部分落地 · ❌ 未做 · 📎 备注

---

## 1. 核心改进指标（PRD §1.2）

| # | 指标 | 状态 | 证据 / 缺口 |
|---|------|------|-------------|
| 1 | 一次写入 base_url、终身接管，网关内热调度 | ✅ | `InjectGateway` + `resolveRoutesForModel`；切换通道无需再改工具配置 |
| 2 | 职责解耦：厂家 / 模型 / 应用 / 入口 / 统计 | ✅ | 五栏导航 + 模块边界基本对齐 |
| 3 | SQLite 替代高频 JSON | 🟡 | 主路径 `aigateway.db`；仍镜像写 `providers.json`；proxy 配置仍为 `proxy.json` |
| 4 | 流式毫秒级熔断（429/额度 → 备用） | 🟡 | 非流式全链路 Failover；流式在 **HTTP 错码** 与 **首包错误 body** 可切换；**已向客户端吐流后无法无感切换** |
| 5 | ChatGPT / Claude / OpenClaw / Harness | ✅ | ToolDriver 注册 4 工具；展示名 ChatGPT；内部 kind `codex` 兼容路径 |

---

## 2. 架构与数据流（PRD §2）

| 步骤 | 状态 | 说明 |
|------|------|------|
| 客户端 → `127.0.0.1:18080/v1` | ✅ | 统一入口页 / 默认端口 |
| 解析 `model` | ✅ | `extractModel` |
| SQLite 路由表匹配虚拟模型组 | ✅ | `model_groups` + `model_group_routes` |
| SSE 计费与上游监控 | 🟡 | SSE 结束解析 usage 写库；非完整「逐 chunk 状态机」 |
| 429/额度 → 下一厂家 | 🟡 | 见 §1.2 #4 |
| OpenAI 规整 / Passthrough | ✅ | `format_standard` + 网关分支 |

---

## 3. 导航与 UI（PRD §3.1）

| 需求 | 状态 | 说明 |
|------|------|------|
| 厂家管理 | ✅ | 原厂家模型瘦身 |
| 模型管理 | ✅ | 虚拟组 + 拖拽优先级 |
| 应用管理 | ✅ | 卡片网格 |
| 统一入口 | ✅ | 原代理页 |
| Token 统计 | ✅ | 多维看板 |
| 暗黑科技风 | ✅ | 现有 dark theme |
| 状态色：绿/黄/红/灰 | 🟡 | `tag ok/warn/err/off` 已用；未强制 emoji 🟢🟡🔴⚪ 文案 |

---

## 4. 厂家管理（PRD §3.2）

| 需求 | 状态 | 说明 |
|------|------|------|
| 预设库，点选 + 仅填 Key | ✅ | 分区芯片：本机/国内/国际/自定义 |
| OpenAI / Passthrough 单选 | ✅ | 详情页 + SQLite |
| 模型列表仅 Toggle + 移除 | ✅ | 已去掉写入工具按钮 |
| 通道全局开关 | 🟡 | 有模型级 enable；厂家级「全局停用」字段 `providers.enabled` 有表无完整 UI |

---

## 5. 模型管理（PRD §3.3）

| 需求 | 状态 | 说明 |
|------|------|------|
| 同名模型横向聚合 | ✅ | 保存厂家时按 `model_id` 建组 |
| 供应厂家 / 状态 / 已用 Token | 🟡 | 有厂家、状态、used_tokens；**无「剩余套餐金额」列**；**无「已应用工具」列** |
| 拖拽优先级队列 | ✅ | HTML5 DnD + `ReorderModelGroupRoutes` |
| Failover 429/401/额度 | 🟡 | 见熔断说明 |
| 全挂自动停用模型组 | 🟡 | 返回 `model_group_all_exhausted`；**未把 group.enabled 置 0 持久化** |
| 标准 OpenAI 错误 JSON | ✅ | `exhaustedErrorJSON` |

---

## 6. 应用管理（PRD §3.4）

| 需求 | 状态 | 说明 |
|------|------|------|
| Codex UI → ChatGPT | 🟡 | **展示名** ChatGPT；代码 kind/路径仍 `codex`（有意兼容） |
| 网格卡片 | ✅ | `apps-grid` |
| Logo + 路径输入框 | 🟡 | 有名称/路径展示与搜索选择；**无独立 Logo 资源**；路径非卡片内直接编辑框 |
| 自动检索 / 手动选择 / 回滚 | ✅ | scan / pick / rollback |
| 一键接管 | ✅ | `InjectGateway` |
| 移除页内切换模型 | ✅ | 应用卡无 model 下拉 |
| 写入前备份 | ✅ | `ensureDefaultBackup` + pre-edit 快照 |
| 一键卸载/还原 | ✅ | `RollbackGateway` → 默认备份 |

---

## 7. 统一入口与计费（PRD §3.5）

| 需求 | 状态 | 说明 |
|------|------|------|
| 按 model 虚拟路由 | ✅ | SQLite 优先级链 |
| SSE 解析 usage | ✅ | `recordUsageFromSSE` → SQLite |
| 写厂家余额 | 🟡 | 更新 `model_group_routes.used_tokens` + 套餐页用代理用量估算；**无独立「账户余额」字段** |
| 网关运行日志 | ✅ | 统一入口日志面板 |

---

## 8. 工程约束（PRD §4）

### 8.1 SQLite Schema

| 表 / 概念 | 状态 | 说明 |
|-----------|------|------|
| `providers` | ✅ | 含 format_standard |
| `model_groups` | ✅ | |
| `provider_models` | ✅ | 厂家模型明细 |
| `model_group_routes` | ✅ | 优先级 / 状态 / used_tokens（PRD 中「绑定关系」落在此表，而非仅 provider_models） |
| 严禁纯 JSON 状态 | 🟡 | 主状态在 SQLite；JSON 镜像与 `proxy.json` 仍存在 |

### 8.2 ToolDriver

| 项 | 状态 | 说明 |
|----|------|------|
| 统一接口 | ✅ | Go `ToolDriver`（非 TS，语义一致） |
| chatgpt / claude / openclaw / harness | ✅ | `tool_drivers.go` |
| injectGateway / rollback | ✅ | inject 在 driver；rollback 走备份还原 |
| detectConfig | 🟡 | 由 `resolveTool` + DefaultPaths 实现，非接口方法名 |

### 8.3 容错 JSON

| 项 | 状态 |
|----|------|
| `model_group_all_exhausted` | ✅ |

---

## 9. 总体得分（主观）

| 维度 | 完成度 | 说明 |
|------|--------|------|
| 架构转型（网关 + 虚拟路由 + SQLite） | **~85%** | 主路径已通 |
| 产品模块（五栏 + 解耦） | **~90%** | UI 骨架完整 |
| Failover / 流式熔断 | **~65%** | 非流式强；流式半程限制 |
| 模型看板信息密度 | **~70%** | 缺套餐剩余/应用工具列 |
| 应用市场打磨 | **~75%** | 缺 Logo；路径联调依赖本机 |
| 存储纯净化 | **~80%** | 仍有 JSON 镜像 |

**综合：约 80% 对齐 PRD，可作 v2.0.0-beta 候选；正式 GA 建议补齐下方 P0 缺口。**

---

## 10. 建议补齐 backlog（按优先级）

### P0（GA 前）

1. **流式已吐包后的策略文档化**：明确「首包可切、中途不切」；或实现缓冲首 N 包再决定（有延迟代价）。  
2. **全渠道耗尽时** `UPDATE model_groups SET enabled=0`（或 routes 全 exhausted）并在 UI 标红。  
3. **模型看板**增加：套餐剩余（接 `GetProviderPackageStatuses`）、可选「已应用工具」粗略标记。  
4. **proxy 配置**迁入 SQLite 或明确「配置仍 JSON、业务状态 SQLite」。

### P1（体验）

5. 应用卡片 Logo / 品牌色。  
6. 厂家级总开关 UI。  
7. OpenClaw / Harness 真机路径说明（`docs/TOOLS.md`）。  
8. 全局文案：用户可见「Codex」→「ChatGPT」扫尾（保留代码 id）。

### P2（发布）

9. `v2.0.0-beta` 多平台打包 + Release notes。  
10. 自动化：`go test` + 前端 build 进 CI。

---

## 11. 手工验收步骤（冒烟）

```bash
git checkout v2
wails dev
```

| # | 步骤 | 期望 |
|---|------|------|
| 1 | 厂家管理 → 添加 → 点 DeepSeek/硅基流动 → 只填 Key | 成功添加，Base URL 正确 |
| 2 | 拉取模型并启用同名模型于两家 | 模型管理出现聚合组、≥2 通道 |
| 3 | 拖拽通道顺序 | 优先级变化，刷新后保持 |
| 4 | 应用管理 → ChatGPT 一键接管 | 配置含 aigateway / 本地 base_url；有备份 |
| 5 | 统一入口启动；客户端请求 model=该组 | 路由到最高优先级厂家 |
| 6 | （可 mock）上游 429 | 切下一通道或返回 exhausted JSON |
| 7 | Token 统计 | 有调用次数与 token；SQLite `usage_events` 有行 |
| 8 | 应用 卸载/还原 | 配置回到备份 |

数据文件：

- `~/.codex-manager/aigateway.db` — 主库  
- `~/.codex-manager/providers.json` — 镜像（兼容）  
- `~/.codex-manager/proxy.json` — 网关监听配置  

---

## 12. 结论

| 问题 | 答案 |
|------|------|
| 是否达到 PRD「可开发/可演示」？ | **是** |
| 是否达到 PRD「正式发布全部条款」？ | **否**（流式中途熔断、看板字段、存储纯净化仍有 🟡） |
| 推荐下一步 | 按 §10 P0 → 再 **E 打 beta 包** |

维护：实现新需求后请同步更新本表状态与「更新日期」。
