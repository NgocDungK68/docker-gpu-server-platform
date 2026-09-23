# Algorithms và decision rules của backend AIWM

Thuật toán đối chiếu tĩnh ngày 11/09/2026; bổ sung Organization hard filter ngày 17/09/2026. Backend source là nguồn duy nhất cho behavior trong tài liệu; ví dụ là tính tay theo code, không phải kết quả benchmark/test. Phạm vi: request → policy/queue → filtering/placement → repository reservation/release, cùng occupancy đầu vào trực tiếp. Không khảo sát thuật toán bên ngoài hoặc frontend.



## A00 — Organization hard filter (ACTIVE)

### Mục tiêu

Giữ ownership giữa các đơn vị trong cùng logical pool, không cho priority hoặc placement policy vượt ranh giới đơn vị.

### Khi nào được gọi

`prepareJob` nhận organization từ authenticated Principal. `matchJob` lọc trước recommendation; `Scheduler.filter` lọc trước readiness, labels, capability, health, count, VRAM và mọi scoring. `CommitAssignment` kiểm tra lại dưới runtime lock trước reserve.

### Đầu vào

`Job.OrganizationID`, `Server.OrganizationID`; ownership Server được resolve từ enrollment metadata, không từ Agent.

### Đầu ra

Tập Server cùng Organization hợp lệ, hoặc không có candidate. Job vẫn QUEUED nếu không đủ tài nguyên trong đơn vị.

### Hard constraints

Hai field phải cùng giá trị và Job.OrganizationID không rỗng. Snapshot cũ thiếu org không tự được đưa vào một đơn vị mặc định. Organization disabled/không tồn tại trong metadata bị `ScheduleOnce` bỏ qua ở cycle đó.

### Công thức / comparator

`eligible(server, job) = job.OrganizationID != "" AND server.OrganizationID == job.OrganizationID`.

Đây là predicate, không phải score hoặc preference. Input validation về dạng request vẫn chạy trước scheduler; ownership là resource filter đầu tiên.

### Pseudocode

```text
for server in snapshot:
    if not SameOrganization(job.OrganizationID, server.OrganizationID): continue
    apply existing readiness / resource filters
score only remaining candidates
commit: check SameOrganization again, then reserve + job + command
```

### Ví dụ

Job thuộc org-A cần 1 GPU; org-A hết GPU, org-B còn 8 GPU phù hợp và score tốt hơn. Job vẫn QUEUED; không có fallback sang org-B.

### Complexity

Predicate O(1), lọc S Server O(S). Không thay complexity hoặc công thức của bốn strategy.

### Source mapping

- `internal/domain/organization.go`: `SameOrganization`.
- `internal/application/allocation.go`: `prepareJob`, `matchJob`.
- `internal/application/scheduler.go`: `Scheduler.filter`.
- `internal/application/controlplane.go`: `ScheduleOnce`; `tenancy.go`: `enabledOrganizations`.
- `internal/store/memory/store.go`: `CommitAssignment` (durable.Store bọc transaction này).

Công thức policy/strategy phía dưới giữ nguyên. Các ví dụ placement cũ phía dưới giả định Job và Server cùng organization không rỗng; không có fallback cross-organization. Business baseline được giải thích riêng trong [CORPORATE_POLICY.md](CORPORATE_POLICY.md).



## T — Planning theo thời gian và execution (ACTIVE, 23/09/2026)

### Mục tiêu và phạm vi

Policy quyết định **Job nào được ưu tiên khi tranh tài nguyên**. Planning Scheduler quyết định **Server/GPU và khoảng thời gian giữ chỗ**. Execution controller quyết định **khi nào tạo START**, sau khi kiểm tra lại trạng thái hiện tại. Bốn công thức placement ở A15–A19 giữ nguyên; chúng chấm candidate của khoảng thời gian yêu cầu, không chỉ snapshot FREE hiện tại.

### Đầu vào / đầu ra

- Đầu vào: Job, neededAt, ttlSeconds, Policy, Server/GPU snapshot, Assignment của các Job khác, thời gian CP.
- ttlSeconds chính là requestedDuration tính bằng giây; không thêm timeout song song. StartAt = neededAt; EndAt = neededAt + ttlSeconds.
- Đầu ra planning: ASSIGNED + Assignment PLANNED, hoặc QUEUED với reason “Không đủ GPU an toàn trong khoảng thời gian yêu cầu”. Chưa tạo command.
- Đầu ra execution: STARTING + Assignment RESERVED + GPU RESERVED + START_CONTAINER PENDING trong một transaction.

### Hard constraints và overlap

Interval nửa mở **[start,end)**:

~~~text
overlap(A, B) = A.start < B.end AND B.start < A.end
~~~

08:00–10:00 và 10:00–12:00 được giữ cùng GPU; 08:00–11:00 và 10:00–12:00 xung đột. Helper duy nhất: domain.TimeWindow.Overlaps.

Order: **Policy → Organization → server freshness / capability → time availability → placement strategy → reservation**. Job.OrganizationID phải khác rỗng và bằng Server.OrganizationID. Policy không được vượt constraint này.

GPU PLANNED không có state vật lý riêng: inventory vẫn có thể FREE. Calendar projection loại UUID có Assignment overlap. GPU RESERVED/ALLOCATED bởi AIWM có EndAt rõ ràng có thể được plan cho một khoảng tương lai sau EndAt; projection không thay đổi inventory. Legacy/unknown/unhealthy vẫn bị chặn. Đến giờ, execution luôn yêu cầu GPU thực tế FREE, không tin dự đoán đã kết thúc.

### Comparator và pseudocode

Policy giữ nguyên: lane → Necessity 1,2,3,4 → auxiliary giảm dần → CreatedAt tăng dần → ID tăng dần. Auxiliary = round(K_importance × K_quota × K_waiting × 1000)/1000. Chỉ so auxiliary khi cùng Necessity và lane.

~~~text
snapshot + lock runtime repository
candidates = QUEUED + ASSIGNED/PLANNED có StartAt > now, CommandID rỗng
frozen = các Assignment còn lại
for job in policy.Order(candidates):
    projected = CalendarServers(job, servers, frozen + plans trước, now)
    placement = strategy.Plan(job, projected)
    nếu đủ: lưu plan và thêm booking để Job sau kiểm overlap
    nếu thiếu: QUEUED + reason
validate tất cả plans; commit một batch, không tạo START

execution tick:
    nếu interval kết thúc: cancel chưa dispatch hoặc request STOP
    nếu PLANNED và start <= now < end:
        revalidate fresh/org/health/GPU thực tế
        commit GPU RESERVED + STARTING + START command
~~~

Chỉ replan future; reservation đã bắt đầu được giữ nguyên đến khi thực thi an toàn hoặc hết interval. Retry không kéo dài EndAt. Không preempt RUNNING. Preview chạy cùng greedy planner trên bản copy, không commit; một request N1 có thể được preview là AVAILABLE dù N2 đang giữ lịch tương lai.

### Ví dụ

Hiện tại 08:00, cùng lane, một GPU phù hợp. A/N2 xin 14:00–16:00 và đã PLANNED. B/N1 tới lúc 09:00 xin cùng interval: B nhận PLANNED, A về QUEUED. Trước 14:00 không có START. Lúc 14:00 nếu inventory fresh và GPU safe: B STARTING → RUNNING. Lúc 16:00 gửi STOP; chỉ inventory xác nhận stopped/absent mới release. Job khác bắt đầu đúng 16:00 có thể phải chờ STOP được xác nhận, không cấp trùng GPU.

### Complexity và giới hạn

Không dùng interval tree. CalendarServers quét Server/GPU × bookings và membership UUID tuyến tính; với G GPU, R bookings, K UUID/booking: O(G × R × K) mỗi Job, cộng policy sort và placement hiện có. Batch revalidate lặp lại kiểm tra dưới mutex; durable thêm chi phí snapshot. Đây là single-writer MVP, callback planning phải không gọi ngược runtime Repository.

Clock quyết định là clock CP; ticker mặc định scheduling 2s, reconcile 5s. Đây không phải realtime deadline: image pull/start và STOP graceful có độ trễ; CP/Agent mất liên lạc có thể kéo dài container quá EndAt. Không release hoặc cấp trùng vì mất kết nối. Đồng bộ clock host cần cho so sánh observed inventory với ACK.

### Source mapping / tests

- internal/domain/reservation.go: TimeWindow, RequestedWindow, Replannable, CalendarServers.
- internal/application/planning.go: planReservations, processReservations.
- internal/application/controlplane.go: ScheduleOnce, Reconcile.
- internal/store/memory/planning.go: ReplanReservations, startDeliverableLocked.
- internal/store/memory/store.go: CommitAssignment, LeaseCommands.
- internal/store/memory/lifecycle.go: RequestStop, stopConfirmedLocked, reconcileObservedLocked.
- application/planning_test.go, domain/reservation_test.go, durable/planning_test.go: thời gian giả, Policy, isolation, revalidation, expiration, concurrency, rollback/restart.

## Pipeline hiện tại

~~~mermaid
flowchart LR
    AUTH["Authenticated Organization"] --> REQ["CreateJobRequest"] --> PREP["validate / resolve / Evaluate"]
    PREP --> MATCH["matchJob: snapshot + Plan"]
    MATCH --> PREVIEW["Preview: chỉ trả assessment"]
    MATCH --> QUEUE["Submit: CreateJob QUEUED"]
    QUEUE --> ORDER["Cycle: policy.Order lại"]
    ORDER --> ORG["SameOrganization hard filter"]
    ORG --> FILTER["Snapshot mới + resource filters"]
    FILTER --> CAND["Candidates + selected GPUs"]
    CAND --> SCORE["Configured strategy"]
    SCORE --> BOOK["ReplanReservations: PLANNED"]
    BOOK --> WAIT["Đợi StartAt; revalidate"]
    WAIT --> COMMIT["CommitAssignment: GPU reserve + START"]
    COMMIT --> CMD["Agent poll trong interval"]
~~~

Preview và submit cùng dùng `prepareJob/matchJob` nhưng là hai request độc lập. Submit không reserve ngay: `CreateJob` lưu QUEUED; `ScheduleOnce` mới xếp queue, chọn placement và commit. Không có Job state SCHEDULING/RESERVED/DISPATCHING. Candidate rỗng hoặc conflict không persist START command mới.

