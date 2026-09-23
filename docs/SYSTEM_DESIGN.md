# Thiết kế hệ thống AIWM

Tài liệu mô tả source hiện tại, cập nhật Organization/auth qua inspection tĩnh ngày 17/09/2026. Đây là thiết kế đã triển khai cùng các giới hạn quan sát được; không phải đề xuất thêm component. Không chạy build/test/runtime trong lần viết tài liệu này.

## Problem, Goal và Core constraint

**Problem:** các GPU server chạy Docker độc lập, có workload tồn tại sẵn và chưa có centralized scheduler.

**Goal:** AIWM gom inventory thành logical GPU resource pool, nhận workload request, quản lý queue/priority, chọn Server + GPU vật lý, reserve/release, chuyển command cho Agent và đối chiếu control state với actual runtime. Pool là góc nhìn tổng hợp trên `Server`; chưa có entity `Pool` hoặc resource fabric riêng.

**Core constraint: existing workloads phải tiếp tục chạy.** Agent/Control Plane chỉ observe và account External containers, không tự restart/stop/delete/adopt. GPU có consumer phải được loại khỏi tài nguyên có thể cấp phát, kể cả container đang idle. Mất kết nối quản lý không kích hoạt cleanup container.

Đơn vị cấp phát là nguyên GPU vật lý trên **một server** cho mỗi Job. Kubernetes, MIG, GPU sharing, time-sharing, preemption, reclaim và checkpoint không thuộc execution flow hiện có.

## Cách đọc và source of truth

D1–D10 là mười luồng chính; S1–S2 là state machines; B1 là trust boundary; P1–P2 là deployment views. Mũi tên biểu diễn lời gọi hoặc dữ liệu theo nhãn, không mặc định là một network connection. Các khối trong Control Plane là trách nhiệm trong cùng process, không phải microservices độc lập.

| Khái niệm | Representation hiện tại |
|---|---|
| Server / Agent | `domain.Server` lưu identity/connectivity; `AgentID = Server.ID`. `MachineID` là khóa nhận diện khi đăng ký lại. Agent là process `agent.Runner`, không có entity/status Agent riêng. |
| GPU | `domain.GPU`, identity `UUID`; index chỉ có nghĩa trong inventory của host. |
| Job / Workload | `domain.Job` chứa `AllocationIntent`, `PolicyDecision`, `ResourceRequest`, status và events. Console gọi Job là Workload. |
| External / Managed Workload | `domain.Container.Origin` phân loại observation. Không có entity `ExternalWorkload`/`ManagedWorkload` riêng. `LEGACY` hiển thị là External. |
| Placement / Reservation | `domain.Placement` là proposal tạm; reservation được lưu trong `Job.Assignment` với `ReservationState`. |
| Agent Command | `domain.Command` lưu tại CP; wire `agentv1.Command` chỉ chuyển ID/type/payload. |
| Runtime | `agent.RuntimeContainer` tại Docker boundary → `agentv1.Container` trong report → `domain.Container` tại CP. |

Domain/control state nằm trong [domain][model] và repository; actual state đến từ Docker/NVML. CP lưu snapshot gần nhất, không đọc trực tiếp GPU ở xa. Không có hai struct `DesiredState`/`ActualState`: `Job.Status` tổng hợp tiến độ điều khiển và kết quả observation.

| Lớp dữ liệu | Nơi định nghĩa / mapping |
|---|---|
| Public input | [CreateJobRequest][dto] + [AllocationResources/AllocationIntent][allocation-model]; [validateJobRequest][validation] và `prepareJob` kiểm tra, normalize, resolve constraint. |
| Public output | [JobView/publicJob][job-view] che giá trị environment; `PolicyDecision.AuxiliaryScore` và `ResourceRequest.ResolvedModels` không serialize JSON. Server/GPU/Container chủ yếu dùng domain trực tiếp. |
| Agent protocol | [agentprotocol/v1/types.go][wire]; [inventoryToDomain][protocol-map] validate wire report; `ControlPlane.ReportInventory` gán `ReceivedAt` theo đồng hồ CP. |
| Persistence đang chạy | [memory.Snapshot][snapshot] version 1 chứa domain maps; [durable.Store][durable] encode gob. Business metadata dùng PostgreSQL qua store/postgres; 001_metadata.sql chạy bằng --migrate. SQL runtime trong migrations/ vẫn chưa dùng; không migrate Job/Assignment/Command sang PostgreSQL. |
| Frontend | [types.ts][fe-types], [api-client.ts][fe-client] và BFF sử dụng public contract; không sở hữu policy, reservation hoặc state transitions. |

Chi tiết từng field, invariant và test liên quan: [DOMAIN_MODEL](DOMAIN_MODEL.md). Cách chạy/demo/test Windows và WSL: [README](../README.md), [WINDOWS-WSL](WINDOWS-WSL.md).

## Phát hành tập trung và triển khai Agent

~~~mermaid
flowchart LR
    DEV["AIWM team / CI"] -->|"Build một lần, default CP URL"| R["Generic Agent release<br/>binary + installer + checksum"]
    R --> A["VTT-01<br/>Enrollment A"]
    R --> B["VTT-02<br/>Enrollment B"]
    R --> C["VDS-01<br/>Enrollment C"]
    R --> D["VTNET-01<br/>Enrollment D"]
    A & B & C & D -->|"Register MachineID + token riêng"| CP["Control Plane"]
    CP --> M["Enrollment → Server → Organization"]
~~~

Organization không compile vào Agent. GPU ownership suy ra qua Server. ADMIN cấp enrollment riêng qua UI hoặc scripts/agent-admin.py; không dùng chung token cho cả đơn vị. Binary là production cmd/aiwm-agent, CGO/NVML thật, không kèm mock library hoặc enrollment.

~~~mermaid
flowchart LR
    FE["Browser / Frontend BFF"] --> CP["Control Plane Host<br/>HTTP(S) public + Agent API"]
    CP --> PG[("PostgreSQL metadata")]
    CP --> RT[("Runtime snapshot")]
    A["Linux GPU host<br/>Agent + config/state"] -->|"Register, heartbeat, inventory, poll, ACK"| CP
    CP -.->|"Command trong poll response"| A
    A --> DO["Docker Engine<br/>Managed + Existing containers"]
    A --> NV["go-nvml → NVML / NVIDIA Driver → GPU"]
~~~

URL: AIWM_CONTROL_PLANE_URL > ReleaseControlPlaneURL > development localhost. CP/Console bind loopback mặc định; AIWM_CP_BIND_HOST/AIWM_CONSOLE_BIND_HOST cho phép expose interface phục vụ deployment. PostgreSQL vẫn loopback. Repo không tự dựng reverse proxy/TLS.

GPU host nhận release, installer và token; không clone source hoặc build Go. Installer không thay Docker/driver, không dừng containers. Service restart chỉ tác động Agent; MachineID/state mismatch phải re-onboard có chủ đích. Hướng dẫn [Real Deployment và E2E](TESTING_RUNBOOK.md); chưa chứng nhận execution trên GPU vật lý trong phiên laptop.

## D1 — Kiến trúc nhiều Organization

~~~mermaid
flowchart LR
    U["User"] --> FE["Frontend / BFF"]
    FE -->|"Session HTTP + organization context"| CP["Go Control Plane"]
    CP --> META["IdentityService / MetadataRepository"]
    META --> PG[("PostgreSQL<br/>Organization / User / Session<br/>Enrollment / ServerOwnership")]
    CP --> POL["Policy / Queue"]
    POL --> ORG["Organization hard filter"]
    ORG --> SCH["Resource filters / Scheduler"]
    SCH --> RT[("memory + durable gob<br/>Job / Assignment / Command / inventory")]
    RT --> OWN["Server thuộc Organization"]
    AG["Agent trên Server"] -->|"register / heartbeat / inventory / poll / ACK"| CP
    CP -.->|"command trong poll response"| AG
    OWN -.->|"AgentID = Server.ID"| AG
    AG --> D["Docker Engine"]
    AG --> N["go-nvml / NVML"]
    D --> G["GPU vật lý thuộc Server"]
    N --> G
~~~

Organization là ownership boundary, không phải pool vật lý mới. Một Job chỉ dùng nguyên GPU trên một Server cùng organization. Preview và reservation commit cùng kiểm tra boundary; injected SchedulingPolicy chỉ score các candidate đã lọc.

