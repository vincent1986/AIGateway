# Xử lý sự cố

**Ngôn ngữ:** [EN](Troubleshooting-en) · [中文](Troubleshooting-zh-CN) · [日本語](Troubleshooting-ja) · [Deutsch](Troubleshooting-de) · [Tiếng Việt](Troubleshooting-vi) · [繁體中文](Troubleshooting-zh-TW) · [Trang chủ](Home)

## Không kết nối được gateway

1. Xác nhận AIGateway đang chạy, **Gateway** hiển thị hoạt động.  
2. Mở hoặc curl `http://127.0.0.1:18080/v1` (kiểm tra xung đột cổng).  
3. Công cụ vẫn trỏ Base URL cũ → chạy lại **kết nối một chạm**.  
4. Tường lửa / proxy có chặn `127.0.0.1` không.

## Codex: Missing environment variable: aigateway_api_key

Dùng `api_key` **nội tuyến** trong cấu hình provider:

```toml
api_key = "aigateway"
```

Không phụ thuộc biến môi trường `aigateway_api_key` chưa đặt.

## Codex: xung đột provider ID dành riêng

Không dùng ID dành riêng `openai` / `ollama` làm tên tùy chỉnh. Ví dụ:

- `openai` → `openai-custom`
- `ollama` → `ollama-local`

## Lỗi model_group_all_exhausted

Mọi kênh trong **nhóm mô hình ảo** đều thất bại hoặc hết hạn mức (thường 429 / quota).

Xử lý:

1. Kiểm tra API Key, số dư, giới hạn tốc độ.  
2. Trong **Mô hình**, thêm kênh dự phòng và chỉnh ưu tiên.  
3. Bật nhà cung cấp và mô hình.

## Stream giữa chừng không chuyển kênh

Lỗi HTTP / **byte đầu** có thể failover; **sau khi đã stream tới client** thường không chuyển mượt.  
Client thử lại sẽ chọn đường theo ưu tiên.

## Sau kết nối vẫn gọi API chính thức

1. Trạng thái đã “kết nối”.  
2. Khởi động lại CLI / IDE.  
3. OpenClaw: dùng `models.providers.aigateway`.  
4. **Gỡ/khôi phục** rồi kết nối lại.

## Thống kê token sai

- Lấy thống kê từ SQLite làm chuẩn.  
- Upstream không trả usage → có thể 0 hoặc thiếu.

## Vẫn chưa xong?

1. Đọc [FAQ](FAQ-vi)  
2. Cập nhật [Release mới nhất](https://github.com/vincent1986/AIGateway/releases)  
3. [Tạo Issue](https://github.com/vincent1986/AIGateway/issues/new/choose) (OS, phiên bản, bước tái hiện; **che API Key**)