## Algorithm inventory

`ACTIVE` nghĩa là có implementation và đường gọi từ runtime entrypoint/API; không có nghĩa đã chạy thử trong audit này. Bốn strategy đều `ACTIVE` theo cấu hình, mỗi scheduler instance dùng một strategy. Default trong source là best-fit; override của process đang triển khai không được kiểm chứng.

| Runtime status | Cách dùng |
|---|---|
| ACTIVE | Có runtime wiring, kể cả nhánh có điều kiện/cấu hình. |
| PARTIAL | Behavior đã có nhưng phạm vi yêu cầu còn thiếu như quota accounting hoặc safety mọi interleaving. |
| DECLARED_ONLY | Chỉ còn declaration/field, chưa có thuật toán thực thi chức năng được mô tả. |
| IMPLEMENTED_NOT_WIRED | Có implementation nhưng không có runtime caller; không tìm thấy strategy nào thuộc nhóm này trong phạm vi audit. |
| NOT_IMPLEMENTED | Chưa có behavior, không suy từ field/config thành enforcement. |

| Category | Algorithm / Rule | Runtime status | Purpose | Main source |
|---|---|---|---|---|
| Tenancy | A00 — Organization hard filter | ACTIVE | Giữ ownership trước resource filtering/scoring, recheck khi commit | domain/organization.go; application/scheduler.go; memory/store.go |
| Request | A01 — validate intent/resources | ACTIVE | Chặn input không hợp lệ, whole-GPU only | [validation][validation], [policy.Validate][policy] |
| Capability | A02 — resolve profile/FP8 + legacy model fallback | ACTIVE | Pin danh sách model phù hợp | [catalog][catalog], [ResourceRequest][allocation-model] |
| Policy | A03 — sizing-plan/quota facts | ACTIVE | Static DEVELOPMENT_CONFIG facts | [DevelopmentFacts.Facts][policy] |
| Policy | Production quota usage/ledger | PARTIAL | Có FactsProvider, chưa cập nhật usage thật | [FactsProvider][policy] |
| Policy | A04 — system importance coefficient | ACTIVE | Category → coefficient do backend derive | [ImportanceCoefficient][policy] |
| Policy | A05 — waiting factor | ACTIVE | Aging theo CreatedAt, cap 28 ngày | [WaitingCoefficient][policy] |
| Policy | A06 — lane/admission + auxiliary product | ACTIVE | AUTO_ELIGIBLE/COMPETITIVE; rounded score | [Engine.Evaluate][policy] |
| Priority | A07 — Necessity/lexicographic/FIFO/ID | ACTIVE | Thứ tự queue deterministic | [Before/Order][policy] |
| Priority | Numeric `Job.Priority` làm comparator | DECLARED_ONLY | Field legacy còn lưu/đọc; không còn rule xếp queue theo nó | [Job][model], [Before][policy] |
| Queue | A08 — lấy queue, attempt, retry cycle | ACTIVE | Không vừa thì giữ QUEUED và thử Job sau | [Queue/ScheduleOnce][cp], [ListQueuedJobs][store] |
| Queue | Chống starvation tuyệt đối / terminal requeue | NOT_IMPLEMENTED | Aging có cap; terminal không tự quay lại queue | [policy][policy], [lifecycle][lifecycle] |
| Readiness | A09 — ONLINE, drain, heartbeat/inventory freshness | ACTIVE | Chặn host không sẵn sàng | [Server.Schedulable][model], [store][store] |
| Accounting | A10 — health / grants / unknown telemetry | ACTIVE | Tạo observation bảo thủ trước CP filtering | [InventoryCollector][inventory], [NVML][nvml] |
| Accounting | PID/container attribution đầy đủ | PARTIAL | Correlation có fallback unknown, không xác nhận mọi owner | [Snapshot][inventory] |
| Accounting | A11 — normalized GPU states / consumers | ACTIVE | Legacy/unknown/managed/reservation precedence | [normalizeGPUState][accounting] |
| Filtering | A12 — health/state/assigned/model/VRAM | ACTIVE | Chọn từng GPU an toàn và tương thích | [Matches][allocation-model], [GPU][model] |
| Filtering | A13 — selector/count/candidate construction | ACTIVE | Đủ GPU cùng server; GPU order cố định | [filter/matchingGPUs][scheduler] |
| Diagnostics | A14 — insufficient-resource reason | ACTIVE | Lý do chờ từ flags qua constraints | [filter][scheduler], [matchJob][allocation-app] |
| Placement | A15 — First Fit | ACTIVE | Score 0, cuối cùng tie Server.ID | [NewScheduler][scheduler] |
| Placement | A16 — Best Fit | ACTIVE | Ít matching GPU còn dư | [NewScheduler][scheduler] |
| Placement | A17 — Bin Packing | ACTIVE | Count residual + occupied host term | [NewScheduler][scheduler] |
| Placement | A18 — Fragmentation-aware | ACTIVE | Phạt mở host trống / để host dở dang | [fragmentationScore][scheduler] |
| Placement | A19 — default/dispatch/select/ties | ACTIVE | Registry, score thấp nhất, server ID tie | [Plan/selectCandidate][scheduler], [config][config] |
| Preview | A20 — read-only matching, submit refresh | ACTIVE | Không giữ tài nguyên từ recommendation | [PreviewJob/matchJob][allocation-app], [CreateJob][cp] |
| Reservation | A21 — atomic commit/double reservation guard | ACTIVE | Recheck rồi update GPU + Job + Assignment + command | [CommitAssignment][store] |
| Persistence | A22 — local durable transaction/rollback | ACTIVE | Serialize snapshot commit, một writer | [durable][durable], [main][main] |
| Dispatch | A23 — command lease/redelivery/ACK | ACTIVE | Dispatch sau reservation; ACK idempotent tại CP | [LeaseCommands/AckCommand][store] |
| Release | A24 — cancel/failed launch/observed terminal | ACTIVE | Kết thúc logical reservation đúng nhánh hiện có | [lifecycle][lifecycle] |
| Safety | Mọi interleaving start/stop và runtime bên ngoài | PARTIAL | Atomic CP state không bao phủ mọi runtime race | [lifecycle][lifecycle], [inventory][inventory] |
| Launch guard | A25 — local occupancy recheck trước start | ACTIVE | Chặn actual consumer mới; same-Job retry exception | [GPUsAvailable][inventory], [Execute][executor] |
| Planning | NeededAt delayed start / duration STOP | ACTIVE | Calendar PLANNED; execution gate và STOP tại EndAt | domain/reservation.go; application/planning.go |
| Limits | CPU-RAM pool accounting | NOT_IMPLEMENTED | Docker limits có, chưa accounting tổng capacity | [scheduler][scheduler] |
| Out of scope | MIG/sharing/preemption/reclaim/checkpoint | NOT_IMPLEMENTED | Chỉ whole physical GPU; không thuật toán tương ứng | [validation][validation], [scheduler][scheduler] |

## Policy/Priority và Scheduler/Placement

`policy.Evaluator` quyết định assessment và Job nào được xét trước. `AUTO_ELIGIBLE` không đảm bảo có GPU; `COMPETITIVE` không có nghĩa bị từ chối scheduling. Source hiện chưa có approval queue hay hard rejection vì vượt quota.

`application.Scheduler` quyết định Job đang được xét chạy trên server/UUID nào. `SchedulingPolicy.Score` nhận server và GPU candidates sau filter, không đọc PolicyDecision để tính score. `ScheduleOnce` gắn `Placement.Policy = job.Policy` trước commit để lưu assessment; đây là metadata, không trộn hai công thức.

| Khái niệm | Đối tượng hiện tại |
|---|---|
| User intent | `AllocationIntent` trong `CreateJobRequest/Job` |
| Physical constraints | Public `AllocationResources` → private `ResourceRequest` |
| Queue assessment | `PolicyDecision`; AuxiliaryScore không ra JSON |
| Proposal | `Placement` — chưa sở hữu tài nguyên |
| Reservation | `Job.Assignment` — ReservationState string |
| Runtime resource state | `Server.GPUs` đã normalize; scheduler dùng methods, không tin presentation field SchedulingReady |

## Quy ước complexity và ví dụ

`S`: số server; `G_i`: số GPU server i; `M_i`: số GPU match trên server i; `C`: số candidate servers; `J`: tổng Jobs lưu; `Q`: Jobs QUEUED; `R`: số resolved model aliases; `L`: số selector keys; `D`: số Commands. `K` trong placement là GPUCount, khác coefficient `K_importance/K_quota/K_waiting`.

Big-O dưới đây là phần xử lý được mô tả, giả sử so sánh key/model có độ dài bị chặn và hash-map lookup trung bình O(1). Nêu riêng chi phí copy payload/snapshot và external I/O; không coi mutex hoặc filesystem có latency O(1). “Not explicitly characterized from current implementation” dùng khi không tách được cận đơn giản đáng tin cậy trong phạm vi đọc.

Mỗi ví dụ giả định các input không được nhắc tới đều hợp lệ; giá trị minh họa không phải inventory của máy đang chạy.

## A01 — Validation của allocation request

### Mục tiêu

Ngăn input sai trở thành Job; tách request hợp lệ với việc hiện có đủ GPU.

### Khi nào được gọi

`POST /api/v1/jobs` và `/jobs/preview` → `prepareJob` → `validateJobRequest`; `/jobs/options` cung cấp choices/limits.

### Đầu vào

`domain.CreateJobRequest`, `RequestLimits`, `capability.Resolver`; business input là `AllocationIntent`.

### Đầu ra

`ValidationError.Fields` hoặc được tiếp tục; chưa tạo ID, reservation hay command.

### Hard constraints

`workloadType` phải TRAINING/INFERENCE; Necessity 1–4; systemImportance thuộc catalog. Reason phải thuộc đúng workload + level; CUSTOM cần explanation không trắng, explanation ≤4096 byte. `backend` rỗng được prepareJob đặt DOCKER; backend khác bị từ chối. `allowSharedGpu=true` bị từ chối.

