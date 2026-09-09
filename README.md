# AIWM — Docker GPU Agent & Scheduler

AIWM gom các Docker GPU server độc lập thành một **logical GPU resource pool**. Agent quan sát Docker/NVML, Control Plane nhận job và cấp phát nguyên GPU vật lý, Console hiển thị inventory và vòng đời job.

Điểm cốt lõi là **đưa server đang có workload vào hệ thống mà không dừng workload đó**. Container External chỉ được quan sát; AIWM không tự stop/restart/delete/adopt. Mất Agent hoặc Control Plane không phải lý do dừng container. Không triển khai Kubernetes, MIG, GPU sharing, time-slicing, checkpoint hay preemption.

## Chạy nhanh toàn bộ demo

**Đang dùng laptop Windows:** xem [hướng dẫn Windows/WSL](docs/WINDOWS-WSL.md). Đường chạy Docker Desktop đã được kiểm chứng; có thể test ngay trong PowerShell, WSL là tùy chọn. Chạy python scripts/doctor.py sau configure để kiểm tra công cụ/cấu hình mà không lộ token.

Thực hiện từ thư mục chứa README này. Cần Docker Desktop đang chạy Linux containers, Python 3.10+, Node.js 24 nếu chạy frontend trên host. Lần build đầu cần mạng để tải image/dependency và NVIDIA nvml-mock.

**Terminal 1 — Control Plane + hai Agent Sim**

~~~powershell
python scripts/configure.py
docker compose up --build -d control-plane agent-a100 agent-t4
docker compose ps
docker compose logs --tail 30 control-plane agent-a100 agent-t4
~~~

Script configure tạo token ngẫu nhiên và đồng bộ các file cấu hình cho demo. Script giữ giá trị đã tồn tại, không in secret. Nếu đã tự cấu hình khác nhau, cần đồng bộ AIWM_API_TOKEN và AIWM_ENROLLMENT_TOKEN giữa các project.

Sau khoảng 10 giây, API tại http://localhost:8080/healthz trả OK. Hai server xuất hiện:

| Server | GPU từ NVML | Workload ban đầu | GPU cấp phát được |
|---|---|---|---|
| fake-server-a100, site=hanoi | 4 × NVIDIA A100-SXM4-40GB | External trên GPU 0 | GPU 1–3 |
| fake-server-t4, site=hcm | 2 × Tesla T4 | Không có | GPU 0–1 |

**Terminal 2 — Frontend**

~~~powershell
cd AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console
npm.cmd ci
npm.cmd run dev -- --hostname 127.0.0.1
~~~

Mở http://127.0.0.1:3000. Trên Linux/macOS dùng npm thay npm.cmd và python3 thay python nếu cần.

Có thể chạy frontend bằng Docker thay Terminal 2:

~~~powershell
docker compose --profile console up --build -d
~~~

Không chạy hai frontend đồng thời trên port 3000. Build production trên host: npm.cmd run check rồi npm.cmd run start -- --hostname 127.0.0.1.

**Terminal 3 — kiểm thử demo tự động, tại thư mục root**

~~~powershell
python scripts/acceptance.py
python scripts/recovery.py
python scripts/acceptance.py --base-url http://127.0.0.1:3000/api/aiwm
~~~

Chạy lần lượt vì các script dùng chung GPU pool. Acceptance tự tạo và dừng/hủy đúng jobs của lần chạy đó. Recovery stop/start riêng dịch vụ demo agent-a100 và control-plane rồi khôi phục; không dùng script này trên deployment production.

Dừng demo: docker compose --profile console down. Các volume state vẫn được giữ để restart. Không thêm -v nếu muốn giữ jobs, identity và runtime mô phỏng.

## Demo từng chức năng

