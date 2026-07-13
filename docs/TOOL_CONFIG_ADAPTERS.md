# 应用配置适配与升级设计

本文档说明 AIGateway 如何定位、修改和恢复下游应用配置，并定义后续新增或升级应用时应遵循的抽象方式。

## 1. 目标

应用管理只负责把下游应用接入本地网关。厂家、模型组和路由由 AIGateway 管理。

接入应用时遵循“一次写入，持续接管”：

1. 定位应用配置文件。
2. 在第一次修改前保存原始配置。
3. 只修改该应用负责的网关入口和默认模型字段。
4. 通过统一入口由 AIGateway 继续调度模型。
5. 需要卸载时恢复接管前的配置。

## 2. 当前支持的应用

| 内部 ID | 展示名称 | 配置格式 | 默认配置位置 | 网关写入方式 |
|---|---|---|---|---|
| `codex` | ChatGPT / Codex | TOML | `~/.codex/config.toml` | 写入 `model_providers.aigateway`、`model_provider`、`model` |
| `claude` | Claude Code | JSON | `~/.claude/settings.json` | 写入 `env.OPENAI_BASE_URL`、`env.ANTHROPIC_BASE_URL`、模型和 `apiBaseUrl` |
| `openclaw` | OpenClaw | JSON5 | `~/.openclaw/openclaw.json` | 写入 `models.providers.aigateway` 和主模型引用 |
| `harness` | Harness | YAML | `~/.harness/config.yaml` | 更新已识别的顶层 `base_url`、`api_key`、`model`、`provider` |

应用也支持 macOS、Windows、Linux 的候选路径，以及环境变量路径覆盖：

- Codex：`CODEX_HOME/config.toml`
- Claude Code：`CLAUDE_CONFIG_DIR/settings.json`
- Windows：`%USERPROFILE%`、`%APPDATA%`、`%LOCALAPPDATA%`
- 用户手动选择的路径：保存到 `~/.codex-manager/paths.json`

完整路径搜索规则位于 `platform_paths.go`，应用驱动位于 `tool_drivers.go`。

## 3. 当前实现流程

### 3.1 发现配置

入口是 `App.DiscoverToolConfigs()`，它遍历 `toolRegistry`，对每个驱动执行：

1. 读取手动路径覆盖。
2. 按当前系统和环境变量生成候选路径。
3. 选取第一个存在的文件。
4. 若文件不存在，返回当前系统的首选创建路径。
5. 解析模型、来源、备份和是否已接管状态。

关键实现：

- `config_tools.go:105`：批量发现
- `config_tools.go:119`：单个应用解析
- `platform_paths.go:120`：Codex / Claude 路径
- `tool_drivers.go:27`：驱动注册

### 3.2 一键接管

入口是 `App.InjectGateway(kind)`，公共流程为：

1. 规范化应用 ID，例如 `chatgpt` 映射到 `codex`。
2. 确保本地代理运行，默认地址为 `http://127.0.0.1:18080/v1`。
3. 获取配置路径。
4. 调用 `ensureDefaultBackup()` 保存第一次接管前的基线。
5. 调用对应驱动的 `InjectGateway()`。
6. 重新读取配置并验证模型字段。
7. 保存手动路径覆盖并刷新状态。

关键实现：

- `tool_takeover.go:15`：统一接管入口
- `tool_drivers.go:72`：各应用网关写入驱动
- `config_backup.go:133`：默认备份
- `config_tools.go:271`：写入后校验

### 3.3 模型切换

接管后模型切换通过 `ApplyToolModel()` 完成。它可以修改应用配置中的模型和厂家上下文，但不应改变已经接管的本地网关入口。

这一区别很重要：

- 应用配置：保存稳定的本地网关地址。
- AIGateway 数据库：保存厂家、模型组、优先级和 Failover 路由。
- 模型切换：优先改变网关内的路由，而不是频繁重写应用配置。

关键实现：`model_apply.go:26`。

### 3.4 回滚

入口是 `RollbackGateway()`，实际由 `RestoreDefaultConfig()` 从 `~/.codex-manager/backups/` 恢复。