`GPUCount` nguyên 1..MaxGPUCount (default 64), `MinVRAMMiB>0`, `FP8Required` phải khác nil và profile tồn tại. CPU/RAM không âm, có guard overflow khi đổi đơn vị. NeededAt parse RFC3339, normalize UTC; TTL 1..MaxTTLSeconds (default 2592000). Không kiểm NeededAt phải ở tương lai.

Name trim dài ít nhất 3 byte, raw name ≤128; image reference hợp lệ ≤512 byte. Env ≤128 keys, key đúng regex, value ≤8192 byte/không NUL; cấm NVIDIA_* và CUDA_VISIBLE_DEVICES. Command ≤256 args, mỗi arg ≤8192 byte/không NUL. HTTP decoder từ chối unknown fields, nên không nhận priority/strategy/serverSelector/gpuModel.

### Công thức / comparator

Không có score. `policy.Validate` kiểm membership/required fields, không chứng minh lý do nghiệp vụ mà user khai báo.

### Pseudocode

`collect field errors → validate business profile → nếu có lỗi: return ValidationError; ngược lại: tiếp tục prepareJob`.

### Ví dụ

`TRAINING + NECESSITY_2 + CUSTOMER_SLA` không hợp lệ; `CUSTOM` chỉ vượt qua reason check nếu có explanation. FP8=false hợp lệ, bỏ field FP8 không hợp lệ.

### Complexity

Phụ thuộc số/độ dài command/env và catalog scan; không gán O(1) cho toàn request. Not explicitly characterized from current implementation.

### Source mapping

[internal/application/validation.go][validation] — `validateJobRequest`, `DefaultRequestLimits`; [internal/policy/engine.go][policy] — `Validate`; [profiles.go][profiles] — `Profiles`; [httpapi/server.go][http] — `decodeJSON`; [domain/dto.go][dto] — `CreateJobRequest`.

## A02 — PerformanceProfile / FP8 resolution và model fallback

### Mục tiêu

Đổi requirement thành exact model constraints dùng chung planner và atomic commit.

### Khi nào được gọi

`prepareJob` gọi `catalog.Resolve(profile, fp8)` sau validation; `CapabilityMatches` được gọi khi filtering và commit.

### Đầu vào

`AllocationResources.PerformanceProfile`, `FP8Required`; `Catalog.profiles[].Models{Name,FP8}`; lúc match: `ResourceRequest`, `GPU.Model`.

### Đầu ra

`ResourceRequest.ResolvedModels` sort theo tên, lưu private cùng Job; unknown profile trả ErrInvalidInput. Profile có thật nhưng không model FP8 trả danh sách rỗng, không lỗi.

### Hard constraints

New request bắt buộc profile được biết và FP8 boolean. Với profile/FP8 constraints, model inventory phải khớp một alias; không substring match hoặc wildcard cho unknown model.

### Công thức / comparator

`Resolve(p,f) = sort([trim(m.Name) | m thuộc profile p và (không f hoặc m.FP8)])`.

| Profile | Model aliases trong source |
|---|---|
| `general` | Tất cả aliases T4 + A100 + H100 bên dưới |
| `a100-equivalent` | `A100`, `NVIDIA-A100-80GB`, `NVIDIA A100-SXM4-40GB`, `NVIDIA A100-SXM4-80GB`, `NVIDIA A100 80GB PCIe` |
| `h100-equivalent` | `H100`, `NVIDIA H100 80GB HBM3`, `NVIDIA H100 PCIe` |
| T4 aliases của general | `T4`, `Tesla T4`, `NVIDIA T4` |

Catalog gắn FP8=true cho H100 aliases, false cho A100/T4. Đây là metadata trong code, không suy từ NVML hoặc lý thuyết bên ngoài. Không có H200 default.

Khi `PerformanceProfile != "" OR FP8Required`: `EqualFold(trim(GPU.Model), resolvedAlias)`. Chỉ nhánh legacy không profile và không FP8 mới dùng `GPUModel=="" OR EqualFold(GPU.Model,GPUModel)`; nhánh legacy không trim model. ResolvedModels rỗng ở nhánh mới không fallback sang GPUModel.

### Pseudocode

`find exact profile → filter aliases theo FP8 flag → sort → persist; lúc match chọn nhánh profile hoặc legacy, không trộn hai nhánh`.

### Ví dụ

`a100-equivalent + fp8Required=true` hợp lệ về profile nhưng resolve rỗng và không placement; `general + true` chỉ match H100 aliases. Model `H100-custom` không tự match.

### Complexity

Resolve: O(P + R_p log R_p) với P profiles và R_p aliases profile; CapabilityMatches: O(1+R). Tính cả scan model trước sort không làm tăng cận.

### Source mapping

[internal/capability/catalog.go][catalog] — `Default`, `Resolve`; [domain/allocation.go][allocation-model] — `CapabilityMatches`; [application/allocation.go][allocation-app] — `prepareJob`.

## A03 — Sizing-plan và quota facts

### Mục tiêu

Cung cấp planning/quota facts để Evaluate chọn lane và quota coefficient.

### Khi nào được gọi

`Engine.Evaluate` → `FactsProvider.Facts`; `cmd/aiwm-server` inject `DevelopmentFacts`.

### Đầu vào

`Job.Resources.GPUCount`, `DevelopmentFacts.InSizingPlan`, `QuotaGPUs`, `UsedGPUs`; context được interface nhận nhưng DevelopmentFacts không dùng.

### Đầu ra

`Facts{InSizingPlan, WithinQuota, QuotaUsage, Source="DEVELOPMENT_CONFIG"}`.

### Hard constraints

Config quota/used đều là int không âm; không validate UsedGPUs ≤ QuotaGPUs. Không có quota lock/ledger theo user/project.

### Công thức / comparator

\(WithinQuota = (Job.Resources.GPUCount \le QuotaGPUs - UsedGPUs)\).

`InSizingPlan` copy cấu hình; `QuotaUsage=UsedGPUs`. Defaults: `AIWM_DEV_IN_SIZING_PLAN=false`, `AIWM_DEV_QUOTA_GPUS=4`, `AIWM_DEV_QUOTA_USED_GPUS=0`. Values dùng chung, không tăng/giảm theo assignment/release.

### Pseudocode

`read configured facts → so GPUCount với quota còn lại → return Facts`.

### Ví dụ

`quota=4, used=1`: request 3 GPU within=true; request 4 GPU within=false. Việc Job 3 GPU đã được assign không tự đổi used từ 1 thành 4.

### Complexity

O(1) cho DevelopmentFacts; FactsProvider được thay có chi phí riêng.

### Source mapping

[internal/policy/engine.go][policy] — `FactsProvider`, `DevelopmentFacts.Facts`; [internal/config/config.go][config] — `Load`; [cmd/aiwm-server/main.go][main] — `main`.

## A04 — System importance coefficient

### Mục tiêu

Backend derive hệ số từ business category, không nhận coefficient trực tiếp từ user.

### Khi nào được gọi

`policy.Validate` xác thực category; `Engine.Evaluate` tính priority.

### Đầu vào

`Job.SystemImportance`.

### Đầu ra

`ImportanceCoefficient` trả `(coefficient, valid)`; Evaluate dùng coefficient cho auxiliary product.

### Hard constraints

New request category không hợp lệ bị validation chặn; không nhận số thực thay category.

### Công thức / comparator

| SystemImportance | Coefficient |
|---|---|
| `CRITICAL_SPECIAL` | 1.4 |
| `VERY_IMPORTANT` | 1.2 |
| `IMPORTANT` | 1.0 |
| `NORMAL` | 0.8 |

Helper trả `(0,false)` nếu không biết; `Evaluate` fallback 0.8 để xử lý Job legacy thiếu intent. Không sửa category đã lưu thành NORMAL.

### Pseudocode

`switch category → hệ số; nếu Evaluate gặp invalid category của stored Job → dùng 0.8`.

### Ví dụ

`VERY_IMPORTANT` cho 1.2; Job legacy category rỗng dùng 0.8 khi Evaluate, nhưng request mới rỗng bị từ chối.

### Complexity

O(1).

### Source mapping

[internal/policy/engine.go][policy] — `ImportanceCoefficient`, `Validate`, `Engine.Evaluate`; [domain/allocation.go][allocation-model] — `SystemImportance`.

## A05 — Waiting factor / bounded aging

### Mục tiêu

Tăng auxiliary priority cho Job chờ lâu, có cap.

### Khi nào được gọi

Mỗi `Engine.Evaluate`, gồm preview, submit, GET Queue và scheduling cycle.

### Đầu vào

`Job.CreatedAt`, `now` do CP truyền vào; không dùng NeededAt, UpdatedAt hoặc LastObservedAt.

### Đầu ra

`WaitingCoefficient` trong [1.0, 1.4].

### Hard constraints

Aging không đổi lane/Necessity và không trigger preemption.

### Công thức / comparator

\[
w=\min(4,\max(0,\operatorname{trunc}((now-CreatedAt)/(7\times24h))))
\qquad K_{waiting}=1+w/10
\]

Go chia duration rồi chuyển int, truncate về 0; clamp âm về 0. Các mốc 0/7/14/21/28 ngày tương ứng 1.0/1.1/1.2/1.3/1.4; sau 28 ngày giữ 1.4.

### Pseudocode

`weeks = số tuần đủ theo duration; clamp weeks vào 0..4; return 1 + weeks/10`.

### Ví dụ

`6 ngày 23 giờ → 1.0`; `7 ngày → 1.1`; `35 ngày → 1.4`; CreatedAt ở tương lai → 1.0. Preview khởi tạo CreatedAt=now nên factor=1.0.

### Complexity

O(1).

### Source mapping

[internal/policy/engine.go][policy] — `WaitingCoefficient`, `Engine.Evaluate`; [application/allocation.go][allocation-app] — `prepareJob`.

## A06 — Admission lane và auxiliary priority product

### Mục tiêu

Đánh giá lane và score phụ; không tự chọn/giữ GPU.

### Khi nào được gọi

`prepareJob` cho preview/submit; `Engine.Order` evaluate lại mỗi queued Job trước sort.

### Đầu vào

