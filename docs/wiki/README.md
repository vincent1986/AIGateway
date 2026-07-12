# Wiki 源文档

本目录是 [GitHub Wiki](https://github.com/vincent1986/AIGateway/wiki) 的源内容。

| 文件 | Wiki 页面 |
|------|-----------|
| `Home.md` | 首页 |
| `Getting-Started.md` | 快速开始 |
| `FAQ.md` | 常见问题 |
| `Data-Paths.md` | 数据与路径 |
| `Troubleshooting.md` | 故障排除 |
| `_Sidebar.md` / `_Footer.md` | 侧栏 / 页脚 |

## 发布到 Wiki

1. **首次**（仅一次）：在浏览器打开  
   https://github.com/vincent1986/AIGateway/wiki/_new  
   标题填 `Home`，正文任意，保存。这样会创建 `*.wiki.git` 仓库。

2. 同步全部页面：

```bash
./scripts/publish-wiki.sh
```

之后修改 `docs/wiki/*.md` 再执行脚本即可更新 Wiki。
