# Bắt đầu nhanh

**Ngôn ngữ:** [EN](Getting-Started-en) · [中文](Getting-Started-zh-CN) · [日本語](Getting-Started-ja) · [Deutsch](Getting-Started-de) · [Tiếng Việt](Getting-Started-vi) · [繁體中文](Getting-Started-zh-TW) · [Trang chủ Wiki](Home)

## 5 bước

1. **Cài và khởi chạy AIGateway**  
   Tải gói phù hợp từ [Releases](https://github.com/vincent1986/AIGateway/releases) và mở ứng dụng.

2. **Xác nhận gateway đang chạy**  
   Địa chỉ mặc định: `http://127.0.0.1:18080/v1`  
   Mở **Gateway / Cổng thống nhất** và kiểm tra trạng thái.

3. **Thêm nhà cung cấp**  
   **Nhà cung cấp** → chọn preset (DeepSeek, SiliconFlow, Ollama, Qwen, …) → dán API Key → lấy mô hình.

4. **Cấu hình nhóm mô hình**  
   **Mô hình** → xem nhóm mô hình ảo → kéo thả thứ tự failover (chính trên, dự phòng dưới).

5. **Kết nối công cụ một chạm**  
   **Ứng dụng** → chọn ChatGPT / Claude Code / OpenClaw / Harness → **Kết nối**.  
   Sau đó chỉ cần đổi model/nhà cung cấp trong AIGateway.

## Cấu hình client

| Tình huống | Gợi ý |
|------------|--------|
| Đã kết nối một chạm | Công cụ đã trỏ gateway cục bộ; `model` = **tên nhóm mô hình** |
| Cấu hình thủ công | `base_url` = `http://127.0.0.1:18080/v1`, API Key có thể là `aigateway` |
| Codex / ChatGPT | `model_provider = "aigateway"`, model = tên nhóm |

## Kiến trúc

```
Công cụ (ChatGPT / Claude Code / OpenClaw / …)
        │  base_url → http://127.0.0.1:18080/v1
        ▼
   AIGateway (định tuyến / failover / thống kê token)
        │
        ▼
Nhà cung cấp upstream (DeepSeek / SiliconFlow / Ollama / …)
```

## Tiếp theo

- [FAQ](FAQ-vi)
- [Đường dẫn dữ liệu](Data-Paths-vi)
- [Xử lý sự cố](Troubleshooting-vi)