`Job`, `FactsProvider`, `now`; dùng A03–A05.

### Đầu ra

`PolicyDecision`: Status, Reason, InSizingPlan, WithinQuota, QuotaUsage, Source, EvaluatedAt UTC, AuxiliaryScore.

### Hard constraints

FactsProvider lỗi thì Evaluate trả lỗi; Order dừng toàn bộ khi một Evaluate lỗi. Cả hai lane hợp lệ đều có thể scheduling.

### Công thức / comparator

\[
K_{quota}=\begin{cases}1.0&WithinQuota\\0.8&\text{ngược lại}\end{cases}
\quad
Aux=\frac{\operatorname{Round}(1000K_{importance}K_{quota}K_{waiting})}{1000}
\]

`AUTO_ELIGIBLE` iff `InSizingPlan AND WithinQuota`, ngược lại `COMPETITIVE`. Sizing plan chỉ quyết định lane, không thêm hệ số. Round đúng 0.001 giúp hai tích toán học bằng nhau không mất FIFO do floating-point. Với factors hiện tại, Aux từ 0.64 đến 1.96.

### Pseudocode

`facts → importance/quota/waiting → rounded product → lane bằng AND(plan,quota) → PolicyDecision`.

### Ví dụ

`plan=false, within=true, VERY_IMPORTANT, chờ 14 ngày`: lane COMPETITIVE, Aux=1.2×1×1.2=1.44. `plan=true, within=true, NORMAL, mới tạo`: AUTO_ELIGIBLE, Aux=0.8.

### Complexity

O(1) ngoài FactsProvider; default provider O(1).

### Source mapping

[internal/policy/engine.go][policy] — `Engine.Evaluate`; [domain/allocation.go][allocation-model] — `PolicyDecision/PolicyStatus`.

## A07 — Necessity và lexicographic queue comparison

### Mục tiêu

Đảm bảo score phụ chỉ phá hòa sau lane và Necessity.

### Khi nào được gọi

`Engine.Order` evaluate toàn bộ Jobs rồi `sort.SliceStable` với `Before`.

### Đầu vào

Hai `Job` đã Evaluate: Policy.Status, NecessityLevel, AuxiliaryScore, CreatedAt, ID.

### Đầu ra

Quan hệ `Before(A,B)` và queued Job slice được sắp thứ tự.

### Hard constraints

Không cộng Necessity và AuxiliaryScore thành weighted sum; Job.Priority không được đọc trong comparator.

### Công thức / comparator

Thứ tự: `AUTO_ELIGIBLE` trước `COMPETITIVE` → Necessity rank tăng → Aux giảm → CreatedAt tăng → ID tăng.

`necessityRank(NECESSITY_1..4)=1..4`; missing/invalid legacy rank=5. Tương đương key cho assessments hợp lệ: `(laneRank, necessityRank, -AuxiliaryScore, CreatedAt, ID)`; laneRank chỉ là notation tài liệu, không field mới.

### Pseudocode

`so lane; nếu hòa so Necessity; nếu hòa so Aux; nếu hòa chọn CreatedAt sớm hơn; cuối cùng ID nhỏ hơn`.

### Ví dụ

Trong cùng AUTO_ELIGIBLE: A=N1, Aux=0.8; B=N2, Aux=1.96 → A trước. Nhưng Auto N4 vẫn trước Competitive N1 vì lane xét trước. Hai Jobs cùng lane/Necessity có tích `1.4×1×1.2` và `1.2×1×1.4` đều 1.68, nên dùng CreatedAt rồi ID.

### Complexity

Before O(1) theo quy ước key; Order có Q lần Evaluate và copy O(Q). Complexity tổng của sort.SliceStable: Not explicitly characterized from current implementation; audit không mở implementation thư viện chuẩn.

### Source mapping

[internal/policy/engine.go][policy] — `necessityRank`, `Before`, `Engine.Order`; [application/controlplane.go][cp] — `Queue`.

## A08 — Queue, replanning và retry

ACTIVE. Queue public chỉ trả QUEUED theo policy.Order. ScheduleOnce lấy snapshot nhất quán qua ReplanReservations; planner xét cả QUEUED và PLANNED chưa đến StartAt. Không vừa thì vẫn QUEUED, thử Job kế tiếp. Lịch tương lai priority thấp có thể mất reservation khi request cao hơn tới; lịch đã bắt đầu không bị thay. Ticker và API run-once dùng cùng flow. CreatedAt/ID vẫn là tie-break; waiting không bảo đảm hết starvation. Xem phần T cho input/output/pseudocode/source và ví dụ.


## A09 — Server readiness, freshness và drain

### Mục tiêu

Loại host không thể nhận placement mới dù còn last-known GPU FREE.

### Khi nào được gọi

`Scheduler.filter`, `matchJob` và `CommitAssignment`; `ControlPlane.Reconcile` chỉ đánh dấu stale server, không chạy placement.

### Đầu vào

`Server.Status`, `Drained`, `LastHeartbeatAt`, `InventoryReceivedAt`, `now`, `OfflineAfter`.

### Đầu ra

`Server.Schedulable` boolean; filter reason tổng quát khi không host nào qua; commit không qua trả ErrConflict.

### Hard constraints

Method kiểm theo thứ tự: Status ONLINE → !Drained → inventory receipt khác zero → heartbeat age ≤ timeout → inventory receipt age ≤ timeout. Không dùng field SchedulingReady hay LastInventoryAt để quyết định.

### Công thức / comparator

\[
ready(s)=ONLINE(s)\land\neg Drained(s)\land receipt(s)\ne0
\land(now-LastHeartbeatAt\le T)\land(now-InventoryReceivedAt\le T)
\]

`T=OfflineAfter` default 20s. CP gán receipt bằng clock của nó; inventory thành công cũng cập nhật LastHeartbeatAt. `MarkStaleServers` đặt OFFLINE khi LastHeartbeatAt trước now−T; stale inventory riêng không nhất thiết đổi enum. CP restart reset receipt, set OFFLINE; re-enroll cùng MachineID reset receipt/version. Drain lúc offline giữ enum OFFLINE.

### Pseudocode

`if any readiness condition fails: exclude server; otherwise continue constraints; before commit apply same readiness check again`.

### Ví dụ

Heartbeat 1s trước nhưng inventory 21s trước, timeout20s: không schedulable dù ONLINE. Inventory age đúng20s vẫn qua `<=`, nếu các điều kiện khác hợp lệ. DRAINING không eviction Jobs đang chạy.

### Complexity

Schedulable O(1); MarkStaleServers duyệt S server cộng chi phí clone; durable mutation thêm snapshot cost A22.

### Source mapping

[domain/model.go][model] — `Server.Schedulable`; [memory/store.go][store] — `MarkStaleServers`, `ReplaceInventory`, `UpsertServer`, `SetServerDrained`; [ControlPlane][cp] — `Heartbeat`, `ReportInventory`, `Reconcile`; [durable.Open][durable].

## A10 — Agent health, grant correlation và unknown telemetry

### Mục tiêu

Cung cấp occupancy/health bảo thủ cho CP; không báo GPU rỗng chỉ vì đọc inventory lỗi.

### Khi nào được gọi

`InventoryCollector.Snapshot` đọc DockerRuntime/GPUReader trực tiếp; kết quả được ReportInventory đưa vào accounting.

### Đầu vào

`RuntimeContainer` grants/labels/state; NVML GPU/process records; `ProcessContainerResolver`; `InventoryPolicy`; positive sequence.

### Đầu ra

`InventoryReport` với Container origin/GPUUUIDs, processes, Healthy và optional OCCUPIED_UNKNOWN; error nếu snapshot dependency lỗi.

### Hard constraints

Active state là running/paused/restarting, kể cả idle. Managed iff label aiwm.managed EqualFold true và aiwm.job-id không trắng; còn lại LEGACY. Không adopt/stop External.

### Công thức / comparator

Index map sang UUID từ snapshot; grant không resolve được hoặc UnboundedGPUAccess ở active container mở rộng thành toàn bộ UUID host. Process owner match thêm UUID vào container. Không match PID owner vẫn giữ process để CP xử lý unknown.

\[
unknownTelemetry(g)=\neg attributedToActiveContainer(g)\land
(MemoryUsedMiB\ge U_m\ \lor\ UtilizationPct\ge U_u)
\]

`U_m=UnknownMemoryMiB`, env `AIWM_UNKNOWN_MEMORY_MIB` default256, config phải >0; `U_u=UnknownUtilizationPct`, env `AIWM_UNKNOWN_UTILIZATION_PCT` default5, config từ chối parse lỗi, giá trị ≤0 hoặc >100. Guard hiện không kiểm `NaN` riêng. Threshold là >=, không phải >. Constructor không có explicit policy cũng default256/5.

NVML bắt đầu Healthy=true; utilization read lỗi, MIG đang enabled khi đọc mode thành công, uncorrected volatile ECC>0 hoặc ECC read lỗi khác NOT_SUPPORTED làm Healthy=false. Device/UUID/model/memory/compute-process read lỗi làm Snapshot lỗi; graphics NOT_SUPPORTED được chấp nhận. Temperature không có ngưỡng unhealthy trong code; MIG read lỗi không tự chuyển Healthy=false.

### Pseudocode

`list Docker; read NVML; resolve grants/PIDs; classify labels; nếu không active owner và vượt một threshold: mark OCCUPIED_UNKNOWN; return report`.

### Ví dụ

GPU không container owner, memoryUsed=256MiB và utilization=0 → UNKNOWN. Legacy active grant với memoryUsed=0/utilization=0 vẫn bị CP chặn; không cần vượt threshold. Process owner không biết cũng bị CP chặn dù usage thấp.

### Complexity

Not explicitly characterized from current implementation: có I/O, resolver, nhiều vòng grant×GPU/consumer và dedup/sort; không coi đây đơn thuần O(G).

### Source mapping

[internal/agent/inventory.go][inventory] — `Snapshot`, `classifyContainer`, `matchContainerID`; [agent/config/config.go][agent-config] — `Load`; [gpu/nvml_linux.go][nvml] — `Reader.Snapshot`; [application/agent_protocol.go][protocol-map] — `inventoryToDomain`.

