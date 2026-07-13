# Wiki 源文档

本目录是 [GitHub Wiki](https://github.com/vincent1986/AIGateway/wiki) 的源内容，支持 **6 种语言**：

- English (`*-en`)
- 中文简体 (`*-zh-CN`)
- 日本語 (`*-ja`)
- Deutsch (`*-de`)
- Tiếng Việt (`*-vi`)
- 繁體中文 (`*-zh-TW`)

## 页面结构

| 文件 | Wiki 页面 | 说明 |
|------|-----------|------|
| `Home.md` | 首页 / 语言入口 | 六语简介 + 导航表 |
| `Getting-Started.md` | 快速开始（语言选择） | 跳转到各语言页 |
| `Getting-Started-{en,zh-CN,ja,de,vi,zh-TW}.md` | 各语言快速开始 | |
| `FAQ.md` | FAQ（语言选择） | |
| `FAQ-{en,zh-CN,ja,de,vi,zh-TW}.md` | 各语言 FAQ | |
| `Data-Paths.md` / `Data-Paths-*.md` | 数据与路径 | |
| `Troubleshooting.md` / `Troubleshooting-*.md` | 故障排除 | |
| `_Sidebar.md` / `_Footer.md` | 侧栏 / 页脚 | 含六语链接 |

## 发布到 Wiki

1. **首次**（仅一次）：在浏览器打开  
   https://github.com/vincent1986/AIGateway/wiki/_new  
   标题填 `Home`，正文任意，保存。这样会创建 `*.wiki.git` 仓库。

2. 同步全部页面：

```bash
./scripts/publish-wiki.sh
```

之后修改 `docs/wiki/*.md` 再执行脚本即可更新 Wiki。

> 旧链接 `FAQ` / `Getting-Started` 等仍可用，会进入语言选择页。