1. **Dashboard:** kiểm tra 2 server online, 6 GPU tổng, 5 GPU sẵn sàng, 1 GPU External; các con số thay đổi theo jobs đang chạy.
2. **Máy chủ:** mở fake-server-a100, xem hardware, Docker version, heartbeat, thời điểm nhận inventory, labels, GPU và container existing-inference-external-0.
3. **GPU inventory / Containers:** GPU 0 A100 là OCCUPIED_LEGACY, container có badge External và không có nút stop.
4. **Tạo workload:** giữ alpine:3.21, command ba dòng sh / -c / sleep 300; chọn TRAINING, 3 GPU, 1024 MiB/GPU, profile A100-equivalent, không yêu cầu FP8; chọn Cần thiết 2, lý do go-live dưới 90 ngày, mức Quan trọng, thời điểm cần và thời lượng 1 giờ. Bấm Đối chiếu tự động rồi Gửi vào hàng đợi. Luồng thường là QUEUED → ASSIGNED → STARTING → RUNNING; có thể bỏ qua STARTING nếu inventory đến trước ACK. Xem UUID, score, explanation và reservation trong Job detail.
5. **Queue:** tạo thêm job 1 GPU với profile A100-equivalent. Khi job 3 GPU vẫn chạy, job mới QUEUED cùng pending reason.
6. **Giải phóng:** dừng job 3 GPU, chờ STOPPED và reservation RELEASED. Job 1 GPU tự chạy. Hủy job QUEUED bằng nút Huỷ job.
7. **Drain:** drain server trong Server detail; scheduling dừng nhận placement mới, container đang chạy giữ nguyên. Bỏ drain để dùng tiếp khi heartbeat/inventory còn mới.
8. **Agent lost/reconnect:** chạy scripts/recovery.py. Server offline sau timeout, job mới chờ; Agent trở lại thì inventory/reconciliation phục hồi, không duplicate container.
9. **Bốn strategy:** trang Scheduler hiển thị quyết định thật của các jobs. Benchmark CLI bên dưới chạy cùng scheduler trên một fixture cố định.

Simulator mô phỏng **Docker runtime**, không thực thi image, lệnh sleep hay CUDA. GPU inventory thực sự đi qua NVIDIA mock shared library và cùng NVML adapter với production Agent. Vì vậy demo chứng minh điều phối và reconciliation, không chứng minh tính toán CUDA hoặc hiệu năng GPU.

## Kiến trúc và các project

~~~mermaid
flowchart LR
    Browser[Console React] --> BFF[Next.js public API proxy]
    BFF --> CP[Go Control Plane]
    CP --> State[Durable snapshot + process lock]
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
| backend/internal/store/memory, durable | Atomic transaction trong mutex; lưu snapshot bền qua process restart |
| backend/internal/agent | Inventory correlation, command policy, runner; Docker/NVML/HTTP là adapters |
| backend/internal/agentprotocol/v1, httpapi, api | Wire DTO Agent, public API/DTO, OpenAPI |
| frontend/src/app, components, features | Routes, UI tái sử dụng, hooks/query cache |
| frontend/src/lib/api, config | Typed API client, BFF whitelist, cấu hình server/client |
| compose.yaml, scripts, docs | Chạy nhiều server, acceptance và tài liệu xuyên suốt |

Trong bảng, backend/frontend là tên viết gọn cho hai project ở trên, không phải thư mục mới.

## Domain và runtime flow

- **Server/Agent:** Agent là process, dùng AgentID = Server.ID; chưa có entity hay status Agent riêng. Server giữ machine ID ổn định, token hash, connectivity và drain. Scheduling yêu cầu liveness và inventory mới; inventory được chấp nhận cũng cập nhật LastHeartbeatAt.
- **GPU:** UUID vật lý, model, VRAM, utilization, health, state, consumer và assignedJobId. FREE chỉ là điều kiện cần; server cũng phải schedulable.
- **Container:** actual state + origin MANAGED/LEGACY/UNKNOWN, GPU UUIDs, job ID, start/finish/exit code. UI gọi LEGACY là External.
- **Job / Workload trên UI:** image, command/env, resource constraints, workloadType, Necessity, systemImportance, policy assessment, status và events. Job.Status kết hợp tiến độ điều khiển với kết quả quan sát; chưa có DesiredState/ActualState riêng hoặc entity Workload thứ hai.
- **Reservation:** nằm trong Job.Assignment, chuyển RESERVED → ALLOCATED → RELEASED hoặc RESERVED → RELEASED; giữ server/UUID/start-command/score/reason. RELEASED là kết thúc logical reservation, GPU chỉ FREE nếu accounting không còn điều kiện chặn.

Agent đăng ký bằng enrollment token, nhận credential riêng và lưu identity/sequence/command results. Mỗi heartbeat dùng thời gian nhận ở Control Plane. Inventory đầy đủ ghép Docker grants, NVML processes và PID/cgroup; phần không xác định được bị bảo vệ. Docker event kích hoạt inventory sớm bên cạnh polling.

Submit tạo job QUEUED. Scheduler lọc → chấm điểm → chọn; repository kiểm tra lại dưới lock và commit reservation + job + command cùng giao dịch. Agent recheck occupancy, thực hiện typed start/stop, ghi kết quả trước ACK. ACK start thành công chưa chứng minh RUNNING; inventory active mới xác nhận đường start. Reconciliation xử lý exit/missing theo Job.Status và ACK failure theo loại command; các guards và giới hạn thứ tự start/stop được ghi trong Domain Model.

Queue theo policy lane → Necessity → auxiliary priority → CreatedAt/ID; user không chọn priority/strategy/server. Job thiếu tài nguyên vẫn chờ với lý do. Offline giữ last-known inventory và reservation; không tự reschedule workload đang chạy sang host khác.

