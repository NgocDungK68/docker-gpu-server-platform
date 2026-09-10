# Thiết kế hệ thống AIWM

Tài liệu mô tả source hiện tại, đối chiếu tĩnh ngày 10/09/2026. Đây là thiết kế đã triển khai cùng các giới hạn quan sát được; không phải đề xuất thêm component. Không chạy build/test/runtime trong lần viết tài liệu này.

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
| Persistence đang chạy | [memory.Snapshot][snapshot] version 1 chứa domain maps; [durable.Store][durable] encode gob. Không có ORM/DB entity mapping đang chạy. SQL trong `migrations` là target schema chưa được startup sử dụng. |
| Frontend | [types.ts][fe-types], [api-client.ts][fe-client] và BFF sử dụng public contract; không sở hữu policy, reservation hoặc state transitions. |

Chi tiết từng field, invariant và test liên quan: [DOMAIN_MODEL](DOMAIN_MODEL.md). Cách chạy/demo/test Windows và WSL: [README](../README.md), [WINDOWS-WSL](WINDOWS-WSL.md).

## D1 — System architecture

~~~mermaid
flowchart LR
    U["User"] --> UI["Frontend"]
    UI --> BFF["Next.js BFF"]
    subgraph CP["Control Plane process"]
        API["httpapi"]
        APP["application.ControlPlane"]
        POL["policy.Engine"]
        CAT["capability.Catalog"]
        SCH["application.Scheduler"]
        REP["ports.Repository"]
        MEM["memory.Store<br/>Job / Assignment / Command<br/>inventory reconciliation"]
        DUR["durable.Store"]
        API --> APP
        APP -->|Evaluate / Order| POL
        APP -->|Resolve| CAT
        APP -->|Plan| SCH
        APP --> REP
        REP --> DUR
        DUR --> MEM
        DUR --> DISK[("gob snapshot")]
    end
    BFF -->|Public API| API
    subgraph AG["Agent process: production hoặc sim"]
        RUN["agent.Runner"]
        CORE["InventoryCollector<br/>CommandExecutor"]
        DOCK["DockerRuntime"]
        GPU["gpu.Reader"]
        RUN --> CORE
        CORE --> DOCK
        CORE --> GPU
    end
    RUN -->|register / heartbeat / inventory / poll / ACK| API
    API -.->|response của poll: commands| RUN
    DOCK -->|production| REAL["Docker Engine"]
    DOCK -->|sim| SIM["simulator.Runtime"]
    GPU -->|production| NV["Driver NVML + GPU"]
    GPU -->|sim| MOCK["NVIDIA mock NVML library"]
~~~

`ControlPlane.Queue` lấy queued Jobs từ repository rồi nhờ Policy Engine sắp thứ tự. Reservation, command lease/ACK và reconciliation inventory là mutation của repository; không có broker, reservation service hay reconciliation service riêng. Background `Reconcile` chỉ đánh dấu server stale; D9 chỉ rõ nơi cập nhật lifecycle theo container.

Production và simulator dùng cùng `Runner`, `InventoryCollector`, `CommandExecutor`, HTTP client, file state và `gpu.New`. Simulator thay Docker boundary bằng `simulator.Runtime`; cả hai vẫn gọi cùng NVML adapter, nhưng simulator nạp NVIDIA mock shared library. PID resolver của simulator lấy từ runtime mô phỏng.

Nguồn: [CP entrypoint][cp-main], [ControlPlane][cp], [Repository][ports], [Agent production][agent-main], [Agent sim][sim-main], [simulator][sim].

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

## D3 — Agent startup / register / discovery