Metadata dùng PostgreSQL qua `ports.MetadataRepository`; runtime scheduling vẫn dùng `ports.Repository` và durable gob một writer. Không có broker hoặc transaction phân tán giữa hai store. Domain/application giữ business logic; PostgreSQL/Docker/NVML/HTTP là adapters. Agent production/sim dùng cùng core và NVML adapter; sim nạp mock library và thay DockerRuntime bằng `simulator.Runtime`.

Nguồn: `internal/domain/organization.go`, `application/{identity,controlplane,allocation,scheduler}.go`, `store/postgres`, `cmd/aiwm-server/main.go`.


## D2 — Agent internal architecture

~~~mermaid
flowchart LR
    subgraph LOCAL["GPU server: actual observation"]
        HOST["OS / proc / runtime"]
        DE["Docker Engine"]
        EC["Existing containers"]
        MC["Managed containers"]
        NV["NVML"]
        GP["GPU UUID / model<br/>VRAM / utilization / PID"]
        EC --> DE
        MC --> DE
        NV --> GP
    end
    DE -->|List + Inspect + Version| ENG["dockerengine.Engine"]
    DE -->|container events| EV["inventoryLoop"]
    HOST --> HW["discoverHost"]
    GP --> NR["gpu.Reader.Snapshot"]
    HOST --> PID["ProcCgroupResolver"]
    ENG --> COL["InventoryCollector.Snapshot"]
    HW --> COL
    NR --> COL
    PID -->|PID to ContainerID| COL
    EV -->|debounce / periodic refresh| COL
    COL --> REPORT["InventoryReport"]
    REPORT --> RUN["Runner.reportInventory"]
    RUN -->|HTTP outbound| CP["Control Plane"]
    POLL["Runner.pollAndExecute"] --> EX["CommandExecutor.Execute"]
    EX -->|GPUsAvailable trước start| COL
    EX -->|start / stop managed| ENG
~~~

`InventoryCollector.Snapshot` đọc Docker containers → NVML GPUs/processes → Docker version, map grants/processes rồi tạo report kèm `discoverHost`. Hardware gồm hostname, OS, architecture, CPU count, RAM tổng; không có host CPU/RAM utilization collector.

Docker adapter list cả stopped containers, inspect containers active hoặc có label managed để lấy state, labels, grants, PID và exit metadata. `running/paused/restarting` đều được xem là consumer. NVML đọc UUID/index/model, memory total/used, utilization, temperature và compute/graphics processes; lỗi telemetry bắt buộc không được biến thành inventory rỗng.

Correlation gồm DeviceRequests/NVIDIA visibility → UUID, numeric index → UUID từ cùng snapshot, rồi NVML PID → `/proc/<pid>/cgroup` → Docker ContainerID. `InitPID` được thu thập nhưng resolver production dùng cgroup, không chỉ so PID với init process. Nếu không đọc được cgroup, pattern không khớp hoặc PID không thuộc container đã list, CP bảo vệ GPU như unknown.

Report của Agent chưa tự tính đầy đủ `RESERVED/ALLOCATED/OCCUPIED_LEGACY`. Collector gắn `OCCUPIED_UNKNOWN` khi telemetry không có owner vượt ngưỡng; CP mới kết hợp report với Job/Assignment để normalize toàn bộ GPU state (D4).

| Trách nhiệm | File / function |
|---|---|
| Hardware | [hardware.go][hardware] — `discoverHost` |
| Docker inventory / events | [engine.go][docker] — `ListContainers`, `fromInspect`, `WatchContainerEvents` |
| GPU / process telemetry | [nvml_linux.go][nvml] — `Reader.Snapshot` |
| Correlation / local start guard | [inventory.go][inventory] — `Snapshot`, `GPUsAvailable`; [cgroup_linux.go][cgroup] — `ContainerID` |
| Heartbeat, report, poll | [runner.go][runner] — ba loops; [client.go][agent-client] — HTTP outbound |
| Commands | [executor.go][executor] — `Execute`; Docker adapter chỉ start/stop managed |

## D3 — Enrollment, startup và discovery

~~~mermaid
sequenceDiagram
    actor U as ADMIN / ORGANIZATION_USER
    participant CP as Control Plane
    participant PG as PostgreSQL metadata
    participant A as Agent
    participant D as Docker / NVML
    U->>CP: Login rồi POST enrollments
    CP->>CP: Derive organization từ Principal (ADMIN được chọn)
    CP->>PG: Lưu enrollment + token hash + ServerID
    CP-->>U: Enrollment token (hiển thị một lần)
    U->>A: Cấu hình token trên Linux server
    A->>D: Preflight chỉ đọc
    A->>A: Load local state và verify MachineID
    A->>CP: Register machineId + enrollment token
    CP->>PG: Khóa enrollment, bind machine/server/org
    PG-->>CP: ServerOwnership bất biến
    CP->>CP: Upsert runtime, reset freshness
    CP-->>A: AgentID + AgentToken
    A->>A: Persist credential và inventory sequence
    A->>D: FULL inventory
    A->>CP: PUT inventory
    CP->>CP: Reconcile, normalize occupancy, fresh readiness
~~~

Không nhập GPU thủ công và không tin organization từ Agent. `RegisterRequest` không có organizationId; name/labels authoritative lấy từ enrollment. Token chưa bind hết hạn sau 24 giờ. Sau bind, chỉ cùng machine được register lại cho đến khi revoke; token phải được giữ an toàn cho restart/re-auth. Revoke enrollment không thu hồi Agent token đã cấp hoặc dừng workload.

Production `agent.Preflight` đọc OS/architecture/kernel/cgroup, truy cập Docker socket/API/version/info, runtime nvidia, NVML và GPU discovery. Non-Linux → UNSUPPORTED; thiếu Docker/NVML/GPU/runtime evidence → DEGRADED và startup dừng, không fake inventory rỗng. SUPPORTED là điều kiện nền tảng, không thay thế GPU health/occupancy/freshness. `--check` không cần token và không register/mutate container. Chưa có probe thực thi CUDA/CDI-only; xem [TESTING_RUNBOOK](TESTING_RUNBOOK.md).

Metadata bind commit trước Upsert durable; nếu ghi runtime lỗi, retry cùng token/machine dùng cùng ServerID. Không có atomic commit xuyên PostgreSQL/gob. Dữ liệu cũ thiếu org không tự adopt/chuyển ownership.


## D4 — Existing workload discovery và occupancy

~~~mermaid
flowchart TD
    OLD["Container đã chạy trước Agent"] --> DOCK["List / Inspect Docker grants"]
    NV["NVML GPU / process snapshot"] --> JOIN["InventoryCollector<br/>UUID + PID correlation"]
    DOCK --> JOIN
    JOIN --> CLASS{"aiwm.managed=true<br/>và aiwm.job-id có giá trị?"}
    CLASS -->|Không| EXT["Origin LEGACY / External"]
    CLASS -->|Có| MAN["Origin MANAGED"]
    EXT --> ACCOUNT["CP normalizeGPUState"]
    MAN --> ACCOUNT
    JOIN -->|unattributed process / telemetry| ACCOUNT
    JOB["Jobs + Assignments"] --> ACCOUNT
    ACCOUNT --> BLOCK["UNHEALTHY / OCCUPIED_LEGACY<br/>OCCUPIED_UNKNOWN / ALLOCATED / RESERVED"]
    ACCOUNT --> FREE["FREE khi không có blocker"]
    BLOCK --> EXCLUDE["Scheduler loại GPU"]
    FREE --> CHECK["Kiểm tra server freshness<br/>và request constraints"]
~~~

External ở đây là category từ ownership labels, không được xác định chỉ bằng thời điểm tạo. Container không có bộ labels hợp lệ là `LEGACY`; Agent không thêm labels/adopt để biến nó thành managed. `ContainerOrigin.UNKNOWN` có trong domain và là fallback khi CP nhận origin không nhận diện được; collector chuẩn thường xuất `MANAGED` hoặc `LEGACY`.

`normalizeGPUState` áp dụng thứ tự:

| Ưu tiên | Điều kiện | GPU state |
|---|---|---|
| 1 | `Healthy=false` | `UNHEALTHY` |
| 2 | Active grant từ External, hoặc managed không khớp Job/Server/UUID của Assignment | `OCCUPIED_LEGACY` |
| 3 | Process không khớp active container/grant; hoặc nhiều managed Jobs cùng tiêu thụ một UUID | `OCCUPIED_UNKNOWN` |
| 4 | Active managed container khớp Job/Server/UUID | `ALLOCATED` |
| 5 | Agent báo usage không xác định owner | `OCCUPIED_UNKNOWN` |
| 6 | Assignment của Job nonterminal giữ UUID | `RESERVED` |
| 7 | Không có điều kiện trên | `FREE` |

