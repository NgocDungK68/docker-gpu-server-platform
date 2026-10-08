# Tài khoản demo

**DEMO / DEVELOPMENT ONLY - DO NOT USE IN PRODUCTION**

Chỉ dùng cho lab tạo bởi `python scripts/demo.py up` hoặc `bash scripts/demo-up.sh`. Đây là mật khẩu mẫu công khai, không phải mật khẩu thật của Viettel. Không dùng bộ seed này cho Control Plane production.

| User | Password | Organization | Role |
|---|---|---|---|
| admin | AIWM-Demo-2026! | VTT; xem toàn hệ thống | ADMIN |
| vtt | AIWM-Demo-2026! | VTT | ORGANIZATION_USER |
| vds | AIWM-Demo-2026! | VDS | ORGANIZATION_USER |
| vtnet | AIWM-Demo-2026! | VTNET | ORGANIZATION_USER |
| vtit | AIWM-Demo-2026! | VTIT | ORGANIZATION_USER |

Nguồn seed: `config/demo-users.json`; có thể đổi code/name trong dữ liệu demo, không cần sửa scheduler. GPU thuộc đơn vị qua Server. User thường chỉ xem và gửi Job trong đơn vị của mình.

Nếu account cũ có mật khẩu khác, script dừng; chỉ lab được phép chạy `python scripts/demo.py up --skip-build --set-demo-passwords` để chuyển các account seed cùng role/ownership sang mật khẩu demo. Script không tự đổi role, ownership hoặc bật lại account bị khóa. ADMIN cũ cần có credentials hợp lệ trong `.env` để thực hiện thay đổi. Không đưa enrollment token thật vào file này.
