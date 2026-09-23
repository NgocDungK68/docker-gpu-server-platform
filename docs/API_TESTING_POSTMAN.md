# API và kiểm thử bằng Postman

Cập nhật Organization/session/enrollment ngày 17/09/2026 bằng inspection tĩnh. Không chạy API, migration, Newman hoặc build/test trong task. Backend source là authority; xem [TESTING_RUNBOOK](TESTING_RUNBOOK.md) để khởi động PostgreSQL/CP/frontend/Agent.


## Time-based planning — contract và Postman (23/09/2026)

Request vẫn dùng neededAt + ttlSeconds; ttlSeconds là requestedDuration bằng giây. Không gửi thêm requestedDuration/organizationId/server/UUID/strategy. Khoảng thời gian [neededAt, neededAt + ttlSeconds). Start quá khứ quá 60 giây hoặc interval đã hết trả 400, error.fields. Giữ image/command/environment/resources hiện tại.

| API / field | Hành vi |
|---|---|
| POST jobs/preview | Chạy planner hypothetical, không mutation. requestedStartAt/requestedEndAt, planningStatus AVAILABLE/CONFLICT. Không K/score/UUID. |
| POST jobs | Re-evaluate, tạo QUEUED. Planning ticker mới giữ interval; chưa START sớm. |
| GET jobs/{id} | requestedStartAt/requestedEndAt; assignment.startAt/endAt/reservationState. ASSIGNED + PLANNED + commandId rỗng = chưa thực thi. |
| GET queue | Chỉ QUEUED; Job đã có future reservation nằm trong jobs, không trong queue. |
| POST scheduler/run-once | ADMIN-only; lập/re-lập future reservations và chạy execution controller. assignedJobs đếm calendar assignments mới/đổi GPU, không phải số container đã RUNNING. |
| POST jobs/{id}/stop | Cancel lịch chưa giao START; đang thực thi thì graceful STOP, chờ inventory rồi release. |

Folder mới **Planning - Lịch tương lai và Policy** trong collection hiện có:

1. Login VTT qua folder Organization, giữ access_token; current user lấy planning_org.
2. A/N2 xin 3 H100 trên scenario vtt-gpu-01, start = now + 3 phút, duration = 60 giây.
3. Đợi ticker 3 giây (GET, không gọi admin scheduler), kiểm A ASSIGNED/PLANNED/commandId rỗng.
4. Thêm C/N3 rồi B/N1 cùng interval, đợi replan; B được giữ, A về QUEUED.
5. GET server của B, Organization phải bằng planning_org. Chạy với cùng snapshot trống lịch cạnh tranh; không dừng Job khác để ép kết quả.
6. Trước start không được STARTING/RUNNING. Sau start gửi request 11: RUNNING nếu Agent/resource vẫn safe.
7. Sau end + thời gian xử lý STOP/inventory: STOPPED, assignment RELEASED; GPU FREE nếu không còn consumer.
8. Cleanup 12–14 chỉ hủy/dừng ba Job của scenario; không xóa server/container ngoài test.

Variables planning_start/planning_a/planning_b/planning_c/planning_org/planning_server được collection tự lưu. Không chứa password/token thật. Normal user không được gọi run-once; ticker production chạy mặc định mỗi 2 giây. Không chạy toàn bộ collection lịch sử vì có fixture/thao tác độc lập.

Tự động hóa nhanh bằng public API thật: python scripts/demo.py planning, xem TESTING_RUNBOOK. Các test focused dùng clock giả, không sửa clock production/Agent.

## 1. Login, current user và logout

Không public self-registration. Bootstrap admin đầu tiên qua CLI `aiwm-server --bootstrap-admin`; ADMIN cấp account còn lại.

| API | Request / quyền | Kết quả |
|---|---|---|
| POST /api/v1/auth/login | `{"username":"{{username}}","password":"{{password}}"}`, không bearer | 200 data.token opaque + expiresAt + user + organization; sai credential/disabled → 401; rate limit → 429. |
| GET /api/v1/auth/me | Bearer access_token | 200 user.id/username/role/organizationId/enabled và organization metadata. |
| POST /api/v1/auth/logout | Bearer access_token | 200 loggedOut=true; token cũ không còn hợp lệ. |