~~~mermaid
sequenceDiagram
    participant M as aiwm-agent main
    participant D as dockerengine.Engine
    participant N as gpu.Reader
    participant R as Runner
    participant F as FileStore
    participant C as InventoryCollector
    participant A as Control Plane
    M->>M: agentconfig.Load
    M->>D: New(local endpoint)
    M->>N: gpu.New / nvml.Init
    M->>R: compose collector, client, executor, state; Run
    R->>F: Load identity, sequence, processed results
    R->>R: kiểm tra MachineID
    loop startup đến khi thành công hoặc context bị hủy
        R->>D: Ping
        opt chưa có AgentID
            R->>A: POST /api/v1/agents/register
            A-->>R: AgentID + AgentToken
            R->>F: Save identity
        end
        R->>F: tăng và Save InventorySequence
        R->>C: Snapshot(sequence)
        C->>D: ListContainers
        C->>N: Snapshot GPUs / processes
        C->>D: Version
        C->>C: correlation + discoverHost + report
        C-->>R: InventoryReport
        R->>A: PUT /api/v1/agents/{agentID}/inventory
        A-->>R: inventory được chấp nhận hoặc lỗi
    end
    R->>R: khởi chạy heartbeatLoop, inventoryLoop, commandLoop
    R->>D: WatchContainerEvents trong inventoryLoop
~~~

**Register diễn ra trước inventory đầu tiên trong `Runner.Run`**, khác với flow giả định “discover tất cả rồi register”. Agent đã có identity thì thử dùng lại; nếu heartbeat/report/poll nhận 401, `ensureRegistered` đăng ký lại. Khi report gặp 401, Agent gửi lại report sau khi nhận token mới. CP dùng MachineID để giữ Server.ID và reset inventory freshness/version khi re-enroll.

Vòng startup thử lại theo `HeartbeatEvery` khi Ping/register/report lỗi; các loops chỉ bắt đầu sau một report thành công. Lỗi load state, MachineID không khớp hoặc lỗi khởi tạo NVML ở main kết thúc process. Chế độ `--check` chỉ Ping + Snapshot rồi thoát, không register.

Simulator có bước riêng **trước Runner**: `gpu.New → Reader.Snapshot → simulator.NewRuntime` để seed/load existing containers theo UUID của mock NVML. Sau đó dùng đúng startup/loops trên. Ngắt Agent không có bước stop/delete containers.

Nguồn: [production main][agent-main], [sim main][sim-main], [Runner.Run/ensureRegistered/reportInventory][runner], [FileStore][agent-state], [RegisterAgent][cp].

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

## D5 — Create request → GPU placement → container

~~~mermaid
sequenceDiagram
    participant U as User / Frontend
    participant B as Next.js BFF
    participant H as httpapi
    participant C as ControlPlane
    participant P as policy.Engine
    participant S as Scheduler
    participant R as Repository
    participant A as Agent
    participant D as Docker Engine
    U->>B: POST /api/aiwm/jobs với intent + resources
    B->>H: POST /api/v1/jobs + public bearer
    H->>C: CreateJob(CreateJobRequest)
    C->>C: prepareJob: validate + catalog.Resolve
    C->>P: Evaluate
    C->>R: ListServers
    C->>S: matchJob / Plan trên snapshot
    C->>R: CreateJob với Status QUEUED
    H-->>B: 201 JobView
    B-->>U: Job đã vào queue
    Note over C,R: Scheduler ticker hoặc POST scheduler/run-once
    C->>R: ListQueuedJobs
    C->>P: Order: evaluate lại và xếp queue
    C->>R: ListServers
    C->>S: Plan: filter, score, select
    S-->>C: Placement hoặc ErrInsufficientGPU
    alt có placement
        C->>R: CommitAssignment + START_CONTAINER
        R-->>C: ASSIGNED + RESERVED + PENDING đã commit
        A->>H: GET commands: lease
        H-->>A: START_CONTAINER + exact GPUUUIDs
        A->>A: GPUsAvailable với snapshot local mới
        A->>D: StartManagedContainer
        D-->>A: ContainerID hoặc lỗi
        A->>A: lưu processed result trước ACK
        A->>H: POST command ACK
        A->>H: PUT inventory sau command
        H->>C: ReportInventory
        C->>R: ReplaceInventory + reconcile + normalize
    else chưa đủ GPU
        C->>R: giữ QUEUED, cập nhật StatusReason
    end
    U->>B: GET Job / inventory qua API client
    B->>H: public read
    H-->>B: JobView / inventory
    B-->>U: kết quả public read
~~~

Đây là flow submit hợp lệ; input lỗi bị chặn trước `CreateJob`. `TRAINING/INFERENCE`, Necessity, reason, systemImportance, neededAt, TTL và GPU requirements được backend validate. User không gửi scheduler/server/GPU/priority score; JSON có field ngoài DTO bị từ chối.