备份设计包括：

- 按应用 ID 和原始路径生成稳定备份名。
- 保存 SHA-256、原始路径、时间和文件大小。
- 原配置不存在时记录 `Missing=true`，回滚时删除接管创建的文件。
- 恢复前再次保存当前文件快照。

关键实现：`config_backup.go:229`。

## 4. 各应用的修改边界

### 4.1 ChatGPT / Codex

Codex 使用 TOML，不能通过通用 JSON/YAML 写入器处理。当前实现使用局部 TOML 文本和结构化辅助函数：

- 创建或更新 `model_providers.aigateway`。
- 设置 `base_url` 和本地 `api_key`。
- 设置顶层 `model_provider = "aigateway"`。
- 设置顶层 `model` 为虚拟模型组。
- 清理旧的 `codex_proxy` 和保留 provider 冲突。
- 保留 `wire_api` 等协议字段。

升级要求：不得删除用户自定义 provider、模型字段或协议字段；必须继续处理 Codex 保留 provider ID 和本地 `api_key` 场景。

### 4.2 Claude Code

Claude Code 使用 JSON，当前写入 `settings.json` 的 `env` 和兼容字段：

- `OPENAI_BASE_URL`
- `ANTHROPIC_BASE_URL`
- `ANTHROPIC_MODEL`
- `model`
- `apiBaseUrl`

升级要求：解析失败时必须停止写入，不能用空对象覆盖原文件；只更新 AIGateway 管理字段，保留其它设置。

### 4.3 OpenClaw

OpenClaw 配置可能是 JSON5，当前使用 JSON5 解析，并兼容注释、无引号 key、尾逗号和单引号字符串。

接管写入位置：

```text
models.mode = "merge"
models.providers.aigateway.baseUrl
models.providers.aigateway.apiKey
models.providers.aigateway.api = "openai-completions"
models.providers.aigateway.models
models.primary = "aigateway/<model>"
```

升级要求：必须保留已有 provider 字段和已有模型列表；使用 OpenClaw 当前 schema 的 `baseUrl`、`apiKey` 和 `api = "openai-completions"`。不能只写根级地址或 `OPENAI_BASE_URL`，否则 OpenClaw 可能不会实际使用网关。

风险：JSON5 重新序列化可能改变注释、缩进和引号样式。后续如需保留原始格式，应引入支持 AST 保留位置信息的 JSON5 编辑器，或只对目标路径做文本级局部替换。

### 4.4 Harness

Harness 当前只安全修改已识别的顶层 YAML 字段：

```yaml
base_url: http://127.0.0.1:18080/v1
api_key: aigateway
model: aiSwitchModel
provider: aigateway
```

如果已有 YAML 不是已识别的顶层结构，必须返回错误并停止写入，不能猜测嵌套 provider 结构。

升级要求：新增 Harness 版本或配置 schema 时，先增加 schema 检测和测试，再扩展写入规则。

## 5. 建议的统一适配器抽象

当前 Go 接口已经覆盖 `ToolID`、名称、格式、候选路径、首选路径、接管和已接管检测。后续建议扩展为以下职责，但不要把所有逻辑塞入一个巨大接口：

```go
type ToolDriver interface {
    ToolID() string
    ToolName() string
    ConfigType() string
    DefaultPaths() []string
    PreferredPath() string

    Detect(path string) (ConfigState, error)
    Inspect(content []byte) (ToolConfigView, error)
    InjectGateway(path string, req GatewayInjection) error
    ApplyModel(content []byte, path string, req ModelInjection) ([]byte, error)
    Validate(path string, expected ExpectedConfig) error
    ValidateConfigContent(content []byte, path string, expected ExpectedConfig) error
}
```

公共层负责：

- 路径搜索和手动路径覆盖。
- 备份、快照、哈希和回滚。
- 原子写入、权限和换行符处理。
- 网关启动和状态刷新。
- 错误展示、日志和前端状态。

应用驱动负责：