Active grant chiếm GPU dù memory/utilization bằng 0. Grant `all` không có exact IDs, index/UUID không giải được hoặc alias không biết sẽ bảo vệ toàn host khi container active. Telemetry không attribution vượt `UnknownMemoryMiB` hoặc `UnknownUtilizationPct` cũng bị chặn (mặc định 256 MiB / 5%). Process không attribution bị chặn ngay cả khi chưa vượt ngưỡng này.

CP chỉ lưu/cập nhật inventory External. External bị chủ sở hữu stop/delete thì report tiếp theo phản ánh thay đổi và accounting tính lại; CP không sinh command để khôi phục hay dọn nó. Managed mới có flow `StopJob → STOP_CONTAINER`; không có public delete-container API. Docker adapter chỉ remove container vừa do nó tạo nếu bước start thất bại.

Nguồn: [classifyContainer/Snapshot][inventory], [fromInspect/StopManagedJob][docker], [inventoryToDomain][protocol-map], [normalizeGPUState][accounting]. Ownership và PID attribution có giới hạn ở phần Design observations.

## D5 — Resource Allocation Planning → Workload Execution

~~~mermaid
sequenceDiagram
    autonumber
    actor U as User
    participant CP as Control Plane
    participant P as Policy / Planning Scheduler
    participant R as Runtime repository
    participant E as Execution controller
    participant A as Agent
    participant D as Docker
    U->>CP: Submit future workload (neededAt + ttlSeconds)
    CP->>CP: Validate / derive OrganizationID
    CP->>R: QUEUED + execution specification
    CP->>P: Planning tick
    P->>P: Policy → org → capability → time availability → placement
    P->>R: Atomic batch reservation PLANNED / ASSIGNED
    Note over R,E: Đợi reservationStart; chưa có START command
    E->>R: start <= now < end: revalidate snapshot hiện tại
    alt Server/GPU safe và reservation còn thuộc Job
        E->>R: Atomically GPU RESERVED + STARTING + START_CONTAINER
        A->>CP: Poll commands
        CP-->>A: START nếu còn trong interval và safe
        A->>D: Pull nếu cần; create/start managed container
        A->>CP: ACK + inventory
        CP->>R: Reconcile RUNNING / ALLOCATED
    else Offline hoặc unexpected consumer
        E->>R: Giữ lịch, ghi reason chờ; không START
    end
    Note over E,D: EndAt cố định; không kéo dài vì khởi động muộn
    E->>R: Hết interval: request STOP managed workload
    A->>CP: Poll STOP
    A->>D: Graceful stop
    A->>CP: ACK + inventory stopped/absent
    CP->>R: Terminal → RELEASED → GPU FREE nếu không còn blocker
~~~

neededAt là start cụ thể; ttlSeconds là thời lượng cấp GPU, interval [start,end). Chỉ future PLANNED chưa tới start được replan theo Policy. Không preempt RUNNING. Reconcile/scheduling tickers dùng processReservations; Agent không biết Policy/calendar, protocol giữ nguyên.

STOP tại EndAt là yêu cầu graceful, không bảo đảm container đã dừng đúng mili-giây. CP/Agent offline thì workloads tiếp tục; last-known reservation/inventory vẫn giữ, không cấp trùng vì dự đoán end.


## D6 — Preview và submit

Preview chạy cùng greedy planner trên bản copy gồm request chờ và reservation; trả requestedStartAt/requestedEndAt, planningStatus AVAILABLE/CONFLICT. Không tạo Job/reservation/command. Preview có thể ưu tiên N1 hơn lịch tương lai N2; submit vẫn kiểm tra lại trạng thái mới nhất.

Submit giữ execution specification, tạo QUEUED; planning cycle mới tạo ASSIGNED/PLANNED. UI nhận giờ địa phương, gửi UTC và ttlSeconds; label “Thời lượng sử dụng”. Max duration từ jobs/options; start quá khứ quá 60 giây hoặc interval đã hết bị từ chối. ASSIGNED trên UI là “Đã lên lịch · Chưa chạy”.

Nguồn: application/allocation.go, planning.go; components/workloads/job-form.tsx và job-lifecycle.tsx.


## D7 — Scheduler internal flow

Trước mọi resource filter/scoring, Scheduler.filter chạy SameOrganization(Job.OrganizationID, Server.OrganizationID). Server khác đơn vị không được dùng để xây candidate/recommendation.

~~~mermaid
flowchart TD
    Q["Queue đã được policy.Order"] --> SNAP["ListServers snapshot"]
    SNAP --> SERVER["Server.Schedulable<br/>ONLINE, không drain, heartbeat + inventory fresh"]
    SERVER --> LABEL["labelsMatch<br/>legacy ServerSelector"]
    LABEL --> CAP["CapabilityMatches<br/>profile / FP8 / legacy model"]
    CAP --> HEALTH["Healthy"]
    HEALTH --> FREE["GPU.Schedulable<br/>FREE và chưa AssignedJobID"]
    FREE --> VRAM["AvailableMemoryMiB >= MinVRAMMiB"]
    VRAM --> COUNT["matchingGPUs<br/>đủ GPUCount trên một server"]
    COUNT --> HAS{"Có candidate?"}
    HAS -->|Không| WAIT["ErrInsufficientGPU<br/>QUEUED + StatusReason"]
    HAS -->|Có| GPU["Sort available VRAM, UUID<br/>chọn GPUCount GPU"]
    GPU --> SCORE["SchedulingPolicy.Score"]
    SCORE --> PICK["Score thấp nhất<br/>tie theo Server.ID"]
    PICK --> PROP["Placement"]
    PROP --> RES["Repository.ReplanReservations"]
~~~

`filter` duyệt constraints theo thứ tự trên để tìm lý do chờ; tập lựa chọn thực tế dùng chung `ResourceRequest.Matches` qua `matchingGPUs`. Planning dùng snapshot cùng transaction; CalendarServers loại reservation overlap trước scoring và thêm booking thành công cho Job kế tiếp; Job không vừa sẽ không chặn mọi Job nhỏ phía sau.

Default `best-fit` được đọc từ `AIWM_SCHEDULER_STRATEGY` trong [config.go][config], nối tại [aiwm-server/main.go][cp-main]. `Job.Strategy` không điều khiển `Plan`; user không được chọn strategy trong Create DTO.

Đặt `F` là GPU schedulable trên host, `N` tổng GPU host, `K` số GPU request và `M` số GPU match constraints. Đây là notation giải thích thuật toán ở backend, không phải input/UI:

| Strategy thực sự có | Score — nhỏ hơn thắng |
|---|---|
| `first-fit` | `0`; tie Server.ID nên không dựa vào thứ tự map |
| `best-fit` | `M - K` |
| `bin-pack` | `10 × (F - K) - (N - F)` |
| `fragmentation-aware` | `2` nếu `F=N`, cộng `1` nếu `F-K>0`, cộng `(F-K)/(N+1)` |

Trong host, GPU sort theo available VRAM tăng dần rồi UUID tăng dần. Scorer chỉ nhận candidates đã filter; không được bỏ qua health/occupancy. Fragmentation là heuristic, không phải tối ưu toàn cục. Bench dùng chính [Scheduler][scheduler], không có bản sao thuật toán.

Business comparator [policy.Before][policy] xét lần lượt:

1. `AUTO_ELIGIBLE` trước `COMPETITIVE`.
2. Trong cùng lane: `NECESSITY_1` trước `2` trước `3` trước `4`.
3. Chỉ cùng lane và Necessity: `AuxiliaryScore` giảm dần.
4. `CreatedAt` tăng dần, rồi `ID` tăng dần.

Do lane đi trước Necessity, Auto N4 có thể đứng trước Competitive N1. Auxiliary score dùng importance category do backend derive coefficient, quota facts và aging từ `Job.CreatedAt`. Aging tối đa sau 28 ngày; không bảo đảm starvation-free. `Job.Priority` cũ không quyết định queue.

Các boundary thay policy/thuật toán hiện có: `Options.Policy` nhận `policy.Evaluator`; `FactsProvider` nhận nguồn planning/quota; `Options.Catalog` nhận `capability.Resolver`; `Options.PlacementPolicy`/`Scheduler.WithPolicy` thay scorer `SchedulingPolicy`. Business ordering, capability constraint và placement scoring có trách nhiệm riêng.

