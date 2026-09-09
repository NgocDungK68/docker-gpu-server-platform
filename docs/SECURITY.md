# Security architecture

| Concern | Implementation | Source chính | Test |
|---|---|---|---|
| Enrollment | X-Enrollment-Token, constant-time compare, không default secret | application/controlplane.go; config/config.go | httpapi/security_test.go |
| Agent identity | Random per-agent token, lưu hash ở CP; token gắn agentID | store/memory/store.go; agent/state/file.go | HTTP integration, Agent authorization |
| Public API | Bearer operator token riêng; /healthz và /readyz công khai | httpapi/middleware.go; cmd/aiwm-server/main.go | security_test.go |
| Browser boundary | Chỉ public route/method whitelist, token thêm server-side, mutation Origin check | frontend/src/lib/api/proxy-policy.ts; app/api/aiwm/[...path]/route.ts | contracts.test.mjs, console.spec.ts |
| Payload | Strict JSON, 1 MiB, tên/image/resource/env/selector giới hạn | httpapi/server.go; application/validation.go | security_test.go |
| GPU isolation | Exact physical UUID, local occupancy recheck, từ chối GPU visibility override | agent/executor.go; inventory.go; dockerengine | inventory_safety_test.go; engine_test.go |
| Docker operations | Local unix/npipe endpoint; không host mount/privileged/arbitrary Docker request; ownership labels bắt buộc | agent/config/config.go; dockerengine | safe defaults, legacy stop refusal |
| Retry/replay | Lease, processed command persistence, idempotent ACK, inventory sequence | agent/runner.go; store/memory/lifecycle.go | runner_test.go; safety_test.go |
| Secret exposure | Env lấy từ file/process; public Job DTO redaction, generic internal errors, không body/token logs | config packages; httpapi/job_view.go; middleware/server | secret redaction, generic500 |
| Transport | Optional TLS cert/key CP; Agent TLS >=1.2 + CA/client cert | cmd/aiwm-server; agent/controlplane/client.go | Compile; chưa live TLS/mTLS integration |

Các đường dẫn backend tính từ Go project; đường dẫn frontend tính từ Next.js project. Public Job DTO ở internal/httpapi/job_view.go; bearer middleware ở internal/httpapi/security.go.

Enrollment credential cho phép đăng ký/rotate identity theo machine ID: chỉ cấp cho operator/host đáng tin. Không chia sẻ state Agent giữa hai máy. Docker socket tương đương quyền root; dùng service account riêng và file mode hạn chế. Labels không phải cryptographic attestation: người có quyền Docker có thể sửa labels, nằm ngoài trust boundary của MVP.

Console chưa có đăng nhập hoặc RBAC. BFF dùng shared operator token nên người truy cập Console có quyền operator. Root Compose bind loopback; deployment nhiều người cần authenticated gateway và HTTPS. Origin check bảo vệ trình duyệt trước cross-origin mutation, không thay thế xác thực người dùng.

Public API không cung cấp route để stop External container. Chỉ /jobs/{id}/stop ánh xạ tới managed command; Agent kiểm tra ownership/job ID trước Docker stop. Agent/CP shutdown không chạy cleanup kill. Atomic reservation chỉ chống double allocation bên trong AIWM; operator Docker bên ngoài vẫn có thể khởi chạy workload sau lần kiểm tra occupancy. Cần quy trình quản lý GPU grant ngoài AIWM để loại bỏ hoàn toàn race này.

TLS trực tiếp ở CP bảo vệ transport. Client certificate của Agent chỉ được xác thực đầy đủ nếu gateway/server phía trước yêu cầu và xác minh mTLS; CP hiện chưa tự cấu hình client-CA enforcement. Không bật insecure certificate bypass.

Snapshot chứa job environment và command payload để Agent chạy đúng job; bảo vệ disk/backups tương tự secrets. Public DTO che env không có nghĩa disk đã mã hóa. Image registry credentials/allowlist, encryption at rest, audit retention và multi-tenant RBAC chưa có trong MVP.
