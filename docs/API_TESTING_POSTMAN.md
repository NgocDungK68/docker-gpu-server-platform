# API audit và kiểm thử bằng Postman

Tài liệu dựa trên routes, handlers, middleware và use cases đang có: **start system → health → cấu hình token → test API → đọc response → tìm source để sửa**. Đã cập nhật contract Policy-driven Allocation: thêm jobs/options, jobs/preview và đổi Create Job sang intent.

## Luồng allocation mới: chạy ngay trên Windows

Nếu đang dùng image CP cũ, chạy lại từ workspace root:

~~~powershell
python scripts/configure.py
docker compose up --build -d control-plane agent-a100 agent-t4
~~~

Chỉ test policy/validation, không cần GPU/inventory: tại thư mục backend chạy `go run ./cmd/aiwm-server` (sau configure, tránh port 8080 đang dùng). Native Windows chạy CP được; production Agent NVML cần Linux. Agent Sim chạy Linux qua Docker Desktop, WSL không bắt buộc.

Import collection/environment, set `base_url=http://localhost:8080` và `access_token` bằng `AIWM_API_TOKEN`. **Không có Login API**; không dùng enrollment/agent token cho public endpoints.

Thứ tự gửi chính xác:

1. `Health - Control Plane`.
2. `Security - Token hợp lệ` trong folder Authentication (hoặc GET Jobs với Bearer, kỳ vọng 200).
3. `Allocation 01 - Options`.
4. `Allocation 02 - Preview TRAINING`.
5. `Allocation 03 - Preview INFERENCE`.
6. `Allocation 04 - Submit TRAINING` (capture `allocation_job_id`).
7. `Allocation 05 - Schedule once`.
8. `Allocation 06 - GET placement status`; gửi lại để xem ASSIGNED/STARTING/RUNNING; đọc `assignment.serverId/gpuUuids` và `statusReason`.
9. `Allocation 07 - Stop Job vừa tạo` khi xem xong; poll GET đến terminal nếu Agent đã dispatch.

Các bước 3–9 nằm trong folder **Allocation - Policy Preview Submit**; chạy cả folder còn có 18 negative validation cases. `allocation_expect_placement=false` mặc định: thiếu GPU vẫn test QUEUED hợp lệ; chỉ set true khi đã chắc đủ inventory. Preview chưa giữ GPU, Submit re-evaluate và vào queue chung. Để submit INFERENCE, dùng chính body Preview INFERENCE gửi POST /jobs; reason CUSTOM đã có explanation.

Defaults: ngoài plan, quota 4/used 0. Đổi demo facts theo [CONFIGURATION](CONFIGURATION.md), recreate CP rồi preview lại để quan sát auto/competitive lanes. Dữ liệu này chưa là quota accounting production. [SCHEDULER](SCHEDULER.md) giải thích policy và giới hạn TTL/neededAt.

## 1. Kết quả audit

- **21 endpoint backend**, tính theo cặp HTTP method + route pattern trong `internal/httpapi/server.go`.
- **16 public endpoints**: 2 health/readiness không auth + 14 business endpoints dùng public bearer.
- **5 Agent/internal endpoints**: register, heartbeat, inventory, command polling, ACK.
- **0 endpoint login/refresh/logout/user session/API-key management**. Register là credential bootstrap cho Agent, thuộc nhóm 5 internal; Security là scope giao nhau, không cộng trùng endpoint.
- Backend phục vụ HTTP JSON, có thể bật HTTPS. Agent gọi outbound protocol v1; không có Agent inbound HTTP server, gRPC, WebSocket, SSE hoặc GraphQL được implement.
- Agent gọi Docker SDK qua local socket/named pipe, NVML qua library; đây là adapter, không phải AIWM public API cho Postman.
- Next.js có `GET /api/aiwm-health` và BFF `GET/POST /api/aiwm/[...path]` whitelist 14 business routes; không cộng vào 21 backend endpoints.
- CORS middleware trả `OPTIONS` 204; framework xử lý method/path mặc định. Không tính chúng như feature endpoints riêng.
- **Workload trên Console = Job**. Actual Container nằm ở `/containers` hoặc `Server.containers`. Không có `/workloads`, `/servers/{id}/workloads`, Container stop/delete, `/reconcile`, policy CRUD. Preview allocation hiện có tại POST /api/v1/jobs/preview.

| API status | Ý nghĩa trong audit |
|---|---|
| READY | Route implement và gọi được với prerequisites đã nêu; không đồng nghĩa production hoàn chỉnh. |
| PARTIAL | Route có thật, nhưng có limitation về contract/semantics/lifecycle cần đọc trước. |
| INTERNAL | Dành cho Agent; Postman chỉ kiểm thử protocol trên CP riêng. |
| DEPRECATED / TODO | Không có endpoint nào được gắn hai status này trong collection; không tạo placeholder API. |

Path `internal/...`, `cmd/...`, `api/...` dưới đây thuộc backend `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`; `src/...` thuộc frontend `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console`.

Tham khảo [Domain Model](DOMAIN_MODEL.md), [Architecture](ARCHITECTURE.md), [Scope traceability](TRACEABILITY.md), [OpenAPI](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml).

## 2. Điều kiện trước khi test API

**Chỉ mở Postman chưa đủ:** Control Plane phải đang chạy tại `base_url`. Postman không tự start CP/Agent/Docker/frontend.

| Mục tiêu | Điều kiện tối thiểu |
|---|---|
| Health/readiness, auth negative tests | CP đang phục vụ HTTP. |
| List Server/GPU/Container/Jobs | CP + token; không có Agent vẫn gọi được, trả empty hoặc last-known data từ snapshot. |
| Submit/inspect/cancel QUEUED Job, Queue, Run once | CP + token. Không có Agent thì Job chưa thực thi. |
| Có inventory demo | CP + Agent Sim; Docker Desktop Linux chạy image chứa Agent core + NVIDIA NVML mock library. DockerRuntime bên trong simulator là mô phỏng. |
| Job RUNNING/STOPPED trong demo | CP + Agent Sim, inventory fresh, GPU phù hợp không bị chiếm. Không cần GPU vật lý trên laptop. |
| Chạy image/container NVIDIA thật | CP + production Agent Linux, Docker Engine, NVIDIA driver/Toolkit, NVML và physical GPU. Native Windows NVML adapter không hỗ trợ. |
| Agent wire fixture bằng Postman | CP riêng + enrollment token; không cần Agent process/Docker/NVML. Chỉ chứng minh protocol/accounting từ JSON fixture, không chứng minh discovery/execution. |
| Frontend BFF | Next + CP; Next có server-side AIWM_API_TOKEN. Backend Postman không cần frontend. |

**Không cần database service:** CP dùng `store/durable`, gob snapshot trên local disk + process lock. `migrations/*.sql` là reference schema; chưa có SQL adapter chạy. State directory cần ghi được; một state file chỉ dùng một CP writer.

## 3. Chạy hệ thống để test Postman

### Recommended demo path — Windows PowerShell + Docker Desktop Linux

Từ workspace root:

~~~powershell
python scripts/configure.py
docker compose up --build -d control-plane agent-a100 agent-t4
docker compose ps
python scripts/doctor.py
~~~

- Docker Desktop phải chạy Linux containers; build lần đầu cần tải images/dependencies/mock library.
- Configure tạo token còn thiếu, giữ giá trị đã có và không in secret.
- `base_url = http://localhost:8080`; **không thêm /api/v1 hoặc trailing slash vào variable**.
- Agent A100: `machineId=sim-a100`, labels `site=hanoi,pool=simulation`, 4 GPU, External grant ở index 0.
- Agent T4: `machineId=sim-t4`, labels `site=hcm,pool=simulation`, 2 GPU.
- Demo sạch có 6 GPU, 1 External, tối đa 5 GPU cấp phát được khi hai Server fresh/undrained. Counts thay đổi theo Jobs/profiles.

### Health check first

~~~powershell
Invoke-RestMethod http://localhost:8080/healthz
Invoke-RestMethod http://localhost:8080/readyz
~~~

Expected: `data.status=ok` và `ready`. Nếu connection refused/timeout, kiểm tra `docker compose ps` và `docker compose logs --tail 30 control-plane agent-a100 agent-t4` trước business API. Health thành công chưa chứng minh Agent/inventory sẵn sàng.

**Frontend tùy chọn:**

~~~powershell
docker compose --profile console up --build -d
~~~

Frontend ở `http://127.0.0.1:3000`. Chỉ bật `enable_frontend_test=true` khi cần folder BFF. Có thể chạy frontend native bằng lệnh trong [README](../README.md); không chạy trùng port 3000.

**Chỉ cần CP để học API cơ bản:**

~~~powershell
python scripts/configure.py
docker compose up --build -d control-plane
~~~

Hoặc chạy `go run ./cmd/aiwm-server` trong backend đã configure. Không chạy cùng port 8080 với Compose; không có bước start DB.

### CP riêng để gọi Agent/Internal thủ công

Mở **PowerShell terminal mới**, từ workspace root:

~~~powershell
cd AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane
$env:AIWM_HTTP_ADDR="127.0.0.1:18080"
$env:AIWM_STATE_FILE=".local/postman-control-plane.gob"
$env:AIWM_AGENT_OFFLINE_AFTER="10m"
go run ./cmd/aiwm-server
~~~

