# AIWM — Docker GPU Agent & Scheduler

## Demo hiện tại — 6 server, 26 GPU

Tại workspace root: **`python scripts/demo.py up`** (PowerShell) hoặc **`bash scripts/demo-up.sh`** (WSL). Image đã build và source không đổi thì thêm `--skip-build`.

Mở **http://127.0.0.1:3000/login**. User: `admin`, `vtt`, `vds`, `vtnet`, `vtit`; password demo: `AIWM-Demo-2026!`. **Chỉ development/demo, không production.** PostgreSQL demo dùng port **15432**, giữ nguyên PostgreSQL Windows tại 5432.

Kiểm tra: `python scripts/demo.py check`; dừng và giữ dữ liệu: `python scripts/demo.py down`. Xem [TESTING_RUNBOOK](docs/TESTING_RUNBOOK.md) cho lệnh chi tiết, failure tests và trạng thái real deployment; [DEMO_ACCOUNTS](docs/DEMO_ACCOUNTS.md) cho tài khoản. Đây là đường chạy được kiểm chứng thay cho các bước demo thủ công lịch sử bên dưới.

AIWM gom các Docker GPU server độc lập thành một **logical GPU resource pool**. Agent quan sát Docker/NVML, Control Plane nhận job và cấp phát nguyên GPU vật lý, Console hiển thị inventory và vòng đời job.

Điểm cốt lõi là **đưa server đang có workload vào hệ thống mà không dừng workload đó**. Container External chỉ được quan sát; AIWM không tự stop/restart/delete/adopt. Mất Agent hoặc Control Plane không phải lý do dừng container. Không triển khai Kubernetes, MIG, GPU sharing, time-slicing, checkpoint hay preemption.

## Chạy/demo trên Windows hoặc Linux

Đường chạy hiện tại cần **PostgreSQL metadata → migration → bootstrap admin → login → enrollment riêng từng server → Agent**. Xem [TESTING_RUNBOOK](docs/TESTING_RUNBOOK.md) để chạy đầy đủ ba mức: backend/frontend local, fake GPU E2E trên laptop, GPU Linux thật và thử mất kết nối.

Khởi đầu tại workspace root (PowerShell; đây là hướng dẫn để tự chạy, task này chưa chạy các lệnh):

~~~powershell
python scripts/configure.py
docker compose -p aiwm-org-demo up -d postgres
docker compose -p aiwm-org-demo logs --tail 30 postgres
~~~

Sau khi PostgreSQL sẵn sàng, trong backend:

~~~powershell
go mod tidy
$env:AIWM_STATE_FILE = ".local/control-plane-org.gob"
go run ./cmd/aiwm-server --migrate
go run ./cmd/aiwm-server --bootstrap-admin
go run ./cmd/aiwm-server
~~~

Trong frontend, terminal riêng: npm.cmd ci, rồi npm.cmd run dev -- --hostname 127.0.0.1. Mở http://127.0.0.1:3000/login bằng account bootstrap trong .env. Không public self-registration. Các lệnh Compose Agent, fake NVML, cấu hình .env.agent và kiểm tra Linux --check nằm trong runbook; không chạy Agent trước khi cấp enrollment token.

scripts/configure.py giữ giá trị cũ, sinh credentials local khi thiếu; không in secret. AIWM_API_TOKEN cũ không xác thực production. Các script doctor.py, acceptance.py, recovery.py và browser fixtures cũ chưa chuyển sang session/enrollment; kết quả trước task không chứng nhận phiên bản tenancy này.

## Demo các chức năng trong scope

1. ADMIN tạo organization/user; VTT, VTNET, VDS chỉ là metadata có thể thay đổi. Login user từng đơn vị để kiểm tra context.
2. Tạo enrollment cho server; Agent tự discovery GPUs. Không nhập GPU thủ công. A100 mock có 4 GPUs, GPU 0 External; T4 mock có 2 GPUs. Số liệu user thấy tùy ownership enrollment.
3. Dashboard/Servers/GPUs/Jobs scope tại backend. ADMIN có bộ lọc organization; normal user không được đổi scope bằng query/body.
4. Preview/submit workload TRAINING hoặc INFERENCE: khai báo nhu cầu, không chọn server/GPU/strategy. Job chỉ placement trong organization của account; ADMIN submit cũng dùng home organization.
5. Stop Job do AIWM quản lý; inventory terminal → release; External giữ nguyên. FREE chỉ khi không còn blocker.
6. Dừng/restart riêng Agent hoặc CP theo runbook: Docker workload tiếp tục, server stale bị chặn placement, reconnect cần FULL inventory mới.

