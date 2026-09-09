# AIWM Docker GPU Control Plane & Agent

Backend Go cho logical pool các Docker GPU server độc lập. **[README workspace](../../README.md)** là hướng dẫn đầy đủ bằng tiếng Việt để chạy backend, hai fake GPU server, frontend và demo/test A–H.

## Bốn chương trình dùng chung module

| Binary | Chức năng |
|---|---|
| cmd/aiwm-server | Public/Agent HTTP API, inventory, jobs, policy admission/queue, scheduling, atomic reservation và reconciliation |
| cmd/aiwm-agent | Production Agent Linux: Docker Engine local SDK + NVIDIA NVML |
| cmd/aiwm-agent-sim | Cùng Agent core/NVML adapter; NVIDIA mock shared library và simulated DockerRuntime |
| cmd/aiwm-scheduler-bench | So sánh 4 policy bằng scheduler production trên fixture deterministic |

Agent chỉ kết nối outbound. Control Plane không SSH hay truy cập Docker socket từ xa. Container External được inventory, không tự stop/restart/delete/adopt. GPU unknown/unhealthy/đang có external grant không được cấp phát.

## Chạy nhanh

Từ workspace root (lên hai cấp từ đây):

~~~sh
python scripts/configure.py
docker compose up --build -d control-plane agent-a100 agent-t4
python scripts/acceptance.py
python scripts/recovery.py
~~~

Nếu chỉ chạy CP native, sau configure trở lại thư mục Go này:

~~~sh
go run ./cmd/aiwm-server
~~~

CP native đọc .env và lưu .local/control-plane.gob. Không chạy đồng thời với Compose CP cùng port 8080. Public API yêu cầu Bearer AIWM_API_TOKEN; /healthz và /readyz không yêu cầu token.

Agent Sim cần Linux/CGO và mock libnvidia-ml.so; dùng Dockerfile.sim/root Compose trên Windows. Không còn GPU-count flags hoặc GPU JSON adapter. [Simulation](../../docs/SIMULATION.md) giải thích YAML và thêm host.

## Source map

- internal/domain: model/status; internal/application: use cases, filter/score/select.
- internal/ports: repository contract; internal/store/memory: atomic transactions; internal/store/durable: persisted snapshot, process lock và rollback.
- internal/agent: collector/executor/runner và interfaces; subpackages config/controlplane/dockerengine/gpu/state là infrastructure adapters.
- internal/agentprotocol/v1: versioned wire DTO, độc lập với persistence.
- internal/httpapi: auth/error middleware, handlers và public Job DTO che env.
- internal/simulator: chỉ Docker runtime mô phỏng; GPU qua NVIDIA library.
- deploy/simulation/profiles: 4 NVIDIA YAML profiles; deploy/agent: service/env cho Linux.
- api/openapi.yaml: contract 0.3.0; examples: public API request thực chạy.
- migrations: PostgreSQL target schema tham khảo, **không dùng khi startup**.

Snapshot durable version1 thay memory-only runtime trước đó. Restart giữ jobs/reservations/commands nhưng server phải báo inventory mới trước scheduling. Đây là single-process MVP, chưa có PostgreSQL adapter hoặc HA.

## Scheduler và lifecycle

Bốn strategy: first-fit, best-fit, bin-pack, fragmentation-aware. Filter luôn loại stale/offline/drained server, GPU unhealthy/unknown/external/reserved và kiểm tra performanceProfile/FP8/VRAM/count (legacy model/labels chỉ cho Job cũ). Repository revalidate toàn bộ UUID rồi commit job + reservation + command cùng một transaction.

Luồng start thường là QUEUED → ASSIGNED → STARTING → RUNNING; inventory có thể bỏ qua STARTING hoặc xác nhận exit sớm. Terminal states là STOPPED/SUCCEEDED/FAILED/CANCELLED. Hủy queued hoặc Start còn PENDING đưa Job về CANCELLED. Stop ACK thành công chưa release; inventory xử lý exit/absence theo Job.Status. Nhánh STOPPING + absence chưa đợi Start ACK, và failed Stop ACK đưa Job về RUNNING; xem đầy đủ guards/test gaps trong [Domain Model](../../docs/DOMAIN_MODEL.md). Reservation là Job.Assignment; RELEASED chưa chắc GPU FREE. AgentID = Server.ID, Workload trên UI là Job, không có entity/status riêng cho hai tên này. [Scheduler](../../docs/SCHEDULER.md) có công thức, queue semantics và benchmark output.

## Kiểm tra

~~~sh
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/...
go run ./cmd/aiwm-scheduler-bench
~~~

Race và NVIDIA integration trên Linux:

~~~sh
docker build -f Dockerfile.sim -t aiwm-agent-sim:local .
docker build -f Dockerfile.test -t aiwm-tests:local .
~~~

Dockerfile.test chạy race, vet, build bốn binaries và production NVML adapter với A100 External/T4 empty/unknown/unhealthy profiles. Nếu Linux host đã có Go+GCC, có thể chạy go test -race ./... trực tiếp.

Production GPU/CUDA chưa được xác minh trong môi trường demo. Agent production cần Docker + NVIDIA driver/Toolkit có sẵn; làm theo [deployment](docs/agent-deployment.md). Agent --check chỉ quan sát, không thay đổi workload.

## Tài liệu

- [Domain Model](../../docs/DOMAIN_MODEL.md), [Architecture](../../docs/ARCHITECTURE.md), [Definition of Done](../../docs/DEFINITION-OF-DONE.md).
- [API/Screen mapping](docs/frontend-api-map.md), [OpenAPI](api/openapi.yaml), [Agent protocol](docs/agent-protocol.md).
- [Config](../../docs/CONFIGURATION.md), [Security mapping](../../docs/SECURITY.md).
- [Scope traceability](../../docs/TRACEABILITY.md), [test scenarios](docs/test-scenarios.md).

Create Job dùng intent TRAINING/INFERENCE và resource constraints; không nhận strategy/priority/serverSelector/gpuModel. GET /api/v1/jobs/options cung cấp catalog/limits, POST /api/v1/jobs/preview đối chiếu không reserve, POST /api/v1/jobs re-evaluate rồi trả QUEUED. Xem [Policy và Scheduler](../../docs/SCHEDULER.md), [Postman runbook](../../docs/API_TESTING_POSTMAN.md). Quota/planning là demo config, TTL chưa tự stop.