## A11 — Normalized GPU states / Existing-Legacy-Unknown accounting

### Mục tiêu

Hợp nhất actual consumers và reservation để scheduler không cấp GPU đang được giữ/chiếm.

### Khi nào được gọi

`ReplaceInventory` sau reconcile Jobs; `releaseLocked` khi logical reservation kết thúc.

### Đầu vào

`serverID`, `[]GPU`, `[]Container`, `[]GPUProcess`, `map[JobID]Job`.

### Đầu ra

GPU slice với State, AssignedJobID, StateReason, ObservedConsumers được tính lại.

### Hard constraints

Chỉ active containers tiêu thụ grant. Managed allocation được công nhận khi Origin MANAGED, Job tồn tại, Assignment cùng server và có UUID; không yêu cầu Job còn nonterminal trong nhánh actual consumer.

### Công thức / comparator

Áp dụng nhánh đầu tiên đúng:

| Thứ tự | Điều kiện | State / accounting |
|---|---|---|
| 1 | !Healthy | UNHEALTHY |
| 2 | Active grant không khớp managed allocation hợp lệ | OCCUPIED_LEGACY, ObservedConsumers chứa ContainerIDs |
| 3 | Process không thuộc active container/grant, hoặc khác managed Jobs cùng dùng UUID | OCCUPIED_UNKNOWN, consumers chứa pid:<PID> hoặc multiple managed consumers |
| 4 | Managed active allocation khớp | ALLOCATED + AssignedJobID |
| 5 | GPU đầu vào đã OCCUPIED_UNKNOWN | OCCUPIED_UNKNOWN, không yêu cầu có consumers list |
| 6 | Assignment của nonterminal Job giữ UUID | RESERVED + AssignedJobID |
| 7 | Không blocker | FREE |

`ContainerLegacy` và `ContainerUnknown` đều không qua điều kiện MANAGED; active grant của chúng đi OCCUPIED_LEGACY. Vì vậy Container origin UNKNOWN không mặc định tương đương GPUOccupiedUnknown. Unknown GPU state có nghĩa chưa xác định owner/usage an toàn.

CP wire mapping chỉ giữ GPUState OCCUPIED_UNKNOWN từ Agent (state khác khởi tạo FREE), rồi normalize; không nhận ALLOCATED/RESERVED do Agent tự tuyên bố. UUID trùng trong report bị mapping từ chối, trùng giữa hai servers bị ReplaceInventory từ chối.

### Pseudocode

`build reserved map từ nonterminal assignments; classify active grants; find unattributed processes; mỗi GPU áp precedence, reset display/owner fields trước khi gán`.

### Ví dụ

GPU vừa có external grant vừa có reservation → OCCUPIED_LEGACY, không RESERVED. Terminal Job còn active managed container khớp Assignment → GPU vẫn ALLOCATED dù Assignment RELEASED.

### Complexity

Not explicitly characterized from current implementation: quét toàn Jobs, grants và processes với containsUUID tuyến tính; không chỉ duyệt GPU một lần. Cross-server UUID check riêng trong ReplaceInventory: O(G_report × tổng GPU của các server khác).

### Source mapping

[internal/store/memory/accounting.go][accounting] — `normalizeGPUState`, `containsUUID`, `containerConsumesGPU`; [lifecycle.go][lifecycle] — `releaseLocked`; [store.go][store] — `ReplaceInventory`; [agent_protocol.go][protocol-map] — `inventoryToDomain`.

## A12 — GPU.Schedulable, capability và available VRAM predicate

### Mục tiêu

Kiểm tra một GPU có thể thỏa ResourceRequest trước candidate và tại commit.

### Khi nào được gọi

`matchingGPUs` và `CommitAssignment` gọi `ResourceRequest.Matches`; diagnostic pass còn gọi CapabilityMatches/Healthy/Schedulable riêng.

### Đầu vào

`GPU{Healthy,State,AssignedJobID,Model,MemoryTotalMiB,MemoryUsedMiB}`, `ResourceRequest`.

### Đầu ra

Matches boolean.

### Hard constraints

`GPU.Schedulable()` chưa kiểm server freshness, model/FP8, VRAM hoặc count. Phải kết hợp với server filter và ResourceRequest.

### Công thức / comparator

\[
GPUReady=Healthy\land(State=FREE)\land(AssignedJobID="")
\]
\[
AvailableMemoryMiB=\max(MemoryTotalMiB-MemoryUsedMiB,0)
\]
\[
Matches=GPUReady\land CapabilityMatches\land(AvailableMemoryMiB\ge MinVRAMMiB)
\]

| State thực tế | Tác động scheduling |
|---|---|
| FREE | Chỉ qua nếu Healthy và AssignedJobID rỗng; vẫn cần model/VRAM/server/count checks |
| RESERVED | Loại; đang giữ logical reservation |
| ALLOCATED | Loại; actual managed consumer được công nhận |
| OCCUPIED_LEGACY | Loại; active external/unrecognized managed grant |
| OCCUPIED_UNKNOWN | Loại; usage/owner chưa an toàn |
| UNHEALTHY | Loại; không có override bởi strategy |

Không có GPU state AVAILABLE/ONLINE. Available VRAM không cấp quyền sharing. CPUMilli/MemoryMiB không có mặt trong predicate; chỉ là limits gửi Agent.

### Pseudocode

`if !GPU.Schedulable: false; if !CapabilityMatches: false; return availableVRAM >= request.minVRAM`.

### Ví dụ

GPU FREE, Healthy=true, AssignedJobID rỗng, total=40960, used=1024: available=39936; request40000 không qua. GPU có VRAM dư nhưng RESERVED vẫn không qua.

### Complexity

GPUReady/VRAM O(1); Matches O(1+R) ở nhánh resolved aliases, legacy model compare O(1) theo quy ước string.

### Source mapping

[internal/domain/model.go][model] — `GPU.Schedulable`, `AvailableMemoryMiB`; [domain/allocation.go][allocation-model] — `ResourceRequest.Matches/CapabilityMatches`.

## A13 — Resource filter pipeline và candidate construction

### Mục tiêu

Tạo candidate gồm một server và tập GPU phù hợp, không ghép host.

### Khi nào được gọi

`Scheduler.Plan → filter`; helper `matchingGPUs` còn dùng trong matchJob.

### Đầu vào

`Job.Resources`, legacy `Job.ServerSelector`, `[]Server`, `now`, OfflineAfter.

### Đầu ra

`[]candidate{server,gpus,score}`; score chỉ tính sau filtering. Mỗi candidate có ít nhất GPUCount GPU.

### Hard constraints

Hai lượt GPU trong `filter` phải phân biệt:

| Bước theo source | Input / condition | Khi không qua | Source |
|---|---|---|---|
| 1. Server readiness | Server.Schedulable(now, timeout), A09 | Skip server; no-online reason nếu không host nào qua | `filter`, `Server.Schedulable` |
| 2. Legacy selector | Mọi requested key/value bằng server.Labels[key] | Skip server; no-labels reason | `labelsMatch` |
| 3a. Diagnostic capability | ResourceRequest.CapabilityMatches(gpu) | Skip GPU trong lượt flags | `filter` |
| 3b. Diagnostic health | gpu.Healthy | Skip GPU trong lượt flags | `filter` |
| 3c. Diagnostic availability | Ghi external/unknown flags, rồi gpu.Schedulable | Không đánh dấu available nếu fail | `filter` |
| 3d. Diagnostic VRAM | AvailableMemoryMiB ≥ MinVRAMMiB | Không đánh dấu vram nếu fail | `filter` |
| 4. Tập GPU thực tế | Duyệt lại GPUs, `Matches` theo thứ tự GPUReady → capability → VRAM | Loại GPU khỏi matching | `matchingGPUs` |
| 5. GPU order | AvailableMemoryMiB ↑ rồi UUID ↑ | Không có random/index preference | `matchingGPUs` |
| 6. Count | len(matching) ≥ GPUCount | Không thêm candidate, count reason nếu flags đủ | `filter` |

Selector rỗng match mọi labels. Field này vẫn hoạt động với stored legacy Job; Create DTO mới không nhận selector. Map lookup key không tồn tại trả "", nên requested value="" cũng khớp missing key trong helper hiện tại.

Không có check selected GPUs cùng model, topology hoặc NVLink; từng GPU chỉ cần qua cùng ResourceRequest. Profile general có thể cho phép mixed model trên một host.

### Công thức / comparator

Không weighted score ở filtering. Candidate chỉ tồn tại khi đủ GPU match **trên cùng server**; Plan lấy first GPUCount phần tử của matching đã sort.

### Pseudocode

`for server: readiness + labels; diagnostic GPU pass; matching = sort(GPUs where Matches); if len(matching)>=GPUCount: append candidate`.

### Ví dụ

Request 2 GPU: server A có 1 phù hợp, B có 1 phù hợp → không candidate dù tổng pool là2. Trong host có GPU free cùng VRAM, UUID nhỏ hơn đứng trước, không dùng NVML Index.

### Complexity

O(SL + Σ(G_i(1+R) + M_i log M_i)) cho filtering/sort theo quy ước; diagnostic và matching đều quét GPU. Không tính repository snapshot cloning.

### Source mapping

[internal/application/scheduler.go][scheduler] — `filter`, `labelsMatch`, `matchingGPUs`, `candidate`; [domain/allocation.go][allocation-model] — `Matches`.

## A14 — Reason khi không đủ tài nguyên

ACTIVE. Scheduler.filter vẫn sinh diagnostic ErrInsufficientGPU từ resource flags. Planning chuyển lỗi thiếu tài nguyên thành reason business chung “Không đủ GPU an toàn trong khoảng thời gian yêu cầu”, bao gồm conflict thời gian, capability và server chưa sẵn sàng. Preview trả cùng reason. Không suy diễn rằng mọi conflict chỉ do calendar. Source: application/planning.go, allocation.go; diagnostic strategy vẫn ở scheduler.go.


## A15 — First Fit

### Mục tiêu

Chọn một placement đủ điều kiện với score bằng nhau; source dùng stable ID để quyết định host.

### Khi nào được gọi

`Plan` khi configured strategy là `StrategyFirstFit` / `first-fit`.

### Đầu vào