- Dùng binary/config có thật; đọc public/enrollment token từ backend `.env` đã configure, trừ khi process env override.
- `internal_base_url=http://127.0.0.1:18080`, khác `base_url`. Kiểm tra GET `http://127.0.0.1:18080/healthz`.
- `access_token` phải khớp cả hai CP nếu chạy chung collection; `enrollment_token` phải khớp CP riêng.
- Bật `enable_agent_internal=true`, chạy **nguyên folder 08**. Folder tự tạo MachineID, GPU/container fixture và Job/commands riêng.
- Không cho Agent Sim/production dùng CP fixture, không lấy ID/token của Agent đang chạy để gửi report/ACK thủ công.
- Timeout 10m cho thao tác tay; nếu nghỉ lâu, gửi lại heartbeat và inventory sequence mới trước scheduling.
- Ctrl+C dừng CP test. State được giữ, không có DELETE Agent/Job API để reset; dùng state file test mới nếu cần phiên sạch.

WSL không bắt buộc cho đường Docker Desktop. Linux/WSL dùng `python3` và shell env syntax tương ứng; xem [Windows/WSL](WINDOWS-WSL.md).

Các lệnh trên đã đối chiếu `README.md`, `compose.yaml`, `scripts/configure.py`, `scripts/doctor.py`, `cmd/aiwm-server/main.go` và `internal/config/config.go`; không tạo một service/database mới cho runbook.

## 4. Authentication & Security Flow

### Có phải gọi API security trước business API không?

**Không có login endpoint.** Token cấu hình trước startup; Postman gọi thẳng business API:

~~~http
Authorization: Bearer {{access_token}}
~~~

`access_token` là giá trị `AIWM_API_TOKEN`, không phải JWT/OAuth token do login cấp.

| Credential | Lấy ở đâu | Header / validate | Expiry / rotation |
|---|---|---|---|
| Public API token | Root `.env` cho Compose; backend `.env` cho native; `AIWM_API_TOKEN` | `Authorization: Bearer ...`; `security.go::publicAuth/bearerToken`, constant-time compare | Không TTL/refresh. Đổi config + restart CP, đồng bộ Next/Postman. |
| Enrollment token | `AIWM_ENROLLMENT_TOKEN` của CP và Agent | `X-Enrollment-Token: ...` chỉ cho Register; `ControlPlane.RegisterAgent` validate | Không TTL; đổi config/restart. Không thay public bearer. |
| Agent token | `RegisterAgent.data.agentToken`; local Agent state | Bearer + đúng AgentID; `withAgentAuth → AuthenticateAgent → store.AuthenticateAgent` so SHA-256 hash | Không expiry. Re-enroll cùng MachineID đổi token/hash, token cũ mất hiệu lực; không phải expired-token flow. |

- CP startup yêu cầu cả hai env tokens; `internal/config/config.go` load `.env` từ working directory, process env ưu tiên.
- Mở `.env` trong editor, copy **giá trị** AIWM_API_TOKEN vào local/private Postman `access_token`. Không thêm `Bearer ` trong value vì collection tự thêm prefix.
- File environment mẫu không chứa secret thật. Không ghi/export credential thật vào artifacts chia sẻ.
- Business tests không cần đọc state/credential của Agent Sim.
- No-token/invalid-token requests override auth để không thừa kế bearer đúng.
- Không có login/refresh/logout/revoke/token-validation API, users/RBAC/tenant/session. Kiểm tra token bằng GET Jobs.
- `publicAuth` bỏ qua prefix `/api/v1/agents/`; các routes này dùng credential boundary riêng.
- Production `cmd/aiwm-server` truyền `httpapi.Options{PublicAPIToken:...}`. Một số integration tests gọi `httpapi.New` không Options nên thiếu public middleware ở harness; không suy ra production API cho anonymous access.

### Frontend và Agent

~~~text
Browser → Next /api/aiwm/<public-path>
        → BFF đọc AIWM_API_TOKEN phía server
        → CP /api/v1/<public-path> với Bearer token

Agent → Register với X-Enrollment-Token
      → lưu agentId + agentToken
      → Heartbeat / Inventory / Poll / ACK với Agent bearer
~~~

- Browser không có login/token exchange. BFF không validate/forward bearer từ browser; nó dùng token của Next server.
- BFF chỉ whitelist GET/POST public routes; không proxy Agent/Docker.
- POST nếu có Origin header phải khớp inbound Host, sai → 403. Không có Origin (thường Postman) được check cho qua.
- Origin check và backend bearer chưa phải user authentication; Console dùng local/trusted operator hoặc authenticated gateway như README.
- Config Next: `src/config/server.ts`; proxy/auth policy: `src/app/api/aiwm/[...path]/route.ts`, `src/lib/api/proxy-policy.ts`.
- Agent client: `internal/agent/controlplane/client.go`; registration/retry/local history: `internal/agent/runner.go`.
- HTTPS CP bật khi có cert/key. Agent hỗ trợ custom CA/client cert; CP listener hiện không tự enforce mTLS client certificate. HTTP demo không cần TLS credential flow.

## 5. Import collection và environment

