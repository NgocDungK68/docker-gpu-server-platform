# Domain model AIWM — implementation hiện tại

Tài liệu mô tả source đang có trong workspace, không mô tả thiết kế tương lai. Các giá trị viết hoa giữ đúng literal trong Go/API; tên như **External**, **Workload**, **Reservation** cần đọc cùng phần ánh xạ dưới đây. Backend thực thi standalone Docker, cấp phát nguyên GPU trên một server cho mỗi Job.

Nguồn định nghĩa chính: [domain/model.go][model]. Nguồn hành vi: [application/controlplane.go][cp], [memory/store.go][store], [memory/lifecycle.go][lifecycle], [memory/accounting.go][accounting]. Các state machine dưới đây tổng hợp các nhánh đang được gọi; code chưa có một bảng chuyển trạng thái tập trung.



## Organization, User, Enrollment và tenancy

Phần bổ sung ngày 17/09/2026 dựa trên implementation tĩnh, chưa chạy kiểm chứng. Các path `internal/...` thuộc backend Go.

| Entity | Identity / field chính | Relationship / lifecycle owner | Source |
|---|---|---|---|
| `Organization` | `ID`, `Code` unique, `Name`, `Enabled`, `CreatedAt/UpdatedAt` | 1 → nhiều User/Server/Job; ADMIN tạo/sửa metadata | `domain/organization.go`, `application/identity.go` |
| `User` | `ID`, username chuẩn hóa chữ thường unique, `PasswordHash`, `Role`, `OrganizationID`, `Enabled` | Mỗi account thuộc một Organization; không public self-registration; tổ chức account không đổi qua update | Cùng domain; `IdentityService.SaveUser` |
| `Principal` | User + Organization; `ScopeOrganizationID` chỉ cho ADMIN lọc đọc | Giá trị request context sau xác thực, không nhận từ body | `domain.WithPrincipal`, `httpapi.sessionAuth` |
| Session | Hash opaque token, user ID, expiry 8 giờ | PostgreSQL kiểm tra User/Organization enabled mỗi request; logout hoặc sửa User revoke; không JWT/refresh token | `postgres.Store.SessionPrincipal/CreateSession/DeleteSession` |
| `Enrollment` | ID, ServerID được cấp trước, OrganizationID, displayName, labels, token hash, expiresAt, machineId, revoked | Token trả một lần; bind đầu trong 24 giờ; đã bind chỉ cùng machine được register lại. Revoke chặn register, không dừng Agent/container đã chạy | `IdentityService.CreateEnrollment`, `postgres.Store.BindEnrollment` |
| `ServerOwnership` | ServerID, MachineID unique, OrganizationID | PostgreSQL authority; enrollment transaction bind machine/server/org, runtime store giữ bản copy bất biến | `ports.MetadataRepository`, `store/postgres/001_metadata.sql` |
| `agent.CompatibilityReport` | status, schedulable, OS/architecture/kernel/cgroup, Docker version/API/runtime, NVML/GPU count, issues | Preflight local, không persist thành Server status; SUPPORTED/DEGRADED/UNSUPPORTED | `agent/preflight.go`, `dockerengine.Engine.Compatibility` |

**Invariant:** `Job.OrganizationID != "" && Job.OrganizationID == Server.OrganizationID`. Preview, Scheduler.filter và CommitAssignment cùng enforce, trước GPU filtering/scoring/reserve. Không lưu organization trên GPU; suy ra GPU → Server → Organization. GPU list API expose `organizationId` dạng derived field.

Job mới lấy OrganizationID từ `Principal.User.OrganizationID`; CreateJobRequest không có field organizationId. ADMIN cũng submit cho đơn vị của chính account; organization selector chỉ dành cho enrollment/quản trị/bộ lọc xem. ORGANIZATION_USER đọc list/detail/dashboard/queue và stop/drain chỉ trong đơn vị; ID ngoài scope trả 404, query cố chọn organization trả 403. Application scope được gọi trước mutation, không dựa vào frontend filtering.

`Server.OrganizationID` không được Agent inventory ghi đè. Re-register resolve enrollment trong PostgreSQL rồi Upsert runtime; thay ownership hoặc ServerID đã bind cùng MachineID bị conflict. Server/Job cũ có org rỗng giữ dữ liệu/assignment, không được placement; chưa có migration tự động hoặc API chuyển ownership.

PostgreSQL chứa organizations/users/sessions/server_enrollments/server_ownership; Job/Assignment/Command/inventory vẫn ở memory/durable gob. Không có transaction chung giữa hai storage; bind metadata commit trước Upsert runtime. Lỗi ghi runtime có thể để metadata đã bind: retry cùng enrollment/machine trả lại cùng ownership, không lấy host khác. Hai store phải được backup nhất quán khi phục hồi. Disable Organization được scheduler đọc mỗi cycle, không tự stop workload hiện có và không atomic với runtime commit của cycle đang chạy.

**Failure:** Agent restart verify MachineID, giữ sequence/processed results, re-register và gửi FULL inventory. Heartbeat sau OFFLINE/khoảng mất heartbeat vô hiệu inventory freshness; chỉ inventory hợp lệ mới mở lại scheduling và lease START. Mất Agent/CP/network không giải phóng GPU hoặc stop container. Heartbeat chỉ cho biết thiếu liên lạc, không xác định chính xác host chết hay network partition.


Kiểm thử boundary mới (đã viết, chưa chạy): backend application/identity_test.go, httpapi/identity_test.go, TestOrganizationFilterPrecedesScoringForEveryStrategy và memory/safety_test.go. Chúng kiểm tra ownership server-side, session scope/spoofing/logout, org filter trước scorer, commit conflict không mutation và reconnect chưa có FULL inventory. PostgreSQL transaction/bind concurrency vẫn cần kiểm chứng trên DB thật.

## Time-window reservation — bổ sung 23/09/2026

Source: internal/domain/reservation.go; application/planning.go; memory/planning.go và lifecycle.go.

| Model/field | Semantics / owner |
|---|---|
| AllocationIntent.NeededAt | Requested start RFC3339, UTC, giữ fractional seconds; tolerance quá khứ 60 giây và interval chưa hết. |
| AllocationIntent.TTLSeconds | Requested duration bằng giây, >0, max từ config; end = start + duration. Không phải lease timeout hay thời gian kể từ RUNNING. |
| TimeWindow | Giá trị [StartAt,EndAt), không entity/ID riêng. Overlaps dùng strict <; endpoint chạm nhau không conflict. |
| Assignment.StartAt/EndAt | Calendar source of truth khi đã reserve; CommandID rỗng lúc PLANNED. Persist gob cùng Job. |
| JobView.requestedStartAt/requestedEndAt | DTO derive từ intent; preview có thêm planningStatus AVAILABLE/CONFLICT. |
| Assignment.ReservationState | PLANNED → RESERVED → ALLOCATED → RELEASED; cancel trước execution có thể đi thẳng RELEASED. |

QUEUED chưa có reservation. ASSIGNED/PLANNED đã giữ interval nhưng chưa START. STARTING bắt đầu khi execution controller atomically tạo START tại start <= now < end. Replanning chỉ áp dụng PLANNED với StartAt > now và CommandID rỗng; loser quay lại QUEUED. Không đổi JobStatus enum.

Một Job có tối đa một Assignment hiện hành; một GPU có thể nằm trong nhiều Assignment không overlap. GPU.AssignedJobID chỉ biểu diễn current physical claim, không phải future calendar. PLANNED không làm GPU.State RESERVED và không cho phép adopt managed-looking container.

EndAt: QUEUED hết thời gian → FAILED; chưa dispatch → CANCELLED/RELEASED; đã giao/chạy → STOPPING/STOP → inventory confirms → STOPPED/RELEASED. Actual blocker còn thì GPU không FREE. Agent/CP disconnect không stop container hoặc tự release; reconnect xử lý interval còn lại hoặc gửi STOP nếu đã hết.

Compatibility: gob version 1 đọc thêm fields bằng zero-value. Job cũ thiếu interval không tự tạo lịch; execution cũ đã có CommandID giữ nguyên, không diễn giải lại TTL thành deadline âm thầm. Tạo Job mới đầy đủ intent để dùng time planning.

Tests: application/planning_test.go; domain/reservation_test.go; durable/planning_test.go; httpapi/TestFutureJobTimeContract.

## 1. Domain entities và các representation thực sự tồn tại

### 1.1 Server

**Type/package:** `domain.Server` trong [model.go][model]. Đại diện một host Docker GPU đã đăng ký và snapshot gần nhất mà Control Plane (CP) biết về host đó.

**Identity:** `ID` do `ControlPlane.RegisterAgent` tạo với prefix `srv_`; `MachineID` là khóa enrollment ổn định. Repository lưu `servers[ID]` và index `machines[MachineID] = ID`. Agent nhận `agentId` chính là `Server.ID`, không phải ID của một entity Agent khác.

| Nhóm field | Field quan trọng và ý nghĩa |
|---|---|
| Identity / metadata | `ID`, `MachineID`, `Name`, `Address`, `AgentVersion`, `Labels`. Selector Job so khớp `Labels`. Address là metadata; CP không dùng để gọi Docker từ xa. |
| Host observation | `Host HostInfo` gồm hostname, OS, architecture, CPU count, RAM tổng; `DockerVersion`. Không phải tài nguyên CPU/RAM đã reserve. |
| Connectivity / drain | `Status ServerStatus`, `Drained bool`, `LastHeartbeatAt`. Drained là ý định của operator, vẫn được giữ khi offline. |
| Inventory freshness | `InventoryVersion` là sequence gần nhất; `LastInventoryAt` là thời gian Agent quan sát; `InventoryReceivedAt` là thời gian CP nhận snapshot. |
| Scheduling presentation | `SchedulingReady` được serialize thành `schedulable` và `SchedulingReason`; tính khi trình bày qua `presentServer`, không phải enum hay nguồn quyết định của scheduler. |
| Nested observations | `GPUs []GPU`, `Containers []Container`; toàn bộ được thay bằng snapshot được chấp nhận. |
| Authentication | `TokenHash [32]byte` là SHA-256 của Agent token, bị loại khỏi JSON bằng `json:"-"`. |