`Server` và matching/selected GPUs đã filter; scorer bỏ qua các giá trị này.

### Đầu ra

Score=0 cho mỗi candidate, sau đó Placement từ A19.

### Hard constraints

Không bypass hard filters. Không có dừng ngay ở server đầu tiên đủ điều kiện.

### Công thức / comparator

\[
score_{first-fit}(s)=0
\]

Repository ListServers trả thứ tự Name tăng (tie Name chưa có tie phụ). Filter duyệt slice này, nhưng Plan score **tất cả** candidates rồi select theo score/Server.ID. Do mọi score=0, chọn Server.ID nhỏ nhất trong candidates. GPU trong host theo available VRAM rồi UUID, lấy đủ K.

### Pseudocode

`build tất cả candidates → gán score0 → sort score/ID → lấy candidate đầu; không early-return trong vòng server`.

### Ví dụ

`srv-z` tên Alpha được duyệt trước `srv-a` tên Zeta; cả hai đủ GPU → first-fit chọn `srv-a`. Đây không phải first feasible theo Name.

### Complexity

Scorer O(1)/candidate; Plan vẫn chịu filtering + O(C log C) selection như A19.

### Source mapping

[internal/application/scheduler.go][scheduler] — scorer trong `NewScheduler`, `Plan`, `selectCandidate`, `matchingGPUs`; [memory/store.go][store] — `ListServers`.

## A16 — Best Fit

### Mục tiêu

Giảm số GPU phù hợp còn dư trên host được chọn.

### Khi nào được gọi

`Plan` khi strategy `best-fit`; đây là source default.

### Đầu vào

`matching []GPU`, `selected []GPU` đã chọn K; scorer không dùng server.

### Đầu ra

Score M−K; candidate score thấp nhất thắng.

### Hard constraints

M là số GPU match toàn bộ request, không phải tổng GPU hoặc tổng GPU FREE. VRAM đã là filter, không phải leftover VRAM score.

### Công thức / comparator

\[
score_{best-fit}=M-K,\qquad M=len(matching),\quad K=len(selected)
\]
Hòa dùng Server.ID, không so leftover VRAM giữa hosts.

### Pseudocode

`score = số matching GPUs - số selected GPUs; chọn score nhỏ nhất`.

### Ví dụ

Request K=2: A có M=2 và B có M=3 → scores0/1, chọn A; dù A còn các GPU free khác không match profile thì chúng không làm tăng score.

### Complexity

O(1)/candidate; cộng filtering và selection A19 cho toàn Plan.

### Source mapping

[internal/application/scheduler.go][scheduler] — `NewScheduler` nhánh `StrategyBestFit`, `Plan`.

## A17 — Bin Packing

### Mục tiêu

Ưu tiên packing bằng residual free GPU count kết hợp số GPU host không schedulable.

### Khi nào được gọi

`Plan` khi strategy `bin-pack`.

### Đầu vào

`Server.GPUs`, selected K; `matching` không được scorer dùng.

### Đầu ra

Score thấp hơn được chọn.

### Hard constraints

F đếm mọi GPU.Schedulable của host, kể cả không match profile/VRAM request. N−F có thể gồm External/Unknown/UNHEALTHY/reserved/allocated; những GPU này vẫn bị hard filter loại khỏi selected.

### Công thức / comparator

\[
F=\#\{g\in server.GPUs: g.Schedulable()\},\quad N=len(server.GPUs)
\]
\[
score_{bin-pack}=10(F-K)-(N-F)
\]

Đây là biểu thức weighted count đúng source; không đo VRAM, không dùng auxiliary business score, không tối ưu toàn pool. Hệ số10 không biến nó thành lexicographic comparator cho mọi kích thước host.

### Pseudocode

`free = count GPU.Schedulable; score = 10*(free-K) - (N-free); select lower score`.

### Ví dụ

K=1: A có N=4,F=2 → score8; B có N=4,F=3 → score19; chọn A nếu cả hai có matching GPU. Score có thể âm khi host ít free và nhiều GPU không schedulable.

### Complexity

O(G_i)/candidate do freeGPUCount; không chỉ O(1).

### Source mapping

[internal/application/scheduler.go][scheduler] — scorer `StrategyBinPack` trong `NewScheduler`, `freeGPUCount`.

## A18 — Fragmentation-aware

### Mục tiêu

Tránh mở host hoàn toàn trống; trong nhóm còn lại ưu tiên không để lại free GPU dở dang.

### Khi nào được gọi

`Plan` khi strategy `fragmentation-aware`.

### Đầu vào

`Server.GPUs`, selected K; F/N như A17; matching slice không dùng trong scorer.

### Đầu ra

Heuristic score thấp hơn thắng; Server.ID phá hòa.

### Hard constraints

Chỉ score candidates đủ hard constraints, nên F≥K và N≥1. “Empty” ở đây nghĩa F=N theo GPU.Schedulable, không phải không có Docker containers.

### Công thức / comparator

\[
r=F-K
\]
\[
score_{frag}=2\mathbf{1}_{F=N}+\mathbf{1}_{r>0}+\frac{r}{N+1}
\]

Penalty2 nếu mở host toàn bộ GPU schedulable; penalty1 nếu sau cấp vẫn còn free GPU; phần dư normalized để so trong cùng nhóm. Với candidate hợp lệ, \(0\le r/(N+1)<1\); vì vậy penalty groups giữ ưu tiên trên. Không có stored fragmentation state hoặc đo topology/VRAM fragmentation.

### Pseudocode

`free=count schedulable; remaining=free-K; score=emptyPenalty + partialPenalty + remaining/(N+1)`.

### Ví dụ

K=2: A N=4,F=2 →0; B N=4,F=3 →1.2; C N=4,F=4 →3.4. Chọn A để lấp phần free của host đang bận; giữ host C nguyên trống. GPU External làm F<N nhưng không được chọn.

### Complexity

O(G_i)/candidate do freeGPUCount; phần số học O(1).

### Source mapping

[internal/application/scheduler.go][scheduler] — `fragmentationScore`, `freeGPUCount`, registry `StrategyFragmentation`.

## A19 — Default strategy, scorer dispatch và deterministic selection

### Mục tiêu

Chọn đúng scorer cấu hình và kết quả cuối deterministic trên cùng Job/snapshot/time.

### Khi nào được gọi

`config.Load → main → application.New → NewScheduler`; mọi `Scheduler.Plan`.

### Đầu vào

`defaultStrategy`, registry `map[SchedulingStrategy]SchedulingPolicy`, Job, candidates.

### Đầu ra

`Placement{ServerID,GPUUUIDs,Strategy,Score,Reason}` hoặc error; Plan không mutate resources.

### Hard constraints

Config chỉ chấp nhận bốn enum qua ValidStrategy. Plan không có registered strategy hoặc GPUCount≤0 trả ErrInvalidInput; không fallback im lặng. application.New chỉ fallback best-fit khi DefaultStrategy rỗng.

### Công thức / comparator

`AIWM_SCHEDULER_STRATEGY` default `best-fit`. Plan dùng `s.defaultStrategy`, **không dùng Job.Strategy để dispatch**. GPU subset cho mọi strategy là K GPU đầu từ matching đã sort; scorer không chọn lại subset.

Final key: `(candidate.score ↑, candidate.server.ID ↑)`. UUID output theo subset đã sort (available VRAM ↑, UUID ↑). Reason chứa strategy, server name, score format4 chữ số thập phân và GPU count; không làm tròn stored Score thành4 chữ số.

Create DTO không nhận strategy; public JobView vẫn expose Job.Strategy và Assignment.Strategy/Score/Reason. Job.Strategy ghi lúc submit có thể khác configured strategy sau restart; Assignment.Strategy ghi lúc commit. `WithPolicy`/`Options.PlacementPolicy` cho thay scorer tại composition; main hiện không inject replacement.

### Pseudocode

`policy=registry[defaultStrategy]; filter; với mọi candidate lấy K GPUs, score; sort score/Server.ID; convert winner sang Placement`.

### Ví dụ

hai candidates score0.0 có IDs srv-b/srv-a → srv-a. Thay env thành bin-pack làm Plan dispatch bin-pack kể cả stored Job.Strategy vẫn best-fit.

### Complexity

Plan: filtering A13 + Σ scorer cost + O(C log C + K) selection/output. First/best scorer O(C); bin-pack/fragmentation cộng ΣG_i của candidates. Không tính repository đọc/copy.

### Source mapping

[internal/application/scheduler.go][scheduler] — `NewScheduler`, `WithPolicy`, `Plan`, `selectCandidate`, `SchedulingPolicy`; [config][config] — `Load`; [main][main]; [ControlPlane.New][cp]; [httpapi/job_view.go][job-view].

## A20 — Preview và submit theo thời gian

ACTIVE. prepareJob validate RFC3339 (giữ fractional seconds), tolerance quá khứ 60 giây, duration >0 trong max config và interval chưa hết. matchJob thêm hypothetical Job vào snapshot rồi gọi cùng planReservations; không ghi Job/Assignment/Command/GPU. Response có requestedStartAt/requestedEndAt, planningStatus AVAILABLE/CONFLICT; matchedGpuCount là số GPU trong phương án hypothetical thành công (0 khi không đủ), recommendedGpuModels là model trong phương án đó. Preview không bảo đảm submit thành công vì inventory/queue có thể đổi. Submit evaluate latest rồi lưu QUEUED; ticker tạo reservation. Không có preview token, K/auxiliary/scorer trong response preview. Nguồn: application/allocation.go và planning.go; tests TestTimeAwarePreviewIsReadOnlyAndPolicyAware.


## A21 — Hai transaction: calendar reservation và activation

ACTIVE. ReplanReservations giữ mutex memory, chạy pure planning callback trên snapshot và revalidate toàn batch trước mutation. Không commit một phần khi một plan conflict; không tạo START. Chỉ sửa QUEUED hoặc PLANNED có StartAt > now và chưa có CommandID.