Nguồn: [scheduler.go][scheduler], [policy.Engine][policy], [capability.Catalog][catalog], [ResourceRequest.Matches][allocation-model], [ScheduleOnce][cp]. Chi tiết coefficients: [SCHEDULER](SCHEDULER.md).

## D8 — Atomic calendar và atomic execution

| Operation | Điều kiện / thay đổi |
|---|---|
| ReplanReservations | Policy order trên snapshot nhất quán; thay toàn batch QUEUED/future PLANNED. Kiểm overlap/ownership/resource trước commit. Không ghi START hoặc đổi physical GPU. |
| CommitAssignment | PLANNED đúng Job/server/UUID, start <= now < end, server fresh, GPU thực tế FREE. Ghi GPU RESERVED + Assignment RESERVED + STARTING + START cùng transaction. |
| RequestStop | Chưa dispatch: cancel/release; đã giao/chạy: STOPPING + STOP, đợi actual state. |
| LeaseCommands | Recheck START về thời gian/freshness/ownership/current GPU. Không redeliver START hết interval. |
| Reconciliation | Release trên observed exit hoặc absence đủ mới sau ACK START/STOP; không coi STARTING mất container trước ACK là failure. |

Calendar nằm trong Job.Assignment; không thêm database/calendar subsystem. Nhiều lịch không overlap có thể trỏ cùng UUID. Metadata PostgreSQL không cùng transaction với runtime gob.

Pure planning callback không được gọi ngược runtime Repository vì đang giữ lock. Durable rollback cả batch nếu disk commit lỗi; restart giữ intervals nhưng vô hiệu freshness. Không phải distributed transaction/HA.

Nguồn: application/planning.go; memory/planning.go, store.go, lifecycle.go. Tests: application/planning_test.go, durable/planning_test.go, memory/safety_test.go. Policy/scorer formulas không đổi.


## D9 — Reconciliation

~~~mermaid
flowchart LR
    ACT["Actual Docker + NVML"] --> COL["Snapshot mới"]
    EVENTS["Docker events / inventory ticker<br/>post-command report / reconnect"] --> COL
    COL --> WIRE["PUT inventory<br/>Sequence + ObservedAt"]
    WIRE --> APP["ReportInventory<br/>validate + CP ReceivedAt"]
    APP --> REP["ReplaceInventory<br/>sequence / UUID conflict check"]
    INTENT["Jobs / Assignments / Commands<br/>control intent"] --> LIFE["reconcileObservedLocked"]
    REP --> LIFE
    LIFE --> JOB["Job status / ContainerID<br/>Assignment / Events"]
    JOB --> NORM["normalizeGPUState"]
    REP --> NORM
    NORM --> STORE["Server inventory + receipt timestamps"]
    STORE --> UI["Public reads / Frontend"]
    STORE --> SCH["Scheduler snapshot"]
~~~

Report đầy đủ được chấp nhận trong một repository transaction: validate sequence mới hơn, không trùng GPU UUID với server khác → reconcile Jobs → normalize GPU → thay inventory và cập nhật liveness. Wire validation cũng từ chối UUID lặp ngay trong report. Lỗi Docker inspect/NVML khiến Agent không gửi một snapshot “rỗng coi như free”; CP giữ last-known inventory cho tới khi có report hợp lệ.

`reconcileObservedLocked` map containers `MANAGED` theo JobID, chỉ xét Job nonterminal được assign vào server đang report; nếu nhiều record cùng Job, ưu tiên active. Nó không reconcile External theo ý muốn của AIWM.

| Observation | Correction hiện tại |
|---|---|
| `running/paused/restarting` | Lưu ContainerID/LastObservedAt, Assignment ALLOCATED; ASSIGNED/STARTING → RUNNING. STOPPING giữ STOPPING. |
| Container bị người vận hành stop từ Docker | `exited/dead` kết thúc Job theo stop intent/exit code như D8. Không tự start lại. |
| Container bị delete/missing | Áp guards missing trong D8; không tự recreate container. |
| External stopped/missing | Chỉ thay container inventory và tính lại occupancy. |
| Agent reconnect | Report mới chạy cùng reconciliation này; scheduling chỉ mở lại khi server/GPU đáp ứng constraints. |

Worker `runReconciler → ControlPlane.Reconcile` **chỉ** gọi `MarkStaleServers`; phần đối chiếu container không nằm trong timer đó. Actual inventory được đẩy bằng HTTP từ Agent; không có CP remote Docker inspect hoặc vòng “ép desired state” đối với mọi container.

Nguồn: [Runner.inventoryLoop/pollAndExecute][runner], [ReportInventory/Reconcile][cp], [ReplaceInventory][store], [reconcileObservedLocked][lifecycle].

## D10 — Mất liên lạc và reconnect

~~~mermaid
sequenceDiagram
    participant A as Agent
    participant CP as Control Plane
    participant D as Docker daemon / containers
    A--xCP: Heartbeat không tới (crash / host / network)
    CP->>CP: Timeout → OFFLINE, chặn placement mới
    Note over CP: Giữ last-known inventory, Job, Assignment, Reservation
    Note over D: Container tiếp tục chạy độc lập với Agent/CP
    A->>A: Restart: load state, verify MachineID, giữ sequence/history
    A->>CP: Re-register nếu restart/credential bị từ chối
    CP-->>A: Identity/token; cần inventory mới
    A->>D: FULL inventory
    A->>CP: Gửi snapshot với sequence mới
    CP->>CP: Reconcile actual/desired, normalize occupancy
    CP->>CP: Chỉ schedulable khi freshness/health/org/drain hợp lệ
~~~

Agent startup/heartbeat/command poll dùng exponential backoff với jitter nửa trên, cap 60 giây; inventory định kỳ giữ nhịp cấu hình, Docker event bursts debounce 500 ms. Mất event stream dùng periodic inventory làm fallback; chưa mở lại stream trong loop hiện tại.

Heartbeat sau OFFLINE hoặc gap vượt timeout đặt inventory receipt về zero; heartbeat một mình không mở scheduling. Register/CP restore cũng reset freshness. Lease START cần server fresh; Stop và inventory/ACK tiếp tục theo guard lifecycle hiện có.

Không có cleanup khi Agent/CP disconnect. `Runner.Run` verify persisted MachineID trước đăng ký; sequence tăng và lưu trước snapshot. Process restart giữ Docker container, không tự stop/restart External. CP chỉ dùng heartbeat không thể phân biệt Agent crash, host chết và network partition; không có out-of-band monitor.

Nguồn: `agent/runner.go`, `store/memory/store.go`, `store/durable/store.go`, `domain.Server.Schedulable`.


## S1 — Job lifecycle

~~~mermaid
stateDiagram-v2
    [*] --> QUEUED: CreateJob
    QUEUED --> QUEUED: chưa đủ GPU / cập nhật reason
    QUEUED --> ASSIGNED: ReplanReservations / PLANNED
    QUEUED --> CANCELLED: RequestStop
    ASSIGNED --> CANCELLED: stop khi START còn PENDING
    ASSIGNED --> STARTING: StartAt reached / CommitAssignment
    ASSIGNED --> RUNNING: inventory active trước ACK
    STARTING --> RUNNING: inventory active
    ASSIGNED --> STOPPING: stop sau START delivery
    STARTING --> STOPPING: RequestStop
    RUNNING --> STOPPING: RequestStop
    STOPPING --> RUNNING: STOP ACK failed
    STOPPING --> STOPPED: inventory exited/dead hoặc missing
    STOPPING --> FAILED: START ACK failed
    ASSIGNED --> FAILED: START failed hoặc observed exit lỗi
    STARTING --> FAILED: START failed / exit lỗi / missing đủ guard
    RUNNING --> FAILED: exit lỗi hoặc missing
    ASSIGNED --> SUCCEEDED: observed exit code 0
    STARTING --> SUCCEEDED: observed exit code 0
    RUNNING --> SUCCEEDED: observed exit code 0
    CANCELLED --> [*]
    STOPPED --> [*]
    SUCCEEDED --> [*]
    FAILED --> [*]
~~~

`QUEUED/ASSIGNED/STARTING/RUNNING/STOPPING/STOPPED/SUCCEEDED/FAILED/CANCELLED` là toàn bộ enum `JobStatus`. Không có Job state `SCHEDULING`, `RESERVED`, `DISPATCHING` hay `PENDING`; RESERVED thuộc GPU/Assignment, PENDING thuộc Command.