Xem [Domain Model: entities, relationships, states và invariants](docs/DOMAIN_MODEL.md), [kiến trúc chi tiết](docs/ARCHITECTURE.md), [scheduler](docs/SCHEDULER.md) và [scope → source → test](docs/TRACEABILITY.md).

## Policy-driven GPU Allocation

User chọn nhu cầu, Policy Engine xếp thứ tự phục vụ, Scheduler chọn GPU/server. Create API bỏ `priority`, `strategy`, `serverSelector`, `resources.gpuModel`; client cũ gửi chúng nhận 400. Xem [input/policy và extension boundaries](docs/SCHEDULER.md).

Form lấy choices từ backend và có **ĐỐI CHIẾU TỰ ĐỘNG**: quy hoạch, quota, tính cần thiết, GPU phù hợp, lý do chờ. Preview chưa giữ GPU; Submit evaluate lại vào queue chung. Strategy do `AIWM_SCHEDULER_STRATEGY` quản lý.

Quy hoạch/quota hiện là cấu hình demo deterministic, chưa tự accounting; `neededAt` chưa hẹn start, `ttlSeconds` chưa tự stop/reclaim. [CONFIGURATION](docs/CONFIGURATION.md) hướng dẫn đổi demo facts. Windows test được policy/API bằng CP native không cần GPU; thêm Agent Sim Linux/Docker Desktop để có inventory/lifecycle. WSL là tùy chọn.

Nếu chạy image cũ, rebuild **control-plane** và restart frontend sau thay đổi này. Agent protocol không đổi. [IMPLEMENTATION_STATUS](docs/IMPLEMENTATION_STATUS.md) phân biệt focused checks mới với Docker/E2E kiểm chứng trước đó.

## API Testing with Postman

Xem [API audit, authentication và runbook Windows](docs/API_TESTING_POSTMAN.md). Import [Postman collection](postman/AIWM.postman_collection.json) và [local environment](postman/AIWM.local.postman_environment.json), điền AIWM_API_TOKEN vào access_token rồi kiểm tra Health trước. Environment mẫu không chứa secret; Agent/Internal dùng CP test riêng theo runbook.

## API frontend sử dụng

