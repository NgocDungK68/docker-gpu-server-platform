# Validation thực tế

Kiểm tra trong workspace Windows F:\Viettel\VDT\demo-project, với Docker Desktop Linux containers và Microsoft Edge. Đọc cùng [DoD](DEFINITION-OF-DONE.md), [Windows/WSL](WINDOWS-WSL.md) và [README](../README.md).

| Check đã chạy | Kết quả |
|---|---|
| Go fmt / vet / test ./... | PASS |
| Go build ./cmd/... trên Windows | PASS |
| Dockerfile.test: go vet, go test -race ./..., go build ./cmd/... trên Linux/CGO | PASS |
| Production NVML adapter + NVIDIA mock A100 External | PASS |
| Production NVML adapter + NVIDIA mock T4 empty | PASS |
| Production NVML adapter + NVIDIA mock unknown process | PASS |
| Production NVML adapter + NVIDIA mock unhealthy ECC | PASS |
| Scheduler benchmark, 4 policies cùng fixture | PASS, FirstFit 4/5 jobs, ba policy còn lại 5/5 |
| scripts/acceptance.py: HTTP A–E/H | PASS |
| scripts/recovery.py: F/G và CP restart | PASS |
| Acceptance qua Next BFF trên Windows | PASS |
| Frontend lint / typecheck / 9 Node contract tests / production build | PASS |
| Playwright Edge với frontend production trên Windows | 2 PASS |
| Playwright Edge với frontend chạy trong Compose | 2 PASS |
| Dockerfile frontend build production | PASS |
| Root Compose: CP + A100 Agent + T4 Agent + Console | Cả 4 dịch vụ Running |
| scripts/doctor.py trên Windows | PASS công cụ/Docker Linux/Compose/config consistency |
| scripts/doctor.py trong Ubuntu WSL2 | PASS cùng Docker engine/config; Node Linux chưa có và không cần cho demo Docker |
| scripts/acceptance.py từ Ubuntu WSL2 qua localhost | PASS A–E/H; dùng cùng Docker demo |
| OpenAPI 0.3: local refs, schema validation response live | PASS |
| README/docs local links | PASS |
| Optional compose.scenarios.yaml merged config | PASS; NVML profile behavior đã có integration tests |

## Lỗi được tìm và sửa trong validation

- BFF so sánh Origin với URL nội bộ Next.js khiến submit từ 127.0.0.1 bị HTTP403. Đã so sánh inbound Host, thêm regression test cho localhost/IP/foreign origins; browser create→RUNNING→stop đã qua.
- UI readiness trước đó đếm FREE GPU trên offline host là available. Đã dùng schedulable/freshness của backend và hiển thị lý do chưa sẵn sàng.
- Unit safety bổ sung bảo vệ unattributed memory/utilization và numeric/unknown Docker GPU mapping.
- E2E assertion cũ thiếu tiền tố NVIDIA ở tên GPU và dùng job- thay job_; đã sửa để khớp contract thực.

## Giới hạn

Không có kiểm chứng GPU thật, NVIDIA Container Toolkit/CUDA computation hoặc continuity của CUDA process khi Agent/CP chết. Simulated Docker không chạy payload. Các test restart kiểm chứng state/identity preservation của runtime mô phỏng và backend thật.

TLS/mTLS live deployment, PostgreSQL migrations, HA/leader election, power-loss durability và stress/retention chưa được kiểm chứng. SQL là target model tham khảo, không phải database đang chạy.

Không có .git tại workspace/hai project, nên không thể xác nhận diff/commit baseline; kết quả trên dựa vào source hiện có, output tests và live application.