Đây là transitions của các callers hiện tại, không phải formal transition table được generic setter enforce đầy đủ. Terminal guards ngăn hồi sinh Job, kể cả ACK lặp/đến muộn; ký hiệu `[*]` kết thúc lifecycle không có nghĩa xóa record. `RUNNING` có cả nhánh STOP failed ACK, nên không phải lúc nào cũng là chứng cứ observation active mới. `created` hoặc runtime state không nằm trong nhánh reconcile xử lý có thể giữ Job nonterminal.

Nguồn: [JobStatus.Terminal][model], [CreateJob/ScheduleOnce][cp], [RequestStop/applyAckLocked/reconcileObservedLocked][lifecycle]; guards missing và release xem D8.

## S2 — Agent / Server lifecycle

~~~mermaid
stateDiagram-v2
    [*] --> ONLINE: RegisterAgent mới
    [*] --> OFFLINE: durable.Open restore
    ONLINE --> DRAINING: SetServerDrained true
    DRAINING --> ONLINE: SetServerDrained false
    ONLINE --> OFFLINE: MarkStaleServers
    DRAINING --> OFFLINE: MarkStaleServers
    OFFLINE --> ONLINE: heartbeat / inventory / re-enroll, không Drained
    OFFLINE --> DRAINING: heartbeat / inventory / re-enroll, Drained
    ONLINE --> ONLINE: heartbeat / inventory / re-enroll
    DRAINING --> DRAINING: heartbeat / inventory / re-enroll
    OFFLINE --> OFFLINE: chỉ đổi Drained flag
~~~

Không có AgentStatus hoặc Server UNKNOWN. `Drained` tồn tại độc lập và được giữ qua offline/restart. Đổi drain lúc offline không đổi enum OFFLINE. DRAINING chỉ chặn placement mới, không drain bằng eviction/stop container.

Status ONLINE là điều kiện cần; phải có `InventoryReceivedAt` còn mới, heartbeat còn mới và GPU đủ điều kiện mới nhận Job. Re-enroll có thể giữ status nhưng reset inventory receipt/version. Xem D10 để phân biệt mất heartbeat, inventory stale và CP restart.

Nguồn: [ServerStatus/Server.Schedulable][model], [UpsertServer/Heartbeat/SetServerDrained/MarkStaleServers][store], [durable.Open][durable].

### State/enum khác cần phân biệt

| Model | Giá trị hiện có / semantics |
|---|---|
| GPU | `FREE, RESERVED, ALLOCATED, OCCUPIED_LEGACY, OCCUPIED_UNKNOWN, UNHEALTHY`; normalize từ observations + reservation, không chuyển tuần tự theo một vòng cố định. |
| Health / schedulability | `Healthy` là bool từ NVML adapter; `Server.Schedulable` xét connectivity/freshness; `GPU.Schedulable` xét Healthy + FREE + chưa assigned. Không có GPU enum ONLINE/AVAILABLE. |
| VRAM available | `max(MemoryTotalMiB - MemoryUsedMiB, 0)`; đủ VRAM không đồng nghĩa được phép sử dụng GPU. |
| Assignment | String `RESERVED, ALLOCATED, RELEASED`, không phải enum Go hay entity Reservation riêng. |
| Command | `PENDING → DELIVERED → SUCCEEDED/FAILED`; expired lease redeliver vẫn DELIVERED. Cancel START chưa lease dùng FAILED. Repository có thể nhận ACK khi PENDING, không có guard bắt buộc đã DELIVERED. |
| Command type | `START_CONTAINER, STOP_CONTAINER`. Không có command adopt/restart/delete External. |
| Container | Origin `MANAGED, LEGACY, UNKNOWN`. `State` là string Docker; code phân nhánh `created`, `running/paused/restarting`, `exited/dead`. Không ép Docker state thành JobStatus. |
| Policy | `AUTO_ELIGIBLE, COMPETITIVE` là assessment tại một thời điểm; không phải Job lifecycle. |
| Business input | `TRAINING/INFERENCE`; `NECESSITY_1..4`; `CRITICAL_SPECIAL/VERY_IMPORTANT/IMPORTANT/NORMAL`. |
| Execution backend | `DOCKER` được validate/thực thi. Constant `KUBERNETES` còn trong domain nhưng không có execution adapter tương ứng. |

Nguồn: [model.go][model], [allocation.go][allocation-model], [NVML][nvml], [command lease/ACK][store]. State details/invariants đầy đủ: [DOMAIN_MODEL](DOMAIN_MODEL.md).

## B1 — Ranh giới xác thực và tenancy

Public API dùng opaque session token của `POST /api/v1/auth/login`, hash lưu PostgreSQL, TTL 8 giờ. `GET auth/me` trả User/Organization; logout xóa session, sửa User revoke các session cũ. Mỗi request kiểm tra User/Organization enabled. Không OAuth/SSO/JWT/public self-registration; bootstrap admin qua CLI riêng.

BFF nhận session token từ login response, đặt cookie HttpOnly/SameSite=Strict; không trả token vào browser JSON. `AIWM_SESSION_COOKIE_SECURE=true` khi frontend chạy HTTPS; false chỉ cho HTTP local. BFF chỉ whitelist public routes, chặn mutation Origin khác host và forward token tương ứng session. Backend `sessionAuth` là security boundary; filter UI không cấp quyền. Rate limit login theo peer IP là in-memory trong một CP, và giới hạn đồng thời password hashing.

ORGANIZATION_USER chỉ list/get/stop/drain tài nguyên cùng organization; query organizationId bị từ chối. ADMIN có thể lọc đọc nhiều đơn vị, quản lý organization/user và chọn org khi tạo enrollment. Job submit của mọi role luôn lấy org của account. Organization filter chỉ dùng để xem không đổi quyền submit.

Agent register dùng enrollment token riêng cho machine; các endpoint inventory/heartbeat/poll/ACK dùng Agent token và AgentID. Metadata resolve ownership, không trust field/labels từ Agent. Agent token chưa có TTL/revocation API riêng; re-register xoay token. Revoke enrollment chỉ chặn register lại.

CP hỗ trợ TLS server; Agent hỗ trợ CA/client cert nhưng CP trực tiếp chưa enforce client CA/mTLS. Docker socket/NVML thuộc host Agent. Không proxy Agent routes hoặc Docker qua BFF. Public Job che env values và làm sạch score trong status/event text; private runtime snapshot/command giữ full execution spec.

Nguồn: `httpapi/identity.go`, `application/identity.go`, `store/postgres/store.go`, frontend `src/app/api/aiwm/[...path]/route.ts`.


## P1 — Deployment demo hiện có

~~~mermaid
flowchart LR
    U["Browser trên laptop"] -->|localhost:3000| NEXT
    subgraph ENV["Docker Desktop Linux / Linux Compose"]
        NEXT["console<br/>optional profile console"]
        CP["control-plane"]
        A["agent-a100<br/>aiwm-agent-sim"]
        T["agent-t4<br/>aiwm-agent-sim"]
        DATA[("control-plane-data<br/>gob")]
        AS[("agent-a100-data")]
        TS[("agent-t4-data")]
        CP --> DATA
        CP --> PG[("postgres / metadata-data")]
        A --> AS
        T --> TS
        A -->|outbound HTTP| CP
        T -->|outbound HTTP| CP
        NEXT -->|HTTP + session token| CP
    end
    NATIVE["Next.js native trên Windows<br/>phương án thay console container"] -->|localhost:8080| CP
~~~

[compose.yaml][compose] định nghĩa CP publish `127.0.0.1:8080`; Console optional profile `console` publish `127.0.0.1:3000`. Có thể chạy Next.js native thay Console container theo README. Hai simulator là hai logical servers trong cùng môi trường demo, không chứng minh hai physical GPU hosts.

`agent-a100` dùng `a100-external.yaml`, seed External ở GPU index 0; `agent-t4` dùng `t4-empty.yaml`. Profile mount read-only; Agent identity và simulated Docker runtime giữ ở volume riêng mỗi Agent. [Dockerfile.sim][sim-dockerfile] nạp `libnvidia-ml.so.1` qua `LD_LIBRARY_PATH`, YAML qua `MOCK_NVML_CONFIG`.

Có PostgreSQL cho metadata; CP vẫn lưu runtime /data/control-plane.gob. `simulator.Runtime` giữ JSON containers và phát events mô phỏng; **không chạy image/command/CUDA thật**, không tự tạo compute utilization tương ứng với Job. Metrics GPU trong demo đi qua adapter NVML nhưng dữ liệu từ mock profile.