`Policy Engine` đánh giá business lane và thứ tự phục vụ; cả `AUTO_ELIGIBLE` lẫn `COMPETITIVE` vẫn có thể được scheduler xét. Chưa có approval workflow riêng hoặc hard rejection vì vượt quota. `Scheduler` chỉ chọn nơi chạy đáp ứng physical constraints. Không đưa auxiliary policy score vào placement score.

Agent chủ động poll; CP không mở kết nối vào Agent để push command. Start guard đọc inventory local mới trước khi tạo container; Docker adapter dùng `DeviceRequests.Driver=nvidia` và exact `DeviceIDs` từ Assignment, gửi CPU/RAM thành Docker limits. Container cùng managed Job đã tồn tại được tái sử dụng cho idempotency; lỗi launch trả failed ACK. Diagram biểu diễn flow tuần tự thông thường; inventory/event loop độc lập có thể thấy container active **trước ACK**.

Nguồn: [form][fe-form], [client][fe-client], [BFF][bff], [httpapi][http], [CreateJob/Queue/ScheduleOnce][cp], [executor][executor], [Docker adapter][docker].

## D6 — Preview vs submit

~~~mermaid
flowchart LR
    INPUT["Intent + resource requirements"] --> PRE["POST jobs/preview"]
    INPUT --> SUB["POST jobs"]
    PRE --> PV["prepareJob<br/>validate / resolve / Evaluate"]
    PV --> PM["matchJob<br/>snapshot + Plan"]
    PM --> OUT["JobPreview<br/>không ID / mutation / reservation"]
    SUB --> SV["prepareJob lại<br/>validate / resolve / Evaluate"]
    SV --> SM["matchJob<br/>resource snapshot mới"]
    SM --> QUE["CreateJob: QUEUED"]
    QUE --> CYCLE["ScheduleOnce<br/>Order lại + snapshot"]
    CYCLE --> PLAN["Plan"]
    PLAN --> COM["CommitAssignment<br/>recheck + reserve"]
    OUT -.->|chỉ tham khảo| INPUT
~~~

`JobPreview` trả policy reason, sizing/quota labels, Necessity, model list, maximum matching GPU count trên một server và khả năng đáp ứng snapshot. Nó **không trả recommended server ID/GPU UUID/score**, dù nội bộ gọi `Plan`. `recommendedGpuModels` là union model phù hợp từ các server fresh; `matchedGpuCount` là max của từng server, không phải tổng pool.

**Preview recommendation không bảo đảm final placement giống lúc submit.** Preview không giữ tài nguyên, không xét việc các Jobs khác sẽ chiếm tài nguyên trước request này. Submit evaluate lại, vẫn nhận Job hợp lệ với `QUEUED` kể cả chưa đủ GPU; cycle mới quyết định assignment sau khi xếp toàn queue. Commit còn kiểm tra lại resource state dưới lock.

Form tải catalog qua `GET /api/v1/jobs/options`; `allocationInput` đổi giờ local thành UTC và giờ TTL thành giây, không tính coefficient. `inputKey/currentPreview` loại kết quả preview của input cũ; submit ở UI cần preview khớp input hiện tại. API không yêu cầu preview token/ID nên client khác có thể submit trực tiếp; backend luôn tự validate/evaluate.

Nguồn: [allocation.go][allocation-app] — `prepareJob/PreviewJob/matchJob`; [form][fe-form], [form-schema][fe-schema], [preview panel][fe-preview].

## D7 — Scheduler internal flow

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
    PROP --> RES["Repository.CommitAssignment"]
~~~

`filter` duyệt constraints theo thứ tự trên để tìm lý do chờ; tập lựa chọn thực tế dùng chung `ResourceRequest.Matches` qua `matchingGPUs`. Sau mỗi assignment hoặc conflict, `ScheduleOnce` refresh server snapshot; Job không vừa sẽ không chặn mọi Job nhỏ phía sau.

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

## D8 — Reservation và concurrency

