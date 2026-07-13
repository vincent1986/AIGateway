# Đường dẫn dữ liệu

**Ngôn ngữ:** [EN](Data-Paths-en) · [中文](Data-Paths-zh-CN) · [日本語](Data-Paths-ja) · [Deutsch](Data-Paths-de) · [Tiếng Việt](Data-Paths-vi) · [繁體中文](Data-Paths-zh-TW) · [Trang chủ](Home)

AIGateway lưu trạng thái trong thư mục `.codex-manager` tại home người dùng (đa nền tảng).

## Đường dẫn chính

| Nội dung | Đường dẫn |
|----------|-----------|
| SQLite (nhà cung cấp, nhóm mô hình, định tuyến, dùng token) | `~/.codex-manager/aigateway.db` |
| Bản sao JSON nhà cung cấp | `~/.codex-manager/providers.json` |
| Cấu hình gateway | `~/.codex-manager/proxy.json` |
| Sao lưu cấu hình công cụ | `~/.codex-manager/backups/` |
| File liên quan biến môi trường | `~/.codex-manager/env/` |

> Trên Windows, `~` là thư mục người dùng (ví dụ `C:\Users\<bạn>`).

## Nâng cấp từ v1

- Lần đầu chạy v2 sẽ migrate `providers.json` / `usage.json` cũ vào SQLite.
- **SQLite** là nguồn chính; một số cấu hình lắng nghe có thể vẫn ở `proxy.json`.

## Sao lưu

- Trước khi nâng cấp/cài lại, hãy sao lưu toàn bộ `~/.codex-manager/`.
- Kết nối một chạm sẽ snapshot cấu hình công cụ; có thể khôi phục trong thẻ ứng dụng.

Xem thêm: [Bắt đầu](Getting-Started-vi) · [Xử lý sự cố](Troubleshooting-vi)