Tại start <= now < end, CommitAssignment kiểm tra Assignment PLANNED đúng server/UUID, Job ASSIGNED, org đúng, fresh inventory/heartbeat, không drain, GPU Healthy + FREE + chưa AssignedJobID, count/VRAM/profile/FP8/labels. Thành công atomically ghi Assignment RESERVED, Job STARTING, GPU RESERVED và START command. Nếu fail giữ lịch, ghi reason chờ, thử lại trong interval; không double allocation.

Legacy internal CommitAssignment cho Job không có time fields vẫn tồn tại để tương thích code cũ; runtime ScheduleOnce không dùng đường này cho Job mới. Các Job cũ thiếu interval không tự có lịch mới. Nguồn: memory/planning.go, memory/store.go; ports.Repository; application/planning.go. Atomicity chỉ trong một runtime process; durable bọc snapshot transaction.


## A22 — Durable reservation transaction / rollback

### Mục tiêu

Giữ cùng atomic mutation khi ghi local persistence lỗi; chặn writer cùng file.

### Khi nào được gọi

`cmd/aiwm-server` dùng durable.Open; durable.Repository methods wrap memory mutations bằng transact.

### Đầu vào

Memory snapshot version1, mutation callback, configured StateFile, local filesystem.

### Đầu ra

Commit trả success sau ghi snapshot hoặc rollback memory và trả lỗi; sau restore server phải có inventory mới để placement.

### Hard constraints

`durable.Store.mu` serialize cả reads/writes của wrapper; OS lock từ Open bảo vệ một state file. Đây là một CP writer, không SQL/distributed transaction.

### Công thức / comparator

Trình tự: Export before → fn() → Export after → encode gob vào temp cùng directory → file.Sync → close → Rename. fn hoặc save lỗi thì Restore(before). Restore lúc startup set Server OFFLINE, InventoryReceivedAt zero; giữ Job/Assignment/Command.

### Pseudocode

`lock wrapper; before=Export; run mutation; nếu lỗi restore; nếu save(after) lỗi restore; nếu thành công return result`.

### Ví dụ

Memory commit đã đặt GPU RESERVED nhưng saveSnapshot lỗi → restore GPU/job/command về before; không có success response từ durable wrapper. Hai CP cùng path không cùng mở writer thành công.

### Complexity

O(B_state) về lượng dữ liệu copy/encode mỗi mutation, cộng fn và filesystem I/O; B_state là toàn snapshot, không chỉ Job đang assign. Latency disk/Sync chưa được characterize.

### Source mapping

[internal/store/durable/store.go][durable] — `Open`, `transact`, `saveSnapshot`, `read`; [durable/repository.go][durable-repo] — `CommitAssignment` wrapper; [memory/snapshot.go][snapshot] — `Export/Restore`; [main][main].

## A23 — Command lease, time gate và ACK

ACTIVE. LeaseCommands chỉ trả đúng AgentID; START kiểm lại freshness, organization, reservation thuộc Job, GPU vẫn RESERVED/ALLOCATED của chính Job và now nằm trong interval. Lease hết hạn có thể redeliver cùng ID trong interval; không redeliver START đã hết interval.

STOP thường đợi START PENDING/DELIVERED hoàn tất. Riêng START DELIVERED đã hết interval: cho STOP đi qua để Agent executor tuần tự dừng cả trường hợp START ACK bị mất. Không phát lại START muộn. Nếu chưa được Agent xác nhận hoặc mất kết nối, giữ claim; không tự FREE.

ACK START lỗi theo lifecycle cũ → FAILED/release, actual active consumer vẫn được accounting giữ. ACK thành công chưa thay inventory. Nguồn: memory/store.go, memory/planning.go, memory/lifecycle.go; Agent executor/Runner không thay đổi.


## A24 — Hết duration, cancel và release

ACTIVE. processReservations kiểm EndAt tại mỗi scheduling/reconcile tick. QUEUED hết interval → FAILED. PLANNED chưa dispatch hoặc START còn PENDING → CANCELLED, RELEASED; command chưa giao bị vô hiệu. Workload đã giao/chạy → STOPPING và STOP_CONTAINER graceful 30 giây, không release ngay.

Inventory running → ALLOCATED/RUNNING; exited/dead → STOPPED nếu đã yêu cầu stop, hoặc SUCCEEDED/FAILED theo exit code. Absence chỉ đủ để release khi START đã ACK thành công và snapshot không cũ hơn ACK, hoặc STOP đã ACK thành công và snapshot không cũ hơn ACK STOP. Điều này bảo vệ STARTING trong lúc pull/create và STOP với lost START ACK.

Release recompute physical state từ consumers/health; chỉ FREE khi không còn blocker. Future PLANNED không giữ physical GPU và không được dùng để adopt một container. Offline/lease timeout không tự release. Test: TestReservationEndStopsManagedAndReleasesOnlyOnObservation, TestExpiredDeliveredStartWithLostACKCanStillStop. Nguồn: memory/lifecycle.go, accounting.go, application/planning.go.


## A25 — Local GPU availability guard trước Agent launch

### Mục tiêu

Chặn launch khi actual occupancy local đã đổi sau CP reservation.

### Khi nào được gọi

`CommandExecutor.Execute` gọi InventoryCollector.GPUsAvailable trước StartManagedContainer; chỉ theo boundary trực tiếp, không khảo sát Docker executor sâu hơn.

### Đầu vào

`wanted []GPUUUID`, `sameJobID`; Snapshot local mới gồm GPU health/state, active containers, processes.

### Đầu ra

nil nếu tất cả requested UUID an toàn, hoặc ErrUnsafeGPU/snapshot error.

### Hard constraints

Requested UUID phải tồn tại và Healthy, không bị unknown telemetry/consumer khác chiếm. Bỏ qua active MANAGED container cùng JobID và processes của nó để idempotent retry. Không recheck profile/VRAM tại đây vì input không chứa ResourceRequest.

### Công thức / comparator

Snapshot → known[UUID]=Healthy; occupied từ UNKNOWN, active grants và processes; exclude same managed Job; mỗi unique wanted UUID phải known && !occupied. Không có transaction với Docker operations ngoài Agent.

### Pseudocode

`report=Snapshot; build known/occupied, ignore sameJob consumers; for unique wanted: nếu missing/unhealthy/occupied return ErrUnsafeGPU`.

### Ví dụ

CP reserve GPU g nhưng local report mới thấy legacy consumer trên g → launch bị chặn. Nếu g đang chạy chính managed Job được redeliver, guard cho qua nhánh idempotency.

### Complexity

Snapshot cost A10 cộng scan GPUs/containers/grants/processes. uniqueStrings dùng tuyến tính để kiểm trùng, có thể O(K²) cho wanted; không gán toàn guard O(K).

### Source mapping

[internal/agent/inventory.go][inventory] — `GPUsAvailable`, `Snapshot`, `uniqueStrings`; [internal/agent/executor.go][executor] — `CommandExecutor.Execute`.

## Một ví dụ end-to-end theo implementation

Ví dụ tính tay, snapshot ở `now`; source config `best-fit`. Hai Jobs cùng `TRAINING`, reason hợp lệ với Necessity, các trường name/image/neededAt/TTL hợp lệ. Provider cấu hình `InSizingPlan=true, QuotaGPUs=4, UsedGPUs=0`.

| Input | J1 | J2 |
|---|---|---|
| GPU requirements | 2 GPU, minVramMiB=16384, a100-equivalent, fp8Required=false | Cùng requirements |
| Necessity / reason | NECESSITY_1 / CONTRACT_PENALTY | NECESSITY_2 / APPROVED_PLAN |
| SystemImportance | NORMAL | CRITICAL_SPECIAL |
| CreatedAt | now | now−28 ngày |
| Policy | AUTO_ELIGIBLE, Aux=0.8×1×1=0.8 | AUTO_ELIGIBLE, Aux=1.4×1×1.4=1.96 |

**Queue:** J1 trước J2 do Necessity1 trước2; AuxiliaryScore cao hơn và CreatedAt sớm hơn của J2 không override Necessity. Preview của J1 lúc tạo không persist; submit lưu QUEUED, scheduling cycle evaluate lại như bảng.

Hai servers ONLINE, không drain, heartbeat/inventory receipt cách now1s. Tất cả GPU Healthy, model `NVIDIA A100-SXM4-40GB` thuộc aliases catalog; GPU FREE chưa AssignedJobID. VRAM available ở các GPU FREE là40960 MiB. GPU active External được accounting trước Plan.

| Server | GPU states hiện tại | Matching GPU của J1 |
|---|---|---|
| srv-a | GPU-A0 OCCUPIED_LEGACY; GPU-A1 FREE; GPU-A2 FREE; GPU-A3 OCCUPIED_UNKNOWN | GPU-A1, GPU-A2: M=2 |
| srv-b | GPU-B0 FREE; GPU-B1 FREE; GPU-B2 FREE; GPU-B3 FREE | GPU-B0..B3: M=4 |

**Filtering:** cả hai servers qua readiness/labels. Capability resolve A100 aliases; FP8=false không thêm FP8 restriction. A0/A3 bị GPU.Schedulable loại bất kể VRAM; những GPU còn lại qua model/health/VRAM. Mỗi host có ít nhất2 matching GPU nên đều là candidates.

**GPU subset:** available VRAM bằng nhau nên UUID phá hòa. A chọn [GPU-A1,GPU-A2]; B chọn [GPU-B0,GPU-B1].

**Strategy:** best-fit score A=2−2=0, B=4−2=2. Chọn srv-a; Server.ID không cần dùng để phá hòa ở ví dụ này. Không phải vì A có auxiliary score tốt hơn: server scorer không dùng business score.

**Reservation:** ReplanReservations lưu J1 ASSIGNED/PLANNED cho interval requested. Chỉ khi đến StartAt và revalidate thành công mới CommitAssignment chuyển STARTING/RESERVED và tạo START. Durable save thành công mới trả success. GPU-A0/A3 giữ blocker; không stop/adopt workload khác.

**Job sau:** planner thêm booking J1 vào calendar trước khi xét J2; B còn4 match nên có thể assign2 tại B trong cùng cycle. DevelopmentFacts.UsedGPUs vẫn0 vì không có ledger; example này không chứng minh quota enforcement.