Session có TTL 8 giờ, hash lưu PostgreSQL; không JWT/refresh token. PasswordHash PBKDF2-SHA256 không serialize JSON. Đổi User thu hồi session cũ. PostgreSQL unavailable không fallback sang bearer chung. Biến cấu hình `AIWM_API_TOKEN` không còn là credential production.

## 2. Organizations và Users (ADMIN)

| API | Nội dung |
|---|---|
| GET /api/v1/organizations | ADMIN xem nhiều đơn vị; ORGANIZATION_USER chỉ thấy đơn vị của mình. |
| POST /api/v1/organizations | ADMIN tạo: code, name, enabled. ID backend sinh; code unique. |
| POST /api/v1/organizations/{organizationID} | ADMIN sửa metadata/status; không chuyển ownership Server/Job. |
| GET /api/v1/users | Chỉ ADMIN, response không chứa passwordHash. |
| POST /api/v1/users | username, password, role ADMIN/ORGANIZATION_USER, organizationId, enabled. |
| POST /api/v1/users/{userID} | Cùng DTO; password rỗng giữ hash cũ. Organization account bất biến, khác org → 409; sửa account revoke session. |

## 3. Server Enrollment

`POST /api/v1/enrollments` nhận displayName, labels; ORGANIZATION_USER **không gửi organizationId**, backend lấy từ account. ADMIN được gửi organizationId. GPU tự discovery, không có API nhập GPU thủ công.

Response 201 gồm id/serverId/organizationId/displayName/labels/expiresAt và **enrollmentToken chỉ trả một lần**. Token hash lưu PostgreSQL. `GET /api/v1/enrollments` scope theo account, không trả token. `POST /api/v1/enrollments/{enrollmentID}/revoke` trả revoked=true; chặn register lại, không thu hồi Agent token đã cấp hoặc stop runtime.

Register vẫn dùng `X-Enrollment-Token` và v1 DTO có machineId/name/protocolVersion. CP resolve enrollment → ServerOwnership → Organization; không tin organization từ Agent. Token bind lần đầu trong 24 giờ. Sau bind chỉ cùng machine được register lại; MachineID trùng ownership khác → conflict, token dùng cho machine khác → unauthorized. Re-register giữ ServerID, xoay Agent token và reset freshness.

## 4. Quyền xem và mutation

- Normal user: list Server/GPU/Container/Job/Queue/Summary chỉ trong Organization; get/stop/drain ID ngoài scope → 404.
- Query `organizationId` chỉ dành ADMIN. Non-admin gửi query này → 403, kể cả ID của chính mình.
- Submit/Preview không nhận organizationId trong JSON → 400 nếu gửi. Job lấy org của account, **kể cả ADMIN**; filter xem không thay org submit.
- ADMIN mới được gọi POST scheduler/run-once. Worker tự chạy theo ticker nên user không cần quyền này để Job được schedule.
- Summary có serversOffline/gpusReserved/gpusAllocated/gpusUnhealthy bổ sung; free count chỉ tính host schedulable. GPU list thêm organizationId derived từ Server.
- Agent API nằm ngoài BFF whitelist; Agent không đổi organization bằng register body/inventory/labels.

## 5. Import, variables và thứ tự gửi

Import `postman/AIWM.postman_collection.json` và `AIWM.local.postman_environment.json`. Mẫu không chứa credentials thật. Điền secrets chỉ ở environment riêng; không export sau chạy.