**Relationships:** chứa 0..n GPU/Container; được tham chiếu bởi `Assignment.ServerID` và `Command.AgentID`.

**Lifecycle owner:** CP tạo/cập nhật qua `RegisterAgent`, `Heartbeat`, `ReportInventory`; repository giữ identity, trạng thái và inventory. Operator đổi drain qua `SetServerDrained`. `MarkStaleServers` đánh dấu offline; `durable.Open` vô hiệu freshness khi restore. Không có API xóa Server hiện tại. Re-enroll cùng MachineID giữ Server.ID, inventory cũ và drain, đổi credential, reset sequence CP về 0 và inventory receipt về zero để yêu cầu snapshot mới.

### 1.2 Agent

**Không có `domain.Agent`, bảng Agent runtime hay `AgentStatus` enum.** “Agent” là process chạy [`agent.Runner`][runner], với interfaces và local state trong [contracts.go][agent-contracts].

| Field của `agent.PersistentState` | Ý nghĩa |
|---|---|
| `AgentID`, `MachineID`, `AgentToken` | Liên hệ với Server/credential CP; token đầy đủ chỉ lưu phía Agent. |
| `InventorySequence` | Tăng và lưu trước mỗi lần collect/report; lần collect lỗi có thể để lại khoảng trống sequence. |
| `ProcessedCommands map[string]CommandResult` | Khóa là Command.ID; mỗi result lưu ACK và `CompletedAt`, để redelivery trả lại kết quả. History có giới hạn cấu hình. |

**Identity/relationships:** AgentID = Server.ID; mỗi process dùng MachineID cấu hình và local state tương ứng. Không có quan hệ “một Server có nhiều Agent records”; nhiều process dùng chung identity không phải mô hình được hỗ trợ.

**Lifecycle owner:** `Runner.Run` load state, kiểm tra MachineID, đợi Docker/register/first inventory thành công rồi chạy heartbeat/inventory/command loops. `ensureRegistered` enroll lại khi unauthorized. [`state.FileStore`][agent-state] persist JSON. CP quan sát Agent qua Server.Status và freshness; Agent không tự ghi trạng thái ONLINE/OFFLINE vào một model riêng. Dừng loop hoặc mất kết nối không gọi stop/cleanup container.

### 1.3 GPU

**Type/package:** `domain.GPU` trong [model.go][model]; là observation kèm kết quả accounting, nằm trong `Server.GPUs`.

**Identity:** UUID vật lý (`UUID`), không phải numeric `Index`. `inventoryToDomain` chặn UUID lặp trong cùng report; `ReplaceInventory` chặn UUID đã nằm trong inventory server khác, kể cả server kia offline. Index chỉ có ý nghĩa trong snapshot của host; runtime chưa enforce unique index như SQL target schema.

**Fields:** `UUID`, `Index`, `Model`; `MemoryTotalMiB`, `MemoryUsedMiB`, `UtilizationPct`, `TemperatureC`; `Healthy bool`; `State GPUState`; `AssignedJobID`, `ObservedConsumers []string`, `StateReason`. Consumers hiện là container IDs hoặc chuỗi như `pid:42`, không phải foreign-key list hoàn chỉnh của mọi consumer.

**Relationships:** một UUID thuộc inventory một Server; nhiều Container/GPUProcess observations có thể nhắc tới UUID đó. Assignment giữ danh sách UUID; `AssignedJobID` là kết quả accounting, không thay thế Assignment.

**Lifecycle owner:** [NVML Reader][nvml] và [InventoryCollector][inventory] lấy observation; [inventoryToDomain][mapping] chuyển wire; CP `normalizeGPUState` tính authoritative state. `CommitAssignment` trực tiếp đặt RESERVED và job ID; `releaseLocked` tính lại accounting. Không có CRUD/health override API cho GPU.

### 1.4 Job và ResourceRequest

**Type/package:** `domain.Job` và `domain.ResourceRequest` trong [model.go][model]. Job lưu yêu cầu chạy workload, quyết định scheduling và lifecycle do CP tổng hợp từ yêu cầu, ACK, inventory.

**Identity:** `ID` có prefix `job_`, khóa `jobs[ID]`. Tên Job không phải unique key.

| Nhóm field | Ý nghĩa |
|---|---|
| Specification | `Name`, `Image`, `Backend`, `Command []string`, `Environment map[string]string`. `Job.Command` là argv của container, khác entity `domain.Command`. |
| ResourceRequest | `GPUCount`, `PerformanceProfile`, `FP8Required`, private `ResolvedModels`, `MinVRAMMiB`, `CPUMilli`, `MemoryMiB`, `AllowSharedGPU`; `GPUModel` chỉ giữ cho Job cũ/benchmark. MinVRAM là bộ nhớ còn lại yêu cầu **trên mỗi GPU**. CPU/RAM gửi thành Docker limits, scheduler chưa accounting capacity CPU/RAM. |
| Scheduling | `Priority`, `ServerSelector`, `Strategy`, `Assignment *Assignment`. Priority/ServerSelector/GPUModel chỉ còn là legacy/internal, không nhận từ Create API; Strategy do config. Count/TTL max theo config, VRAM >0; sharing bị từ chối ở [validateJobRequest][validation]. |
| Lifecycle | `Status`, `StatusReason`, `CreatedAt`, `UpdatedAt`, `LastObservedAt`, `ContainerID`, `Events []JobEvent`. |

Không có hai field `DesiredState` / `ActualState` riêng. `Status` kết hợp tiến độ điều khiển và kết quả quan sát; actual Docker state còn nằm trong Container. `LastObservedAt` chỉ cập nhật khi tìm thấy container tương ứng, không phải timestamp của mọi lần reconciliation.

**Relationships:** 0..1 Assignment; 0..n JobEvent; Start/Stop commands tham chiếu jobId qua payload. `ContainerID` là một ID được ACK/observation chọn; không có constraint buộc inventory chỉ có duy nhất một container cho Job.

**Lifecycle owner:** CP `CreateJob`, `ScheduleOnce`, `StopJob`; repository commit/ACK/inventory reconciliation đổi trạng thái dưới lock. Agent thực thi command rồi báo kết quả, không trực tiếp sửa Job trong store. Terminal Job giữ lịch sử; không có retry/resubmit/delete Job API hoặc tự chuyển terminal về queue.

### 1.4a AllocationIntent, PolicyDecision và public request

Định nghĩa tại `internal/domain/allocation.go`; cùng identity/owner với Job, không thêm Allocation Request/Quota/Planning entity độc lập.

- `AllocationIntent`: WorkloadType, NecessityLevel, NecessityReason, NecessityExplanation, SystemImportance, NeededAt (RFC3339 normalize UTC), TTLSeconds. USER-PROVIDED; xem [policy/input mapping](SCHEDULER.md).
- `AllocationResources`: public DTO GPUCount, MinVRAMMiB, PerformanceProfile, FP8Required (pointer để reject missing/null), optional CPU/RAM; không decode GPUModel/ResolvedModels.
- `ResourceRequest`: domain/persistence constraints. `prepareJob` resolve models tương thích profile **và** FP8, lưu private trước scheduling; `CapabilityMatches/Matches` dùng chung Plan/CommitAssignment.
- `PolicyDecision`: Status, Reason, InSizingPlan, WithinQuota, QuotaUsage, Source, EvaluatedAt; AuxiliaryScore chỉ nội bộ. SYSTEM-DERIVED bởi `policy.Evaluator`. Snapshot lúc submit/commit; GET Queue evaluate mới. `JobView.NecessityLabel` lấy từ backend catalog.
- `capability.Profile`: ID, Label, Models; backend catalog sở hữu. Đây là compatibility group, chưa chứng nhận measured performance.
- `policy.FactsProvider` tách Planning/Quota; `DevelopmentFacts` là cấu hình demo tĩnh, không quota ledger.
- `application.JobPreview/ResourceMatch/AllocationOptions` là read DTO; preview không identity/persistence/command/reservation.

NeededAt là start cố định; TTLSeconds là duration tính từ start. Waiting priority vẫn từ Job.CreatedAt. Controller dispatch/stop theo interval, không có timeout song song. Gob version 1 đọc được Job cũ với zero-value intent, resource constraints/Assignment cũ giữ nguyên; public Create mới yêu cầu đủ intent.

### 1.5 Workload, External/Existing Workload và Managed Workload

| Tên được dùng | Representation đang có | Identity / fields / relationship | Lifecycle owner |
|---|---|---|---|
| Workload trên Console | Không có Go `Workload` type hoặc WorkloadStatus. Trang [workloads][workloads-page] dùng Job API. | Dùng toàn bộ `Job`, key Job.ID, Assignment và JobStatus; không phải entity thứ hai tạo sau Job. | CP quản lý Job; Console chỉ gửi API request và hiển thị. |
| External / Existing / LEGACY workload | Thường là `domain.Container` có `Origin=LEGACY`; tiến trình GPU không map được container chỉ là `GPUProcess`, không tự có Job/Container record. | Container.ID trong ngữ cảnh Server; State, labels, GPUUUIDs là observation. Không tạo Assignment hoặc Job bằng discovery. “Existing” mô tả workload ngoài quyền quản lý, không chỉ workload tạo trước enrollment. | Chủ workload / Docker bên ngoài AIWM; Agent chỉ quan sát. |
| Managed workload | Yêu cầu là Job; runtime là Container mang ownership labels và jobId. Không có `ManagedWorkload` struct. | Job.ID liên hệ `Container.JobID`, `Job.ContainerID`, ownership labels, Assignment và commands. Container chưa tồn tại khi Job mới QUEUED/ASSIGNED. | CP quản lý ý định start/stop; Agent kiểm tra ownership và gọi Docker; Docker giữ actual lifecycle. |

Phân loại “MANAGED” của Container và việc CP công nhận GPU allocation có điều kiện khác nhau; xem mục 5, không suy quyền stop chỉ từ badge.

### 1.6 Reservation / Assignment và Placement

