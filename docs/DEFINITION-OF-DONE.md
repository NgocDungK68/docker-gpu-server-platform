# Definition of Done — audit tiếp nối

Đọc AGENTS.md root/frontend, README hai project, source và test hiện có trước khi sửa tiếp. Workspace root và hai project đều không có .git, nên git diff/status không cung cấp được diff với implementation trước. Không tạo Git repository hoặc giả định phần nào đã commit.

## Trạng thái tại thời điểm tiếp tục

| DoD | Đã có / đã kiểm tra trước khi sửa tiếp | Còn thiếu lúc audit |
|---|---|---|
| Backend Control Plane | Auth, inventory, jobs, queue, 4 policies, atomic reserve/stop/ACK/reconcile, durable snapshot | Final race/build, targeted safety tests, live restart verification |
| Agent | Shared runner/executor/collector; real Docker/NVML adapters, credential/state; external protection | Một số test numeric/unknown telemetry; hardware/readiness public presentation |
| Simulator | NVIDIA shared library đã build, A1004+T4 2 hoạt động với External0 | Kiểm tra đủ unknown/unhealthy profile và docs multi-host |
| Frontend | Đã có Dashboard/servers/GPU/containers/jobs/queue/form/detail, typed BFF, actual scheduling explanation | Server table/readiness, actual events, browser E2E, final build |
| Documentation | AGENTS.md và architecture audit đã có; README cũ còn lệnh simulator trước refactor | README root chưa có; security/config/scope/API chưa đồng bộ |

Không viết lại các flows đã có. Phần tiếp nối tập trung đóng các gap trong cột cuối.

## Trạng thái cuối

Lần đối chiếu tiếp theo theo yêu cầu Windows/WSL: backend/race/NVML, frontend 9 unit tests + 2 Edge E2E tests và API contract đã qua; không viết lại các phần đó. Bổ sung docs/WINDOWS-WSL.md, scripts/doctor.py và xác nhận Console trong root Compose để hoàn tất đường test trên laptop.

| Nhóm DoD gốc | Kết quả |
|---|---|
| CP chạy, register/heartbeat, server/GPU/Docker inventory | Đạt trong demo HTTP + cùng Agent core/NVML mock; real Docker adapter có tests |
| Existing workload distinction / occupied protection | Đạt; External ID/start time giữ nguyên qua acceptance/restart |
| Scheduler / queue / atomic reservation | Đạt; 4 policy, deterministic tie/filter, priority FIFO, concurrent no double allocation |
| Managed lifecycle / reconciliation / Agent failure | Đạt trên simulator và adapter/integration tests; F/G/CP restart qua |
| Config/security/tests | Có typed config, auth boundaries, strict validation, public DTO redaction và tests; chi tiết SECURITY/CONFIGURATION |
| Simulator NVIDIA mock, không GPU JSON, >=1 host, multi-host, existing workload | Đạt; 2 host demo + 4 profile tests; docs và Compose scenarios |
| Dashboard, server list/detail, GPU, External/Managed, Jobs, Queue, Submit/detail | Đã nối public API thật; unit/build/browser kiểm chứng |
| README VN, architecture, folder structure, API/security/config/simulation/scheduler/traceability/run/test | Có README root + docs liên kết, OpenAPI 0.3 và README hai project đã cập nhật |
| Production GPU/CUDA | Chưa kiểm chứng do môi trường không có NVIDIA GPU runtime; không quy kết thành kết quả simulator |

Các giới hạn thực tế còn lại nằm trong README root. "Đạt" ở đây chỉ phạm vi MVP và mức kiểm chứng nêu rõ, không phải chứng nhận production HA/security.

Đường laptop đã hoàn tất: Windows và Ubuntu WSL2 preflight PASS; root Compose đủ 4 service; 2 Edge E2E tests qua Console Docker PASS; acceptance chạy từ Ubuntu WSL2 qua localhost PASS. Hướng dẫn copy/paste tại [Windows/WSL](WINDOWS-WSL.md), kết quả tại [Validation](VALIDATION.md).