| Variable | Nguồn |
|---|---|
| base_url | CP đang kiểm thử, mặc định localhost:8080. |
| username / password | Account do bootstrap hoặc ADMIN cấp. |
| access_token | Login tự capture; không nhập token cấu hình dùng chung. |
| organization_id / current_role | Current user tự capture. |
| enrollment_id / enrollment_token | Create enrollment tự capture. |
| agent_id / agent_token | Register fixture capture, **không dùng credential của Agent thật đang chạy**. |
| manual_machine_id / manual_gpu_uuid / manual_external_gpu_uuid | Fixture độc lập, sinh khi tạo enrollment/register nội bộ. |
| internal_username / internal_password / internal_access_token | Account ADMIN riêng của CP fixture; không dùng session của base_url. |
| job_id / command_id / inventory_sequence | Response hoặc script fixture cập nhật. |
| other_organization_id / other_server_id / other_job_id | ID ngoài đơn vị để thử tenancy; lấy bằng account ADMIN, không lấy secret của người khác. |
| organization_code / organization_name / new_username / new_password | Metadata tạo qua folder ADMIN, không hardcode vào business logic. |

Gửi từng bước, không chạy toàn collection cũ một lượt:

1. Health.
2. Login.
3. Current user.
4. Organizations; ADMIN có thể mở folder quản trị tạo User/Organization.
5. Create enrollment.
6. Với Agent Sim/thật: start Agent theo runbook. Với **CP fixture riêng**: dùng folder `Organization - Protocol nội bộ` để Register/Inventory. Không gửi fake report vào Agent production.
7. Servers và GPUs, kiểm tra organizationId và occupancy.
8. Options → Preview → Submit, body giữ AllocationIntent; không chọn UUID/server/priority/strategy.
9. Queue/Get Job; chờ ticker hoặc ADMIN run-once. Nếu thao tác tay vượt freshness timeout, refresh inventory fixture trước schedule.
10. Nếu fixture: Poll → ACK → Inventory running (không phải Docker execution thật).
11. Stop → actual terminal inventory → Job terminal + reservation RELEASED.
12. Logout; dùng token vừa logout GET me phải 401.

Các folders cũ được giữ để không mất fixture/error examples, nhưng phải login bằng session và chuẩn bị inventory phù hợp khi dùng lại; folder protocol đã thêm login/enrollment riêng. Số liệu PASS/Newman của task trước không chứng nhận tenancy/session mới. Không dùng scripts acceptance/recovery cũ như một phép kiểm thử tenant-aware.

## 6. Ma trận kiểm thử tenancy tối thiểu

| Tình huống | Mong đợi |
|---|---|
| VTT user GET servers/gpus/jobs/summary | Chỉ dữ liệu VTT. |
| VTT user GET hoặc stop Job VDS | 404; Job VDS không thay đổi. |
| VTT user get/drain Server VDS | 404; Server không thay đổi. |
| Non-admin query organizationId / body enrollment organizationId | 403. |
| Submit thêm organizationId, strategy, priority, GPU UUID | 400 strict DTO. |
| Agent register hoặc inventory thêm organizationId | 400 strict DTO. |
| Cùng token enrollment, machine khác | 401; không đổi ownership. |
| Job org-A thiếu GPU, org-B đủ GPU | QUEUED; không reservation/START trên org-B. |
| Preview lặp | Không tăng Job/Reservation/Command. |
| Agent heartbeat sau timeout, chưa FULL inventory | Không placement/lease START; last-known inventory/reservation giữ nguyên. |
| Logout / disabled User / expired session | Public API 401. |
| ADMIN query organizationId | Đọc đúng đơn vị được chọn; submit vẫn org account. |

## 7. Contract cũ còn dùng

Phần dưới giữ chi tiết endpoint inventory/Job/Agent. Đối với mọi public API, hiểu **public bearer = session access_token**. Đường start/demo hiện tại dùng TESTING_RUNBOOK; không dùng hướng dẫn token chung hoặc kết quả kiểm thử lịch sử làm authority.

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
  "neededAt": "{{planning_start}}",
  "ttlSeconds": 3600
}
~~~

Các fields bắt buộc: name/image, workloadType, resources.gpuCount/minVramMiB/performanceProfile/fp8Required, necessityLevel/necessityReason, systemImportance, neededAt, ttlSeconds. CUSTOM cần necessityExplanation. GPU/VRAM phải số nguyên >0; FP8 phải boolean (không null/string), profile hợp lệ. Backend yêu cầu TTL trong giới hạn và RFC3339 có múi giờ. Max GPU/TTL đọc Options, không giả định luôn 64/2592000. neededAt chỉ cho phép lệch quá khứ tối đa 60 giây; interval phải chưa hết. ttlSeconds tính EndAt và trigger STOP.

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

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
- Auth: `Bearer {{access_token}} (session login), Required.`