**Không có `domain.Reservation`, Reservation.ID hay repository reservation độc lập.** Reservation nằm trong `Job.Assignment *domain.Assignment` ([model.go][model]); key hiệu dụng là Job.ID. Chưa được assign thì pointer nil, không có state CREATED.

| Field của Assignment | Ý nghĩa |
|---|---|
| StartAt, EndAt | Reservation interval [start,end), planner lấy từ intent. |
| `ServerID`, `GPUUUIDs` | Một server và đúng tập UUID được commit cho Job. |
| `CommandID` | ID của **START_CONTAINER**, không đổi thành Stop command ID. |
| `AssignedAt` | Thời điểm reserve/assign tại CP. |
| `ReservationState string`, `ReleasedAt *time.Time` | Literal hiện được ghi: PLANNED, RESERVED, ALLOCATED, RELEASED. Go chưa có typed enum cho field này. |
| `Strategy`, `Score`, `Reason` | Quyết định đã chọn, lưu để giải thích lịch sử placement. |

**Owner:** ReplanReservations tạo PLANNED cùng ASSIGNED; CommitAssignment tại StartAt tạo RESERVED cùng STARTING và START command; `reconcileObservedLocked` đặt ALLOCATED; `releaseLocked` đặt RELEASED và giữ nguyên Assignment để xem lịch sử. EndAt điều khiển STOP/cancel; không tự release vì Agent offline.

`domain.Placement` là **giá trị tạm** trả về từ `Scheduler.Plan`: ServerID, GPUUUIDs, Strategy, Score, Reason. Không có ID/status/persistence độc lập; Plan chưa giữ GPU. ReplanReservations tạo calendar reservation; CommitAssignment activation lúc đến giờ.

### 1.7 Agent Command

**Type/package:** `domain.Command` ([model.go][model]); API wire dùng `agentv1.Command` ([protocol types][wire]), OpenAPI gọi schema wire là `AgentCommand`.

**Identity:** `ID` có prefix `cmd_`, key `commands[ID]`; `AgentID` tham chiếu Server.ID.

**Fields:** `Type`, `Status`, `Payload json.RawMessage`, `Attempts`, `CreatedAt`, `DeliveredAt`, `LeaseUntil`, `CompletedAt`, `Error`. Không có typed `JobID` trong domain.Command: Job ID nằm trong payload. Start payload có job/name/image/argv/env/GPU UUIDs/CPU/RAM/labels; Stop payload có jobId, optional containerId, graceSeconds.

**Relationships:** một Server nhận nhiều commands; Job thường có một Start và có thể nhiều Stop commands nếu stop thất bại rồi operator yêu cầu lại. Assignment chỉ trỏ Start. Poll response chỉ gồm ID/Type/Payload; không chứa trạng thái lease/attempts của CP.

**Owner:** CP tạo command; repository lease khi Agent poll và ghi kết quả ACK. Runner lưu local CommandResult trước gửi ACK. CP lưu kết quả status/error/timestamps; không lưu đầy đủ ACK thành một entity riêng. Thành công có ContainerID được áp dụng vào Job theo nhánh xử lý, còn full ACK nằm trong local processed history.

### 1.8 Container / runtime representation và các value phụ

**`domain.Container`** ([model.go][model]) là snapshot CP: ID, Name, Image, `State string`, `Origin ContainerOrigin`, JobID, GPUUUIDs, Labels, StartedAt, FinishedAt, ExitCode. Key để tra cứu an toàn là Server.ID + Container.ID; runtime store không có map Container riêng hoặc foreign key SQL đang hoạt động.

**`agent.RuntimeContainer`** ([contracts.go][agent-contracts]) ở boundary Docker: có thêm `GPUIndexes`, `UnboundedGPUAccess`, `InitPID`; **không có Origin/JobID riêng**, Collector suy ra từ Labels. [Engine.ListContainers/fromInspect][engine] chuyển Docker SDK model sang RuntimeContainer. Docker là owner của actual state; CP chỉ thay snapshot khi inventory hợp lệ.

[Simulator.Runtime][sim] persist map container ID → RuntimeContainer, seed External ở `running`, start Managed thành `running`, stop thành `exited`/exitCode 0. Đây là Docker adapter mô phỏng, không thực thi image/argv. GPU của simulator vẫn đến từ NVML adapter qua mock library.