- 解析自己的配置格式。
- 识别自己的有效配置位置。
- 修改最小字段集合。
- 判断是否已经接管。
- 写入后的语义校验。

不要在公共流程中增加以下形式的扩展：

```go
if kind == ToolX { ... }
else if kind == ToolY { ... }
```

新增应用应通过 `toolRegistry` 注册一个独立驱动。只有路径覆盖、备份和状态模型中的枚举值需要同步增加。

## 6. 新增或升级应用的标准步骤

1. 确认官方配置格式、字段含义和实际生效优先级。
2. 在驱动中增加应用 ID、名称、格式和跨平台候选路径。
3. 实现 `InjectGateway()`，只写入最小必要字段。
4. 实现 `IsManaged()`，判断配置是否真实指向网关。
5. 将读取模型和写入校验放在该应用的 schema 逻辑中。
6. 接入统一备份和回滚，不为单个应用另建备份目录。
7. 为空配置、已有配置、格式错误、重复接管和回滚补充测试。
8. 用真实应用启动验证配置确实生效，而不只验证文件包含字符串。
9. 更新应用列表、路径说明、升级说明和故障排除文档。
10. 运行 `go test ./...`、前端构建，并执行一次 `wails dev` 实机检查。

## 7. 版本升级兼容策略

### 配置 schema 变化

每个驱动应识别 schema 版本或字段变体，并采用以下顺序：

1. 识别新 schema。
2. 优先保留用户原字段。
3. 只迁移 AIGateway 管理字段。
4. 写入后重新解析并验证实际生效字段。
5. 无法识别时拒绝写入，并提示用户手动选择或升级适配器。

### AIGateway 版本变化

- 先读取当前文件，再备份，再修改。
- 不把新模型组名硬编码到所有应用驱动中，统一从请求传入。
- 网关地址从代理状态获取，不在驱动中固定端口。
- 变更字段前保留旧配置快照。
- 升级失败时保持原文件不变。
- 任何新字段都必须有回滚路径。

### 驱动能力声明

后续可在状态中增加能力字段，例如：

```text
supportsGatewayInjection
supportsModelSwitch
supportsRollback
preservesComments
schemaVersion
```

前端据此决定显示“一键接管”“切换模型”或“仅手动配置”，避免把所有应用都当成同一种配置结构。

## 8. 测试矩阵

每个驱动至少覆盖：

| 场景 | 断言 |
|---|---|
| 配置不存在 | 创建正确首选路径，且回滚会删除新文件 |
| 已有合法配置 | 保留无关字段，只修改管理字段 |
| 已接管配置再次接管 | 幂等，不重复追加 provider 或模型 |
| 配置格式错误 | 返回错误，原文件字节内容不变 |
| 手动路径 | 使用覆盖路径，不误写自动搜索路径 |
| 写入后读取 | 能由同一驱动重新识别模型和已接管状态 |
| 回滚 | 恢复原始字节内容或删除接管创建的文件 |
| 应用升级字段 | 新旧 schema 均能识别，未知 schema 拒绝猜测 |

当前相关测试文件：

- `tool_drivers_test.go`
- `config_tools_test.go`
- `config_backup_test.go`
- `model_apply_test.go`
- `platform_paths_test.go`

## 9. 当前待改进项

1. OpenClaw JSON5 重写可能改变注释和格式，后续应评估保留格式的 AST 编辑方案。
2. Harness 目前只支持已识别的顶层 schema，新增嵌套 schema 必须单独适配。
3. `ToolDriver` 的回滚继续由公共备份层完成，避免各应用重复实现备份逻辑。
4. 真实应用启动验证还应纳入发布前检查，而不是只依赖 Go 单元测试。

## 10. 相关代码索引

- 驱动注册和应用专用写入：`tool_drivers.go`
- 应用发现、路径覆盖和状态：`config_tools.go`
- 网关接管和回滚入口：`tool_takeover.go`
- 备份、快照和恢复：`config_backup.go`
- 模型写入：`model_apply.go`
- 跨平台路径：`platform_paths.go`
- 应用管理前端动作：`frontend/src/main.js`