Simulator chỉ mô phỏng Docker runtime; không thực thi payload/CUDA. GPU snapshot đi qua cùng NVML adapter với production bằng NVIDIA mock shared library.

## Kiến trúc và các project

~~~mermaid
flowchart LR
    Browser[Console React] --> BFF[Next.js public API proxy]
    BFF --> CP[Go Control Plane]
    CP --> State[Durable runtime snapshot + process lock]
    CP --> Meta[PostgreSQL metadata: Organization / User / Enrollment]
    A[Agent A] -->|register / heartbeat / inventory / poll / ACK| CP
    B[Agent B] -->|outbound HTTP hoặc HTTPS| CP
    A --> DA[Docker Engine local socket]
    A --> NA[NVIDIA NVML]
    B --> DB[Docker runtime mô phỏng]
    B --> NB[Cùng NVML adapter + NVIDIA nvml-mock]
~~~

| Đường dẫn | Trách nhiệm |
|---|---|
| AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane | Backend Go hiện có; bốn binaries trong cùng module |
| backend/cmd/aiwm-server | Ghép API, repository, scheduler và reconcile loop |
| backend/cmd/aiwm-agent | Agent production dùng Docker Engine SDK và NVML |
| backend/cmd/aiwm-agent-sim | Ghép cùng Agent core với mock Docker runtime; GPU vẫn qua NVML |
| backend/cmd/aiwm-scheduler-bench | Thí nghiệm deterministic dùng scheduler production |
| backend/internal/domain, application, ports | State, use cases, filtering/scoring và repository boundary |
| backend/internal/store/postgres | Organization, User, session, enrollment và server ownership; không lưu runtime Jobs/inventory |
| backend/internal/store/memory, durable | Atomic transaction trong mutex; lưu snapshot bền qua process restart |
| backend/internal/agent | Inventory correlation, command policy, runner; Docker/NVML/HTTP là adapters |
| backend/internal/agentprotocol/v1, httpapi, api | Wire DTO Agent, public API/DTO, OpenAPI |
| frontend/src/app, components, features | Routes, UI tái sử dụng, hooks/query cache |
| frontend/src/lib/api, config | Typed API client, BFF whitelist, cấu hình server/client |
| compose.yaml, scripts, docs | Chạy nhiều server, acceptance và tài liệu xuyên suốt |

Trong bảng, backend/frontend là tên viết gọn cho hai project ở trên, không phải thư mục mới.

## Domain và runtime flow

- **Organization/User:** PostgreSQL giữ account, role và ownership. Job.organizationId phải bằng Server.organizationId và không rỗng; GPU ownership suy ra từ Server.
- **Server/Agent:** Agent là process, dùng AgentID = Server.ID; chưa có entity hay status Agent riêng. Server giữ machine ID ổn định, token hash, connectivity và drain. Scheduling yêu cầu liveness và inventory mới; inventory được chấp nhận cũng cập nhật LastHeartbeatAt.
- **GPU:** UUID vật lý, model, VRAM, utilization, health, state, consumer và assignedJobId. FREE chỉ là điều kiện cần; server cũng phải schedulable.
- **Container:** actual state + origin MANAGED/LEGACY/UNKNOWN, GPU UUIDs, job ID, start/finish/exit code. UI gọi LEGACY là External.
- **Job / Workload trên UI:** image, command/env, resource constraints, workloadType, Necessity, systemImportance, policy assessment, status và events. Job.Status kết hợp tiến độ điều khiển với kết quả quan sát; chưa có DesiredState/ActualState riêng hoặc entity Workload thứ hai.
- **Reservation:** nằm trong Job.Assignment, chuyển RESERVED → ALLOCATED → RELEASED hoặc RESERVED → RELEASED; giữ server/UUID/start-command/score/reason. RELEASED là kết thúc logical reservation, GPU chỉ FREE nếu accounting không còn điều kiện chặn.