~~~mermaid
flowchart TD
    P["Placement + START command đề xuất"] --> TX["durable.transact<br/>snapshot trước mutation"]
    TX --> LOCK["memory.CommitAssignment<br/>mu.Lock"]
    LOCK --> CHECK["Recheck Job QUEUED, server freshness<br/>UUID/count/constraints/command identity"]
    CHECK -->|Conflict| NONE["Không mutate<br/>refresh snapshot, cycle sau thử lại"]
    CHECK -->|Hợp lệ| MUT["GPU RESERVED + AssignedJobID<br/>Job ASSIGNED + Assignment RESERVED<br/>START command PENDING"]
    MUT --> SAVE["gob temp + Sync + Rename"]
    SAVE -->|Lỗi ghi| RB["Restore memory snapshot<br/>trả lỗi"]
    SAVE -->|Thành công| LEASE["Agent poll: DELIVERED"]
    LEASE --> START["Agent local guard + Docker start"]
    START -->|ACK thành công khi ASSIGNED| ACK["Job STARTING<br/>vẫn RESERVED"]
    START -->|ACK thất bại, Job đủ điều kiện| FAIL["Job FAILED<br/>releaseLocked"]
    ACK --> OBS["Inventory thấy active"]
    START -.->|inventory active trước ACK| OBS
    OBS --> RUN["Job RUNNING<br/>Assignment ALLOCATED"]
    RUN --> TERM["Terminal observation / stop lifecycle"]
    TERM --> REL["Assignment RELEASED<br/>normalize GPU theo consumers"]
~~~

`Placement` chưa sở hữu tài nguyên. `CommitAssignment` kiểm tra **toàn bộ UUID trước mutation**, số lượng phải đúng GPUCount, không lặp/thiếu UUID, command thuộc server, Job còn QUEUED và từng GPU còn đáp ứng `Matches`. Các updates GPU + Assignment + Job + command nằm trong cùng memory critical section. Hai commit cạnh tranh cùng GPU không cùng thành công trên snapshot repository hợp lệ.

Adapter durable giữ mutex ngoài, snapshot trước/sau, ghi gob và rollback memory khi mutation/ghi file lỗi; public reads cũng đi qua mutex này. OS file lock ngăn hai writer dùng cùng state file. Đây là atomic local commit cho một CP, không phải distributed transaction với Docker. `ErrConflict` không hủy Job; job còn QUEUED sẽ được xét ở cycle sau. Nếu caller khác đã assign/cancel Job thì giữ trạng thái do caller đó commit.

| Trigger | Hiệu ứng reservation |
|---|---|
| Hủy QUEUED | Chưa có Assignment để release. |
| Hủy sau assignment, START còn `PENDING` | START chuyển `FAILED` với lý do cancel; Job `CANCELLED`, Assignment `RELEASED`. |
| START failed ACK khi Job `ASSIGNED/STARTING/STOPPING` | Job `FAILED` và release; ACK thất bại sau khi Job đã `RUNNING` không áp nhánh này. |
| Inventory `exited/dead` | Job `STOPPED` nếu đang STOPPING; nếu không thì `SUCCEEDED` khi exit code 0, còn lại `FAILED`; release. |
| Container missing | `RUNNING` → FAILED; `STOPPING` → STOPPED; `STARTING` → FAILED chỉ khi report ObservedAt không trước Job UpdatedAt. `ASSIGNED` missing được giữ. |
| STOP succeeded ACK | Chỉ hoàn tất command; đợi observation, không tự release. |
| STOP failed ACK khi STOPPING | Job trở về RUNNING; không release. |
| Offline / CP mất kết nối / hết command lease | Không tự release, migrate hoặc enqueue Job lại. |

`Assignment.ReservationState` dùng string `RESERVED → ALLOCATED → RELEASED` và cả `RESERVED → RELEASED`. Assignment vẫn lưu lịch sử sau terminal; active consumer còn được inventory ghi nhận sẽ tiếp tục chặn GPU. Vì thế **release logical reservation không đồng nghĩa GPU FREE**.

Guard lease STOP chờ START hết PENDING/DELIVERED có tồn tại, nhưng nhánh STOPPING + missing chưa có guard tương tự. Không khẳng định chống double runtime allocation trong mọi interleaving hoặc trước Docker operations ngoài AIWM; xem observations.

