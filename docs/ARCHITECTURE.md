# Architecture

## Quyết định sau audit

Giữ Go module và Next.js project đang có; giữ outbound HTTP protocol v1 và cả bốn composition roots. Control Plane trước đó chỉ có memory repository, SQL là target schema chưa sử dụng. Adapter durable mới bọc đúng memory transaction để thêm process restart recovery, không thay một database đang vận hành.

Simulator chỉ thay DockerRuntime boundary. Cả production/simulator dùng InventoryCollector, CommandExecutor, Runner, HTTP client, file state và gpu.New. GPU UUID/model/memory/process đến từ cùng Linux NVML adapter. YAML là cấu hình chính thức của NVIDIA mock library.

## Boundaries

- domain: server/GPU/container/job/command state; không gọi Docker, HTTP hoặc filesystem.
- application: register, schedule, queue, summary, wire/domain mapping; Job.Status tổng hợp tiến độ điều khiển và observation, chưa có DesiredState/ActualState riêng. SchedulingPolicy có filter/score/select riêng.
- ports.Repository: commit reservation và stop/ACK/reconcile atomic.
- store/memory: critical section bảo vệ toàn bộ các UUID, jobs, command. Kiểm tra đủ điều kiện trước mutation.
- store/durable: serialize transaction, snapshot trước/sau, ghi temp cùng thư mục, file Sync và rename; rollback memory khi ghi lỗi. File lock ngăn hai writer. Không phải distributed store.
- agent core: independent heartbeat/inventory/poll/event loops; no cleanup/stop on disconnect.
- agent/dockerengine: official Moby SDK, local socket, ownership và exact UUID DeviceRequests.
- agent/gpu: Linux CGO NVML; unsupported platform trả lỗi rõ.
- httpapi: middleware auth, body limit, error envelope, explicit public Job DTO che environment.
- frontend: UI → hooks → centralized typed client → BFF whitelist → public API; không chứa scheduler.

## Domain và lifecycle

[DOMAIN_MODEL.md](DOMAIN_MODEL.md) là bản audit chi tiết của entities, relationships, toàn bộ enum/literal state và các guards chuyển trạng thái trong source.

- Server chứa GPU/Container observations và cả identity/connectivity của Agent; AgentID = Server.ID, không có AgentStatus riêng.
- Workload trên Console là Job. External/Managed là cách phân loại container; không có entity Workload độc lập.
- Reservation nằm trong Job.Assignment, dùng string RESERVED/ALLOCATED/RELEASED; có cả nhánh RESERVED → RELEASED. Placement chỉ là proposal của scheduler.
- Job có QUEUED, ASSIGNED, STARTING, RUNNING, STOPPING, STOPPED, SUCCEEDED, FAILED, CANCELLED. RUNNING có thể được quan sát trước Start ACK; exit sớm cũng có thể kết thúc Job từ ASSIGNED/STARTING.
- Command có PENDING, DELIVERED, SUCCEEDED, FAILED. DELIVERED là lease, chưa chứng minh Agent đang thực thi; hủy Start chưa delivery dùng FAILED, không có Command CANCELLED.

ACK lặp không hồi sinh terminal Job. Stop command chỉ được lease khi Start không còn PENDING/DELIVERED; Stop ACK thành công không tự đổi Job.Status hoặc release. Failed Stop ACK đưa Job về RUNNING mà chưa cần observation mới. Inventory thấy exited/dead hoặc absence khi Job STOPPING đưa Job về STOPPED và release logical reservation. Nhánh absence hiện chưa kiểm tra Start còn đang chờ ACK, nên không thể khẳng định giữ reservation qua mọi interleaving start/stop; xem audit và test gaps trong Domain Model.

Terminal Job giữ Assignment với reservationState RELEASED để xem lịch sử. GPU vẫn có thể ALLOCATED nếu active container còn được quan sát; release logical reservation không đồng nghĩa GPU vật lý FREE.

Nếu offline, giữ job/reservation/container last-known; scheduler loại server theo cả LastHeartbeatAt lẫn inventory receipt time. Inventory được chấp nhận cũng cập nhật LastHeartbeatAt. CP restore đặt Server OFFLINE và reset inventory receipt; heartbeat có thể hồi status ONLINE/DRAINING, nhưng phải có inventory mới trước placement. Agent state và Docker runtime nằm ngoài CP. Agent restart dùng identity/sequence/processed results để reconnect; re-enrollment cùng MachineID giữ Server.ID, đổi credential và reset freshness.

## Occupancy

Ưu tiên unhealthy → external container → unknown process → known managed allocation → unknown telemetry → reservation → free. Running/paused/restarting container có GPU grant đều chiếm GPU, ngay cả khi idle. Numeric index được map với UUID của lần NVML snapshot đó. Grant không giải được hoặc --gpus all chưa có exact IDs bảo vệ cả host.

Container.Origin MANAGED được nhận từ Agent; CP không rewrite origin khi Job/Assignment không khớp. Accounting chỉ công nhận managed allocation khi khớp Job, Server và UUID, nếu không thì active grant được tính OCCUPIED_LEGACY. Job reconciliation match JobID/assigned Server, chưa kiểm tra exact GPU grant như accounting.

NVML process map qua PID/cgroup khi được phép; nếu không chắc owner thì OCCUPIED_UNKNOWN. Significant memory/utilization không có attribution cũng bị chặn. Đọc lỗi inventory không được coi là GPU free. Global UUID trùng giữa hai server bị từ chối.

## Persistence và schema

Snapshot gob version 1 lưu server, token hash, inventory last-known, jobs, assignments, commands và trạng thái kết quả/lease/timestamps. Full processed ACK nằm trong Agent local state; CP không có ACK entity riêng. Agent file chứa bearer token, machine/agent ID, sequence, processed command history; cần quyền file hạn chế. Simulator file .docker.json chứa DockerRuntime state riêng, không chứa GPU profile.

migrations/001_init.sql và 002_lifecycle_target.sql chỉ mô tả future PostgreSQL model. Server startup không kết nối PostgreSQL hoặc tự migrate. Chưa có hai CP replicas, per-row transaction, retention hay power-loss guarantee. Khi triển khai snapshot cần local durable disk và một writer; không đặt chung state trên nhiều CP.

## Observability

JSON slog có request ID, job/server/command ID và scheduler decision. Không ghi env payload hoặc token vào log. /healthz và /readyz là process probes; readiness không cam kết có Agent online hay GPU đủ tài nguyên. Dashboard summary và JobEvent phục vụ demo reconciliation; đây chưa phải immutable audit log.