| Value / read model | Purpose, key, relationship và owner |
|---|---|
| `HostInfo` | Hardware observation nested trong Server; không ID riêng; Collector `discoverHost` tạo. [hardware.go][hardware] |
| `GPUProcess` | PID + GPUUUID + UsedMemoryMiB + optional ContainerID/ProcessName trong InventoryReport. PID chỉ có ngữ cảnh host/thời điểm; NVML gộp theo UUID:PID. Không persist danh sách process riêng trong Server; accounting giữ một phần evidence trong GPU.ObservedConsumers. [NVML][nvml], [inventory][inventory] |
| `InventoryReport` | Snapshot chuyển giữa Agent/CP: Host, Sequence, ObservedAt, DockerVersion, GPUs, Containers, Processes; bản domain thêm ReceivedAt nội bộ. Key kiểm tra thứ tự là Server.ID + Sequence. Không phải entity có lịch sử reports trong store. [wire][wire], [dto.go][dto] |
| `JobEvent` | JobID, Status, Reason, At; nested trong Job, không Event.ID/actor riêng. CP tạo khi submit hoặc status/**reason** thay đổi. Có thể hai event cùng Status; không phải immutable audit log. [lifecycle][lifecycle] |
| `ClusterSummary` | Read model do `ControlPlane.Summary` tổng hợp; không identity/persistence riêng. RecentEvents lấy tối đa 20 event gần nhất. [controlplane.go][cp] |

## 2. Relationships thực tế

Mũi tên liền mô tả chứa/tham chiếu bằng field; mũi tên chấm là liên hệ runtime/observation. Sơ đồ không thêm bảng hoặc entity không tồn tại.

~~~mermaid
flowchart LR
    O["Organization"] -->|"1 : nhiều"| USR["User"]
    O -->|"1 : nhiều"| S
    O -->|"1 : nhiều"| J
    O -->|"Enrollment bind"| ENR["Enrollment / ServerOwnership"]
    ENR --> S
    AP["Agent process + PersistentState"] -. "AgentID = Server.ID" .-> S["domain.Server"]
    S -->|"GPUs: 0..n"| G["domain.GPU"]
    S -->|"Containers: 0..n"| C["domain.Container"]
    J["domain.Job - Workload tren UI"] -->|"Assignment: 0..1"| A["domain.Assignment - reservation"]
    A -->|"ServerID"| S
    A -->|"GPUUUIDs: 1..n khi commit"| G
    A -->|"CommandID - START"| CMD["domain.Command"]
    CMD -->|"AgentID"| S
    CMD -. "Payload.jobId" .-> J
    J -->|"Events: 0..n"| E["domain.JobEvent"]
    J -. "ContainerID - optional" .-> C
    C -. "JobID - optional, khong FK" .-> J
    C -. "GPUUUIDs - observed grants / usage" .-> G
    P["GPUProcess - trong InventoryReport"] -. "GPUUUID" .-> G
    P -. "ContainerID - optional" .-> C
    AP -. "collect / execute qua DockerRuntime" .-> R["agent.RuntimeContainer"]
    R -. "Collector + wire mapping" .-> C
~~~

Một Job được cấp nhiều GPU **trên cùng một Server**, không có distributed multi-server Job. External Container không bắt buộc liên kết Job. Nhiều container có thể khai cùng JobID trong report: reconciliation chọn một, ưu tiên container active hơn inactive; đây không phải quan hệ 1:1 được enforce.

## 3. State catalog, ý nghĩa và quyền thay đổi

### 3.1 Server status và Agent connectivity

Đủ ba giá trị `ServerStatus` trong [model.go][model]:

| State | Ý nghĩa hiện tại | Ai / trigger / function thay đổi |
|---|---|---|
| `ONLINE` | Đã nhận registration, heartbeat hoặc inventory; drain=false tại lần cập nhật. Chưa bảo đảm inventory mới hay có GPU dùng được. | `RegisterAgent → UpsertServer`; `Heartbeat`; `ReplaceInventory`; operator undrain qua `SetServerDrained` nếu chưa OFFLINE. |
| `DRAINING` | Host không được nhận placement mới vì drained=true, khi trạng thái chưa OFFLINE. Không có eviction/đợi “drain completed”. | Operator drain; heartbeat/inventory/re-enroll khi Drained=true. |
| `OFFLINE` | CP đánh dấu liveness quá hạn hoặc vừa restore snapshot. Không khẳng định Docker/container đã dừng. | `ControlPlane.Reconcile → MarkStaleServers`; [`durable.Open`][durable]. |

Không có Agent `UNKNOWN`, `REGISTERING`, `READY` hay `DEGRADED` enum. Chưa đăng ký nghĩa là chưa có Server record. `Drained` và `SchedulingReady` là boolean, không phải state machine độc lập.

`Heartbeat` sử dụng giờ CP nhận, bỏ qua thời gian gửi trong HeartbeatRequest. **Inventory được chấp nhận cũng cập nhật LastHeartbeatAt**: nếu chỉ endpoint heartbeat ngừng nhưng inventory vẫn về, Server không bị offline vì lý do đó. Ngược lại heartbeat còn về mà inventory stale thì Status có thể ONLINE nhưng schedulable=false.

### 3.2 GPU health và allocation/occupancy state

`Healthy` là bool observation, không có GPUHealth enum. Trong [NVML Reader.Snapshot][nvml], default true; false khi đọc utilization lỗi, MIG đang bật, ECC uncorrected > 0, hoặc truy vấn ECC lỗi ngoài NOT_SUPPORTED. Một số lỗi bắt buộc (UUID/memory/compute processes...) làm **cả snapshot thất bại**, không phát ra một GPU FREE. Đọc temperature lỗi chỉ bỏ metric; Healthy không đại diện mọi chẩn đoán phần cứng.

Đủ sáu `GPUState`; owner trạng thái là CP [`normalizeGPUState`][accounting], ngoại trừ reserve trực tiếp:

| State | Điều kiện / ý nghĩa | Trigger và function |
|---|---|---|
| `FREE` | Healthy, không có consumer/reservation nào thắng các nhánh ưu tiên. Chưa xét freshness Server hay yêu cầu Job. | Inventory hoặc release tính lại, nhánh default. |
| `RESERVED` | Tập UUID đã dành cho một Job; chưa có managed active observation thắng accounting. `AssignedJobID` được đặt. | `CommitAssignment`; hoặc normalize thấy nonterminal Job có Assignment mà không có điều kiện ưu tiên cao hơn. |
| `ALLOCATED` | Có container active, Origin MANAGED, Job tồn tại và Assignment khớp Server/UUID. | Normalize từ inventory hoặc last-known observations khi release. |
| `OCCUPIED_LEGACY` | Active container có GPU grant nhưng không đạt điều kiện managed allocation phía CP. | Normalize: external, UNKNOWN origin, hoặc MANAGED nhưng không khớp Job/Assignment đều vào nhánh này. |
| `OCCUPIED_UNKNOWN` | Có process không được map đúng container/GPU, nhiều managed Job consumers trên một UUID, hoặc telemetry usage chưa rõ owner. | Normalize từ process/conflict hoặc bảo lưu observation OCCUPIED_UNKNOWN do Agent gửi. |
| `UNHEALTHY` | `Healthy=false`; ưu tiên cao nhất, che các state occupancy/reservation ở field State. | Normalize từ GPU health observation. |

Không có API operator set trực tiếp GPU.State. `inventoryToDomain` khởi tạo FREE và chỉ bảo lưu wire state **OCCUPIED_UNKNOWN**; không tin ALLOCATED/RESERVED do Agent khai để cấp phát.

### 3.3 Job status

Đủ chín `JobStatus` trong [model.go][model]; mutation cụ thể ở [lifecycle.go][lifecycle] và [store.go][store]:

| State | Ý nghĩa | Owner / điểm vào |
|---|---|---|
| `QUEUED` | Đang đợi scheduler, có thể kèm lý do thiếu tài nguyên. | `ControlPlane.CreateJob`; `ScheduleOnce → SetJobStatus` cập nhật lý do khi vẫn queued. |
| ASSIGNED | Assignment PLANNED, chưa có START; đợi đến giờ và tài nguyên safe. | ReplanReservations. |
| STARTING | Đến giờ và revalidation thành công, START đã được enqueue; đợi inventory. | CommitAssignment; legacy ACK path vẫn giữ. |
| `RUNNING` | Thông thường đã quan sát container running/paused/restarting. Cũng là state phục hồi khi Stop ACK thất bại, chưa cần observation mới. | `reconcileObservedLocked`; `applyAckLocked` nhánh failed stop. |
| `STOPPING` | Đã yêu cầu dừng, Stop command được enqueue; container có thể còn active hoặc chưa thấy. | `RequestStop` khi Job có Assignment và Start không còn PENDING. |
| `STOPPED` | Sau stop request, inventory thấy exited/dead hoặc không thấy container. Không phải “stop ACK đã thành công”. | `reconcileObservedLocked` từ STOPPING. Terminal. |
| `SUCCEEDED` | Inventory thấy exited/dead, ExitCode=0, Job không STOPPING. | `reconcileObservedLocked`. Terminal. |
| `FAILED` | Start thất bại ở nhánh được phép; exit khác 0/không có exit code; hoặc missing ở nhánh được phép. | `applyAckLocked` / `reconcileObservedLocked`. Terminal. |
| `CANCELLED` | Hủy trước placement, hoặc hủy khi Start command còn PENDING. | `RequestStop`. Terminal. |

`JobStatus.Terminal()` trả true cho STOPPED/SUCCEEDED/FAILED/CANCELLED. Không có Job PENDING, SCHEDULING, RESERVED hay DISPATCHING. “Pending reason” trong UI/docs là `StatusReason` của Job QUEUED, không thêm state.

### 3.4 Reservation state

Field `Assignment.ReservationState` là string, bốn literal đang được ghi:

| State | Ý nghĩa | Owner / trigger |
|---|---|---|
| PLANNED | Giữ interval; chưa chiếm physical GPU/chưa có command. | ReplanReservations. |
| RESERVED | Đến giờ, GPU thực tế được giữ và START được tạo. | CommitAssignment. |
| `ALLOCATED` | Reconciliation thấy container cùng Job trên Server assigned ở running/paused/restarting. | `reconcileObservedLocked`, kể cả khi Job đang STOPPING. |
| `RELEASED` | Logical reservation đã kết thúc; Assignment vẫn giữ UUID và decision history. | `releaseLocked` khi Job chuyển terminal có Assignment. |

Không có Created/Expired/Cancelled reservation enum; queued Job chưa có Assignment. RESERVED có thể đi thẳng RELEASED; release không mặc định làm GPU FREE.

### 3.5 Command type/status

Type và payload tạo bởi [ControlPlane][cp], dispatch trong [CommandExecutor.Execute][executor]:

| Type | Ý nghĩa |
|---|---|
| `START_CONTAINER` | Recheck local GPU occupancy rồi tạo/reuse container đúng Job ownership, dùng exact UUIDs. |
| `STOP_CONTAINER` | Dừng container của Job sau kiểm tra labels; absent/already stopped được xem là thành công. |

Đủ bốn `CommandStatus`:

| State | Ý nghĩa | Owner / trigger |
|---|---|---|
| `PENDING` | Đã enqueue, chưa lease. | `ScheduleOnce → CommitAssignment` hoặc `StopJob → RequestStop`. |
| `DELIVERED` | CP đã lease cho Agent; **không phải xác nhận Agent đang thực thi**. | `LeaseCommands`; mỗi lần delivery/redelivery tăng Attempts, ghi DeliveredAt và LeaseUntil mới. |
| `SUCCEEDED` | ACK đầu tiên khai succeeded=true. Không đồng nghĩa Job đã SUCCEEDED hoặc STOPPED. | `AckCommand`. |
| `FAILED` | ACK đầu tiên khai failure, hoặc CP hủy Start PENDING trước dispatch. | `AckCommand` hoặc `RequestStop`. |

Không có EXECUTING, ACKED, EXPIRED hay CANCELLED command state. Lease hết hạn vẫn DELIVERED đến khi poll lease lại; terminal command không được retry tự động.

### 3.6 Container origin và runtime state

`ContainerOrigin` ([model.go][model]) có ba giá trị, là **phân loại observation**, không phải tiến độ thực thi:

| Origin | Cách tạo/cập nhật |
|---|---|
| `MANAGED` | Agent `classifyContainer` thấy `aiwm.managed=true` (EqualFold) và `aiwm.job-id` không rỗng sau TrimSpace. CP `inventoryToDomain` nhận origin MANAGED không kiểm chứng lại labels. |
| `LEGACY` | Agent phân loại mọi container còn lại. UI gọi là External. CP cũng nhận literal LEGACY. |
| `UNKNOWN` | CP map origin ngoài MANAGED/LEGACY (bao gồm rỗng) về UNKNOWN. Collector hiện tại không tự sinh UNKNOWN origin. |

Origin được tính lại/thay cùng snapshot; không có API “adopt” hoặc transition command đổi LEGACY thành MANAGED.

`Container.State`, `RuntimeContainer.State` và wire Container.State đều là **string mở**, không có ContainerState/WorkloadState enum của AIWM. Docker adapter copy raw state từ SDK, CP không validate danh sách state. Đây là toàn bộ các literal runtime mà code AIWM có nhánh xử lý rõ:

| Runtime string | Cách code sử dụng / owner |
|---|---|
| `created` | Engine gặp container cùng Job đã tạo thì thử ContainerStart khi xử lý Start lại. Accounting không coi active; reconciliation chưa có nhánh riêng cho state này. |
| `running` | Active, chiếm GPU grants; có thể đưa Job ASSIGNED/STARTING → RUNNING. Docker hoặc simulator tạo/cập nhật. |
| `paused` | Vẫn active/chiếm GPU; Job dùng RUNNING, không có Job PAUSED. Docker observation, AIWM không có pause API. |
| `restarting` | Vẫn active/chiếm GPU; Job dùng RUNNING, không có Job RESTARTING. Docker observation, không có AIWM restart command. |
| `exited` | Không active; reconciliation kết thúc Job theo stop intent/ExitCode. Docker hoặc simulator stop cập nhật. |
| `dead` | Xử lý kết thúc như exited. Docker observation. |
| Giá trị khác hoặc rỗng | Copy và hiển thị như nhận được; không active theo helper hiện tại. Nếu có container record nhưng state không được xử lý, reconciliation chỉ cập nhật ContainerID/LastObservedAt, không coi là missing và không kết thúc Job. |

Không đưa một Docker lifecycle đầy đủ vào tài liệu như thể AIWM đã enforce nó. Mất container khỏi snapshot là **điều kiện absence**, không phải một literal DELETED/MISSING trong model.

### 3.7 Các enum/flag khác đã audit

| Scope | Giá trị đang có | Owner / semantics |
|---|---|---|
| `SchedulingStrategy` | `first-fit`, `best-fit`, `bin-pack`, `fragmentation-aware` | Control Plane config chọn, Create DTO không nhận strategy; `ValidStrategy` validate; [Scheduler.Plan][scheduler] thực thi bốn policy. Không phải lifecycle state. |
| `ExecutionBackend` | Go khai báo `DOCKER` **và `KUBERNETES`** | `CreateJob` default DOCKER; `validateJobRequest` chỉ nhận DOCKER. KUBERNETES là constant còn tồn tại, chưa có execution implementation; OpenAPI/frontend chỉ nhận DOCKER. |
| `AllowSharedGPU` | bool, public API chỉ chấp nhận false | Không có mode sharing/MIG/time-slicing implementation. |
| Protocol | `ProtocolVersion="v1"` | Register kiểm tra version; không phải Agent status. |
| Process probes | CP `/healthz`: `ok`; `/readyz`: `ready` | [httpapi handlers][http]. Không mô tả readiness của GPU/Agent. |
| Console connectivity | `HealthStatus.status`: `ok` / `unreachable` | [BFF health GET][bff-health] dựa vào fetch /healthz; không lưu vào domain.Server. |

### 3.8 Business enums và PolicyStatus

`domain/allocation.go` thêm bốn typed enums, độc lập Job.Status:

| Type | Toàn bộ giá trị | Owner/trigger |
|---|---|---|
| WorkloadType | TRAINING, INFERENCE | User chọn; validate admission, bất biến sau Create. |
| NecessityLevel | NECESSITY_1, NECESSITY_2, NECESSITY_3, NECESSITY_4 | User chọn; policy.Validate kiểm tra level/reason. |
| SystemImportance | CRITICAL_SPECIAL, VERY_IMPORTANT, IMPORTANT, NORMAL | User chọn category; backend derive 1.4/1.2/1.0/0.8. |
| PolicyStatus | AUTO_ELIGIBLE, COMPETITIVE | Engine.Evaluate: plan && quota → AUTO_ELIGIBLE; còn lại COMPETITIVE. |

Evaluate lại có thể chuyển hai PolicyStatus qua lại khi facts đổi, tại submit/GET Queue/cycle; đây là kết quả đánh giá, không thêm Job state. Provider demo không tự đổi facts khi Job chạy. Job cũ mang zero-value business fields; không invent enum UNKNOWN.

## 4. State transitions

### 4.1 Job lifecycle

Sơ đồ gồm các đường lifecycle chính và đường tắt do inventory/ACK; bảng phía dưới ghi đầy đủ guards của các nhánh hiện hành.

~~~mermaid
stateDiagram-v2
    [*] --> QUEUED
    QUEUED --> ASSIGNED: PLANNED interval; chưa có command
    QUEUED --> CANCELLED: stop before placement
    ASSIGNED --> CANCELLED: cancel hoặc hết interval trước dispatch
    ASSIGNED --> STARTING: start reached + revalidate + START
    ASSIGNED --> RUNNING: active inventory before ACK
    STARTING --> RUNNING: active inventory
    ASSIGNED --> SUCCEEDED: early exit code 0
    STARTING --> SUCCEEDED: exit code 0
    RUNNING --> SUCCEEDED: exit code 0
    ASSIGNED --> FAILED: failed START or unsuccessful exit
    STARTING --> FAILED: failed START or exit or eligible absence
    RUNNING --> FAILED: unsuccessful exit or absence
    ASSIGNED --> STOPPING: stop after START delivery
    STARTING --> STOPPING: stop request
    RUNNING --> STOPPING: stop request
    STOPPING --> RUNNING: failed STOP ACK
    STOPPING --> FAILED: failed START ACK
    STOPPING --> STOPPED: exit or absence
~~~

| Từ / sang | Guard và hiệu ứng chính | Function |
|---|---|---|
| Mới → QUEUED | Request hợp lệ; tạo event đầu tiên. | `ControlPlane.CreateJob` |
| QUEUED → QUEUED | Chưa tìm được placement, đổi StatusReason nếu cần; không thêm event nếu cả status/reason không đổi. | `ScheduleOnce → SetJobStatus → transitionLocked` |
| QUEUED → ASSIGNED | Revalidate Server/GPU/count/model/VRAM/labels dưới lock; commit Assignment + GPU RESERVED + Start PENDING cùng transaction. | `CommitAssignment` |
| QUEUED → CANCELLED | Không enqueue Stop; chưa có reservation trong flow bình thường. | `RequestStop` |
| ASSIGNED/STARTING/RUNNING → CANCELLED nếu Start còn PENDING | Guard thực tế dựa trên **command.Status**, không chỉ Job.Status. ASSIGNED là ca thường; RUNNING vẫn có thể xảy ra nếu inventory đi trước lease. Start bị đánh FAILED, release Assignment. STARTING+PENDING không được tạo bởi flow bình thường. | `RequestStop` |
| ASSIGNED/STARTING/RUNNING → STOPPING nếu Start không PENDING | Có Assignment; tạo Stop command PENDING, giữ reservation. Job STOPPING hoặc terminal nhận stop lặp thì không đổi. | `RequestStop` |
| ASSIGNED → STARTING | Start ACK succeeded; lưu ContainerID. ACK thành công ở RUNNING/STOPPING không lùi Job về STARTING. | `applyAckLocked` |
| ASSIGNED/STARTING → RUNNING | Có MANAGED Container cùng Job trên assigned Server ở running/paused/restarting; Assignment → ALLOCATED. RUNNING/STOPPING giữ Job.Status nhưng vẫn cập nhật observation/Assignment. | `reconcileObservedLocked` |
| ASSIGNED/STARTING/STOPPING → FAILED | Start ACK thất bại; release. Nếu Job đã RUNNING, failed Start ACK không làm Job FAILED. | `applyAckLocked` |
| STOPPING → RUNNING | Stop ACK thất bại; giữ Assignment. Nhánh này không yêu cầu container đã được quan sát active. | `applyAckLocked` |
| ASSIGNED/STARTING/RUNNING → SUCCEEDED hoặc FAILED | Có exited/dead: exitCode=0 → SUCCEEDED, khác 0 hoặc nil → FAILED; release. Không cần Start ACK đến trước. | `reconcileObservedLocked` |
| STOPPING → STOPPED | Inventory thấy exited/dead (bất kể exit code), hoặc không thấy Container cùng Job. Release; không kiểm tra Stop ACK đã thành công. | `reconcileObservedLocked` |
| RUNNING → FAILED khi missing | Complete inventory không có MANAGED Container cùng Job. | `reconcileObservedLocked` |
| STARTING → FAILED khi missing | Chỉ khi `report.ObservedAt >= job.UpdatedAt`; dùng giờ Agent so với giờ CP của state update. | `reconcileObservedLocked` |
| ASSIGNED giữ nguyên khi missing | Chưa có timeout launch/reservation riêng. | `reconcileObservedLocked` |
| Terminal giữ nguyên | ACK, inventory hoặc stop lặp không hồi sinh Job; Assignment/event history vẫn còn. | `Terminal`, `transitionLocked`, `applyAckLocked`, `RequestStop` |

Stop ACK thành công **không tự chuyển state Job**, kể cả chưa có inventory. `ControlPlane.Reconcile` theo timer chỉ đánh dấu Server offline; Job reconciliation diễn ra ngay trong transaction `ReplaceInventory`.

**Giới hạn cần biết:** STOPPING + absence chưa kiểm tra Start command còn DELIVERED hay không. Vì vậy một inventory không thấy container trong lúc Start đang thực thi có thể làm Job STOPPED/release sớm; quy tắc lease Stop chờ Start ACK không ngăn nhánh này. Đây là suy luận từ nhánh code, chưa có test riêng chứng minh thứ tự cạnh tranh đó.

### 4.2 Agent/Server lifecycle

Sơ đồ là **Server.Status mà CP dùng quan sát Agent**, không định nghĩa AgentStatus mới.

~~~mermaid
stateDiagram-v2
    [*] --> ONLINE: first enrollment
    ONLINE --> DRAINING: drain true
    DRAINING --> ONLINE: drain false
    ONLINE --> OFFLINE: stale LastHeartbeatAt or CP restore
    DRAINING --> OFFLINE: stale LastHeartbeatAt or CP restore
    OFFLINE --> ONLINE: heartbeat or inventory or re-enroll, drained false
    OFFLINE --> DRAINING: heartbeat or inventory or re-enroll, drained true
~~~

- `SetServerDrained` khi OFFLINE chỉ đổi Drained, giữ OFFLINE; undrain không tự hồi sinh connectivity.
- Heartbeat/inventory tiếp theo có thể giữ ONLINE/DRAINING. Re-enroll cũng đặt state tương ứng Drained nhưng **reset inventory freshness**.
- Restore durable đặt mọi Server OFFLINE và InventoryReceivedAt zero. Heartbeat đủ đổi status về ONLINE/DRAINING; cần inventory mới để nhận placement.
- Offline không đổi Job, Assignment, GPU state hoặc Container state last-known; vẫn có thể thấy GPU ALLOCATED / Job RUNNING trên Server OFFLINE.

### 4.3 Reservation lifecycle

~~~mermaid
stateDiagram-v2
    [*] --> PLANNED: ReplanReservations
    PLANNED --> RESERVED: StartAt reached; CommitAssignment
    PLANNED --> RELEASED: Cancel trước dispatch
    RESERVED --> ALLOCATED: observed active managed container
    RESERVED --> RELEASED: terminal before active observation
    ALLOCATED --> RELEASED: terminal Job
~~~

Node đầu biểu diễn Assignment chưa tồn tại, không phải state CREATED. Không có nhánh ALLOCATED → RESERVED của **Assignment** trong flow hiện tại. GPU.State có thể khác ReservationState vì được normalize từ health/consumer evidence. EndAt tạo STOP/cancel; lease timeout, drain hoặc heartbeat timeout không tự release.

### 4.4 Command lifecycle

~~~mermaid
stateDiagram-v2
    [*] --> PENDING: atomic enqueue
    PENDING --> DELIVERED: Agent poll and lease
    DELIVERED --> DELIVERED: expired lease, Agent polls again
    DELIVERED --> SUCCEEDED: successful ACK
    DELIVERED --> FAILED: failed ACK
    PENDING --> SUCCEEDED: ACK accepted without lease guard
    PENDING --> FAILED: failed ACK or cancel before delivery
~~~

`LeaseCommands` chỉ lease đúng AgentID; Stop command thường đợi START PENDING/DELIVERED, ngoại lệ START DELIVERED đã hết interval. Không có timer đổi DELIVERED về PENDING hoặc FAILED, không có max-attempt cutoff phía CP. START leasing kiểm freshness/drain, ownership, GPU và interval lần nữa. START DELIVERED hết interval không giao lại; STOP đi qua để giải quyết execution/ACK chưa rõ.

`AckCommand` chỉ kiểm tra command thuộc Agent và chưa terminal, chưa bắt buộc đã DELIVERED. ACK đầu tiên kết thúc command và áp dụng hiệu ứng Job cùng transaction; ACK lặp trả command cũ. Runner persist result trước ACK, replay ACK cho ID đã xử lý; đây không phải bảo đảm exactly-once vô hạn qua mọi failure, vì local history có giới hạn.

## 5. External/Existing và Managed: ownership, mutation, reconciliation

### 5.1 Ba lớp xác định ownership

1. **Docker → Agent:** `classifyContainer` chỉ đọc hai labels `aiwm.managed` và `aiwm.job-id`. Thiếu/sai labels → LEGACY, không tự adopt.
2. **Agent → CP inventory:** `inventoryToDomain` normalize origin literal; giữ JobID/Labels theo wire. Không xác thực lại quan hệ labels–Job.
3. **CP GPU accounting:** chỉ ALLOCATED cho UUID khi Container MANAGED, Job tồn tại, Assignment cùng Server và chứa UUID. Container không thỏa điều kiện được tính như external occupancy, **không rewrite Container.Origin**.

Vì vậy Container có badge MANAGED nhưng GPU là OCCUPIED_LEGACY là kết quả có thể xảy ra khi mất Job/Assignment hoặc GPU grant khác assignment. UNKNOWN origin với active grant cũng có thể cho GPU OCCUPIED_LEGACY; không đồng nhất UNKNOWN origin với OCCUPIED_UNKNOWN GPU.

### 5.2 Workload nào được stop/delete?

| Thao tác | Quyền và hành vi đang implement |
|---|---|
| Stop từ public API | Chỉ `POST /jobs/{id}/stop`; CP tìm Job, hủy nếu chưa dispatch hoặc tạo Stop theo Assignment.ServerID. Không có public stop-by-arbitrary-container-ID. |
| Agent stop Managed | `CommandExecutor.Execute → Engine.StopManagedJob`; kiểm tra label managed=true **và** JobID đúng payload. Optional ContainerID nếu chỉ tới một container có thật nhưng không thuộc Job sẽ trả ErrLegacyTarget. |
| External workload | Không gửi stop/restart/delete/adopt tự động. Không tạo Job hoặc Assignment từ discovery. Chủ bên ngoài quyết định lifecycle. |
| Delete container / Job / workload | Không có public API hay command DELETE. Stop thông thường để lại container đã exited trong Docker inventory. |
| Cleanup khi create-start lỗi | Có **ContainerRemove(Force=true)** trong `Engine.StartManagedContainer`, chỉ nhắm ID container vừa được lần gọi đó tạo khi ContainerStart lỗi. Không phải chức năng delete workload tùy ý, không nhắm External. |
| Disconnect / shutdown Agent | Runner ngừng/retry loops; không duyệt container để stop/remove. Actual runtime nằm ngoài CP/Agent state. |

Ownership hiện dựa trên labels và trusted Agent, không có chứng thực nguồn tạo container chống operator Docker tự đặt labels. Production adapter idempotently reuse container cùng labels/JobID; nếu đã `created` thì thử start, các state khác trả lại ID hiện có. Không có general restart/retry workload API.

### 5.3 Occupancy và reconciliation

- **External:** active grant chiếm GPU dù utilization=0. Container exited/dead không chiếm qua grant; nếu vẫn còn process/unknown telemetry thì GPU có thể tiếp tục bị chặn.
- **Managed:** nonterminal Assignment đã activate bảo vệ physical UUID trước container; PLANNED chỉ giữ interval. Active observation đúng assignment chuyển GPU ALLOCATED. Job reconciliation dùng Container.JobID/Origin và assigned Server; không đối chiếu exact GPUUUIDs như accounting.
- **Unknown process:** PID không map được vào active container cùng UUID vẫn chặn GPU. Không có một managed Job được tạo từ PID.
- **Grant không rõ:** numeric index map sang UUID bằng snapshot NVML hiện tại; unknown index/UUID hoặc unbounded grant trên active container bảo vệ toàn bộ UUID của host. Không giải quyết bằng giả định GPU free.
- **Job exit/missing:** reconciliation chỉ duyệt Job nonterminal có Assignment đúng Server. External không tham gia Job lifecycle; terminal Job không tự reopen nếu container xuất hiện lại. Accounting vẫn xét active consumer đó để bảo vệ GPU.
- **Inventory lỗi/stale:** Agent không publish snapshot khi Docker/NVML collect bắt buộc lỗi; CP từ chối sequence cũ/UUID conflict. Giữ last-known state, freshness cuối cùng sẽ chặn placement. Không coi mất kết nối là snapshot rỗng.

## 6. GPU state semantics và resource accounting

| Khái niệm | Biểu diễn / điều kiện chính xác | Không suy ra được |
|---|---|---|
| Healthy | `GPU.Healthy=true` theo observation NVML hiện tại. | Không bảo đảm free, connected hay không có workload. |
| Online | State/connectivity của **Server**, không có GPU.Online. | GPU trong last-known inventory vẫn tồn tại khi server offline. |
| Available memory | `AvailableMemoryMiB() = max(total-used, 0)`. | VRAM còn nhiều không cho phép chia GPU đang được chiếm. |
| Free | `GPU.State=FREE` sau accounting. | Không bảo đảm Server fresh/undrained hoặc khớp model/count/labels/VRAM Job. |
| Reserved | Assignment RESERVED hoặc GPU RESERVED: logical claim đang bảo vệ UUID; hai field phải đọc theo scope riêng. | Không chứng minh container đã tạo/chạy. |
| Allocated | GPU ALLOCATED là active managed consumer khớp assignment; Assignment ALLOCATED là Job đã được quan sát active. | Không đồng nghĩa GPU utilization > 0, Job chắc chắn đang RUNNING, hoặc Server đang online. |
| Occupied | Có consumer/grant/unknown evidence; hoặc từ tổng hợp `GPUsOccupied` với nghĩa rộng hơn bên dưới. | Không có một bool Occupied riêng hay phép đo duy nhất bao trùm mọi semantics. |
| Schedulable GPU | `GPU.Schedulable()` = Healthy && State==FREE && AssignedJobID rỗng. | Helper này chưa xét Server hoặc Job. |
| Schedulable Server | Status==ONLINE, !Drained, receipt inventory khác zero, tuổi LastHeartbeatAt và InventoryReceivedAt đều <= offlineAfter. | Server schedulable vẫn có thể không có đủ GPU cho Job. |
| Schedulable cho Job | Hai helper trên + profile/FP8 qua ResolvedModels, MinVRAM trên từng GPU, đủ GPUCount **ở cùng host**; được recheck tại commit. | Chưa có host CPU/RAM admission accounting, GPU sharing, placement đa host. |
| Unknown occupancy | OCCUPIED_UNKNOWN do process không rõ owner hoặc telemetry vượt ngưỡng nhưng không có active attribution. Collector default memory >=256 MiB **hoặc** utilization >=5%; có thể override bằng InventoryPolicy/config. | Unknown không có nghĩa free; usage thấp hơn ngưỡng không chứng minh tuyệt đối không có consumer. |

**Thứ tự ưu tiên thật của `normalizeGPUState`** (chọn nhánh đầu tiên phù hợp):

~~~text
!Healthy
→ active container không khớp managed assignment
→ process chưa quy thuộc / multiple managed Job consumers
→ active managed container khớp assignment
→ Agent observation OCCUPIED_UNKNOWN
→ nonterminal Job có Assignment chứa UUID
→ FREE
~~~

Các mũi tên trên là **ưu tiên đánh giá**, không phải vòng đời GPU. Mỗi inventory có thể đổi giữa bất kỳ kết quả phù hợp; không có state machine GPU tuyến tính được enforce.

Reservation map được dựng từ nonterminal Job + Assignment, **không đọc ReservationState**. Active managed map không yêu cầu Job nonterminal. Do đó:

- Assignment RELEASED nhưng active container vẫn thấy → GPU vẫn có thể ALLOCATED, tiếp tục không schedulable.
- Assignment ALLOCATED nhưng GPU unhealthy/external/unknown → GPU.State phản ánh nhánh ưu tiên cao hơn.
- GPU AssignedJobID có thể rỗng khi reservation vẫn tồn tại nhưng bị health/occupancy che; phải đọc Job.Assignment để xem claim.
- `releaseLocked` normalize last-known containers với processes=nil; nhánh OCCUPIED_UNKNOWN có thể được bảo lưu từ GPU state cũ cho đến inventory mới. Không thể xem release là bằng chứng vật lý “đã hết process”.

**Dashboard/summary không phải một partition đầy đủ của pool:** `GPUsTotal` đếm cả last-known GPU offline; `GPUsOccupied` đếm mọi State != FREE, kể cả RESERVED và UNHEALTHY. `GPUsFree` chỉ đếm GPU FREE trên server schedulable. Bởi vậy gpusFree + gpusOccupied có thể nhỏ hơn gpusTotal (ví dụ FREE trên host drained/offline). `ServersOnline` bao gồm DRAINING có heartbeat mới; `JobsRunning` chỉ đếm Job RUNNING, không đếm STARTING/STOPPING. Các công thức ở [ControlPlane.Summary][cp].

## 7. Domain invariants đang enforce và bằng chứng test

“Enforce” dưới đây nằm trong đường đi ứng dụng hiện tại và các điều kiện được nêu. Không suy thành distributed lock với mọi ứng dụng ngoài AIWM. Test được liệt kê là test có trong source, **không phải tuyên bố đã chạy lại trong lần viết tài liệu này**.

| Invariant / phạm vi | Implementation | Test có thật / giới hạn bằng chứng |
|---|---|---|
| Hai commit đồng thời không reserve cùng UUID cho hai Job trong repository này. | [`CommitAssignment`][store] lock, recheck `GPU.Schedulable` trước mutation. [durable][durable] serialize/rollback, lock một writer. | [memory safety][memory-safety]: `TestConcurrentReservationHasOneWinner` (40 goroutines); [memory tests][memory-tests]: `TestCommitAssignmentIsAtomic`. Không chứng minh exclusion với process Docker ngoài AIWM hoặc nhiều CP dùng state file khác nhau. |
| Commit sai UUID/count/duplicate không để lại partial reservation; Job + Assignment + GPU + Start command commit cùng nhau. | [store][store], [ports.Repository][ports], [durable transactions][durable]. | `TestInvalidReservationDoesNotMutateGPU`; `TestCommitAssignmentIsAtomic`; [durable tests][durable-tests] `TestDiskFailureRollsBackAndLockExcludesSecondWriter`. |
| Start trong flow `ScheduleOnce` chỉ được dispatch sau reservation commit. | [CP][cp] tạo payload → `CommitAssignment`; poll chỉ đọc command trong store. | Atomic tests ở trên kiểm tra commit/resource conflict. Chưa có test riêng cho mọi thứ tự commit-vs-poll. `EnqueueCommand` vẫn là port nội bộ tổng quát, không enforce reservation; không được application dùng để enqueue Start hiện tại. |
| Không nhận **placement mới** trên offline/drained/stale/no-inventory Server, kể cả Plan xong rồi state đổi. | [Server.Schedulable][model], [Scheduler.filter][scheduler], commit recheck. | [memory safety][memory-safety] `TestReservationRechecksServerAndInventory`; [scheduler tests][scheduler-tests] `TestPlanRejectsOfflineServer`; [scheduler safety][scheduler-safety] `TestSchedulerFiltersAndPendingReasons`. Inventory cũng là heartbeat; không viết invariant “mất riêng heartbeat endpoint luôn offline”. |
| External/unknown/unhealthy/reserved/allocated GPU không được scheduler chọn; trước start có local recheck. | [GPU.Schedulable][model], [accounting][accounting], [InventoryCollector.GPUsAvailable][inventory], [CommandExecutor mutex][executor]. | `TestBestFitExcludesLegacyGPU`; `TestGPUAvailabilityRejectsLegacyAndUnmappedProcesses`; `TestUnattributedTelemetryBlocksPlacement`; `TestKnownContainerDoesNotHideUnexpectedGPUProcess`; `TestNVIDIAMockProfiles`. External actor vẫn có thể tạo workload sau lần recheck. |
| Active External grant phải chiếm GPU kể cả idle; stopped External không chiếm qua grant. | [inventory][inventory], [containerConsumesGPU / normalizeGPUState][accounting]. | [memory tests][memory-tests] `TestInventoryClassifiesLegacyAndUnknownConsumers`, `TestStoppedLegacyContainerDoesNotOccupyGPU`; [inventory tests][inventory-tests] `TestUnboundedLegacyGPUGrantProtectsWholeServer`; [inventory safety][inventory-safety] `TestDockerNumericAndUnknownDeviceMapping`. |
| AIWM không tùy ý stop/delete External; chỉ stop đúng managed labels + Job. | [Engine.StopManagedJob][engine], [executor][executor]; public API nhận Job ID, không arbitrary container operation. | [engine tests][engine-tests] `TestStopManagedJobRefusesLegacyContainerID`; [integration][integration] `TestBrownfieldEndToEndLegacyWorkloadSurvives`. Cleanup container vừa create-start lỗi là ngoại lệ Managed được mô tả ở mục 5. |
| Device request Managed dùng đúng UUID, không broad “all”; public request không cho sharing/GPU visibility override. | [engine][engine], [validateJobRequest][validation]. | `TestStartManagedContainerUsesExactGPUUUIDsAndSafeDefaults`; [HTTP tests][http-tests] `TestPublicValidationAndSecretRedaction`. |
| Terminal Job không hồi sinh vì ACK lặp/muộn; failed launch release logical reservation cùng transaction. | [AckCommand][store], [applyAckLocked / transitionLocked / releaseLocked][lifecycle]. | `TestFailedStartReleasesAndDuplicateACKCannotResurrect`; `TestObservedLifecycleReleasesResources` (inventory trước ACK, success/failure/missing/stopped). Không bao phủ mọi interleaving stop/start. |
| Successful Stop ACK không tự giải phóng reservation; release theo Job terminal, vẫn bảo vệ last-known active consumers. | [applyAckLocked / releaseLocked][lifecycle], [accounting][accounting]. | `TestObservedLifecycleReleasesResources` và integration cover terminal release; chưa có test riêng cho Stop ACK đến trước inventory hoặc terminal Job còn active. Nhánh STOPPING+absence có giới hạn ở mục 4.1. |
| Redelivered Command.ID đã lưu result không execute lại trong local history; ACK đầu tiên quyết định phía CP. | [Runner.pollAndExecute/remember][runner], [Agent FileStore][agent-state], [AckCommand][store]. | [runner test][runner-tests] `TestRunnerExecutesRedeliveredCommandOnlyOnce`; [state test][state-tests] `TestFileStoreRoundTrip`; duplicate-ACK test ở memory. History eviction/crash trước persist không nằm trong bảo đảm của test. |
| Offline/CP restart giữ last-known Job/inventory; không tự stop container hoặc tự reschedule Job đã assign. | [MarkStaleServers][store], [Runner.Run/loops][runner], [durable.Open][durable]. | `TestCancelQueuedJobAndOfflineInventoryPreserved`; `TestRestartPreservesStateAndExcludesStaleInventory`; [recovery.py][recovery] kiểm tra disconnect/restart với simulator. Chưa có real NVIDIA/CUDA survival test. |
| Sequence cũ bị từ chối; UUID không được xuất hiện trên hai Server inventories cùng lúc. | [inventoryToDomain][mapping] + [ReplaceInventory][store]. | Chưa thấy test chuyên biệt cho stale sequence/global duplicate UUID trong các Go test đã audit; không gán atomic-reservation test thành bằng chứng cho invariant này. |

Invariants allocation bổ sung:

| Invariant | Implementation | Test |
|---|---|---|
| Preview không tạo Job/command/reservation | PreviewJob: prepare + read/Plan | TestResourceMatchingAndPreviewHasNoMutation |
| Submit không tin preview, không bypass queue | prepareJob/matchJob + Queue.Order + CommitAssignment | TestSubmitReevaluatesPreviewAndSchedulingRechecks |
| Necessity không bị auxiliary score đảo trong cùng lane | policy.Before | TestLexicographicOrdering |
| Profile/FP8 recheck lúc commit | ResourceRequest.Matches + private ResolvedModels | TestPolicyOrderDeterminesWinnerAndCommitRechecksCapability; TestCatalogFP8ResolutionAndUnknownModels |

## 8. Source of truth: domain, persistence, API và runtime

| Lớp | Nguồn chính xác | Mapping / khác biệt |
|---|---|---|
| CP business state | [domain/model.go][model]; mutation qua [ports.Repository][ports], [store][store], [lifecycle][lifecycle], [accounting][accounting]. | Source of truth của Job/Assignment/command/normalized GPU tại CP. `Scheduler.Plan` là proposal, không phải allocation đã commit. |
| Actual runtime / hardware | Docker Engine và NVML; [RuntimeContainer][agent-contracts], [engine][engine], [NVML][nvml]. | Source của actual observation. CP chỉ biết snapshot gần nhất; JobStatus không thay thế Docker State, OFFLINE không đồng nghĩa container dừng. |
| CP persistence đang chạy | [memory.Snapshot][snapshot] Version=1; [durable.Store][durable] ghi gob. | Dùng trực tiếp domain.Server/Job/Command trong maps; không có ORM/SQL entity mapping hiện hành. Machines map là index; GPU/Container nested Server, Assignment/Events nested Job. Gob chứa TokenHash và Job spec đầy đủ dù JSON có redaction. |
| SQL target chưa chạy | [001_init.sql][sql1], [002_lifecycle_target.sql][sql2]. | Chỉ reference schema. Có columns như agent_commands.job_id, jobs.server_id/assigned_gpu_uuids, observed_containers table; không được xem là runtime entity/constraint đã enforce. Không có SQL adapter/conversion/migration runner. |
| Agent persistence | [agent.PersistentState][agent-contracts], [state/file.go][agent-state]. | JSON chứa token, identity, sequence, processed results; độc lập với CP gob và Docker runtime. |
| Simulator persistence | [simulator/agent.go][sim]. | JSON map RuntimeContainer, độc lập Agent identity file. Không có GPU domain DB mô phỏng thay NVML. |
| Agent wire v1 | [agentprotocol/v1/types.go][wire]. | DTO độc lập domain: enum fields nhiều chỗ là string; GPU chưa có AssignedJobID/StateReason; Command chỉ ID/Type/Payload. Collector tạo report, `ControlPlane.PollCommands` tạo command wire. |
| Wire → domain | [application/agent_protocol.go][mapping], [controlplane.go][cp]. | `inventoryToDomain` validate/mapping GPU, origin, container, process; CP gán ReceivedAt. `AckCommand` map ACK wire → domain ACK; enrollment map request → Server. |
| Public API | [domain/dto.go][dto], [httpapi/server.go][http], [httpapi/job_view.go][job-view], [OpenAPI][openapi]. | CreateJobRequest/DrainServerRequest là inputs; Job response qua `publicJob/publicJobs` thành `JobView`, environment values = [redacted]. Server/GPU/Container phần lớn serialize domain trực tiếp; flat inventory thêm serverId/serverName. TokenHash không ra JSON. |
| Server read presentation | [ControlPlane.presentServer][cp]. | `SchedulingReady`/SchedulingReason tính ở ListServers/GetServer/SetServerDrained. Scheduler gọi helper trực tiếp, không tin field trình bày này. Agent heartbeat/inventory responses chưa qua bước decorate tương tự. |
| Frontend/BFF | [types.ts][frontend-types], [API endpoints][frontend-endpoints], [workloads page][workloads-page]. | Types sao chép contract công khai, không sở hữu lifecycle. “Workload” = Job; LEGACY hiển thị External. Assignment.reservationState là TS union trong khi Go là string. Không có DTO ManagedWorkload/Reservation riêng để CRUD. |

## 9. Consistency audit và giới hạn hiện tại

Đã đối chiếu [README](../README.md), [SYSTEM_DESIGN](SYSTEM_DESIGN.md), [TRACEABILITY](TRACEABILITY.md), backend README, Agent protocol, OpenAPI, frontend types và các source/test được dẫn ở trên. Các state chính Server/GPU/Job/Command khớp tên khai báo; những khác biệt đáng chú ý:

| Phát hiện từ source | Hệ quả cần hiểu / mức kiểm chứng |
|---|---|
| `BackendKubernetes="KUBERNETES"` còn trong domain nhưng validation chỉ cho DOCKER; API/frontend cũng chỉ DOCKER. | Constant không phản ánh execution capability đang chạy. Không document Kubernetes như một backend đã implement. |
| Không có Agent/Workload/ExternalWorkload/ManagedWorkload/Reservation entity độc lập. | Các tên này trong UI/docs là process, category hoặc field embedded, như mapping mục 1. Đây là khác biệt thuật ngữ, không phải thiếu type cần tạo thêm cho tài liệu. |
| ReservationState và Container.State là string; `SetJobStatus`/`transitionLocked` chưa validate toàn bộ adjacency graph. | TS/OpenAPI chặt hơn Go ở ReservationState. Generic repository setter có thể nhận state ngoài lifecycle bình thường nếu caller nội bộ dùng sai; các guard cụ thể, không một formal FSM, đang giữ flow hiện tại. |
| STOPPING + inventory absence không đợi Start DELIVERED hoàn tất. | Có khả năng Job terminal/release trước một late start; lease Stop chờ Start ACK không đủ chứng minh loại trừ race này. Phân tích tĩnh, chưa có test interleaving riêng; không sửa code trong lần làm tài liệu. |
| Stop ACK failure đặt Job RUNNING, kể cả chưa có active observation. | Đọc RUNNING cần hiểu cả nhánh điều khiển này; không luôn là chứng cứ container đang chạy. |
| Inventory nói MANAGED không được CP kiểm chứng lại labels; accounting và Job reconciliation match khác nhau. | Badge không bảo đảm allocation đã được CP nhận; Job có thể được reconcile active trong khi GPU grant không khớp assignment và accounting chặn như LEGACY. Quyền thực thi stop vẫn kiểm tra labels ở Agent. |
| Terminal Job vẫn có thể có active managed consumer; release không xóa Assignment và managed accounting không lọc terminal Job. | Có thể thấy Assignment RELEASED + GPU ALLOCATED. Đây là cơ chế giữ occupancy, không được ép FREE để “đồng bộ” tên state. |
| `presentServer` chỉ dùng trên public Server read/drain endpoints; heartbeat/inventory trả raw Server. | Cùng Server có thể trả schedulable=false/default và reason rỗng ở Agent response nhưng public Server GET tính true. Scheduler dùng helper nên không chịu quyết định từ field mặc định này. |
| `CommitAssignment` đổi GPU.State/AssignedJobID nhưng chưa đổi StateReason/ObservedConsumers. | Ngay sau commit có thể thấy State=RESERVED cùng StateReason cũ mô tả free; normalize inventory tiếp theo sửa lại. Phát hiện từ phép gán, chưa có test riêng cho presentation này. |
| Reconciliation chọn một container cho Job, ưu tiên active; không enforce unique JobID trên inventory. | ContainerID không chứng minh một-một tuyệt đối. Unknown runtime state có record không đi nhánh missing; có thể giữ Job/reservation vô hạn do chưa có timeout cho trường hợp này. |
| Freshness dùng CP receipt, nhưng STARTING-missing guard so Agent ObservedAt với CP UpdatedAt. | Sequence ngăn replay thứ tự, chưa giải quyết clock skew cho guard này. Chưa có test clock-skew chuyên biệt. |
| Summary dùng “occupied” = mọi state khác FREE; online gồm cả DRAINING. | Không coi mọi counter là actual GPU utilization hay cộng free+occupied thành total trong mọi tình huống. |
| Trang GPU inventory frontend đếm State=FREE cho ô “Sẵn sàng”, chưa xét Server readiness. | Có thể khác CP Summary.gpusFree trên host offline/stale/drained; xem [API audit](API_TESTING_POSTMAN.md). Không dùng số UI này như bằng chứng GPU schedulable. |
| SQL reference có constraints/columns khác struct đang persist. | Không dùng SQL target để khẳng định runtime đã có unique GPU index, relational Command.JobID hay observed_containers table. |

Tài liệu cũ đã được chỉnh để nói rõ reservation có đường RESERVED → RELEASED, Job có thể bỏ qua STARTING, trạng thái Server được dùng cho Agent, và release logic không đồng nghĩa GPU vật lý free. Không refactor model/transition để khớp tài liệu. Hướng dẫn chạy/test vẫn ở [README](../README.md) và [Windows/WSL](WINDOWS-WSL.md); bảng invariants phân biệt test sẵn có với nhánh chưa có test riêng.

## 10. Scope mapping

Các link dưới đây trỏ đúng file đang có; tên test chi tiết và giới hạn nằm ở mục 7.

| Domain / State | Scope liên quan | Main files | Test |
|---|---|---|---|
| Server / Agent connectivity | Enrollment, heartbeat, drain, failure handling | [model][model], [CP][cp], [memory store][store], [Runner][runner] | [memory safety][memory-safety], [integration][integration], [recovery][recovery] |
| Agent local state | Identity/sequence/processed results, reconnect | [contracts][agent-contracts], [Runner][runner], [file store][agent-state] | [state round-trip][state-tests], [runner redelivery][runner-tests] |
| GPU / Healthy | Hardware discovery, health gating | [NVML adapter][nvml], [Collector][inventory], [domain][model] | [four NVML mock profiles][nvml-tests], [inventory safety][inventory-safety] |
| GPU allocation / occupancy | Scheduler/resource accounting, exclusive physical UUIDs | [accounting][accounting], [scheduler][scheduler], [commit][store] | [memory accounting][accounting-tests], [scheduler safety][scheduler-safety], [atomic safety][memory-safety] |
| Job state / Workload UI | Queue + Scheduler + ACK + observed lifecycle | [model][model], [CP][cp], [lifecycle][lifecycle], [Console workloads][workloads-page] | [memory lifecycle/priority][memory-safety], [integration][integration], [Console E2E][console-tests] |
| Reservation / Assignment | Atomic GPU reservation, allocation, release history | [Assignment][model], [CommitAssignment][store], [release/reconcile][lifecycle] | [atomic tests][memory-tests], [safety tests][memory-safety], [durable tests][durable-tests] |
| External/Existing Workload | Discovery, ownership protection, no adoption | [Collector][inventory], [cgroup resolver][cgroup], [Docker adapter][engine], [accounting][accounting] | [inventory tests][inventory-tests], [engine tests][engine-tests], [brownfield integration][integration] |
| Managed Workload / Container | Label ownership, exact DeviceRequests, start/stop | [executor][executor], [engine][engine], [lifecycle][lifecycle] | [engine tests][engine-tests], [integration][integration], [acceptance][acceptance] |
| Container.State / Origin | Actual observations and ownership classification | [RuntimeContainer][agent-contracts], [Collector][inventory], [wire mapping][mapping] | [inventory tests][inventory-tests], [memory tests][memory-tests] |
| Agent Command | Lease, ACK, idempotency, dependency of Stop on Start | [Command][model], [lease/ACK][store], [ACK effects][lifecycle], [Runner][runner] | [runner tests][runner-tests], [memory safety][memory-safety]; lease/stop race gaps ở mục 7/9 |
| Policy admission/order | Lane, Necessity, quota/waiting; không là placement score | internal/policy; domain/allocation.go; application/allocation.go | policy/engine_test.go; application/allocation_test.go |
| Profile/FP8 | Hard constraints Plan/commit | internal/capability; domain/allocation.go | capability/catalog_test.go; application/allocation_test.go |
| Placement / strategy | Filter, score, select; không reserve trong Plan | [scheduler][scheduler], [validation][validation] | [policy/tie/filter tests][scheduler-safety], [legacy/offline tests][scheduler-tests] |
| Persistence / recovery | Domain snapshot, atomic save/rollback, freshness reset | [snapshot][snapshot], [durable][durable], [Agent file][agent-state], [sim runtime][sim] | [durable tests][durable-tests], [state tests][state-tests], [recovery][recovery] |
| DTO / public read model | Protocol v1, env redaction, BFF/Console contracts | [wire][wire], [mapping][mapping], [JobView][job-view], [OpenAPI][openapi], [TS types][frontend-types] | [HTTP tests][http-tests], [frontend contracts][frontend-tests], [Console E2E][console-tests] |

[model]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go>
[dto]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go>
[cp]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go>
[store]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go>
[lifecycle]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go>
[accounting]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/accounting.go>
[runner]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go>
[agent-contracts]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/contracts.go>
[agent-state]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/state/file.go>
[nvml]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_linux.go>
[inventory]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/inventory.go>
[mapping]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/agent_protocol.go>
[validation]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/validation.go>
[workloads-page]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/workloads/page.tsx>
[wire]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go>
[engine]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/dockerengine/engine.go>
[sim]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/simulator/agent.go>
[hardware]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/hardware.go>
[durable]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store.go>
[executor]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/executor.go>
[scheduler]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler.go>
[http]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go>
[bff-health]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/api/aiwm-health/route.ts>
[memory-safety]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/safety_test.go>
[memory-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store_test.go>
[ports]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go>
[durable-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store_test.go>
[scheduler-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler_test.go>
[scheduler-safety]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler_safety_test.go>
[inventory-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/inventory_test.go>
[inventory-safety]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/inventory_safety_test.go>
[engine-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/dockerengine/engine_test.go>
[integration]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/integration/agent_controlplane_test.go>
[http-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/security_test.go>
[runner-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner_test.go>
[state-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/state/file_test.go>
[recovery]: <../scripts/recovery.py>
[snapshot]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/snapshot.go>
[sql1]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/migrations/001_init.sql>
[sql2]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/migrations/002_lifecycle_target.sql>
[job-view]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go>
[openapi]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml>
[frontend-types]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/types.ts>
[frontend-endpoints]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/endpoints.ts>
[nvml-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_mock_test.go>
[accounting-tests]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/accounting_safety_test.go>
[console-tests]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/tests/e2e/console.spec.ts>
[cgroup]: <../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/cgroup_linux.go>
[acceptance]: <../scripts/acceptance.py>
[frontend-tests]: <../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/tests/contracts.test.mjs>
