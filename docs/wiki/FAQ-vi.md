# FAQ

**Ngôn ngữ:** [EN](FAQ-en) · [中文](FAQ-zh-CN) · [日本語](FAQ-ja) · [Deutsch](FAQ-de) · [Tiếng Việt](FAQ-vi) · [繁體中文](FAQ-zh-TW) · [Trang chủ](Home)

Câu hỏi thường gặp. Xem thêm [Xử lý sự cố](Troubleshooting-vi) và [Bắt đầu](Getting-Started-vi).

---

## 1. Sản phẩm

### AIGateway là gì?

**Quản lý mô hình AI + gateway lưu lượng cục bộ**. Nối nhiều API LLM với ChatGPT/Codex, Claude Code, OpenClaw, Harness:

- **Tiết kiệm token** — theo nhà cung cấp / mô hình  
- **Dịch vụ rẻ hơn** — nhiều API một chỗ  
- **Cấu hình một lần** — công cụ chỉ trỏ gateway cục bộ  
- **Định tuyến linh hoạt** — nhóm mô hình ảo + failover theo ưu tiên  

### Khác chỉnh tay từng công cụ?

V2: **kết nối một lần, định tuyến mãi**. `base_url` chỉ đổi một lần; sau đó đổi model/nhà cung cấp trong AIGateway.

### Dữ liệu có rời máy?

AIGateway chạy local. Request tới upstream bạn cấu hình. Dữ liệu mặc định `~/.codex-manager/`. Tự bảo vệ API Key.

---

## 2. Cài đặt

### Hệ điều hành?

macOS (Apple Silicon / Intel), Windows, Linux — [Releases](https://github.com/vincent1986/AIGateway/releases).

### Cần Docker / Node?

Bản desktop: không. Build từ source cần Go và toolchain frontend.

---

## 3. Gateway

### URL mặc định?

```
http://127.0.0.1:18080/v1
```

### Công cụ trỏ thế nào?

**Ứng dụng → kết nối một chạm**. Thủ công: Base URL như trên, API Key `aigateway`, `model` = **tên nhóm**.

### Phải mở AIGateway?

Có. Tắt app thì proxy cục bộ ngừng.

---

## 4. Nhà cung cấp

**Nhà cung cấp** → preset → API Key → lấy mô hình. Cloud thường chỉ cần key.

| Chế độ | Ý nghĩa |
|--------|---------|
| **OpenAI chuẩn** | Chuẩn hóa request/response |
| **Passthrough** | Chuyển body gần như nguyên |

Ưu tiên OpenAI chuẩn trước.

---

## 5. Mô hình & failover

Nhóm mô hình ảo gom mô hình tương đương giữa các nhà cung cấp. Failover khi 429, hết hạn mức, 401… Hết kênh → `model_group_all_exhausted`.

Giữa stream: hạn chế — [Xử lý sự cố](Troubleshooting-vi).

---

## 6. Kết nối ứng dụng

Một chạm: **ChatGPT (Codex)**, **Claude Code**, **OpenClaw**, **Harness**. Sao lưu rồi trỏ Base URL. Khôi phục trên thẻ hoặc `~/.codex-manager/backups/`.

Codex ID dành riêng / `aigateway_api_key`: [Xử lý sự cố](Troubleshooting-vi).

---

## 7. Token & dữ liệu

Tab **Thống kê**, SQLite `aigateway.db` — [Đường dẫn dữ liệu](Data-Paths-vi). Key không upload GitHub.

---

## 8. Ngôn ngữ UI

Popup: Tiếng Trung giản thể / phồn thể, English, 日本語, 한국어, Deutsch, Tiếng Việt, ไทย.

Năm tab: Nhà cung cấp / Mô hình / Ứng dụng / Gateway / Thống kê.

---

## 9. Nâng cấp

v2: migrate JSON → SQLite. Kiểm tra lại gateway và trạng thái kết nối.

---

## 10. Góp ý

Issue: https://github.com/vincent1986/AIGateway/issues/new/choose  
Giấy phép: [MIT](https://github.com/vincent1986/AIGateway/blob/main/LICENSE) © Mars Waller

| Mục | Giá trị |
|-----|---------|
| Base URL | `http://127.0.0.1:18080/v1` |
| API Key local | `aigateway` |
| Dữ liệu | `~/.codex-manager/` |
| Mã exhausted | `model_group_all_exhausted` |