NVML adapter hiện yêu cầu Linux/CGO; [nvml_unsupported.go][nvml-unsupported] trả lỗi ở native Windows. Vì vậy Windows laptop dùng Linux containers/WSL cho Agent sim; việc đọc tài liệu này không yêu cầu khởi chạy chúng.

## P2 — Ánh xạ process production

~~~mermaid
flowchart LR
    USER["Browser"] --> FE["Next.js frontend + BFF"]
    FE --> CP["aiwm-server<br/>một writer"]
    CP --> DISK[("local durable gob")]
    CP --> PG[("PostgreSQL metadata")]
    subgraph A["GPU Server A - Linux"]
        AG1["aiwm-agent"]
        DK1["Docker Engine<br/>External + Managed containers"]
        NV1["NVIDIA driver / NVML"]
        G1["Physical GPUs"]
        AG1 --> DK1
        AG1 --> NV1
        NV1 --> G1
        DK1 -->|NVIDIA runtime| G1
    end
    subgraph B["GPU Server B - Linux"]
        AG2["aiwm-agent"]
        DK2["Docker Engine<br/>External + Managed containers"]
        NV2["NVIDIA driver / NVML"]
        G2["Physical GPUs"]
        AG2 --> DK2
        AG2 --> NV2
        NV2 --> G2
        DK2 -->|NVIDIA runtime| G2
    end
    AG1 -->|HTTP outbound| CP
    AG2 -->|HTTP outbound| CP
~~~

Đây là ánh xạ các process/adapter đã có sang host theo [hướng dẫn Agent production][agent-deploy], **không phải thông báo đã triển khai/kiểm chứng trên GPU thật**. Frontend và CP có thể đặt cùng hoặc khác host theo URL cấu hình; source không bắt buộc một management host cụ thể. Mỗi GPU host có Agent state riêng, local Docker socket và NVIDIA devices/library. Docker/driver/Container Toolkit được chuẩn bị ngoài Agent; Agent không cài lại hoặc restart Docker khi enroll.

Bốn composition roots giữ vai trò riêng:

| Entrypoint | Vai trò |
|---|---|
| [cmd/aiwm-server][cp-main] | Config → durable repository → application/policy/scheduler → HTTP; scheduler/reconciler tickers. |
| [cmd/aiwm-agent][agent-main] | Production Docker/NVML adapters, collector, executor, outbound client, local state. |
| [cmd/aiwm-agent-sim][sim-main] | Cùng Agent core/NVML adapter; mock NVML library và simulated Docker boundary. |
| [cmd/aiwm-scheduler-bench][bench] | Thí nghiệm deterministic dùng application.Scheduler; không phải daemon hoặc service trong deployment. |

PostgreSQL metadata đã nối runtime; schema không tự migrate. Production quota service, HA/leader election và distributed runtime lock vẫn chưa implement.

## Component → Source code mapping

Các links trỏ trực tiếp vào workspace source. Cột Diagram cho biết nơi theo dõi luồng; D1 dùng các component trong cùng process, không hàm ý tách service.

| Component/Flow | Responsibility | Main files/packages | Diagram |
|---|---|---|---|
| Control Plane entrypoint | Compose config, repository, application và tickers | [cmd/aiwm-server/main.go][cp-main], [internal/config/config.go][config] | D1, D10, P2 |
| HTTP API | Routes, decode/validation envelope, auth, public read DTO | [internal/httpapi/server.go][http], [security.go][security], [job_view.go][job-view] | D5, B1 |
| Request preparation | Input validation, normalize timestamp, resolve capability, policy evaluation | [internal/application/allocation.go][allocation-app], [validation.go][validation], [domain/dto.go][dto] | D5–D6 |
| Policy Engine / Queue | Evaluate facts; lane/Necessity/auxiliary/FIFO; lấy queue | [internal/policy/engine.go][policy], [profiles.go][profiles], [ControlPlane.Queue][cp] | D5, D7 |
| Capability | Profile/FP8 → pinned model aliases; predicate dùng chung | [internal/capability/catalog.go][catalog], [domain/allocation.go][allocation-model] | D6–D8 |
| Scheduler | Filter/score/select, deterministic ties, strategy registry | [internal/application/scheduler.go][scheduler] | D7 |
| Reservation | Recheck và atomic commit resources + Job + command | [internal/ports/repository.go][ports], [memory/store.go][store], [durable/store.go][durable] | D8 |
| Reconciliation / release | Job control + observed active/exit/missing, logical release | [memory/lifecycle.go][lifecycle], [accounting.go][accounting], [ControlPlane.Reconcile][cp] | D8–D10 |
| Persistence | Snapshot version 1, local file lock, rollback/restore | [memory/snapshot.go][snapshot], [durable/store.go][durable], [durable/repository.go][durable-repo] | D1, D8, P1–P2 |
| Agent registration / heartbeat | Local identity, retry, CP liveness, enrollment | [internal/agent/runner.go][runner], [agent/controlplane/client.go][agent-client], [ControlPlane.RegisterAgent][cp] | D3, D10 |
| Hardware discovery | Host metadata | [internal/agent/hardware.go][hardware] | D2–D3 |
| Docker discovery | List/inspect, GPU grants, lifecycle events | [internal/agent/dockerengine/engine.go][docker] | D2–D4 |
| NVML discovery | Physical GPU telemetry + compute/graphics processes | [internal/agent/gpu/nvml_linux.go][nvml] | D2–D4 |
| Existing workload accounting | Grants/PID correlation tại Agent, normalize với Job/Assignment tại CP | [internal/agent/inventory.go][inventory], [cgroup_linux.go][cgroup], [memory/accounting.go][accounting] | D4 |
| Agent commands | Lease/redelivery tại CP; local guard, execution, processed result/ACK tại Agent | [memory/store.go][store], [agent/executor.go][executor], [runner.go][runner], [state/file.go][agent-state] | D5, D8, D10 |
| Agent wire mapping | Protocol DTO độc lập; validate/map report sang domain | [agentprotocol/v1/types.go][wire], [application/agent_protocol.go][protocol-map] | D3, D9 |
| Frontend request / reads | Intent form, preview freshness, query/mutation qua typed client | [job-form.tsx][fe-form], [form-schema.ts][fe-schema], [use-workloads.ts][fe-hooks], [api-client.ts][fe-client] | D5–D6 |
| Frontend BFF | Public route whitelist + upstream bearer | [src/app/api/aiwm/[...path]/route.ts][bff], [proxy-policy.ts][proxy-policy], [src/config/server.ts][fe-config] | B1 |
| Simulator / deployment | Mock library, cùng core, runtime JSON độc lập | [internal/simulator/agent.go][sim], [Dockerfile.sim][sim-dockerfile], [compose.yaml][compose] | D1, D3, P1 |

## Scope → Architecture Mapping

`IMPLEMENTED` = có flow trong source cho phạm vi mô tả; không đồng nghĩa đã chứng minh runtime production. `PARTIAL` = đã có một phần nhưng còn giới hạn nêu rõ. `NOT_IMPLEMENTED` = chưa có behavior thực thi. Các rows granular tránh đánh đồng tồn tại field với đã enforce behavior.