Browser gọi cùng origin /api/aiwm/*; Next.js chuyển sang /api/v1/* và thêm API token ở server. Các response thành công có data; lỗi có error.code, error.message và error.fields khi validation thất bại.

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

Giữ endpoint /stop hiện có cho cả cancellation, tránh thêm API trùng. Server detail đã chứa GPU/containers nên không cần endpoint con mới.
[OpenAPI](AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml) là contract chi tiết. Ví dụ request trong backend/examples/requests/create-job.json.

## Test từng project

**Backend**, chạy trong thư mục Go:

~~~powershell
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/...
go run ./cmd/aiwm-scheduler-bench
~~~

Trên Linux có CGO/GCC, thêm go test -race ./.... Trên Windows, dùng image kiểm thử Linux sau khi build Agent Sim:

~~~powershell
docker build -f Dockerfile.sim -t aiwm-agent-sim:local .
docker build -f Dockerfile.test -t aiwm-tests:local .
~~~

Dockerfile.test chạy vet, race, build cả bốn binaries và integration NVML với bốn YAML: A100 External, T4 empty, unknown process, unhealthy ECC. Fixture Go trong unit test chỉ kiểm tra business logic; simulator chạy thật không lấy GPU từ các fixture này.

**Frontend**, chạy trong thư mục Next.js:

~~~powershell
npm.cmd ci
npm.cmd run check
# Giữ frontend và Compose demo đang chạy, không chạy acceptance đồng thời.
$env:AIWM_BROWSER_CHANNEL="msedge"
npm.cmd run test:e2e
~~~

E2E dùng Edge đã cài trên Windows. Máy khác có thể chạy npx playwright install chromium, bỏ AIWM_BROWSER_CHANNEL rồi npm run test:e2e. Các test trình duyệt dùng backend thật: inventory, create, placement, stop, External preservation, BFF authorization boundary; xem screenshots/trace trong test-results nếu lỗi.

**Agent production trên Linux:** xem [hướng dẫn triển khai](AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/docs/agent-deployment.md). Cần Docker/NVIDIA driver/Container Toolkit sẵn có, GCC và Go 1.24+. Build CGO_ENABLED=1, thiết lập .env.agent, chạy --check trước foreground/systemd. Preflight chỉ inventory, không register hoặc tạo/dừng container.

## Chạy riêng từng binary

Control Plane native: chạy python scripts/configure.py từ root một lần; trong backend chạy go run ./cmd/aiwm-server. Không chạy đồng thời CP native và Compose trên port 8080. State native mặc định .local/control-plane.gob.

Agent Sim cần Linux và NVIDIA mock shared library. Không còn các flag --gpu-count/--legacy-gpus sinh GPU trong Go. Cách hỗ trợ đầy đủ là root Compose; muốn chạy riêng một fake server: docker compose up -d control-plane agent-a100. Cấu hình thêm host/profile trong [Simulation](docs/SIMULATION.md).

Benchmark không cần Docker/Agent/credential: go run ./cmd/aiwm-scheduler-bench trong backend. Kết quả fixture: First Fit xếp 4/5 jobs; ba strategy còn lại xếp 5/5. Đây là một trace minh họa, không phải cam kết policy nào luôn tốt hơn.

## Security và configuration

Hai token có mục đích khác nhau: AIWM_ENROLLMENT_TOKEN cho đăng ký Agent, AIWM_API_TOKEN cho public/operator API. Agent nhận token riêng cho machine đã đăng ký. Token frontend chỉ nằm ở Next.js server; BFF chặn Agent/Docker routes và cross-origin mutation.

Không expose Docker TCP socket; Agent dùng local socket. Job không cho arbitrary mount, privileged, sharing hoặc sửa NVIDIA visibility env. Job API che giá trị environment. TLS có thể bật ở CP; Agent hỗ trợ custom CA và client cert để ghép gateway mTLS. Console MVP dành cho operator trong môi trường tin cậy, chưa có user login/RBAC.

| Cấu hình chính | Nơi đọc | Mặc định / yêu cầu |
|---|---|---|
| AIWM_API_TOKEN, AIWM_ENROLLMENT_TOKEN | backend/internal/config; Agent config | Bắt buộc, không có secret mặc định |
| AIWM_HTTP_ADDR, AIWM_STATE_FILE | backend/internal/config | 127.0.0.1:8080; .local/control-plane.gob |
| AIWM_SCHEDULER_STRATEGY | backend/internal/config | best-fit; hỗ trợ bốn policy |
| AIWM_AGENT_OFFLINE_AFTER | backend/internal/config | 20s; lớn hơn heartbeat |
| AIWM_CONTROL_PLANE_URL, AIWM_AGENT_STATE_FILE | backend/internal/agent/config | localhost:8080; file identity riêng mỗi host |
| MOCK_NVML_CONFIG, LD_LIBRARY_PATH | NVIDIA mock/loader | YAML profile + shared library, Dockerfile/Compose thiết lập |
| AIWM_API_BASE_URL, AIWM_API_TOKEN | frontend/src/config/server.ts | Backend origin; token server-side |
| NEXT_PUBLIC_REFRESH_INTERVAL_MS | frontend/src/config/app.ts | 5000ms |

[Bảng cấu hình đầy đủ](docs/CONFIGURATION.md) gồm default, precedence, TLS và simulator. [Security mapping](docs/SECURITY.md) chỉ rõ source/test cho từng boundary.

## Trạng thái hoàn thành và giới hạn kiểm chứng

Xem [Definition of Done](docs/DEFINITION-OF-DONE.md) để phân biệt phần đã có khi tiếp tục và phần bổ sung sau audit. Các chức năng MVP chạy được với backend thật và NVML mock; README này là entry point chạy/demo/test cho cả hai project.

[Kết quả validation thực tế](docs/VALIDATION.md) ghi rõ các lệnh đã qua trên Windows, Linux container và các phần chưa kiểm chứng.

[Báo cáo implementation](docs/IMPLEMENTATION-REPORT.md) tổng hợp 12 mục bàn giao và các nhóm file được tạo/cập nhật.

Production Agent đã build và kiểm tra Docker SDK bằng HTTP adapter tests. Môi trường hiện tại chưa kiểm chứng NVIDIA GPU thật, Container Toolkit, CUDA hay tính liên tục của process CUDA khi Agent chết. Simulator giữ runtime state nhưng không thực thi container payload.

Persistence hiện là snapshot version 1 cho một Control Plane process, có exclusive lock, rollback khi ghi lỗi, restore và yêu cầu inventory mới sau restart. Chưa có PostgreSQL adapter/HA/leader election; SQL trong migrations là target schema tham khảo, không được thực thi khi startup. Chưa benchmark độ bền trước power loss hoặc tải lớn, chưa có retention/compaction cho jobs và snapshots.

CPU/RAM được gửi thành giới hạn Docker; scheduler MVP lọc GPU/VRAM/labels, chưa admission-control tổng CPU/RAM. Image registry auth/allowlist, user RBAC, audit trail bền vững và retry budget nâng cao còn ngoài bản triển khai này.