Agent đăng ký bằng enrollment token, nhận credential riêng và lưu identity/sequence/command results. Mỗi heartbeat dùng thời gian nhận ở Control Plane. Inventory đầy đủ ghép Docker grants, NVML processes và PID/cgroup; phần không xác định được bị bảo vệ. Docker event kích hoạt inventory sớm bên cạnh polling.

Submit tạo job QUEUED. Scheduler lọc cùng organization trước GPU/capability → chấm điểm → chọn; repository kiểm tra lại dưới lock và commit reservation + job + command cùng giao dịch. Agent recheck occupancy, thực hiện typed start/stop, ghi kết quả trước ACK. ACK start thành công chưa chứng minh RUNNING; inventory active mới xác nhận đường start. Reconciliation xử lý exit/missing theo Job.Status và ACK failure theo loại command; các guards và giới hạn thứ tự start/stop được ghi trong Domain Model.

Queue theo policy lane → Necessity → auxiliary priority → CreatedAt/ID; user không chọn priority/strategy/server. Job thiếu tài nguyên vẫn chờ với lý do. Offline giữ last-known inventory và reservation; không tự reschedule workload đang chạy sang host khác.

## Tài liệu

| Document | Purpose |
|---|---|
| [SYSTEM_DESIGN.md](docs/SYSTEM_DESIGN.md) | Kiến trúc canonical, ownership, onboarding, failure/reconnect và provisioning. |
| [ALGORITHMS.md](docs/ALGORITHMS.md) | Policy, queue, organization hard filter, placement và reservation theo source. |
| [DOMAIN_MODEL.md](docs/DOMAIN_MODEL.md) | Entities, quan hệ, state và invariant của domain. |
| [TECH_STACK.md](docs/TECH_STACK.md) | Công nghệ thực dùng và nhiệm vụ từng tầng. |
| [CORPORATE_POLICY.md](docs/CORPORATE_POLICY.md) | Policy baseline của project, Necessity và các hệ số ưu tiên. |
| [API_TESTING_POSTMAN.md](docs/API_TESTING_POSTMAN.md) | Contract auth/tenancy/enrollment và Postman. |
| [TESTING_RUNBOOK.md](docs/TESTING_RUNBOOK.md) | Lệnh chạy local, fake GPU, GPU Linux thật và failure tests. |
| [DEMO_ACCOUNTS.md](docs/DEMO_ACCOUNTS.md) | User/password mẫu và đơn vị để đăng nhập demo. |
| [IMPLEMENTATION_STATUS.md](docs/IMPLEMENTATION_STATUS.md) | DONE/PARTIAL/TODO, checkpoint và bước tiếp tục. |

Tài liệu audit/validation cũ được giữ khi còn thông tin lịch sử; canonical docs ở trên là điểm vào cho phiên bản hiện tại.

## Policy-driven GPU Allocation

User chọn nhu cầu, Policy Engine xếp thứ tự phục vụ, Scheduler chọn GPU/server. Create API bỏ `priority`, `strategy`, `serverSelector`, `resources.gpuModel`; client cũ gửi chúng nhận 400. Xem [input/policy và extension boundaries](docs/SCHEDULER.md).

Form lấy choices từ backend và có **ĐỐI CHIẾU TỰ ĐỘNG**: quy hoạch, quota, tính cần thiết, GPU phù hợp, lý do chờ. Preview chưa giữ GPU; Submit evaluate lại vào queue chung. Strategy do `AIWM_SCHEDULER_STRATEGY` quản lý.

Quy hoạch/quota hiện là cấu hình demo deterministic, chưa tự accounting; `neededAt` chưa hẹn start, `ttlSeconds` chưa tự stop/reclaim. [CONFIGURATION](docs/CONFIGURATION.md) hướng dẫn đổi demo facts. Windows test được policy/API bằng CP native không cần GPU; thêm Agent Sim Linux/Docker Desktop để có inventory/lifecycle. WSL là tùy chọn.

