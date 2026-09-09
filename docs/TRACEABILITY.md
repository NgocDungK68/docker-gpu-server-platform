# Scope traceability

Backend = AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane. Frontend = AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console. Path internal/cmd/api dưới đây thuộc backend; src/tests thuộc frontend.

| Scope | Implementation | Main files | Test / demo |
|---|---|---|---|
| Agent registration | Stable MachineID, AgentID = Server.ID, token riêng | application/controlplane.go; agent/runner.go; agent/state/file.go | integration/agent_controlplane_test.go; acceptance A |
| Heartbeat / fresh inventory | CP receive time; heartbeat và inventory đều cập nhật liveness; drained được giữ khi offline | domain/model.go; application/controlplane.go; store/memory/store.go | memory/safety_test.go; recovery F/G |
| Hardware discovery | Hostname/OS/arch/CPU/RAM/Docker version | agent/hardware.go; inventory.go; dockerengine | Browser Server detail; NVML profile tests |
| GPU discovery | Same NVML adapter real/mock | agent/gpu/nvml_linux.go; cmd/aiwm-agent-sim | gpu/nvml_mock_test.go, Dockerfile.test |
| Existing workloads | Docker grants + PID/cgroup; không adopt/stop | agent/inventory.go; agent/cgroup_linux.go; store/memory/accounting.go | inventory_test.go; engine_test.go; acceptance B |
| Unknown/unhealthy protection | Telemetry thresholds, ECC/MIG blocked, unknown grants | agent/gpu; inventory.go; memory/accounting.go | inventory_safety_test.go; accounting_safety_test.go; four NVML profiles |
| Policy-driven allocation | TRAINING/INFERENCE, lexicographic queue, demo facts boundary | internal/policy; domain/allocation.go; application/allocation.go | policy/engine_test.go; application/allocation_test.go |
| Preview + capability | Không reserve; re-evaluate; profile/FP8 | capability/catalog.go; domain/allocation.go; httpapi/server.go | capability/catalog_test.go; httpapi/allocation_test.go |
| Frontend intent | Catalog backend, limits động, panel không K/score | job-form.tsx; allocation-preview.tsx; form-schema.ts; api-client.ts | 10 contract tests; lint/typecheck PASS |
| Scheduler policies | Filter/score/select riêng, 4 strategy | application/scheduler.go | scheduler_test.go; scheduler_safety_test.go; cmd/aiwm-scheduler-bench |
| Atomic reservation | Validate all UUIDs, commit job/command/resources under lock | store/memory/store.go; ports/repository.go; store/durable/store.go | memory/safety_test.go concurrent40, invalid UUID, failed launch |
| Queue/Priority | Priority ↓ / createdAt ↑ / ID ↑; reason | store/memory/store.go; application/controlplane.go | memory/safety_test.go; acceptance D/E |
| Managed lifecycle | Typed start/stop, exact UUID, ownership | agent/executor.go; agent/dockerengine/engine.go; store/memory/lifecycle.go | engine_test.go; integration test; acceptance C/E |
| Command retry / idempotency | Lease + processed results + one ACK effect | agent/runner.go; agent/state; memory/lifecycle.go | runner_test.go; memory/safety_test.go |
| Reconciliation | Inventory transaction xử lý active/exited/missing; release logical reservation khi terminal | store/memory/lifecycle.go; application/controlplane.go | memory/safety_test.go; integration test; recovery.py |
| Persistence/restart | Snapshot v1, process lock, rollback, stale on restart | store/durable; store/memory/snapshot.go | durable/store_test.go; recovery.py |
| Security | Separate enrollment/Agent/public tokens; whitelist BFF | httpapi/security.go; application/validation.go; src/lib/api/proxy-policy.ts | security_test.go; contracts.test.mjs; console.spec.ts |
| Input/model contract | Strict JSON, limits, env redaction, typed UI | httpapi/job_view.go; application/validation.go; api/openapi.yaml; src/lib/api/types.ts | security_test.go; frontend contracts; live BFF acceptance |
| Dashboard | Pool counts, capacity, actual job events | src/app/(console)/page.tsx; features/dashboard | Playwright live navigation, summary API |
| Server list/detail | Table, hardware, readiness, GPUs/containers/drain | src/app/(console)/servers | Playwright browser inventory |
| GPU/Container inventory | Filters and External/Managed badges | src/app/(console)/gpus; containers | acceptance B; Playwright |
| Jobs/Queue/Submit/detail | Form intent, preview, Necessity, policy reason, Assignment/events, stop/cancel | src/app/(console)/workloads; queue; components/workloads | 9 contract tests; Playwright create→run→stop |
| Simulation | NVIDIA mock library; shared core; persisted Docker runtime | Dockerfile.sim; internal/simulator; deploy/simulation | Compose2 hosts, 4 NVML profiles |
| Configuration | Typed backend/Agent, Next server/client config | internal/config; agent/config; simulator/config; src/config | Startup validation; README and CONFIGURATION |
| Domain model / states | Entities thực tế, bốn state machines, GPU semantics, ownership và test gaps | docs/DOMAIN_MODEL.md; internal/domain/model.go; store/memory | Source/enum/link audit; tests cụ thể trong Domain Model |
| API audit / Postman | 21 backend endpoints, auth, prerequisites, API → scope → source | docs/API_TESTING_POSTMAN.md; postman; internal/httpapi/server.go | Task allocation: Newman 25 requests/51 assertions PASS; 195 assertions là kiểm chứng trước contract intent |
| Architecture/docs | VN README root, API/security/config/scope mapping | README.md; docs; backend/api/openapi.yaml | Link/spec validation and final build |