**Dispatch/release:** Agent poll START của J1, local guard A25 kiểm lại; START success ACK đưa ASSIGNED→STARTING, active inventory đưa RUNNING/Assignment ALLOCATED (có thể active tới trước ACK). Khi inventory thấy exited code0 mà không có stop intent: SUCCEEDED + Assignment RELEASED, rồi normalize actual occupancy. Nếu A thay đổi trước commit: Conflict, không giữ một phần UUID, J1 còn QUEUED để cycle sau thử nếu chưa bị caller khác đổi trạng thái.

## Implementation traceability

Paths dưới đây thuộc backend `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`; link mở file tương ứng. Status theo định nghĩa đầu tài liệu.

| Decision / Algorithm | Source file | Function/type | Runtime status |
|---|---|---|---|
| A01 — validation/intent reasons | [internal/application/validation.go][validation]; [internal/policy/engine.go][policy]; [profiles.go][profiles] | validateJobRequest, Validate, Profiles | ACTIVE |
| A02 — capability resolution | [internal/capability/catalog.go][catalog]; [internal/domain/allocation.go][allocation-model] | Resolve, CapabilityMatches | ACTIVE |
| A03 — sizing/quota facts | [internal/policy/engine.go][policy]; [internal/config/config.go][config] | DevelopmentFacts.Facts, Load | ACTIVE |
| Production quota source | [internal/policy/engine.go][policy] | FactsProvider, DevelopmentFacts | PARTIAL |
| A04 — importance | [internal/policy/engine.go][policy] | ImportanceCoefficient | ACTIVE |
| A05 — waiting | [internal/policy/engine.go][policy] | WaitingCoefficient | ACTIVE |
| A06 — lane/aux score | [internal/policy/engine.go][policy] | Engine.Evaluate | ACTIVE |
| A07 — Necessity/lexicographic/FIFO | [internal/policy/engine.go][policy] | necessityRank, Before, Order | ACTIVE |
| Legacy numeric priority rule | [internal/domain/model.go][model]; [internal/policy/engine.go][policy] | Job.Priority declaration; Before không đọc | DECLARED_ONLY |
| A08 — queue/cycle/retry | [internal/application/controlplane.go][cp]; [internal/store/memory/store.go][store]; [cmd/aiwm-server/main.go][main] | Queue, ScheduleOnce, ListQueuedJobs, runScheduler | ACTIVE |
| A09 — host eligibility | [internal/domain/model.go][model]; [internal/store/memory/store.go][store] | Server.Schedulable, MarkStaleServers | ACTIVE |
| A10 — occupancy input | [internal/agent/inventory.go][inventory]; [gpu/nvml_linux.go][nvml]; [agent/config/config.go][agent-config] | Snapshot, InventoryPolicy, Reader.Snapshot, Load | ACTIVE |
| A11 — resource accounting | [internal/store/memory/accounting.go][accounting]; [store.go][store] | normalizeGPUState, ReplaceInventory | ACTIVE |
| A12 — GPU filters | [internal/domain/model.go][model]; [allocation.go][allocation-model] | GPU.Schedulable, AvailableMemoryMiB, Matches | ACTIVE |
| A13 — candidates | [internal/application/scheduler.go][scheduler] | filter, matchingGPUs, labelsMatch | ACTIVE |
| A14 — no-candidate reasons | [internal/application/scheduler.go][scheduler]; [allocation.go][allocation-app] | filter, planReservations, matchJob | ACTIVE |
| A15 — First Fit | [internal/application/scheduler.go][scheduler] | NewScheduler, StrategyFirstFit scorer | ACTIVE |
| A16 — Best Fit | [internal/application/scheduler.go][scheduler] | NewScheduler, StrategyBestFit scorer | ACTIVE |
| A17 — Bin Packing | [internal/application/scheduler.go][scheduler] | NewScheduler, freeGPUCount | ACTIVE |
| A18 — Fragmentation-aware | [internal/application/scheduler.go][scheduler] | fragmentationScore | ACTIVE |
| A19 — registry/default/selection | [internal/application/scheduler.go][scheduler]; [config.go][config]; [main.go][main] | Plan, selectCandidate, Load, main | ACTIVE |
| A20 — preview/submit | [internal/application/allocation.go][allocation-app]; [controlplane.go][cp] | PreviewJob, matchJob, prepareJob, CreateJob | ACTIVE |
| A21 — reservation | [internal/store/memory/store.go][store]; [internal/ports/repository.go][ports] | CommitAssignment | ACTIVE |
| A22 — local persistence transaction | [internal/store/durable/store.go][durable]; [repository.go][durable-repo]; [memory/snapshot.go][snapshot] | transact, saveSnapshot, Export, Restore | ACTIVE |
| A23 — dispatch retry/ACK | [internal/store/memory/store.go][store]; [controlplane.go][cp] | LeaseCommands, AckCommand, PollCommands | ACTIVE |
| A24 — allocation/release | [internal/store/memory/lifecycle.go][lifecycle] | RequestStop, applyAckLocked, reconcileObservedLocked, releaseLocked | ACTIVE |
| All-interleaving reservation safety | [internal/store/memory/lifecycle.go][lifecycle] | STOPPING + missing branch | PARTIAL |
| A25 — Agent launch guard | [internal/agent/inventory.go][inventory]; [executor.go][executor] | GPUsAvailable, CommandExecutor.Execute | ACTIVE |
| Time-window planning / delayed start / duration stop | internal/domain/reservation.go; application/planning.go; memory/planning.go | TimeWindow, planReservations, processReservations, ReplanReservations | ACTIVE |
| CPU/RAM capacity admission | [scheduler.go][scheduler] | Chưa accounting tổng capacity | NOT_IMPLEMENTED |

## Design observations

### 1. Policy facts và request thời gian chưa enforce allocation production

**Observation:** provider đang wired là DevelopmentFacts tĩnh; usage không cập nhật khi commit/release. NeededAt/TTL đã enforce theo phần T; CPU/RAM vẫn chưa được tổng hợp vào placement admission.

**Impact:** có thể mô tả đầy đủ demo policy/whole-GPU placement, chưa gọi đó là quota ledger; lịch đặt trước hiện là MVP greedy, STOP phụ thuộc kết nối runtime. Không tự xem admission lane là quota transaction.

**Relevant source:** [policy/engine.go][policy], [main.go][main], [allocation.go][allocation-app], [validation.go][validation], [scheduler.go][scheduler].

### 2. Queue deterministic trên snapshot, không đảm bảo starvation-free/global priority transaction

**Observation:** lane đi trước Necessity; aging có cap và không vượt primary keys. Jobs không vừa bị bỏ qua để thử Jobs sau; Order và calendar commit cùng lock snapshot; metadata/policy facts bên ngoài runtime store không nằm trong transaction này.

**Impact:** một Job có thể đợi lâu dù aging đạt cap; Job mới vào sau snapshot được xét ở cycle kế tiếp và có thể thay lịch còn future; không preempt lịch đã bắt đầu.

**Relevant source:** [Before/Order][policy], [ScheduleOnce][cp], [CommitAssignment][store].

### 3. Strategy names và capability groups cần đọc theo công thức

**Observation:** First Fit score mọi candidate rồi chọn ID nhỏ nhất; Best Fit dùng matching GPU count, không VRAM. Bin-pack dùng N−F kể cả GPU unhealthy/External; fragmentation dùng F/N theo schedulability. “Equivalent” là aliases catalog, không có measured equivalence; không enforce selected GPUs cùng model/topology.

**Impact:** không diễn giải các tên bằng textbook hoặc suy ra throughput/optimal packing. Đổi catalog không tự cập nhật ResolvedModels của stored Jobs; legacy Job.Strategy cũng không điều khiển configured Plan.

**Relevant source:** [scheduler.go][scheduler], [catalog.go][catalog], [ResourceRequest][allocation-model], [prepareJob][allocation-app].

### 4. Reservation atomic tại CP nhưng release/observation còn runtime race

**Observation:** CP check/mutate dưới mutex và durable rollback có thực. Missing nay cần ACK START hoặc ACK STOP và inventory đủ mới; STOP failed ACK đặt RUNNING mà không cần fresh active observation. Missing so Agent ObservedAt với CP command.CompletedAt; local start check không tạo lock với runtime bên ngoài.

**Impact:** regression cho pending START/lost ACK đã có; clock skew/late ACK không được giải quyết hoàn toàn chỉ bằng inventory sequence. Không khẳng định mọi double runtime allocation đã bị loại bỏ. Active consumers vẫn cần accounting sau logical release.

**Relevant source:** [lifecycle.go][lifecycle], [LeaseCommands][store], [GPUsAvailable][inventory], [executor.go][executor].

### 5. Error propagation và observation trust có giới hạn

**Observation:** planning batch và execution controller trả lỗi repository lên caller. CP nhận Container.Origin từ report; accounting kiểm Job/server/UUID nhưng Job reconciliation chỉ match JobID/server. Preview/planner dùng reason business chung, diagnostic flags vẫn ở scheduler.

**Impact:** lỗi persistence làm transaction thất bại; reason/badge/Job RUNNING riêng lẻ chưa đủ để chứng minh GPU đúng Assignment hoặc toàn bộ snapshot còn sẵn sàng. Source vẫn có hard guards khi commit; không thay code để “đồng bộ” description.

**Relevant source:** [ScheduleOnce][cp], [agent_protocol.go][protocol-map], [accounting.go][accounting], [lifecycle.go][lifecycle], [matchJob][allocation-app].

[model]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/model.go
[allocation-model]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/allocation.go
[dto]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/dto.go
[policy]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/policy/engine.go
[profiles]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/policy/profiles.go
[catalog]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/capability/catalog.go
[cp]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/controlplane.go
[allocation-app]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/allocation.go
[validation]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/validation.go
[scheduler]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/scheduler.go
[protocol-map]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/agent_protocol.go
[config]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/config/config.go
[main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-server/main.go
[store]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go
[accounting]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/accounting.go
[lifecycle]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/lifecycle.go
[snapshot]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/snapshot.go
[durable]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store.go
[durable-repo]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/repository.go
[ports]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/ports/repository.go
[http]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go
[job-view]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/job_view.go
[inventory]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/inventory.go
[agent-config]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/config/config.go
[nvml]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_linux.go
[executor]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/executor.go