Nguồn: [CommitAssignment/LeaseCommands/AckCommand][store], [RequestStop/applyAckLocked/releaseLocked][lifecycle], [durable.transact/saveSnapshot][durable], [accounting][accounting]. Test source có `TestConcurrentReservationHasOneWinner` và `TestDiskFailureRollsBackAndLockExcludesSecondWriter` tại [memory safety tests][memory-tests], [durable tests][durable-tests]; không chạy lại trong task này.

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

## D10 — Failure handling

~~~mermaid
flowchart TD
    LOSS["Không nhận heartbeat và inventory mới"] --> STALE["LastHeartbeatAt quá OfflineAfter"]
    STALE --> OFF["MarkStaleServers: OFFLINE"]
    OFF --> BLOCK["Server.Schedulable=false<br/>không placement mới"]
    CPDOWN["Control Plane unavailable"] --> RETRY["Agent HTTP lỗi<br/>loops tiếp tục thử"]
    RETRY --> NOPOLL["Không lấy được command mới"]
    OFF --> KEEP["Giữ Job / reservation / last-known"]
    NOPOLL --> RUNTIME["Không phát sinh stop/cleanup<br/>Docker runtime độc lập"]
    KEEP --> RUNTIME
    BACK["Kết nối trở lại"] --> ID["Dùng lại identity<br/>re-enroll nếu 401"]
    ID --> SNAP["Snapshot Docker + NVML mới"]
    SNAP --> REPORT["Inventory được chấp nhận<br/>reconcile + freshness"]
    REPORT --> ELIG["Không drain + server fresh<br/>GPU healthy/free và match mới được chọn"]
~~~

Inventory được chấp nhận cũng cập nhật `LastHeartbeatAt`; riêng việc endpoint heartbeat thất bại chưa đủ làm OFFLINE nếu report vẫn tới. Ngược lại, heartbeat thành công nhưng inventory cũ khiến server **không schedulable dù Status ONLINE**. `Server.Schedulable` kiểm tra freshness trực tiếp, nên scheduler không cần đợi timer đổi enum OFFLINE.

Agent không dừng container khi CP unreachable hoặc context shutdown. Command đã nhận có thể đang thực thi; mất CP không phải tín hiệu hủy workload. Mất kết nối không chứng minh host/Docker/GPU còn khỏe, chỉ chứng minh code quản lý không chủ động stop chúng. CP không tự release reservation hoặc chuyển workload sang host khác vì mất heartbeat.

CP restart dùng gob restore, đặt server OFFLINE và reset `InventoryReceivedAt`. Heartbeat có thể đổi status ONLINE/DRAINING nhưng vẫn cần inventory mới để placement. Agent giữ sequence và kết quả command đã xử lý trên local file; lease hết hạn cho phép redelivery, result được lưu trước ACK và ACK terminal lặp không áp hiệu ứng lần hai. Không có đảm bảo exactly-once cho toàn bộ crash window giữa Docker và filesystem.

Nguồn: [Schedulable][model], [MarkStaleServers][store], [Runner][runner], [durable.Open][durable], [Agent FileStore][agent-state].

## S1 — Job lifecycle

~~~mermaid
stateDiagram-v2
    [*] --> QUEUED: CreateJob
    QUEUED --> QUEUED: chưa đủ GPU / cập nhật reason
    QUEUED --> ASSIGNED: CommitAssignment
    QUEUED --> CANCELLED: RequestStop
    ASSIGNED --> CANCELLED: stop khi START còn PENDING
    ASSIGNED --> STARTING: START ACK succeeded
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

## B1 — Frontend → Control Plane → Agent trust boundary

~~~mermaid
flowchart LR
    UI["Browser UI"] -->|HTTP /api/aiwm| BFF["Next.js BFF<br/>whitelist + origin check"]
    BFF -->|public bearer phía server| PUB["CP public API"]
    POST["Operator API client"] -->|public bearer| PUB
    AG["Agent local"] -->|enrollment token| REG["CP register endpoint"]
    REG -.->|AgentID + token riêng| AG
    AG -->|Agent bearer: heartbeat / inventory / poll / ACK| INT["CP Agent protocol"]
    INT -.->|poll response| AG
    AG -->|local socket| DE["Docker Engine"]
    AG -->|local library| NV["NVML / NVIDIA GPU"]