Nguồn chính xác cho các file adapter có thể xem bằng rg --files internal/agent; bảng trỏ cả package khi trách nhiệm nằm ở nhiều file.

## Domain model và state semantics

[DOMAIN_MODEL.md](DOMAIN_MODEL.md) bổ sung mapping entity/state → implementation → test, gồm guards và các nhánh chưa có test riêng. AgentID chính là Server.ID; Workload trên UI dùng Job; reservation là Job.Assignment. Release là kết thúc logical reservation, không mặc định GPU FREE. Các tests trong bảng không chứng minh mọi interleaving start/stop; nhánh STOPPING + missing khi Start còn DELIVERED được ghi rõ là giới hạn của code hiện tại.

Runbook và collection: [API testing bằng Postman](API_TESTING_POSTMAN.md). Bảng endpoint phân biệt 16 public (gồm probes) và 5 Agent/internal; không có login/refresh API. Test protocol manual không chứng minh Docker/NVML discovery hoặc CUDA execution.

## Kiểm chứng môi trường

Đã chạy trên Windows host + Docker Desktop Linux: Go fmt/vet/test/build, Linux go test -race ./..., bốn NVIDIA mock profiles, Next lint/typecheck/unit tests/production build, acceptance A–H với CP thật và Next BFF, browser test với Edge.

Chưa chạy trên máy có GPU NVIDIA thật; không đánh dấu CUDA execution hoặc real-GPU runtime survival là đã chứng minh. SQL target migration không được apply vì chưa có PostgreSQL adapter.

## Kiểm chứng task Policy-driven Allocation

Focused Go application/policy/capability/httpapi/memory/durable/config/integration PASS; vet và build bốn cmd roots PASS. Frontend 10 contract tests/lint/typecheck PASS. Newman folder Allocation: CP riêng Windows + inventory fixture, 25 requests/51 assertions/0 failures. Không chạy lại Docker/full E2E/production frontend build; kiểm chứng môi trường ở trên thuộc implementation trước. [IMPLEMENTATION_STATUS](IMPLEMENTATION_STATUS.md) ghi trạng thái hiện tại.

Limitation: planning/quota demo, catalog chưa chứng nhận hiệu năng, chưa hẹn start/TTL enforcement. Demo scripts/browser fixture đã đổi body nhưng chưa chạy lại full acceptance.
