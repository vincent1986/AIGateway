# 故障排除 / Troubleshooting

## 网关连不上

1. 确认 AIGateway 已启动，**统一入口**显示运行中。  
2. 浏览器或 curl 访问：`http://127.0.0.1:18080/v1`（端口是否被占用）。  
3. 工具是否仍指向旧 Base URL；重新执行 **一键接管**。  
4. 本机防火墙 / 代理是否拦截 `127.0.0.1`。

## Codex：Missing environment variable: aigateway_api_key

在 provider 配置中使用 **内联** `api_key`，不要只写未设置的 `env_key`：

```toml
api_key = "aigateway"
```

不要依赖未定义的环境变量 `aigateway_api_key`。

## Codex：保留 provider ID 冲突

不要使用内置保留 ID `openai` / `ollama` 作为自定义 provider 名。应改为例如：

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## 返回 model_group_all_exhausted

表示该 **虚拟模型组** 下所有厂家通道都失败或额度耗尽（常见 429 / 配额）。

处理：

1. 检查厂家 API Key、余额与限流。  
2. 在 **模型管理** 增加备用通道并调整优先级。  
3. 确认厂家与模型均已 **启用**。

错误体为标准 OpenAI JSON，避免下游工具崩溃：

```json
{
  "error": {
    "message": "AIGateway Error: All provider backups for this model group have been exhausted or rate-limited.",
    "type": "insufficient_quota",
    "param": "model_group",
    "code": "model_group_all_exhausted"
  }
}
```

## 流式请求中途不切换通道

当前限制：HTTP / **首包**错误可 Failover；**已向客户端吐流后**无法无感中途切换。  
若首包成功后上游中断，需在客户端重试（将再次按优先级选路）。

## 一键接管后工具仍走官方

1. 确认接管状态为「已接管」。  
2. 重启对应 CLI / IDE 会话使配置生效。  
3. OpenClaw：应使用 `models.providers.aigateway`，不要只设错误的根级 `baseUrl` / `OPENAI_BASE_URL`。  
4. 用 **一键卸载/恢复** 回滚后再重新接管。

## 用量统计不准

- 以 SQLite 中的统计为准；异常可备份后重启应用。  
- 部分上游不返回 usage 字段时，计数可能为 0 或估算不全。

## 仍无法解决

1. 查看 [FAQ](FAQ)  
2. 升级到 [最新 Release](https://github.com/vincent1986/AIGateway/releases)  
3. [提交 Issue](https://github.com/vincent1986/AIGateway/issues/new/choose)（附系统、版本、复现步骤；**打码 API Key**）