~~~

`src/app/api/aiwm/[...path]/route.ts` chỉ forward public GET/POST được whitelist. Browser không có đường BFF tới Agent routes/Docker socket/NVML; CP không gọi remote NVML. Public và Agent protocol hiện cùng HTTP server, tách bằng route/auth, không phải hai process/API gateway riêng.

`AIWM_API_TOKEN` chỉ được đọc ở Next.js server và đính vào upstream public request. Register dùng `X-Enrollment-Token`; các requests theo AgentID dùng token riêng, CP lưu SHA-256 hash trong Server, Agent lưu token thật ở local state. Job public response che environment; command payload và private persistence vẫn cần full spec để chạy container.

CP hỗ trợ server TLS tùy cấu hình. Agent client hỗ trợ custom CA/client certificate; CP trực tiếp chưa enforce client CA/mTLS. Console hiện chưa có user login/RBAC/session authorization: BFF origin check và token upstream không thay thế xác thực người dùng, và mutation không có Origin vẫn được helper cho qua. Quyền điều khiển local Docker thuộc Agent trên host.

Nguồn: [BFF][bff], [proxy-policy][proxy-policy], [server config][fe-config], [publicAuth][security], [RegisterAgent][cp], [Agent HTTP/TLS client][agent-client], [production deployment guide][agent-deploy].

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
        A --> AS
        T --> TS
        A -->|outbound HTTP| CP
        T -->|outbound HTTP| CP
        NEXT -->|HTTP + public token| CP
    end
    NATIVE["Next.js native trên Windows<br/>phương án thay console container"] -->|localhost:8080| CP
~~~

[compose.yaml][compose] định nghĩa CP publish `127.0.0.1:8080`; Console optional profile `console` publish `127.0.0.1:3000`. Có thể chạy Next.js native thay Console container theo README. Hai simulator là hai logical servers trong cùng môi trường demo, không chứng minh hai physical GPU hosts.

`agent-a100` dùng `a100-external.yaml`, seed External ở GPU index 0; `agent-t4` dùng `t4-empty.yaml`. Profile mount read-only; Agent identity và simulated Docker runtime giữ ở volume riêng mỗi Agent. [Dockerfile.sim][sim-dockerfile] nạp `libnvidia-ml.so.1` qua `LD_LIBRARY_PATH`, YAML qua `MOCK_NVML_CONFIG`.

Không có database service: CP lưu `/data/control-plane.gob`. `simulator.Runtime` giữ JSON containers và phát events mô phỏng; **không chạy image/command/CUDA thật**, không tự tạo compute utilization tương ứng với Job. Metrics GPU trong demo đi qua adapter NVML nhưng dữ liệu từ mock profile.

NVML adapter hiện yêu cầu Linux/CGO; [nvml_unsupported.go][nvml-unsupported] trả lỗi ở native Windows. Vì vậy Windows laptop dùng Linux containers/WSL cho Agent sim; việc đọc tài liệu này không yêu cầu khởi chạy chúng.

## P2 — Ánh xạ process production

~~~mermaid
flowchart LR
    USER["Browser"] --> FE["Next.js frontend + BFF"]
    FE --> CP["aiwm-server<br/>một writer"]
    CP --> DISK[("local durable gob")]
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

Production quota service, PostgreSQL, HA/leader election và distributed lock: **Planned / Not implemented**, không nằm trong diagrams như component đang hoạt động.

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
| TTL auto-stop / neededAt delayed start | NOT_IMPLEMENTED | D6, D8 | Fields được [validate][validation]/[lưu][allocation-app], chưa có timer enforce |
| Host CPU/RAM admission accounting | NOT_IMPLEMENTED | D7 | Docker nhận limits qua [engine][docker]; [Scheduler][scheduler] không cộng trừ host capacity |
| Production quota ledger, user/project RBAC | NOT_IMPLEMENTED | D7, B1 | [DevelopmentFacts][policy], [BFF][bff], [publicAuth][security] |
| PostgreSQL / HA / distributed scheduling lock | NOT_IMPLEMENTED | P2 | [durable][durable] dùng gob; SQL target chưa nối runtime |
| MIG/sharing/time-sharing/preemption/reclaim/checkpoint | NOT_IMPLEMENTED | D1, D7 | Ngoài scope; whole-GPU filter và [validation][validation]; NVML đánh dấu MIG enabled không healthy |