| Scope item | Status | Diagram | Main implementation |
|---|---|---|---|
| Agent register / heartbeat | IMPLEMENTED | D3, D10, S2 | [Runner][runner], [RegisterAgent/Heartbeat][cp], [UpsertServer][store] |
| Server hardware discovery | IMPLEMENTED | D2–D3 | [discoverHost][hardware]: hostname/OS/arch/CPU count/RAM tổng |
| Docker discovery | IMPLEMENTED | D2–D4 | [ListContainers/fromInspect][docker] |
| GPU/NVML discovery | IMPLEMENTED | D2–D4 | [Reader.Snapshot][nvml], Linux/CGO |
| Existing workload discovery | IMPLEMENTED | D4 | [classifyContainer/Snapshot][inventory], không adopt |
| Docker PID ↔ NVML attribution đầy đủ | PARTIAL | D2, D4 | [cgroup resolver][cgroup], grant/index mapping; quyền đọc/pattern/unknown còn giới hạn |
| GPU allocation accounting | IMPLEMENTED | D4, D8 | [normalizeGPUState][accounting]: consumers + active reservations |
| Docker lifecycle events | PARTIAL | D2, D9 | [WatchContainerEvents][docker], [inventoryLoop][runner]: debounce, periodic fallback; chưa tự mở lại stream khi lỗi |
| Agent start/stop commands | IMPLEMENTED | D5, D8 | [Execute][executor], [StartManagedContainer/StopManagedJob][docker] |
| Actual inventory + GPU metrics | IMPLEMENTED | D2, D9 | [collector][inventory], [NVML][nvml]; snapshots, chưa có metric time-series service |
| Host CPU/RAM utilization metrics | NOT_IMPLEMENTED | D2 | [hardware.go][hardware] chỉ metadata, không usage collector |
| Centralized server/GPU/container inventory | IMPLEMENTED | D1, D9 | [ReportInventory][cp], [ReplaceInventory][store] |
| Logical resource pool | IMPLEMENTED | D1 | [ListServers/GPUs/Summary][cp], không entity Pool riêng |
| Managed/external separation | IMPLEMENTED | D4, D9 | [inventory][inventory], [ownership guard][docker], [accounting][accounting] |
| Job/Queue/Priority | IMPLEMENTED | D5, D7, S1 | [Queue][cp], [policy.Order/Before][policy] |
| Business planning/quota integration | PARTIAL | D5–D7 | [FactsProvider/DevelopmentFacts][policy]: interface + static demo facts, chưa ledger thật |
| Preview không reserve / submit re-evaluate | IMPLEMENTED | D6 | [PreviewJob/prepareJob/matchJob][allocation-app], [CreateJob][cp] |
| User chỉ khai báo nhu cầu | IMPLEMENTED | D5–D6 | [CreateJobRequest][dto], [validation][validation], [form][fe-form] |
| Frontend hoàn toàn không hiển thị internal score | PARTIAL | D6, S1 | Preview không score; [JobLifecycle][fe-lifecycle] vẫn hiển thị Assignment.Score |
| Scheduler placement | IMPLEMENTED | D7 | [Scheduler.Plan][scheduler] |
| Server online + fresh + không drain | IMPLEMENTED | D7, D10 | [Server.Schedulable][model], commit recheck |
| GPU healthy | IMPLEMENTED | D4, D7 | [NVML reader][nvml], [GPU.Schedulable][model] |
| GPU available: không external/unknown/reserved/allocated | IMPLEMENTED | D4, D7 | [accounting][accounting], [ResourceRequest.Matches][allocation-model] |
| GPU type/profile/FP8 matching | IMPLEMENTED | D6–D8 | [catalog.Resolve][catalog], pinned ResolvedModels |
| Chứng nhận profile tương đương hiệu năng | NOT_IMPLEMENTED | D7 | [catalog][catalog] là compatibility aliases, không có measured equivalence |
| GPU count trên cùng server | IMPLEMENTED | D7–D8 | [filter][scheduler], [CommitAssignment][store] |
| Available VRAM từng GPU | IMPLEMENTED | D7–D8 | [AvailableMemoryMiB][model], [Matches][allocation-model] |
| First Fit | IMPLEMENTED | D7 | [NewScheduler][scheduler]: `first-fit` |
| Best Fit | IMPLEMENTED | D7 | [NewScheduler][scheduler]: `best-fit`, default |
| Bin Packing | IMPLEMENTED | D7 | [NewScheduler][scheduler]: `bin-pack` |
| Fragmentation-aware | IMPLEMENTED | D7 | [fragmentationScore][scheduler] |
| Queue khi không có candidate + reason | IMPLEMENTED | D7 | [ScheduleOnce][cp], [filter][scheduler] |
| Reserve-before-execute / atomic CP commit | IMPLEMENTED | D8 | [CommitAssignment][store], [durable.transact][durable] |
| Release theo terminal/failed launch/cancel | IMPLEMENTED | D8, S1 | [releaseLocked][lifecycle]; không ép actual consumer thành FREE |
| Reservation safety mọi interleaving start/stop | PARTIAL | D8–D9 | [reconcileObservedLocked][lifecycle]: STOPPING + missing có gap |
| Agent command/control + lease/idempotency | IMPLEMENTED | D5, D10 | [LeaseCommands/AckCommand][store], [Runner][runner]; không cam kết exactly-once mọi crash |
| Observed lifecycle reconciliation | IMPLEMENTED | D9 | [ReplaceInventory][store], [reconcileObservedLocked][lifecycle] |
| Failure handling / reconnect | IMPLEMENTED | D10, S2 | Freshness, giữ runtime/reservation, report mới; không auto failover |
| Durable CP restart recovery | IMPLEMENTED | D10, P1–P2 | [durable.Open/transact][durable], single writer |
| Duration stop / neededAt delayed start | ACTIVE | D5–D8 | application/planning.go: processReservations |
| Host CPU/RAM admission accounting | NOT_IMPLEMENTED | D7 | Docker nhận limits qua [engine][docker]; [Scheduler][scheduler] không cộng trừ host capacity |
| Production quota ledger, user/project RBAC | NOT_IMPLEMENTED | D7, B1 | [DevelopmentFacts][policy], [BFF][bff], [publicAuth][security] |
| PostgreSQL metadata | ACTIVE, chưa chạy kiểm chứng | D1/P2 | store/postgres; runtime vẫn gob |
| HA / distributed scheduling lock | NOT_IMPLEMENTED | P2 | local one-writer lock |
| MIG/sharing/time-sharing/preemption/reclaim/checkpoint | NOT_IMPLEMENTED | D1, D7 | Ngoài scope; whole-GPU filter và [validation][validation]; NVML đánh dấu MIG enabled không healthy |

## Design observations / Inconsistencies

Các observations dưới đây đến từ source đã đọc, không phải lỗi đã tái hiện bằng runtime test trong task này. Không sửa code hoặc tài liệu cũ ngoài link README.

### O1 — Policy facts và thời gian request chưa phải enforcement production

**Observation:** `DevelopmentFacts` đọc planning/quota tĩnh từ cấu hình, không tăng usage khi assign. neededAt/ttlSeconds đã enforce bằng planning/execution; quota facts vẫn tĩnh.

**Impact:** demo chứng minh policy ordering/validation; chưa chứng minh quota admission tổng hợp hoặc đảm bảo thời gian chạy. Không diễn giải TTL như deadline của command lease/reservation.

**Relevant files:** [policy/engine.go][policy], [application/allocation.go][allocation-app], [validation.go][validation], [scheduler.go][scheduler].

### O2 — STOPPING + missing có thể release trước late START

**Observation (đã sửa 23/09):** missing chỉ release sau ACK START/STOP và inventory đủ mới. START hết interval không được redeliver; STOP được phép theo sau START DELIVERED đã hết hạn qua executor tuần tự. Failed STOP ACK cũng có thể đưa Job RUNNING khi chưa từng có active observation.

**Impact:** focused regression bảo vệ inventory trong lúc START còn thực thi và lost ACK. Atomic CP reservation và guard lease STOP chưa chứng minh an toàn cho mọi interleaving runtime. `RUNNING` không luôn đồng nghĩa vừa quan sát container active.

**Relevant files:** [memory/store.go — LeaseCommands][store], [memory/lifecycle.go — RequestStop/applyAckLocked/reconcileObservedLocked][lifecycle].

### O3 — Local availability check không khóa Docker operations ngoài AIWM

**Observation:** `CommandExecutor.mu` serialize executor; `GPUsAvailable` đọc snapshot rồi `StartManagedContainer` mới inspect/pull/create/start. Không có shared lock với operator hoặc process khác dùng Docker trên host.

**Impact:** tránh duplicate CP reservation không đồng nghĩa chặn tuyệt đối một external container start trong khoảng check → launch. Đặc biệt image pull có thể làm khoảng này dài hơn. Đây là giới hạn coordination thực tế, không phải GPU sharing feature.

**Relevant files:** [executor.go][executor], [inventory.go][inventory], [dockerengine/engine.go][docker], [agent-deployment.md][agent-deploy].

### O4 — Attribution bảo thủ, ownership checks giữa các lớp khác nhau

**Observation:** PID resolver phụ thuộc quyền đọc `/proc` và cgroup pattern. Collector phân loại bằng labels; CP nhận `Origin` từ report mà không kiểm lại labels. Accounting kiểm Job + server + từng UUID, còn Job reconciliation kiểm JobID + assigned server và chọn một container, chưa kiểm exact grant.

**Impact:** không xác định owner sẽ khóa GPU để tránh cấp trùng; badge MANAGED không chứng minh Assignment đã được CP công nhận. Job có thể được reconcile RUNNING trong khi grant ngoài Assignment bị accounting coi như External. Agent/Docker admin là trust boundary, không có chứng thực ownership bằng mật mã.

