# AIWM workspace

Workspace gồm hai project hiện có:
- Backend Go: `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`.
- Frontend Next.js: `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console`.

AIWM gom các Docker GPU server độc lập thành logical resource pool. Không dùng Kubernetes, MIG, time-sharing, preemption hay GPU sharing. Chỉ cấp phát nguyên GPU vật lý.

## Ràng buộc kiến trúc

- External/LEGACY containers chỉ được inventory; tuyệt đối không tự stop, restart, delete hoặc adopt.
- Agent và Control Plane mất kết nối không được ảnh hưởng runtime của container.
- Scheduler dùng inventory mới, loại trừ GPU unhealthy/unknown/external; reserve + job + command phải atomic.
- Giữ bốn composition roots trong `cmd`; simulator dùng cùng Agent core và cùng NVML adapter qua NVIDIA mock shared library. Chỉ Docker runtime mô phỏng được thay ở boundary.
- Business logic ở domain/application/agent core; Docker/NVML/HTTP/persistence là adapter.
- Frontend chỉ gọi public API qua BFF; không proxy Agent/Docker endpoints.
- Cấu hình/env tập trung, không hardcode secret hoặc ghi sensitive environment vào log.
- Khi đổi API/domain, cập nhật OpenAPI, frontend types/client, README và scope traceability.

## Kiểm tra

Backend: `go fmt ./...`, `go vet ./...`, `go test ./...`, `go build ./cmd/...`; chạy race và NVML integration trên Linux có CGO/GCC.
Frontend: đọc hướng dẫn Next.js được chỉ định bởi AGENTS.md của frontend; `npm run lint`, `npm run typecheck`, `npm test`, `npm run build`.

Giữ package nhỏ, doc comment ngắn cho exported/core functions, test hành vi và các race quan trọng. Không rewrite project hoặc thay framework/database không có lý do. README ở workspace root là hướng dẫn tiếng Việt cho run/demo/test xuyên suốt cả hai project.