## Design observations / Inconsistencies

Các observations dưới đây đến từ source đã đọc, không phải lỗi đã tái hiện bằng runtime test trong task này. Không sửa code hoặc tài liệu cũ ngoài link README.

### O1 — Policy facts và thời gian request chưa phải enforcement production

**Observation:** `DevelopmentFacts` đọc planning/quota tĩnh từ cấu hình, không tăng usage khi assign. `neededAt/ttlSeconds` được validate/lưu nhưng scheduler không chờ neededAt và không tự stop theo TTL.

**Impact:** demo chứng minh policy ordering/validation; chưa chứng minh quota admission tổng hợp hoặc đảm bảo thời gian chạy. Không diễn giải TTL như deadline của command lease/reservation.

**Relevant files:** [policy/engine.go][policy], [application/allocation.go][allocation-app], [validation.go][validation], [scheduler.go][scheduler].

### O2 — STOPPING + missing có thể release trước late START

**Observation:** `LeaseCommands` chặn STOP khi START còn PENDING/DELIVERED; `reconcileObservedLocked` lại cho STOPPING + absence → STOPPED/release mà không kiểm tra START. Failed STOP ACK cũng có thể đưa Job RUNNING khi chưa từng có active observation.

**Impact:** từ phân tích nhánh code, inventory trong lúc START đang thực thi có thể release logical reservation trước late start. Atomic CP reservation và guard lease STOP chưa chứng minh an toàn cho mọi interleaving runtime. `RUNNING` không luôn đồng nghĩa vừa quan sát container active.

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

**Observation:** JobStatus tổng hợp control/observed progress; ReservationState và Container.State là strings, generic `SetJobStatus` không enforce toàn bộ adjacency graph. Missing guard của STARTING so `report.ObservedAt` từ Agent với `job.UpdatedAt` từ CP. Assignment RELEASED vẫn có thể đi cùng GPU ALLOCATED nếu consumer còn active.

**Impact:** không tự dựng DesiredState/ActualState entities, hoặc ép state GPU khớp tên reservation. Clock skew chưa được giải quyết chỉ bằng inventory sequence; unknown runtime state có record cũng có thể giữ reservation lâu vì không vào nhánh missing/exit.

**Relevant files:** [domain/model.go][model], [memory/store.go][store], [lifecycle.go][lifecycle], [accounting.go][accounting].

### O6 — Điểm số vẫn xuất hiện ở trang chi tiết

**Observation:** form/preview không có K hoặc score; `PolicyDecision.AuxiliaryScore` không ra JSON. Tuy nhiên `JobView.Assignment` vẫn serialize Score/Reason và `JobLifecycle` render trực tiếp `job.assignment.score`, assignment reason, event reason. `Scheduler.Plan` đưa score vào placement reason.

**Impact:** yêu cầu trước đó “Frontend không hiển thị internal K/score” chưa đạt trên toàn Console. Không nhầm placement score đang lộ với auxiliary policy score đã được ẩn. Đây là inconsistency được ghi nhận, không sửa UI trong task documentation.

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

### O10 — Persistence và auth có phạm vi một môi trường operator

**Observation:** CP serialize toàn bộ mutation/read quanh memory + gob snapshot, một file writer. Server restore offline để chờ inventory mới. Public API dùng static bearer; BFF không có user login/RBAC. Agent client cert support chưa tương đương CP enforce mTLS.

**Impact:** không coi SQL target, browser origin check hoặc client certificate config là PostgreSQL transaction, multi-user authorization hay HA đã implement. Full snapshot mỗi mutation và giữ lịch sử chưa có retention đặt giới hạn khi state lớn; task này không đo tải/power-loss durability.

**Relevant files:** [durable/store.go][durable], [snapshot.go][snapshot], [security.go][security], [BFF][bff], [Agent client][agent-client], [CP entrypoint][cp-main].

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