**Relevant files:** [cgroup_linux.go][cgroup], [inventory.go][inventory], [agent_protocol.go][protocol-map], [accounting.go][accounting], [lifecycle.go][lifecycle].

### O5 — State names và timestamps cần đọc cùng guards

**Observation:** JobStatus tổng hợp control/observed progress; ReservationState và Container.State là strings, generic `SetJobStatus` không enforce toàn bộ adjacency graph. Missing guard của STARTING so `report.ObservedAt` từ Agent với command.CompletedAt từ CP. Assignment RELEASED vẫn có thể đi cùng GPU ALLOCATED nếu consumer còn active.

**Impact:** không tự dựng DesiredState/ActualState entities, hoặc ép state GPU khớp tên reservation. Clock skew chưa được giải quyết chỉ bằng inventory sequence; unknown runtime state có record cũng có thể giữ reservation lâu vì không vào nhánh missing/exit.

**Relevant files:** [domain/model.go][model], [memory/store.go][store], [lifecycle.go][lifecycle], [accounting.go][accounting].

### O6 — Điểm số được bỏ khỏi phần lịch trên UI

**Observation:** form/preview không có K hoặc score; `PolicyDecision.AuxiliaryScore` không ra JSON. JobView.Assignment vẫn serialize Score/Reason để tương thích API; JobLifecycle hiện hiển thị interval/trạng thái, không render score. `Scheduler.Plan` đưa score vào placement reason.

**Impact:** form/preview/lịch cấp phát không hiển thị internal score. API Assignment vẫn là DTO legacy; không coi score là business priority.

**Relevant files:** [httpapi/job_view.go][job-view], [application/scheduler.go][scheduler], [job-lifecycle.tsx][fe-lifecycle], [workload detail page][fe-detail], [allocation-preview.tsx][fe-preview].

### O7 — Một số docs và presentation còn semantics cũ

**Observation:** row Queue/Priority của `TRACEABILITY.md` còn ghi `Priority ↓ / createdAt ↑ / ID ↑`; implementation hiện dùng lane/Necessity/auxiliary/FIFO. README API table vẫn viết Priority/FIFO khá chung. Agent deployment guide còn hướng dẫn submit bằng selector riêng trong khi Create DTO mới không nhận selector.

**Impact:** developer nên dùng D5–D7 và source làm chuẩn. Chưa cập nhật những đoạn cũ này vì task chỉ cho phép thêm SYSTEM_DESIGN và link README.

**Relevant files:** [TRACEABILITY.md](TRACEABILITY.md), [README](../README.md), [agent-deployment.md][agent-deploy], [policy.Before][policy], [domain/dto.go][dto].

### O8 — “Sẵn sàng” trên UI và GPU reason chưa luôn tương ứng scheduler

**Observation:** GPU page đếm `State=FREE` cho ô “Sẵn sàng” mà chưa xét server freshness/drain. `Summary.GPUsFree` có xét `Server.Schedulable`. Commit đổi GPU.State/AssignedJobID nhưng không cập nhật StateReason/ObservedConsumers ngay lúc đó.

**Impact:** UI có thể đếm khác summary trên host stale/offline/drained; sau commit có thể thấy RESERVED với reason cũ mô tả free cho tới normalize tiếp theo. Không dùng badge/counter/reason riêng lẻ để kết luận placement khả thi.

**Relevant files:** [GPU page][fe-gpus], [ControlPlane.Summary/GPUs][cp], [CommitAssignment][store], [normalizeGPUState][accounting].

### O9 — Catalog và CPU/RAM chưa đại diện đầy đủ năng lực tính toán

**Observation:** `general/a100-equivalent/h100-equivalent` là exact model aliases tập trung; FP8 là catalog metadata. Model chưa biết không match, kể cả general. `ResolvedModels` pin lúc submit. CPU/RAM chỉ đi thành Docker limits, chưa có host resource accounting.

**Impact:** “equivalent” chưa chứng nhận hiệu năng workload; sửa catalog không tự đổi constraints của Jobs đã nhận. Request qua GPU filter chưa chứng minh host còn đủ tổng CPU/RAM.

**Relevant files:** [capability/catalog.go][catalog], [domain/allocation.go][allocation-model], [application/allocation.go][allocation-app], [scheduler.go][scheduler], [engine.go][docker].

### O10 — Hai loại persistence, một runtime writer

**Observation:** PostgreSQL lưu metadata/session/enrollment, runtime reservation dùng gob. Bind và Upsert runtime là hai commit nối tiếp. Organization enabled đọc theo cycle, không atomic với runtime commit đang chạy.

**Impact:** không phải distributed transaction/HA. Runtime write lỗi có thể để enrollment đã bind; retry cùng machine/token. Chưa có migration org cho snapshot cũ hoặc backup coordinator hai store. Chưa xác minh power-loss durability/tải.

**Source:** store/postgres/store.go, application/controlplane.go, store/durable/store.go.


### O11 — Simulator và event fallback có giới hạn kiểm chứng

**Observation:** simulator dùng chung NVML adapter nhưng mock library/profile; DockerRuntime chỉ mô phỏng state. Production event stream đóng/lỗi thì `inventoryLoop` vẫn refresh định kỳ, chưa tự subscribe lại stream.

**Impact:** demo quan sát được lifecycle/accounting/reconnect ở protocol, không chứng minh CUDA execution/real GPU survival. Sau lỗi events, độ trễ nhận thay đổi phụ thuộc periodic inventory.

**Relevant files:** [sim main][sim-main], [simulator.Runtime][sim], [Dockerfile.sim][sim-dockerfile], [Runner.inventoryLoop][runner], [Engine.WatchContainerEvents][docker].

[model]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go
[allocation-model]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/allocation.go
[dto]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go
[cp-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-server/main.go
[agent-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-agent/main.go
[sim-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-agent-sim/main.go
[bench]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-scheduler-bench/main.go
[cp]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go
[allocation-app]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/allocation.go
[validation]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/validation.go
[protocol-map]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/agent_protocol.go
[scheduler]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler.go
[policy]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/policy/engine.go
[profiles]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/policy/profiles.go
[catalog]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/capability/catalog.go
[config]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/config/config.go
[ports]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go
[store]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go
[lifecycle]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go
[accounting]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/accounting.go
[snapshot]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/snapshot.go
[durable]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store.go
[durable-repo]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go
[http]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go
[security]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/security.go
[job-view]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go
[wire]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agentprotocol/v1/types.go
[runner]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/runner.go
[agent-state]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/state/file.go
[agent-client]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go
[hardware]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/hardware.go
[inventory]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/inventory.go
[cgroup]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/cgroup_linux.go
[executor]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/executor.go
[docker]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/dockerengine/engine.go
[nvml]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_linux.go
[nvml-unsupported]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_unsupported.go
[sim]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/simulator/agent.go
[sim-dockerfile]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/Dockerfile.sim
[agent-deploy]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/docs/agent-deployment.md
[memory-tests]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/safety_test.go
[durable-tests]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store_test.go
[fe-types]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/types.ts
[fe-client]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/api-client.ts
[fe-hooks]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/features/workloads/use-workloads.ts
[fe-form]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/job-form.tsx
[fe-schema]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/jobs/form-schema.ts
[fe-preview]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/allocation-preview.tsx
[fe-lifecycle]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/job-lifecycle.tsx
[fe-detail]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/workloads/[jobId]/page.tsx
[fe-gpus]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/gpus/page.tsx
[bff]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/api/aiwm/[...path]/route.ts
[proxy-policy]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/proxy-policy.ts
[fe-config]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/config/server.ts
[compose]: ../compose.yaml

## Nội dung hợp nhất từ ARCHITECTURE.md

Giữ bốn composition roots và adapter boundaries. Durable bọc critical section của memory, rollback khi ghi snapshot lỗi, file lock một writer; PostgreSQL metadata không thay cơ chế này. JSON slog có request/job/server/command ID, không log token/env payload. Health/ready là process probes, không chứng minh đủ GPU hoặc PostgreSQL schema đã đúng.

External classification, normalize occupancy, Stop/late-START interleaving và terminal Assignment được giữ ở D4/D8/D9 và Design observations. Catalog snapshot của Job không tự cập nhật khi catalog thay; health/occupancy/VRAM được recheck. Full processed ACK chỉ lưu ở Agent. Chi tiết lifecycle ở DOMAIN_MODEL, policy nghiệp vụ ở CORPORATE_POLICY.