1. Import [collection](../postman/AIWM.postman_collection.json) và [local environment](../postman/AIWM.local.postman_environment.json), chọn environment đó.
2. Điền `access_token`, kiểm tra `base_url`, Send Health rồi `Security - Token hợp lệ`.
3. Demo đầy đủ: chạy collection bằng **Collection Runner**, 1 iteration. Không chạy đồng thời acceptance/recovery hoặc collection khác dùng chung pool.
4. Chỉ CP: chọn Health/Security/Jobs/Queue; bỏ các folder cần inventory/E2E.
5. `Send` một request không tự chuyển request kế tiếp. Các bước poll phải Send nhiều lần hoặc dùng Runner; scripts lưu IDs bằng `pm.environment.set` và điều khiển Runner bằng `pm.execution.setNextRequest`. Nguồn: [environment variables](https://learning.postman.com/docs/use/send-requests/variables/environment-variables/), [request order](https://learning.postman.com/docs/tests-and-scripts/running-collections/building-workflows/).
6. Internal/BFF/Drain mặc định skip; chỉ bật variables tương ứng khi đủ prerequisites. Dùng [pm.execution.skipRequest](https://learning.postman.com/docs/tests-and-scripts/write-scripts/postman-sandbox-reference/pm-execution/).
7. Collection theo [schema v2.1](https://schema.postman.com/json/collection/v2.1.0/collection.json). Không cần Postman API key hoặc mock server.

### Variables

| Variable | Giá trị / cách dùng |
|---|---|
| `base_url` | `http://localhost:8080`, CP demo. |
| `access_token` | Trống trong file; nhập AIWM_API_TOKEN. |
| `machine_id` / `server_id` | `sim-a100` để chọn demo host; script capture Server.ID. |
| `job_id` / `container_id` | Capture từ Job/observation; không có `workload_id` vì không có entity đó. |
| `job_image` | `alpine:3.21`; Sim không chạy image, production Agent có chạy. |
| `max_polls` | 60; khoảng 1 giây mỗi vòng chưa đạt. Đây là test setting, không phải API field. |
| `enable_drain_test` / `previous_drained` | Mặc định false; Detail lưu drain cũ, cặp Drain/Restore đổi rồi khôi phục. |
| `internal_base_url` / `enable_agent_internal` | CP riêng port 18080, mặc định không chạy. |
| `enrollment_token` / `agent_id` / `agent_token` | Credential CP fixture; Register capture Agent ID/token. Không dùng Agent Sim identity. |
| `manual_machine_id` / `manual_gpu_uuid` / `manual_external_gpu_uuid` | Fixture IDs tự tạo; không phải hardware được NVML xác minh. |
| `manual_job_id` / `manual_container_id` / `command_id` | Job/command capture từ CP riêng; container ID là fixture do test đặt. |
| `inventory_sequence` / `observed_at` | Counter/time cho reports; replay cố ý giữ sequence để nhận 409. |
| `frontend_url` / `enable_frontend_test` | `http://127.0.0.1:3000`, chỉ bật khi Next chạy. |
| `case_id`, `e2e_*`, `negative_id` | Script tạo counters/preconditions/baseline/ID cho test; không phải domain entity fields. |

Không export environment đã chạy vào file mẫu: runtime environment có thể chứa Agent token mới cấp.

### Tổ chức collection

| Folder | Chạy khi nào |
|---|---|
| 00 - Health | CP là đủ. |
| 01 - Authentication / Security | CP + public token cho positive cases; không có login endpoint. |
| 02 - Dashboard | Agent giúp có inventory; CP độc lập vẫn đọc được. |
| 03 - Servers | Agent demo để có server_id; Drain/Restore tùy chọn. |
| 04 - GPUs | Đọc GPU inventory. |
| 05 - Containers / Existing Workloads | External/Managed observations; không Workload CRUD. |
| 06 - Jobs | Tạo yêu cầu H100/FP8 chưa có trên demo A100/T4 → queue → cancel; cần fixture không có H100 phù hợp. |
| 07 - Queue / Scheduler | Run once xử lý toàn queue, count0 hợp lệ. |
| 08 - Agent Internal APIs | Opt-in, CP riêng; protocol fixtures, reconciliation, failed start, replay/ACK/drain. |
| 09 - Failure / Reconciliation Tests | 400/404/invalid-origin; reconciliation nằm ở Internal/E2E. |
| E2E - Submit Job | Demo có External và GPU free; submit/poll/stop/release/preservation. |
| 10 - Frontend BFF (tùy chọn) | Next + CP; health/proxy/cross-origin rejection. |

## 6. Thứ tự test API từ đầu

~~~text
Start CP → GET /healthz → GET /readyz
→ nhập access_token → GET /api/v1/jobs (200)
→ start Agent Sim / đợi inventory
→ GET /servers → Server schedulable=true
→ GET /gpus + GET /containers?origin=LEGACY
→ POST /jobs → capture job_id
→ POST /scheduler/run-once (tùy chọn)
→ GET /jobs/{job_id} đến RUNNING
→ GET /containers?origin=MANAGED, match jobId
→ POST /jobs/{job_id}/stop
→ GET /jobs/{job_id} đến STOPPED + reservation RELEASED
→ GET /servers/{server_id}, verify UUID đã free
→ GET /containers?origin=LEGACY, verify External còn nguyên
~~~

- Path business viết ngắn ở flow có prefix `/api/v1`.
- Agent Sim **tự register/heartbeat/inventory/poll/ACK**; không gọi Register tay trước E2E Sim.
- Thiếu GPU: submit vẫn **201 QUEUED**, scheduler ghi reason và thử lại; không normal 422.
- Inventory có thể đến trước ACK: ASSIGNED → RUNNING không quan sát STARTING.
- Test bị dừng: giữ job_id, Stop rồi GET đến terminal. Không stop Job khác/External; không có delete API.
- CP có background scheduler/reconciler; không cần gọi run-once liên tục.

### E2E - Submit Job

- E2E 01 kiểm tra demo host fresh, GPU free và active External; lưu External ID/state/start time/grants cùng tập GPU free.
- Create capture job_id; poll RUNNING capture Assignment/server/UUID/container, verify UUID thuộc tập free ban đầu.
- Managed inventory phải có đúng JobID, origin MANAGED, state active.
- Stop nhận 202, poll đến STOPPED + Assignment RELEASED.
- Demo không có Job khác giành GPU vừa release: UUID trở FREE/no assignedJobId.
- External ID/state/startedAt/GPU grants không đổi.
- Poll có giới hạn; start timeout/terminal sớm fail assertion rồi nhảy Stop để cleanup Job vừa tạo. Stop timeout giữ ID để xử lý tiếp.
- Sim chứng minh Agent core/NVML mock/orchestration; không chứng minh image chạy, CUDA hoặc natural workload completion.

### Existing Workload

1. Agent discover/Sim seed External trước enrollment.
2. GET `/containers?origin=LEGACY`: lấy serverId, container.id, state, gpuUuids, startedAt.
3. GET `/servers/{serverID}`: container nằm trong `containers[]`; đây là cách đọc “Server Workloads” hiện tại.
4. Match UUID ở `gpus[]`: active External grant thường cho OCCUPIED_LEGACY, observedConsumers có container ID, stateReason giải thích. Health/unknown có ưu tiên accounting riêng.
5. Phân biệt `Server.schedulable/schedulingReason` (fresh/online/drain) với `GPU.state/stateReason`. Một Server vẫn schedulable khi chỉ một GPU bị External chiếm.
6. Managed chạy/dừng trên UUID khác, External giữ nguyên. Không có adopt/stop/delete External API.

## 7. Contract chung và source traceability

- Business dùng public bearer; Agent dùng Agent bearer; Register dùng enrollment header.
- JSON nên gửi `Content-Type: application/json`, `Accept: application/json`. Decoder không enforce strict Content-Type.
- CP nhận/echo `X-Request-ID` hoặc tự sinh; BFF không forward đầy đủ header này.
- Success bình thường: `{"data": ...}`. Error: `{"error":{"code":"...","message":"..."}}`; không giả định có cả hai hoặc meta.
- Unknown route/method có thể trả text từ framework, không phải mọi response đều JSON envelope.
- Decoder `DisallowUnknownFields`, chỉ một JSON value, body limit 1 MiB khi đọc. “Required” trong OpenAPI không tự được Go decoder enforce, ví dụ `{}` ở drain/ACK.
- Lists chưa pagination; không invent filters ngoài `origin` và `limit` có xử lý trong handler.
- Public JobView che environment values; internal Command payload/ACK response có thể chứa environment gốc.

~~~text
cmd/aiwm-server/main.go → config.Load + durable.Open
→ httpapi.New / net/http ServeMux
→ requestContext → recoverPanic → accessLog → cors → bodyLimit → publicAuth
→ Handler (Agent routes thêm withAgentAuth)
→ application.ControlPlane
→ ports.Repository
→ store/durable wrapper → store/memory transaction
→ gob snapshot nếu mutation
~~~

- Probes không gọi Application/Repository.
- ScheduleOnce → Scheduler.Plan → CommitAssignment; CP không gọi Docker.
- ReportInventory → inventoryToDomain → ReplaceInventory → reconcileObservedLocked + normalizeGPUState.
- Agent outbound: `runner.go → agent/controlplane/client.go`; execution: `CommandExecutor → InventoryCollector.GPUsAvailable → DockerRuntime`.
- `ControlPlane.Reconcile` timer chỉ đánh dấu Server offline; actual Job reconciliation nằm trong inventory transaction.
- DTO/source of truth chi tiết trong [Domain Model](DOMAIN_MODEL.md).

## 8. Public APIs — từng endpoint

### 8.1. `GET /healthz`

- **API status:** READY.

**Mục đích**

- Kiểm tra process HTTP Control Plane đang phục vụ.

**Scope**

- Health / Readiness

**Caller / Authentication**

- Caller: Postman/probe; frontend dùng BFF health.
- Auth: `None.`

**Request**

- URL: `{{base_url}}/healthz`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; {"data":{"status":"ok"}}
- Không kiểm tra Agent/GPU/DB; không có dependency probe trong handler.

**Luồng xử lý**

~~~text
GET /healthz
→ httpapi.Server.health
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `health`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Gửi đầu tiên, Auth = No Auth.
- Kiểm tra tiếp: `GET /readyz`.

**Error thường gặp**

- Connection refused/timeout khi CP chưa phục vụ; handler không probe dependency.

### 8.2. `GET /readyz`

- **API status:** READY.

**Mục đích**

- Kiểm tra handler readiness hiện tại.

**Scope**

- Health / Readiness

**Caller / Authentication**

- Caller: Postman/probe; frontend dùng BFF health.
- Auth: `None.`

**Request**

- URL: `{{base_url}}/readyz`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; {"data":{"status":"ready"}}
- Hiện trả ready cố định khi handler phục vụ; không bảo đảm có inventory hoặc GPU cấp phát được.

**Luồng xử lý**

~~~text
GET /readyz
→ httpapi.Server.ready
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `ready`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Gửi sau health; không cần token.
- Kiểm tra tiếp: `GET /api/v1/servers`.

**Error thường gặp**

- Connection refused/timeout khi CP chưa phục vụ; handler không probe dependency.

### 8.3. `GET /api/v1/system/summary`

- **API status:** PARTIAL.

**Mục đích**

- Đọc tổng hợp logical GPU pool và 20 JobEvent gần nhất.

**Scope**

- Dashboard
- GPU Discovery
- Queue/Priority
- Reconciliation

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/system/summary`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: ClusterSummary. serversTotal/serversOnline, gpusTotal/gpusFree/gpusOccupied/gpusOccupiedLegacy/gpusOccupiedUnknown, jobsQueued/jobsRunning, recentEvents[].
- gpusOccupied đếm State != FREE, kể cả RESERVED/UNHEALTHY; free chỉ đếm trên server schedulable. free + occupied không luôn bằng total; serversOnline có cả DRAINING.

**Luồng xử lý**

~~~text
GET /api/v1/system/summary
→ httpapi.Server.summary
→ ControlPlane.Summary
→ Repository.ListServers; ListJobs
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `summary`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `Summary`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListServers; ListJobs`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Demo sạch có 2 Server, 6 GPU, 1 GPU LEGACY; số free thay đổi theo Job và freshness.
- Kiểm tra tiếp: `GET /api/v1/servers; GET /api/v1/jobs`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.4. `GET /api/v1/servers`

- **API status:** READY.

**Mục đích**

- Liệt kê Server đã enroll cùng GPU/Container snapshot gần nhất.

**Scope**

- Agent Registration
- Heartbeat
- GPU Discovery
- Existing Workload Discovery

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/servers`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: Server[]. id/machineId/name/status/drained, host, lastHeartbeatAt/lastInventoryAt/inventoryReceivedAt, inventoryVersion, schedulable/schedulingReason, gpus[], containers[].
- Sort theo Name; chưa có pagination/filter API. Không có Server DELETE. Có thể trả [] khi chưa có Agent.

**Luồng xử lý**

~~~text
GET /api/v1/servers
→ httpapi.Server.listServers
→ ControlPlane.ListServers → presentServer
→ Repository.ListServers
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `listServers`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `ListServers → presentServer`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListServers`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Chọn machineId=sim-a100 để lưu server_id; chỉ nhìn ONLINE là chưa đủ, phải xem schedulable.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.5. `GET /api/v1/servers/{serverID}`

- **API status:** READY.

**Mục đích**

- Đọc chi tiết một host; đây cũng là cách đọc containers của riêng Server.

**Scope**

- Heartbeat
- GPU Discovery
- Existing Workload Discovery
- External vs Managed Workload

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/servers/{serverID}`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: Server, có gpus[] và containers[] nested.

**Luồng xử lý**

~~~text
GET /api/v1/servers/{serverID}
→ httpapi.Server.getServer
→ ControlPlane.GetServer → presentServer
→ Repository.GetServer
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `getServer`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `GetServer → presentServer`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `GetServer`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Dùng {{server_id}} từ List Servers; xem GPU.StateReason và Server.schedulingReason ở đúng scope.
- Kiểm tra tiếp: `GET /api/v1/gpus; GET /api/v1/containers`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 404 NOT_FOUND: Server ID không có.

### 8.6. `POST /api/v1/servers/{serverID}/drain`

- **API status:** READY.

**Mục đích**

- Bật/tắt việc nhận placement mới của Server; không dừng container.

**Scope**

- Scheduler
- Heartbeat
- Security

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/servers/{serverID}/drain`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body raw JSON (ví dụ khi variables được resolve):

~~~json
{
  "drained": true
}
~~~

**Response / Expected result**

- 200; data: Server. drained=true chặn scheduling; nếu đang offline thì status vẫn OFFLINE.
- {} được decode thành drained=false hiện tại; không dựa vào missing field để coi là validation error.

**Luồng xử lý**

~~~text
POST /api/v1/servers/{serverID}/drain
→ httpapi.Server.drainServer
→ ControlPlane.SetServerDrained → presentServer
→ Repository.SetServerDrained
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `drainServer`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `SetServerDrained → presentServer`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `SetServerDrained`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Gửi true rồi false trong CP test riêng, hoặc chạy cặp Drain/Restore tùy chọn trên demo.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 400: JSON sai/unknown field; 404: Server không tồn tại.

### 8.7. `GET /api/v1/gpus`

- **API status:** PARTIAL.

**Mục đích**

- Liệt kê GPU toàn pool với Server chứa GPU.

**Scope**

- GPU Discovery
- Scheduler
- Atomic Reservation
- Existing Workload Discovery

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/gpus`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: [{serverId, serverName, serverStatus, gpu}]. gpu: uuid/index/model/memoryTotalMiB/memoryUsedMiB/utilizationPct/temperatureC/healthy/state/assignedJobId/observedConsumers/stateReason.
- Không có GPU detail/update endpoint. Flat item chưa có server schedulable/inventoryReceivedAt; FREE + ONLINE chưa chứng minh inventory fresh. Không có serverId/state/model query filter trong handler.

**Luồng xử lý**

~~~text
GET /api/v1/gpus
→ httpapi.Server.listGPUs
→ ControlPlane.GPUs
→ Repository.ListServers
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `listGPUs`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `GPUs`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListServers`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Tìm GPU OCCUPIED_LEGACY và đối chiếu UUID với External Container; UUID chưa được assign phải kiểm tra thêm Server.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.8. `GET /api/v1/containers`

- **API status:** READY.

**Mục đích**

- Đọc Docker container observations, không điều khiển arbitrary container.

**Scope**

- Existing Workload Discovery
- External vs Managed Workload
- Reconciliation

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/containers`; thay route IDs bằng variables được capture.
- Query: origin: optional MANAGED / LEGACY / UNKNOWN; bỏ query để lấy tất cả. Handler uppercase, không trim/validate enum.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: [{serverId, serverName, container}]. Container có id/name/image/state/origin/jobId/gpuUuids/labels/startedAt/finishedAt/exitCode.
- origin=EXTERNAL hoặc literal không biết trả 200 với [] hiện tại, không 400. Không có /workloads, /servers/{id}/workloads hay Container stop/delete API.

**Luồng xử lý**

~~~text
GET /api/v1/containers
→ httpapi.Server.listContainers
→ ControlPlane.Containers
→ Repository.ListServers
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `listContainers`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `Containers`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListServers`.
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- origin=LEGACY để thấy External; origin=MANAGED rồi lọc client theo container.jobId để tìm runtime của Job.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}; GET /api/v1/gpus`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.9. `POST /api/v1/jobs`

- **Purpose:** submit nhu cầu; hệ thống tự chọn physical placement.
- **Scope:** Policy/Admission, Queue, Scheduler, Atomic Reservation, Agent command.
- **Prerequisite:** CP + public bearer. Có Agent/inventory phù hợp mới RUNNING.
- **Input:** JSON ví dụ TRAINING dưới đây; DTO `domain.CreateJobRequest` + `AllocationResources`. INFERENCE đổi workloadType và reason theo `jobs/options`.
- **Policy behavior:** validate → evaluate facts mới → đọc snapshot/Plan → lưu QUEUED. Background/run-once evaluate toàn queue trước commit; không bypass các contenders bằng submit trực tiếp.
- **Expected result:** 201 data.JobView (status QUEUED, id, intent, policy snapshot, necessityLabel). Thiếu GPU vẫn nhận QUEUED, chưa Assignment. Env vẫn [redacted]. Final placement đọc GET Job; có thể khác preview.
- **Source files:** `domain/{dto,allocation}.go`; `application/{allocation,validation,controlplane,scheduler}.go`; `policy/engine.go`; `capability/catalog.go`; `store/memory/store.go`; `httpapi/{server,job_view}.go`.

~~~json
{
  "name": "policy-training-demo",
  "image": "alpine:3.21",
  "backend": "DOCKER",
  "command": [
    "sh",
    "-c",
    "sleep 300"
  ],
  "workloadType": "TRAINING",
  "resources": {
    "gpuCount": 1,
    "minVramMiB": 1024,
    "performanceProfile": "a100-equivalent",
    "fp8Required": false
  },
  "necessityLevel": "NECESSITY_2",
  "necessityReason": "GO_LIVE_90_DAYS",
  "systemImportance": "IMPORTANT",
  "neededAt": "2026-09-09T09:00:00+07:00",
  "ttlSeconds": 3600
}
~~~

Các fields bắt buộc: name/image, workloadType, resources.gpuCount/minVramMiB/performanceProfile/fp8Required, necessityLevel/necessityReason, systemImportance, neededAt, ttlSeconds. CUSTOM cần necessityExplanation. GPU/VRAM phải số nguyên >0; FP8 phải boolean (không null/string), profile hợp lệ. Backend yêu cầu TTL trong giới hạn và RFC3339 có múi giờ. Max GPU/TTL đọc Options, không giả định luôn 64/2592000. neededAt cho phép quá khứ; TTL chưa tự stop.

400 INVALID_INPUT trả `error.fields` theo field, ví dụ:

~~~json
{"error":{"code":"INVALID_INPUT","message":"Dữ liệu yêu cầu chưa hợp lệ","fields":{"resources.gpuCount":"Số GPU phải là số nguyên từ 1 đến 64"}}}
~~~

`priority`, `strategy`, `serverSelector`, `resources.gpuModel`, `inSizingPlan`, `withinQuota` và selected GPU UUID không được nhận; unknown field trả lỗi body. Không có Job PATCH/retry/delete. Queue demo cũ dùng H100/FP8 để chờ trên pool A100/T4, không dùng selector.

### 8.10. `GET /api/v1/jobs`

- **API status:** READY.

**Mục đích**

- Liệt kê Job; Console gọi danh sách này là Workloads.

**Scope**

- Job Management
- Queue/Priority
- Scheduler

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/jobs`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: JobView[]. Specification, status/statusReason, assignment, containerId, lastObservedAt, events; env được che.
- Sort CreatedAt giảm dần; không có pagination, query status/limit không được handler xử lý.

**Luồng xử lý**

~~~text
GET /api/v1/jobs
→ httpapi.Server.listJobs
→ ControlPlane.ListJobs
→ Repository.ListJobs
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `listJobs`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `ListJobs`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListJobs`.
- [internal/httpapi/job_view.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Đọc danh sách trước/sau submit; tự lọc status phía client.
- Kiểm tra tiếp: `GET /api/v1/jobs/{jobID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.11. `GET /api/v1/jobs/{jobID}`

- **API status:** READY.

**Mục đích**

- Đọc lifecycle và decision của Job, làm điểm poll E2E.

**Scope**

- Job Management
- Scheduler
- Atomic Reservation
- Reconciliation

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/jobs/{jobID}`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: JobView. assignment gồm serverId/gpuUuids/commandId/assignedAt/reservationState/releasedAt/strategy/score/reason.
- Không có Job SCHEDULING/RESERVED/DISPATCHING. State hợp lệ: QUEUED, ASSIGNED, STARTING, RUNNING, STOPPING, STOPPED, SUCCEEDED, FAILED, CANCELLED.

**Luồng xử lý**

~~~text
GET /api/v1/jobs/{jobID}
→ httpapi.Server.getJob
→ ControlPlane.GetJob
→ Repository.GetJob
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `getJob`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `GetJob`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `GetJob`.
- [internal/httpapi/job_view.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Gửi lại đến RUNNING khi start; đến STOPPED sau stop. Assignment.commandId là Start command. Nếu FAILED, xem statusReason/events.
- Kiểm tra tiếp: `GET /api/v1/containers?origin=MANAGED; GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 404 NOT_FOUND: Job ID không có.

### 8.12. `POST /api/v1/jobs/{jobID}/stop`

- **API status:** PARTIAL.

**Mục đích**

- Hủy Job chưa dispatch hoặc enqueue Stop đúng managed Job.

**Scope**

- Job Management
- Agent Command/Control
- Atomic Reservation
- Reconciliation
- External vs Managed Workload

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/jobs/{jobID}/stop`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 202; data: JobView. QUEUED hoặc Start còn PENDING → CANCELLED; Job assigned đã delivery → STOPPING; terminal/STOPPING gọi lại không đổi.
- Body không được handler decode (để trống). Stop ACK thành công chưa release. STOPPING + inventory absence có thể terminal trước Start ACK; failed Stop ACK đưa Job về RUNNING. Xem DOMAIN_MODEL.md; không đổi behavior trong audit này.

**Luồng xử lý**

~~~text
POST /api/v1/jobs/{jobID}/stop
→ httpapi.Server.stopJob
→ ControlPlane.StopJob
→ Repository.RequestStop
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `stopJob`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `StopJob`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go); [internal/store/memory/lifecycle.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go) — `RequestStop`.
- [internal/agent/executor.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/executor.go).
- [internal/agent/dockerengine/engine.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/dockerengine/engine.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Chỉ dùng job_id do collection vừa tạo; poll Job và Server sau response, không coi 202 là container đã dừng.
- Kiểm tra tiếp: `GET /api/v1/jobs/{jobID}; GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 404: Job không có; 409 CONFLICT: trạng thái không stoppable (nonterminal không có Assignment ngoài QUEUED) hoặc command conflict.

### 8.13. `GET /api/v1/queue`

- **API status:** READY.

**Mục đích**

- Đọc các Job còn QUEUED cùng lý do chưa được placement.

**Scope**

- Queue/Priority
- Scheduler

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/queue`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: JobView[], chỉ QUEUED. Sort policy lane → Necessity → auxiliary priority → CreatedAt → ID; facts/aging evaluate lại.
- Pending reason là StatusReason, không có status PENDING. Thiếu GPU vẫn QUEUED, không phải HTTP error.

**Luồng xử lý**

~~~text
GET /api/v1/queue
→ httpapi.Server.queue
→ ControlPlane.Queue
→ Repository.ListQueuedJobs
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `queue`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `Queue`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListQueuedJobs`.
- [internal/httpapi/job_view.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Chạy Create queued Job trong folder Jobs rồi đọc Queue trước khi Cancel; không cần Agent để Job vào queue.
- Kiểm tra tiếp: `GET /api/v1/jobs/{jobID}; POST /api/v1/scheduler/run-once`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL nếu repository/storage lỗi; xem request ID và log CP.

### 8.14. `POST /api/v1/scheduler/run-once`

- **API status:** READY.

**Mục đích**

- Chạy một lượt scheduler trên toàn bộ queue; đây là mutation, không phải preview/read API.

**Scope**

- Scheduler
- Queue/Priority
- Atomic Reservation
- Agent Command/Control

**Caller / Authentication**

- Caller: Frontend qua BFF; Postman/manual test.
- Auth: `Bearer {{access_token}} (AIWM_API_TOKEN), Required.`

**Request**

- URL: `{{base_url}}/api/v1/scheduler/run-once`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; {"data":{"assignedJobs":0}} hoặc số commit thành công trong lượt này.
- Body không được decode. Plan không reserve; CommitAssignment revalidate rồi atomically ghi GPU + Assignment + Job + Start command. ErrConflict được refresh/retry ở cycle sau; jobs/options cung cấp catalog/limits; không có API đổi strategy.

**Luồng xử lý**

~~~text
POST /api/v1/scheduler/run-once
→ httpapi.Server.runScheduler
→ ControlPlane.ScheduleOnce → Scheduler.Plan
→ Repository.ListQueuedJobs; ListServers; SetJobStatus; CommitAssignment
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `runScheduler`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `ScheduleOnce → Scheduler.Plan`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `ListQueuedJobs; ListServers; SetJobStatus; CommitAssignment`.
- [internal/application/scheduler.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/domain/dto.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go).

**Cách test / kiểm tra sau API**

- Gửi một lần nếu muốn kích hoạt ngay; count=0 vẫn hợp lệ vì background scheduler có thể đã assign hoặc chưa đủ tài nguyên.
- Kiểm tra tiếp: `GET /api/v1/jobs/{jobID}; GET /api/v1/queue`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 500 INTERNAL: lỗi repository/commit không được xử lý; thiếu tài nguyên bình thường không trả 422.

### 8.15. `GET /api/v1/jobs/options`

- **Purpose:** lấy choices/labels/reasons/profile IDs và limits từ backend.
- **Scope:** Create Request, Policy profile, Capability catalog.
- **Prerequisite:** CP + Authorization Bearer public token; không cần Agent.
- **Input:** không body.
- **Policy behavior:** đọc cấu hình, không evaluate/reserve.
- **Expected result:** 200 data gồm limits.maxGpuCount/maxTtlSeconds, workloadTypes, performanceProfiles, necessityProfiles, systemImportance, customReason. Có 8 necessity profiles theo 2 workloads × 4 cấp; không trả coefficient/model mapping nội bộ.
- **Source files:** `httpapi/server.go::jobOptions` → `application/allocation.go::AllocationOptions` → `policy/profiles.go`, `capability/catalog.go`.

### 8.16. `POST /api/v1/jobs/preview`

- **Purpose:** ĐỐI CHIẾU TỰ ĐỘNG, không giữ tài nguyên.
- **Scope:** Validation, Policy, GPU matching.
- **Prerequisite:** CP + public bearer; thêm Agent Sim cho inventory.
- **Input:** cùng CreateJobRequest ở mục 8.9.
- **Policy behavior:** prepareJob validate/evaluate rồi matchJob đọc snapshot/Plan. Không tạo Job/ID/command/reservation; waiting chưa có Job = 0.
- **Expected result:** 200 data theo ví dụ dưới; thiếu GPU là satisfiable=false, không 422. Invalid input = 400 với error.fields; thiếu/sai token = 401.
- **Source files:** `httpapi/server.go::previewJob` → `application/allocation.go::PreviewJob/prepareJob/matchJob` → policy/capability/Scheduler.Plan.

~~~json
{
  "data": {
    "workloadType": "TRAINING",
    "policyStatus": "COMPETITIVE",
    "policyReason": "Ngoài quy hoạch hoặc vượt hạn mức; xử lý theo thứ tự ưu tiên khi có tài nguyên.",
    "necessityLabel": "Cần thiết 2",
    "sizingPlanStatus": "Ngoài Quy hoạch định cỡ",
    "quotaStatus": "Trong hạn mức hiện tại",
    "policySource": "DEVELOPMENT_CONFIG",
    "resourceMatch": {
      "satisfiable": true,
      "recommendedGpuModels": ["NVIDIA A100-SXM4-40GB"],
      "matchedGpuCount": 3,
      "recommendationText": "Có thể đáp ứng 1 GPU trên một server; cấp phát theo thứ tự hàng đợi."
    },
    "evaluatedAt": "2026-09-09T02:00:00Z"
  }
}
~~~

Ví dụ theo demo A100/T4 trống, quota defaults; số lượng thay đổi theo inventory. matchedGpuCount là maximum trên **một** host fresh, không cộng nhiều host. Preview không trả assignment/score/UUID. Submit kiểm tra lại và placement cuối có thể thay đổi.

## 9. Agent/Internal APIs — CP protocol test riêng

- Fixture JSON ở folder Internal không phải observation được Docker/NVML thật xác minh.
- Đây là protocol Agent, không phải frontend onboarding/user login API. Chạy đầy đủ folder 08 để capture IDs đúng thứ tự.

### 9.1. `POST /api/v1/agents/register`

- **API status:** INTERNAL.

**Mục đích**

- Bootstrap Agent credential và Server identity.

**Scope**

- Agent Registration
- Security

**Caller / Authentication**

- Caller: Agent nội bộ; Postman trên CP protocol riêng. Không dành cho frontend/BFF.
- Auth: `X-Enrollment-Token: {{enrollment_token}}, Required.`

**Request**

- URL: `{{internal_base_url}}/api/v1/agents/register`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body raw JSON (ví dụ khi variables được resolve):

~~~json
{
  "protocolVersion": "v1",
  "machineId": "{{manual_machine_id}}",
  "name": "postman-contract-agent",
  "agentVersion": "postman-manual",
  "labels": {
    "postman": "{{manual_machine_id}}"
  },
  "capabilities": [
    "docker-inventory",
    "nvml-inventory",
    "managed-container-v1"
  ]
}
~~~

**Response / Expected result**

- 201; data: {agentId, agentToken, heartbeatIntervalSeconds}. agentId chính là Server.ID; chỉ response này cấp Agent token.
- Re-enroll cùng machineId giữ Server.ID, đổi Agent token, reset inventory freshness/version. Không phải user login. Capabilities nhận qua DTO nhưng chưa persist/enforce.

**Luồng xử lý**

~~~text
POST /api/v1/agents/register
→ httpapi.Server.registerAgent
→ ControlPlane.RegisterAgent
→ Repository.UpsertServer
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `registerAgent`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `RegisterAgent`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `UpsertServer`.
- [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- Agent outbound caller: [internal/agent/controlplane/client.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go); [internal/agent/runner.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go).

**Cách test / kiểm tra sau API**

- Chạy folder Agent Internal trên internal_base_url của CP riêng; tự tạo machineId bắt đầu postman-manual-. Không lấy identity của Agent Sim đang chạy.
- Kiểm tra tiếp: `POST /api/v1/agents/{agentID}/heartbeat; PUT /api/v1/agents/{agentID}/inventory`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 401: thiếu/sai X-Enrollment-Token; 400: JSON sai, protocolVersion khác v1, machineId/name rỗng.

### 9.2. `POST /api/v1/agents/{agentID}/heartbeat`

- **API status:** INTERNAL.

**Mục đích**

- Cập nhật liveness của Server/Agent.

**Scope**

- Heartbeat
- Security
- Scheduler

**Caller / Authentication**

- Caller: Agent nội bộ; Postman trên CP protocol riêng. Không dành cho frontend/BFF.
- Auth: `Bearer {{agent_token}} của đúng agentID, Required.`

**Request**

- URL: `{{internal_base_url}}/api/v1/agents/{agentID}/heartbeat`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body raw JSON (ví dụ khi variables được resolve):

~~~json
{
  "observedAt": "{{observed_at}}"
}
~~~

**Response / Expected result**

- 200; data: raw domain.Server; lastHeartbeatAt là giờ CP nhận, status ONLINE hoặc DRAINING.
- {} được chấp nhận; field observedAt không được dùng cho freshness. Heartbeat chưa cung cấp inventory; raw response chưa decorate schedulable/schedulingReason.

**Luồng xử lý**

~~~text
POST /api/v1/agents/{agentID}/heartbeat
→ httpapi.Server.heartbeat
→ ControlPlane.Heartbeat
→ Repository.Heartbeat
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `heartbeat`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `Heartbeat`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `Heartbeat`.
- [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- Agent outbound caller: [internal/agent/controlplane/client.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go); [internal/agent/runner.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go).

**Cách test / kiểm tra sau API**

- Gửi với Agent token đã enroll ở CP riêng; đọc Server qua public API để xem schedulable chính xác.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 401: Agent ID/token không khớp; 400: JSON/timestamp sai.

### 9.3. `PUT /api/v1/agents/{agentID}/inventory`

- **API status:** INTERNAL.

**Mục đích**

- Gửi full snapshot, normalize occupancy và reconcile Job trong cùng transaction.

**Scope**

- GPU Discovery
- Existing Workload Discovery
- External vs Managed Workload
- Heartbeat
- Reconciliation
- Atomic Reservation

**Caller / Authentication**

- Caller: Agent nội bộ; Postman trên CP protocol riêng. Không dành cho frontend/BFF.
- Auth: `Bearer {{agent_token}} của đúng agentID, Required.`

**Request**

- URL: `{{internal_base_url}}/api/v1/agents/{agentID}/inventory`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body raw JSON (ví dụ khi variables được resolve):

~~~json
{
  "sequence": 1,
  "observedAt": "{{observed_at}}",
  "host": {
    "hostname": "postman-contract-agent",
    "os": "linux",
    "architecture": "amd64",
    "cpuCount": 1,
    "memoryTotalMiB": 1024
  },
  "dockerVersion": "manual-protocol-fixture",
  "gpus": [
    {
      "uuid": "{{manual_gpu_uuid}}",
      "index": 0,
      "model": "POSTMAN-CONTRACT-ONLY",
      "memoryTotalMiB": 8192,
      "memoryUsedMiB": 0,
      "utilizationPct": 0,
      "healthy": true,
      "state": "FREE"
    }
  ],
  "containers": [],
  "processes": []
}
~~~

**Response / Expected result**

- 200; data: raw Server chứa normalized GPUs/Containers, inventoryVersion, inventoryReceivedAt. Job có thể chuyển RUNNING/terminal cùng transaction.
- Đây là full replacement, không PATCH. Empty containers có thể kết thúc Job theo missing logic. Chỉ gửi khi đã có complete observation; Postman fixture không kiểm chứng Docker/NVML thật. Wire chỉ giữ state OCCUPIED_UNKNOWN trước CP accounting.

**Luồng xử lý**

~~~text
PUT /api/v1/agents/{agentID}/inventory
→ httpapi.Server.inventory
→ ControlPlane.ReportInventory → inventoryToDomain
→ Repository.ReplaceInventory → reconcileObservedLocked → normalizeGPUState
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `inventory`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `ReportInventory → inventoryToDomain`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go); [internal/store/memory/lifecycle.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go) — `ReplaceInventory → reconcileObservedLocked → normalizeGPUState`.
- [internal/application/agent_protocol.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/agent_protocol.go).
- [internal/store/memory/accounting.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/accounting.go).
- [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- Agent outbound caller: [internal/agent/controlplane/client.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go); [internal/agent/runner.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go).

**Cách test / kiểm tra sau API**

- Chỉ fixture CP riêng. Sequence phải tăng; collection có snapshot external-only → managed running → managed exited, rồi replay sequence để test 409.
- Kiểm tra tiếp: `GET /api/v1/servers/{serverID}; GET /api/v1/jobs/{jobID}`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 400: sequence=0, GPU invalid/duplicate trong report, container ID rỗng, PID invalid; 401: credential; 409: sequence cũ hoặc UUID đang nằm ở server khác.

### 9.4. `GET /api/v1/agents/{agentID}/commands`

- **API status:** INTERNAL.

**Mục đích**

- Lease command cho Agent; GET này có side effect.

**Scope**

- Agent Command/Control
- Atomic Reservation
- Reconciliation
- Security

**Caller / Authentication**

- Caller: Agent nội bộ; Postman trên CP protocol riêng. Không dành cho frontend/BFF.
- Auth: `Bearer {{agent_token}} của đúng agentID, Required.`

**Request**

- URL: `{{internal_base_url}}/api/v1/agents/{agentID}/commands`; thay route IDs bằng variables được capture.
- Query: limit optional; <=0/không parse được → 10; >100 → 100. DELIVERED quá LeaseUntil được redelivery.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; data: [{id,type,payload}]. Type START_CONTAINER hoặc STOP_CONTAINER; [] nếu không có command lease được.
- Lease tăng attempts, cập nhật deliveredAt/leaseUntil, không đổi Job.Status. Stop đợi Start không còn PENDING/DELIVERED; server drain/freshness không được kiểm tra lúc delivery command cũ.

**Luồng xử lý**

~~~text
GET /api/v1/agents/{agentID}/commands
→ httpapi.Server.pollCommands
→ ControlPlane.PollCommands
→ Repository.LeaseCommands
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `pollCommands`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `PollCommands`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go) — `LeaseCommands`.
- [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- Agent outbound caller: [internal/agent/controlplane/client.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go); [internal/agent/runner.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go).

**Cách test / kiểm tra sau API**

- Trên CP riêng, tạo Job → run-once → poll, lưu command_id của đúng manual_job_id. Không poll thay Agent đang chạy.
- Kiểm tra tiếp: `POST /api/v1/agents/{agentID}/commands/{commandID}/ack`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 401: credential sai; không có lỗi 400 cho limit không parse được hiện tại.

### 9.5. `POST /api/v1/agents/{agentID}/commands/{commandID}/ack`

- **API status:** INTERNAL.

**Mục đích**

- Ghi kết quả command và áp dụng hiệu ứng Job.

**Scope**

- Agent Command/Control
- Reconciliation
- Atomic Reservation
- Security

**Caller / Authentication**

- Caller: Agent nội bộ; Postman trên CP protocol riêng. Không dành cho frontend/BFF.
- Auth: `Bearer {{agent_token}} của đúng agentID, Required.`

**Request**

- URL: `{{internal_base_url}}/api/v1/agents/{agentID}/commands/{commandID}/ack`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body raw JSON (ví dụ khi variables được resolve):

~~~json
{
  "succeeded": true,
  "message": "manual protocol test only",
  "containerId": "{{manual_container_id}}"
}
~~~

**Response / Expected result**

- 200; data: domain.Command gồm id/agentId/type/status/payload/attempts/timestamps/error. Khác response poll chỉ có 3 fields. Payload Start có thể chứa environment gốc, chỉ dành Agent.
- ACK đầu tiên quyết định, ACK lặp trả kết quả cũ. Chưa bắt buộc command DELIVERED; {} decode succeeded=false. Không có endpoint ACK status/read riêng hoặc token expiry.

**Luồng xử lý**

~~~text
POST /api/v1/agents/{agentID}/commands/{commandID}/ack
→ httpapi.Server.ackCommand
→ ControlPlane.AckCommand
→ Repository.AckCommand → applyAckLocked
~~~

**Source code**

- Route/handler: [internal/httpapi/server.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) — `ackCommand`.
- Application: [internal/application/controlplane.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) — `AckCommand`.
- Repository boundary: [internal/ports/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go); persistence wrapper: [internal/store/durable/repository.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go).
- Transaction: [internal/store/memory/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go); [internal/store/memory/lifecycle.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go) — `AckCommand → applyAckLocked`.
- [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- DTO/model: [internal/domain/model.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go); [internal/agentprotocol/v1/types.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go).
- Agent outbound caller: [internal/agent/controlplane/client.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go); [internal/agent/runner.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go).

**Cách test / kiểm tra sau API**

- ACK Start true → STARTING nếu chưa active; inventory running → RUNNING. ACK Stop true chưa đổi STOPPING; inventory exited mới STOPPED. Collection thử ACK lặp và failed Start.
- Kiểm tra tiếp: `GET /api/v1/jobs/{jobID}; PUT /api/v1/agents/{agentID}/inventory`.

**Error thường gặp**

- 401: thiếu/sai credential của endpoint này.
- 401: Agent credential sai; 404: command không có hoặc thuộc Agent khác; 400: JSON/unknown field sai.

## 10. Frontend HTTP routes bổ sung

### `GET /api/aiwm-health`

- **Status:** READY; Scope: Health / Frontend Connectivity; Caller: frontend hoặc Postman.
- **Auth:** không có browser token.
- **Request:** `{{frontend_url}}/api/aiwm-health`, không body.
- **Response:** HTTP 200, `{"status":"ok"}` nếu CP /healthz HTTP success; `{"status":"unreachable"}` nếu fetch lỗi/non-OK. Không data envelope.
- **Flow/source:** `src/app/api/aiwm-health/route.ts::GET → src/config/server.ts::getServerConfig → fetch(CP /healthz)`.
- **Test:** bật enable_frontend_test, chạy BFF Health. Chỉ HTTP 200 chưa đủ, phải assert body status=ok.
- **Kiểm tra sau:** BFF GET Servers.
- **Error:** handler catch upstream error rồi vẫn 200/unreachable; Next chưa chạy thì connection error.

### `GET/POST /api/aiwm/[...path]`

- **Status:** PARTIAL; chưa có user authentication.
- **Scope:** Frontend API Proxy / Security / 14 business scopes được whitelist.
- **Caller:** Browser/Postman tới frontend; cần Next và CP chạy.
- **Auth:** Next tự lấy AIWM_API_TOKEN phía server. POST nếu có Origin phải khớp Host; không yêu cầu bearer browser.
- **Request:** thay `/api/v1/` của **14 business endpoints** bằng `/api/aiwm/`, giữ method/query/body. Ví dụ GET /api/aiwm/servers, POST /api/aiwm/jobs.
- **Response:** body/status từ CP; own errors có error.code/message. Không proxy probes/Agent/Docker endpoints.
- **Flow/source:** `src/app/api/aiwm/[...path]/route.ts::proxy → proxy-policy.ts::allowedPublicRoute/allowedMutationOrigin → config/server.ts → fetch(CP)`.
- **Test:** BFF GET Servers dùng No Auth nhận 200 khi token Next đúng; POST Jobs với Origin cố ý khác host nhận 403 trước khi tạo Job.
- **Kiểm tra sau:** đọc Jobs để kiểm tra negative POST không tạo Job; xem config Next nếu proxy lỗi.
- **Errors:** 404 ngoài whitelist; 403 Origin sai; 413 body quá lớn; 503 thiếu Next token; 502 network/timeout/config URL/fetch lỗi; có thể forward 400/401/... từ CP.
- Chỉ export GET/POST; method khác do Next xử lý mặc định. Không xem BFF là login/token-validation endpoint.

## 11. Security test cases và error response guide

### Cases trong collection

| Case | Request / expected | Prerequisites |
|---|---|---|
| No token | GET Jobs, No Auth → 401 UNAUTHORIZED | CP |
| Invalid token | GET Jobs, invalid bearer → 401 | CP |
| Valid token | GET Jobs → 200 | access_token đúng |
| Thiếu Bearer prefix | Authorization chỉ chứa token → 401 | CP |
| Public token thay Agent token | Poll ID fixture chưa đăng ký → 401 | CP, không poll Agent Sim |
| Enrollment token sai | Register → 401, không tạo Server | CP |
| Agent credential sai | Heartbeat fixture với token sai → 401 | CP riêng, sau Register |
| Invalid body | POST Jobs unknown field/sharing/GPUCount invalid → 400 | Public token |
| Protected API | GET Jobs no-token so với valid-token | Public token |
| Job ID không có | GET Jobs/{negative_id} → 404 | Public token |
| Stale inventory | Replay sequence đã accepted → 409 | CP riêng/Agent fixture |
| ACK lặp | ACK Stop true rồi false cùng ID vẫn SUCCEEDED | CP riêng/Agent fixture |
| Failed Start | ACK false → Command/Job FAILED, reservation RELEASED | CP riêng/Agent fixture |
| BFF cross-origin | POST Jobs Origin khác Host → 403 | Next |
| Expired token | **Không có case**, vì không implement TTL/JWT expiry | Không invent behavior |

### HTTP codes có trong source

| HTTP code | Meaning in AIWM | Example causes / phạm vi |
|---|---|---|
| 200 | Đọc hoặc xử lý sync thành công | List/detail, drain, scheduler, heartbeat/inventory/poll/ACK. |
| 201 | Tạo record | Job QUEUED; Agent registered, kể cả re-enroll. |
| 202 | Stop/cancel được xử lý/chấp nhận | Có thể CANCELLED/STOPPING hoặc terminal đã có; chưa chứng minh runtime stop xong. |
| 204 | CORS preflight | OPTIONS middleware; không phải health code. |
| 400 | INVALID_INPUT | JSON/unknown fields/Job/protocol/inventory invalid. Business route kiểm tra auth trước decode. |
| 401 | UNAUTHORIZED | Public/enrollment/Agent credential thiếu hoặc không khớp. |
| 403 | FORBIDDEN | **BFF** Origin check; CP không có role-based 403 branch. |
| 404 | NOT_FOUND hoặc router text | Entity không có; ACK command thuộc Agent khác; BFF ngoài whitelist; unknown route có thể text. |
| 405 | Framework method not allowed | Method không có handler; không bảo đảm JSON envelope. |
| 409 | CONFLICT | Stale sequence, global UUID collision, Job không stoppable hoặc exposed repository conflict. Scheduler commit conflict thường được xử lý nội bộ. |
| 413 | BODY_TOO_LARGE | Vượt 1 MiB khi CP/BFF đọc body. Malformed/oversized body có thể 400 tùy lỗi decode trước. |
| 422 | INSUFFICIENT_GPU mapping trong respond | Mapping tồn tại; normal submit/run-once thiếu GPU không trả 422, Job vẫn QUEUED. Không có request expect-422 giả. |
| 500 | INTERNAL | Persistence/disk/panic, client nhận generic message. Không cần cố làm hỏng state file để test. |
| 502 | CONTROL_PLANE_UNREACHABLE | BFF fetch CP thất bại. |
| 503 | CONFIGURATION | BFF thiếu token; CP thiếu token fail startup, không phục vụ 503. |

Normal error ví dụ:

~~~json
{
  "error": {
    "code": "UNAUTHORIZED",
    "message": "valid public API bearer token required"
  }
}
~~~

Assertions ưu tiên HTTP code + error.code; message phụ thuộc branch. Unknown route/method không bắt buộc có JSON envelope.

## 12. API dependency map

`Data`: API vẫn gọi được khi thiếu Agent, nhưng inventory empty/last-known. `Execution`: Agent cần để thực thi, chưa cần lúc submit. Sim dùng Docker Desktop để chạy Agent image; DockerRuntime bên trong mô phỏng. Các path business/Agent bên dưới bỏ prefix `/api/v1` để ngắn gọn.

| API | Control Plane | DB | Agent | Docker | NVML/nvml-mock | Auth |
|---|---|---|---|---|---|---|
| GET /healthz, /readyz | Required | Không | Không | Không nếu CP native | Không | None |
| GET /system/summary | Required | Không; gob local | Data | Cho Agent discovery/execution | Cho GPU discovery | Public bearer |
| GET /servers, /servers/{id} | Required | Không | Data; detail cần record có thật | Cho Agent inventory | Cho GPU inventory | Public bearer |
| POST /servers/{id}/drain | Required | Không | Record có thật, Agent không cần online | Không để mutate drain | Không | Public bearer |
| GET /gpus | Required | Không | Data | Cho attribution container | Có nếu Agent/Sim collect | Public bearer |
| GET /containers | Required | Không | Data | Cho Docker observations; Sim runtime mô phỏng | Collector dùng GPU snapshot | Public bearer |
| POST /jobs | Required | Không | Execution, chưa cần lúc submit | Execution; Sim mô phỏng | Discovery/placement qua Agent | Public bearer |
| GET /jobs, /jobs/{id}, /queue | Required | Không | Không để đọc; giúp cập nhật actual lifecycle | Không để đọc | Không để đọc | Public bearer |
| POST /jobs/{id}/stop | Required | Không | Runtime stop cần Agent; queued cancel không cần | Managed runtime; Sim mô phỏng | Inventory giúp reconcile; Stop adapter không trực tiếp gọi NVML | Public bearer |
| POST /scheduler/run-once | Required | Không | Fresh inventory để assign; thiếu vẫn count0 | Scheduler không gọi trực tiếp | Snapshot Agent đã gửi | Public bearer |
| POST /agents/register | Required | Không | Manual protocol client được | Không cho fixture | Không cho fixture | Enrollment header |
| POST /agents/{id}/heartbeat | Required | Không | Đã enroll; không cần process nếu manual | Không | Không | Agent bearer |
| PUT /agents/{id}/inventory | Required | Không | Real Agent hoặc manual fixture CP riêng | Real collector cần; fixture không | Real collector cần; fixture không | Agent bearer |
| GET /agents/{id}/commands | Required | Không | Đã enroll, manual có thể lease | Không để lease; có để execute thật | Local check ở Start thật | Agent bearer |
| POST /agents/{id}/commands/{cid}/ack | Required | Không | Đúng identity + command có thật | ACK JSON không gọi Docker tại CP | Không tại CP | Agent bearer |
| GET Next /api/aiwm-health | Để body ok | Không | Không | Không nếu native | Không | None |
| GET/POST Next /api/aiwm/... | Required + Next | Không | Theo business route | Theo route/Agent | Theo route/Agent | Next server token; Origin check cho POST |

Không endpoint nào yêu cầu standalone database service hiện tại.

## 13. Inconsistencies và limitations

- **UI/API naming:** Workload dùng Job API, External là LEGACY. `origin=EXTERNAL` không bị validate lỗi mà trả [].
- **OpenAPI required/enum vs Go:** decoder không enforce required tự động. `{}` ở drain thành false, `{}` ở ACK thành failure; invalid origin trả [], invalid poll limit default10. Không sửa behavior để làm đẹp spec.
- **OpenAPI coverage:** health và một số Agent mutation success responses chỉ có description, thiếu schema body chi tiết. Source trả data.Server hoặc data.Command như mô tả từng endpoint.
- **Auth harness vs production:** httpapi.New không Options có thể thiếu public middleware trong test; production cmd luôn truyền Options/require token.
- **BFF:** chưa có user login/session. Health BFF có thể HTTP200 nhưng body unreachable; token Next không phải browser authentication.
- **Readiness:** public Server GET/drain decorate schedulable, Agent heartbeat/inventory trả raw chưa decorate. GET GPU flat response thiếu freshness/readiness của Server. Trang frontend GPU inventory hiện đếm State=FREE cho ô “Sẵn sàng”, chưa xét offline/stale/drained, khác CP Summary.gpusFree; đã sửa mô tả quá rộng trong frontend-api-map.md.
- **GPU accounting:** RELEASED chưa chắc FREE nếu active/unknown/unhealthy evidence còn. Counter occupied không chỉ actual utilization; StateReason có thể cũ ngay sau reserve.
- **Scheduler:** thiếu GPU → queued reason, không normal422; run-once là mutation toàn queue, count0 có thể vì background cycle đã xử lý.
- **Stop/reconciliation:** ACK Stop true chưa terminal; failed Stop ACK đưa RUNNING. STOPPING + missing chưa kiểm tra Start còn DELIVERED; happy-path Postman chưa chứng minh loại trừ race ở [Domain Model](DOMAIN_MODEL.md).
- **Command GET:** poll có tác dụng lease, không dùng làm read-only inspector cho Agent thật. ACK chưa bắt buộc đã DELIVERED.
- **Execution:** Sim không chạy image/argv; không kiểm chứng CUDA, real-container natural exit hay survival qua disconnect trên GPU vật lý.
- Không có login/refresh/expiry, external mutation, workload CRUD riêng, GPU update, command-history read, policy CRUD hay explicit reconciliation API. Collection không tạo TODO endpoint cho các scope chưa có.

## 14. Kiểm chứng artifacts

- Contract hiện tại: 21 backend route patterns; collection 94 request definitions.
- Task allocation: **Newman 6.2.2, 25 requests / 51 assertions / 0 failures**, chỉ folder `Allocation - Policy Preview Submit` trên CP native riêng + inventory fixture; xác nhận placement và External vẫn protected.
- Focused Go tests/vet/build và frontend 10 contract tests/lint/typecheck PASS. Không chạy lại Docker/full E2E/production frontend build.
- Lượt API audit trước contract intent từng qua 69 requests/195 assertions; số đó **không là chứng nhận cho collection mới**. Existing Agent/E2E folders và demo scripts đã đổi request body; chưa chạy lại toàn bộ.
- Environment mẫu trống credential; test không export token/response chứa secret. Manual protocol fixture không chứng minh Docker/NVML/CUDA thật.
- [acceptance.py](../scripts/acceptance.py), [recovery.py](../scripts/recovery.py) dùng sau khi rebuild CP; recovery chỉ cho deployment demo riêng.

## 15. API → Master Scope → Source

21 backend endpoints dưới đây, cộng hai dòng transport Next riêng. Security là scope giao nhau; không cộng Register lần nữa như login API.

| API | Purpose | Scope | Caller | Auth | Main handler/service |
|---|---|---|---|---|---|

| `GET /healthz` (READY) | Kiểm tra process HTTP Control Plane đang phục vụ. | Health / Readiness | Postman / probe | None | [health](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) |
| `GET /readyz` (READY) | Kiểm tra handler readiness hiện tại. | Health / Readiness | Postman / probe | None | [ready](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) |
| `GET /api/v1/system/summary` (PARTIAL) | Đọc tổng hợp logical GPU pool và 20 JobEvent gần nhất. | Dashboard; GPU Discovery; Queue/Priority; Reconciliation | Frontend BFF / Postman | Public bearer | [summary](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [Summary](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/servers` (READY) | Liệt kê Server đã enroll cùng GPU/Container snapshot gần nhất. | Agent Registration; Heartbeat; GPU Discovery; Existing Workload Discovery | Frontend BFF / Postman | Public bearer | [listServers](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [ListServers → presentServer](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/servers/{serverID}` (READY) | Đọc chi tiết một host; đây cũng là cách đọc containers của riêng Server. | Heartbeat; GPU Discovery; Existing Workload Discovery; External vs Managed Workload | Frontend BFF / Postman | Public bearer | [getServer](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [GetServer → presentServer](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/servers/{serverID}/drain` (READY) | Bật/tắt việc nhận placement mới của Server; không dừng container. | Scheduler; Heartbeat; Security | Frontend BFF / Postman | Public bearer | [drainServer](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [SetServerDrained → presentServer](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/gpus` (PARTIAL) | Liệt kê GPU toàn pool với Server chứa GPU. | GPU Discovery; Scheduler; Atomic Reservation; Existing Workload Discovery | Frontend BFF / Postman | Public bearer | [listGPUs](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [GPUs](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/containers` (READY) | Đọc Docker container observations, không điều khiển arbitrary container. | Existing Workload Discovery; External vs Managed Workload; Reconciliation | Frontend BFF / Postman | Public bearer | [listContainers](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [Containers](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/jobs` (READY) | Tạo Job QUEUED; không trực tiếp chạy Docker container. | Job Management; Queue/Priority; Scheduler; Atomic Reservation; Agent Command/Control | Frontend BFF / Postman | Public bearer | [createJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [CreateJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/jobs` (READY) | Liệt kê Job; Console gọi danh sách này là Workloads. | Job Management; Queue/Priority; Scheduler | Frontend BFF / Postman | Public bearer | [listJobs](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [ListJobs](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/jobs/{jobID}` (READY) | Đọc lifecycle và decision của Job, làm điểm poll E2E. | Job Management; Scheduler; Atomic Reservation; Reconciliation | Frontend BFF / Postman | Public bearer | [getJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [GetJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/jobs/{jobID}/stop` (PARTIAL) | Hủy Job chưa dispatch hoặc enqueue Stop đúng managed Job. | Job Management; Agent Command/Control; Atomic Reservation; Reconciliation; External vs Managed Workload | Frontend BFF / Postman | Public bearer | [stopJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [StopJob](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/queue` (READY) | Đọc các Job còn QUEUED cùng lý do chưa được placement. | Queue/Priority; Scheduler | Frontend BFF / Postman | Public bearer | [queue](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [Queue](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/scheduler/run-once` (READY) | Chạy một lượt scheduler trên toàn bộ queue; đây là mutation, không phải preview/read API. | Scheduler; Queue/Priority; Atomic Reservation; Agent Command/Control | Frontend BFF / Postman | Public bearer | [runScheduler](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [ScheduleOnce → Scheduler.Plan](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/agents/register` (INTERNAL) | Bootstrap Agent credential và Server identity. | Agent Registration; Security | Agent / Postman CP riêng | X-Enrollment-Token | [registerAgent](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [RegisterAgent](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/agents/{agentID}/heartbeat` (INTERNAL) | Cập nhật liveness của Server/Agent. | Heartbeat; Security; Scheduler | Agent / Postman CP riêng | Agent bearer | [heartbeat](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [Heartbeat](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `PUT /api/v1/agents/{agentID}/inventory` (INTERNAL) | Gửi full snapshot, normalize occupancy và reconcile Job trong cùng transaction. | GPU Discovery; Existing Workload Discovery; External vs Managed Workload; Heartbeat; Reconciliation; Atomic Reservation | Agent / Postman CP riêng | Agent bearer | [inventory](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [ReportInventory → inventoryToDomain](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET /api/v1/agents/{agentID}/commands` (INTERNAL) | Lease command cho Agent; GET này có side effect. | Agent Command/Control; Atomic Reservation; Reconciliation; Security | Agent / Postman CP riêng | Agent bearer | [pollCommands](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [PollCommands](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `POST /api/v1/agents/{agentID}/commands/{commandID}/ack` (INTERNAL) | Ghi kết quả command và áp dụng hiệu ứng Job. | Agent Command/Control; Reconciliation; Atomic Reservation; Security | Agent / Postman CP riêng | Agent bearer | [ackCommand](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go) → [AckCommand](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go) |
| `GET Next /api/aiwm-health` (READY) | Connectivity probe | Health; Frontend | Frontend / Postman | None | [GET](../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/api/aiwm-health/route.ts) → CP /healthz |
| `GET/POST Next /api/aiwm/[...path]` (PARTIAL) | Proxy 14 business routes | Security; Frontend; scopes của route | Frontend / Postman | Server bearer + Origin check | [proxy](<../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/api/aiwm/[...path]/route.ts>) → [policy](../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/proxy-policy.ts) → CP |

| `GET /api/v1/jobs/options` (READY) | Catalog input | Policy; Capability; Frontend | Form/Postman | Public bearer | jobOptions → AllocationOptions |
| `POST /api/v1/jobs/preview` (READY) | Đối chiếu, không reserve | Validation; Policy; Matching | Form/Postman | Public bearer | previewJob → PreviewJob → Plan |