Nếu chạy image cũ, rebuild **control-plane** và restart frontend sau thay đổi này. Wire DTO Agent vẫn v1; enrollment nay bind server/organization/machine. [IMPLEMENTATION_STATUS](docs/IMPLEMENTATION_STATUS.md) phân biệt focused checks mới với Docker/E2E kiểm chứng trước đó.

## Kiểm thử API bằng Postman

Xem [API audit, authentication và runbook Windows](docs/API_TESTING_POSTMAN.md). Import [Postman collection](postman/AIWM.postman_collection.json) và [local environment](postman/AIWM.local.postman_environment.json), điền username/password rồi gửi folder Organization - Luồng chính; Login tự lưu access_token. Environment mẫu không chứa secret; Agent/Internal dùng CP test riêng theo runbook.

## API frontend sử dụng

Browser gọi cùng origin /api/aiwm/*; Next.js chuyển sang /api/v1/* và chuyển session từ cookie HttpOnly thành bearer ở server. Các response thành công có data; lỗi có error.code, error.message và error.fields khi validation thất bại.

| Method | Endpoint tại Control Plane | Caller | Purpose | Request | Response data |
|---|---|---|---|---|---|
| GET | /healthz, /readyz | Health probe; BFF chỉ gọi /healthz | Liveness/readiness process | — | status |
| GET | /api/v1/system/summary | Dashboard | Tổng pool + recent events | — | ClusterSummary |
| GET | /api/v1/servers | Servers, Dashboard, Onboarding | Server inventory | — | Server[] |
| GET | /api/v1/servers/{id} | Server detail | Host/GPU/containers + freshness | — | Server |
| POST | /api/v1/servers/{id}/drain | Server detail | Chặn/mở placement mới | drained boolean | Server |
| GET | /api/v1/gpus | GPU inventory, Dashboard | GPU toàn pool | — | GPUInventoryItem[] |
| GET | /api/v1/containers | Containers | External/Managed inventory | origin query tùy chọn | ContainerInventoryItem[] |
| GET | /api/v1/jobs/options | Create form | Catalog labels/reasons/profiles/limits | — | AllocationOptions |
| POST | /api/v1/jobs/preview | Create form | Đối chiếu, không reserve | CreateJobRequest | JobPreview |
| POST | /api/v1/jobs | Submit Job | Re-evaluate, tạo QUEUED | CreateJobRequest | Job, HTTP 201 |
| GET | /api/v1/jobs | Jobs, Scheduler, Dashboard | Jobs và quyết định scheduling | — | Job[] |
| GET | /api/v1/jobs/{id} | Job detail | Lifecycle/reservation/reason | — | Job |
| POST | /api/v1/jobs/{id}/stop | Job detail | Hủy queued hoặc stop managed | — | Job, HTTP 202 |
| GET | /api/v1/queue | Queue | Priority/FIFO + pending reasons | — | Job[] |
| POST | /api/v1/scheduler/run-once | Queue operator | Chạy một cycle | — | assignedJobs |
| POST | /api/v1/agents/register | Agent, không qua BFF | Enrollment | RegisterRequest + X-Enrollment-Token | agentId/token/heartbeat interval |
| POST | /api/v1/agents/{id}/heartbeat | Agent token | Connectivity | observedAt | Server |
| PUT | /api/v1/agents/{id}/inventory | Agent token | Full actual snapshot | InventoryReport | Server |
| GET | /api/v1/agents/{id}/commands | Agent token | Lease command | limit query | Command[] |
| POST | /api/v1/agents/{id}/commands/{commandId}/ack | Agent token | Ghi kết quả idempotent | succeeded/message/containerId | Command |

Các route auth/me/logout, organizations/users và enrollments được mô tả đầy đủ trong OpenAPI/Postman. Public resources lấy organization từ identity; ADMIN có query filter, scheduler/run-once chỉ dành ADMIN.

Giữ endpoint /stop hiện có cho cả cancellation, tránh thêm API trùng. Server detail đã chứa GPU/containers nên không cần endpoint con mới.
[OpenAPI](AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml) là contract chi tiết. Ví dụ request trong backend/examples/requests/create-job.json.

## Kiểm thử và từng binary

Xem [TESTING_RUNBOOK](docs/TESTING_RUNBOOK.md) cho exact commands, điều kiện và kết quả mong đợi; không chạy đồng thời CP native và Compose cùng port/state. Bốn entrypoint vẫn là aiwm-server, aiwm-agent, aiwm-agent-sim, aiwm-scheduler-bench. Production Agent nhắm Linux + Docker + NVIDIA Driver/NVML + runtime nvidia; fake Agent dùng Linux và mock library.

Task tenancy chỉ được kiểm tra tĩnh; chưa chạy dependency resolution, migration, build, tests hoặc GPU thật. Fixture cũ có ownership rỗng/token chung cần chuyển trước khi dùng để chứng nhận tenancy.

## Bảo mật và cấu hình

Public API dùng session nội bộ TTL 8 giờ, lưu hash tại PostgreSQL; BFF giữ cookie HttpOnly/SameSite Strict. ADMIN quản lý metadata và xem nhiều đơn vị; ORGANIZATION_USER chỉ thao tác trong đơn vị mình. Enrollment token dành riêng từng server, Agent token chỉ dùng protocol nội bộ; BFF không proxy Agent/Docker routes.

Không expose Docker socket qua TCP; không cho Job arbitrary mount/privileged/sharing. Environment trong public Job được che giá trị. TLS có config ở CP; Agent có custom CA/client cert. Đặt cookie Secure khi frontend chạy HTTPS.

| Cấu hình chính | Nơi đọc | Ý nghĩa |
|---|---|---|
| AIWM_DATABASE_URL | backend/internal/config | DSN PostgreSQL metadata, bắt buộc; không hardcode credential |
| AIWM_BOOTSTRAP_ORGANIZATION_CODE/NAME, AIWM_BOOTSTRAP_USERNAME/PASSWORD | backend/internal/config | CLI bootstrap khi chưa có user |
| AIWM_HTTP_ADDR, AIWM_STATE_FILE | backend/internal/config | 127.0.0.1:8080; runtime snapshot một writer |
| AIWM_SCHEDULER_STRATEGY | backend/internal/config | best-fit mặc định, giữ bốn strategy |
| AIWM_AGENT_OFFLINE_AFTER | backend/internal/config | 20s mặc định; kết hợp freshness inventory |
| AIWM_CONTROL_PLANE_URL, AIWM_AGENT_STATE_FILE, AIWM_ENROLLMENT_TOKEN | backend/internal/agent/config | Endpoint, state riêng mỗi host, enrollment đã bind Organization |
| MOCK_NVML_CONFIG, LD_LIBRARY_PATH | NVIDIA mock/loader | Profile YAML và shared library cho simulator |
| AIWM_API_BASE_URL, AIWM_SESSION_COOKIE_SECURE | frontend/src/config/server.ts | CP origin và cookie HTTPS |
| NEXT_PUBLIC_REFRESH_INTERVAL_MS | frontend/src/config/app.ts | 5000ms |

## Giới hạn hiện tại

PostgreSQL chỉ giữ metadata/auth/ownership; Job/Assignment/Command/inventory vẫn memory + durable gob snapshot với process lock. Hai store chưa có distributed transaction; enrollment bind có thể retry cùng machine nếu runtime upsert lỗi. Không có HA/leader election, tự chuyển ownership dữ liệu runtime cũ hoặc quota ledger theo organization.

Policy giữ lane → Necessity → auxiliary → CreatedAt/ID; planning/quota dùng DEVELOPMENT_CONFIG. NeededAt chưa là lịch start; TTL chưa tự stop/reclaim. CPU/RAM truyền Docker limits, chưa admission-control tổng CPU/RAM. Không preemption/MIG/time-sharing.

Preflight kiểm tra điều kiện nền tảng, không chứng nhận mọi CUDA image. Simulator không thực thi payload. Các kết quả validation lịch sử thuộc task trước; xem status/runbook để phân biệt phần implement với phần chưa chạy kiểm chứng.