**Request**

- URL: `{{base_url}}/api/v1/scheduler/run-once`; thay route IDs bằng variables được capture.
- Query: không có query được handler này xử lý.
- Body: không cần; handler không decode body.

**Response / Expected result**

- 200; {"data":{"assignedJobs":0}} hoặc số commit thành công trong lượt này.
- Body không được decode. Placement Plan thuần không reserve; ReplanReservations commit calendar trước; đến start CommitAssignment revalidate rồi atomically ghi GPU + STARTING + START. ErrConflict được refresh/retry ở cycle sau; jobs/options cung cấp catalog/limits; không có API đổi strategy.

**Luồng xử lý**

~~~text
POST /api/v1/scheduler/run-once
→ httpapi.Server.runScheduler
→ ControlPlane.ScheduleOnce → Scheduler.Plan
→ Repository.ReplanReservations; CommitAssignment; RequestStop
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
- **Prerequisite:** CP + Authorization Bearer session token; không cần Agent.
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

- Fixture JSON ở folder protocol không phải observation được Docker/NVML thật xác minh.
- Chỉ bật enable_agent_internal=true khi đã có CP fixture riêng (internal_base_url), state và PostgreSQL database test riêng. Điền internal_username/internal_password của ADMIN trên CP đó; không dùng session base_url. Repo chưa có script tự provision database/CP fixture độc lập: xem TODO trong runbook. Full fake E2E dùng Agent Sim, không cần fixture thủ công này.
- Đây là protocol Agent, không phải frontend onboarding/user login API. Dùng folder Organization - Protocol nội bộ theo thứ tự login → enrollment → register → inventory → Job/command/lifecycle.

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

## 10. Frontend/BFF

GET /api/aiwm-health chỉ là connectivity probe. GET/POST /api/aiwm/[...path] whitelist public routes. Login đặt cookie HttpOnly/SameSite=Strict, không trả token ra browser JSON; request tiếp theo forward bearer theo cookie. Cookie Secure cấu hình bằng AIWM_SESSION_COOKIE_SECURE, bật khi frontend HTTPS.

Chưa login → 401; Origin mutation khác host → 403; route ngoài whitelist → 404; network/timeout → 502. Logout thành công xóa cookie; nếu upstream mất kết nối, UI báo chưa logout được thay vì giả đã revoke session.

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
| Expired token | **Session hết hạn hoặc logout phải 401**, không dùng JWT | Không invent behavior |

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
| 503 | CONFIGURATION | Không dùng CONFIGURATION/thiếu bearer cấu hình trong đường session mới; thiếu cookie trả 401. |

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
| GET/POST Next /api/aiwm/... | Required + Next | Không | Theo business route | Theo route/Agent | Theo route/Agent | Session cookie/bearer; Origin check cho POST |

Không endpoint nào yêu cầu standalone database service hiện tại.

## 13. Giới hạn và kiểm chứng

Chỉ static inspection/implementation trong task mới. Chưa chạy Postman, PostgreSQL, migration, Go/Frontend build/test. Protocol fixture chứng minh JSON contract nếu người dùng chạy, không chứng minh Docker/NVML/CUDA thật.

Schema metadata ở store/postgres/001_metadata.sql, runtime vẫn gob. Test harness cũ có thể dùng httpapi.New không Identity; production aiwm-server bắt buộc nối Identity/PostgreSQL. Agent token chưa có expiry; session người dùng có TTL. Quota vẫn DEVELOPMENT_CONFIG, không quota ledger riêng từng org. Chưa có migration ownership runtime cũ.

## 15. API → Master Scope → Source

Bảng dưới giữ mapping endpoint trước khi thêm metadata; auth/organizations/users/enrollments bổ sung ở mục 1–3, source httpapi/identity.go và application/identity.go.

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
