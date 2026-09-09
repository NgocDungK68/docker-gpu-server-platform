# Scheduler và queue

Code dùng chung: backend/internal/application/scheduler.go. Binary benchmark import đúng package này; không có bản sao thuật toán trong simulator/frontend.

## Filter → score → select → reserve → release

Filter yêu cầu server online, không drain, heartbeat và inventory received còn trong OfflineAfter. GPU phải healthy, FREE, chưa assigned; profile/FP8 giải qua catalog tập trung; model inventory khớp ResolvedModels không phân biệt hoa thường, đủ available VRAM từng GPU, count trên cùng một server (selector/model cũ chỉ giữ cho persisted Job trước intent và benchmark). Không ghép GPU của hai server cho một job.

Mọi policy chọn score thấp nhất. Tie theo server.ID tăng dần. GPU trong server theo available VRAM tăng dần rồi UUID. Cùng snapshot, thời điểm và job tạo cùng placement, không phụ thuộc thứ tự map.

Đặt F = số GPU schedulable của server, N = tổng GPU vật lý, K = số GPU cần, M = số GPU free phù hợp model/VRAM.

| Policy | Score | Ý nghĩa / trade-off |
|---|---|---|
| first-fit | 0 | Chọn server ID nhỏ nhất đủ điều kiện; dễ giải thích, không xét phần dư |
| best-fit | M − K | Ít GPU phù hợp còn dư nhất; không xét tổng host occupancy |
| bin-pack | 10 × (F − K) − (N − F) | Giảm GPU free còn dư và ưu tiên host đã bận; model heterogeneity có thể ảnh hưởng |
| fragmentation-aware | 2 nếu F=N, cộng 1 nếu F−K>0, cộng (F−K)/(N+1) | Ưu tiên giữ host hoàn toàn trống, rồi ưu tiên lấp kín phần còn lại; đánh đổi packing với mở host mới |

Score fragmentation là heuristic bounded, không phải optimizer toàn cục. N−F bao gồm GPU External/unknown/unhealthy, nhưng chúng đã bị loại khỏi tập lựa chọn. Không policy nào được override filter hoặc chiếm GPU đang có consumer.

Repository Commit kiểm tra lại snapshot và đủ toàn bộ UUID trong cùng mutex; job/reservation/start command thành công cùng nhau hoặc không mutate. Durable wrapper chỉ công nhận mutation sau khi ghi state thành công. Scheduler cycle sau có thể thử job còn QUEUED nếu conflict xảy ra.

## Policy/Admission và queue

User khai báo nhu cầu; `policy.Evaluator` đánh giá admission và thứ tự phục vụ; `Scheduler` quyết định physical placement. Không đưa `AuxiliaryScore` vào placement score.

`policy.Engine.Order` evaluate lại planning/quota và waiting time mỗi lần GET Queue hoặc scheduling cycle. Comparator `policy.Before` là lexicographic:

1. `AUTO_ELIGIBLE` (trong sizing plan **và** quota) trước `COMPETITIVE`.
2. Trong cùng lane: `NECESSITY_1 > NECESSITY_2 > NECESSITY_3 > NECESSITY_4`.
3. Chỉ cùng Necessity: `AuxiliaryScore` giảm dần.
4. `CreatedAt` tăng dần, rồi `ID` tăng dần.

`AuxiliaryScore = K_importance × K_quota × K_waiting`. Importance do user chọn category; backend map `CRITICAL_SPECIAL=1.4`, `VERY_IMPORTANT=1.2`, `IMPORTANT=1.0`, `NORMAL=0.8`. Trong quota = 1; vượt quota = 0.8. Waiting từ `Job.CreatedAt` bằng đồng hồ CP: dưới 7 ngày = 1; mỗi 7 ngày đủ tăng 0.1; tối đa 1.4 sau 28 ngày. Preview chưa có Job nên waiting = 0. Không dùng `neededAt` hoặc thời gian frontend cho aging.

Trong cùng lane, N1 score 0.64 vẫn trước N2 score 1.96. Auto N4 có thể trước Competitive N1 vì lane được xét trước. `store/memory.ListQueuedJobs` chỉ trả FIFO/ID; application sở hữu business order. Field `Job.Priority` cũ không quyết định queue. Job lớn không vừa không chặn mọi job nhỏ phía sau. Aging có giới hạn; không bảo đảm starvation-free; không preempt/reclaim.

## Input và dữ liệu hệ thống

| Nguồn | Fields | Semantics |
|---|---|---|
| USER-PROVIDED | `name/image/command/environment`, CPU/RAM limits nếu cần | Docker workload hiện có; env được che trong public response. |
| USER-PROVIDED | `workloadType` | Bắt buộc TRAINING hoặc INFERENCE. |
| USER-PROVIDED | `resources.gpuCount/minVramMiB/performanceProfile/fp8Required` | Count nguyên >0; VRAM nguyên MiB >0 mỗi GPU; profile catalog; FP8 boolean bắt buộc. |
| USER-PROVIDED | `necessityLevel/necessityReason/necessityExplanation` | Reason theo workload + cấp; CUSTOM cần giải trình. |
| USER-PROVIDED | `systemImportance` | Business category; không nhập coefficient. |
| USER-PROVIDED | `neededAt/ttlSeconds` | RFC3339 có timezone, normalize UTC; cho phép quá khứ. TTL dương theo giây, max cấu hình. Chưa hẹn start hoặc tự stop khi hết TTL. |
| SYSTEM-DERIVED | `InSizingPlan/WithinQuota/QuotaUsage` | Qua `policy.FactsProvider`; hiện là DEVELOPMENT_CONFIG, chưa production quota accounting. |
| SYSTEM-DERIVED | lane, waiting, coefficients, auxiliary score | `policy.Engine`; không nhận từ Create DTO hoặc tính trong React. |
| SYSTEM-DERIVED | resolved models, server/GPU selection, strategy/score/Assignment | Catalog → matcher → scorer → atomic commit. |

`policy/profiles.go` cung cấp catalog lý do và nhãn tiếng Việt qua `GET /api/v1/jobs/options`:

| Cấp | TRAINING | INFERENCE |
|---|---|---|
| N1 | Ban TGĐ Tập đoàn; hợp đồng có phạt; retraining sự cố khẩn; pháp lý sắp đến hạn | Ban TGĐ Tập đoàn; khẩn cấp ảnh hưởng rộng; khắc phục sự cố; pháp lý bắt buộc |
| N2 | Go-live dưới 90 ngày; doanh thu/tiết kiệm lớn; kế hoạch/phương án kinh doanh đã phê duyệt | Sản phẩm phục vụ khách hàng có SLA chặt |
| N3 | Retraining định kỳ; cải tiến nội bộ | Quy trình nội bộ; batch; không ràng buộc độ trễ tức thời |
| N4 | Nghiên cứu ban đầu; thử nghiệm mô hình mới | R&D; thử nghiệm; demo; Dev/UAT |

## Capability và cách thay policy/thuật toán

- `internal/capability/catalog.go` tập trung mapping `general`, `a100-equivalent`, `h100-equivalent` sang exact model aliases. Đây là compatibility group do backend quản lý, **chưa chứng nhận tương đương hiệu năng qua benchmark**. Không match bằng substring tên GPU.
- FP8 lấy từ catalog, không suy đoán ở React. H100 có FP8 theo [NVIDIA Transformer Engine](https://docs.nvidia.com/deeplearning/transformer-engine-releases/release-2.0/user-guide/examples/fp8_primer.html). Catalog mặc định bảo thủ; model chưa biết không match, kể cả `general`. Mock hiện có A100/T4 nên yêu cầu H100/FP8 sẽ chờ tài nguyên.
- `capability.Resolver` có `Profiles/Resolve`, inject qua `Options.Catalog`. Resolve lúc submit, lưu private `ResourceRequest.ResolvedModels` trong gob; request đã nhận giữ constraint snapshot qua restart. Đổi catalog áp dụng request mới.
- `ResourceRequest.CapabilityMatches/Matches` dùng chung planner và atomic commit. Commit recheck model/VRAM/health/occupancy dưới lock. Agent kiểm tra runtime lại trước start.
- `policy.FactsProvider` là boundary Planning/Quota thật. `DevelopmentFacts` dùng `AIWM_DEV_IN_SIZING_PLAN`, `AIWM_DEV_QUOTA_GPUS`, `AIWM_DEV_QUOTA_USED_GPUS`. Usage là số GPU used cấu hình; WithinQuota xét used + request ≤ quota, không tự tăng usage khi Job chạy.
- Thay business policy bằng `Options.Policy` (`policy.Evaluator`). Thay scorer bằng `Options.PlacementPolicy` hoặc `Scheduler.WithPolicy` (`SchedulingPolicy.Score`); scorer chỉ nhận candidate sau filter, không được override hard constraints.
- CP strategy: `internal/config/config.go`, env `AIWM_SCHEDULER_STRATEGY` (default best-fit), nối tại `cmd/aiwm-server/main.go`. `Job.Strategy` không chọn thuật toán; benchmark tạo scheduler riêng cho từng strategy.

## Preview → Submit → placement

~~~mermaid
flowchart TD
    U[User: intent + constraints] --> V[Validate và Policy evaluate]
    V --> M[Đọc inventory mới và Plan]
    M --> Preview[Preview: không mutation]
    U --> Submit[Submit: validate/evaluate lại]
    Submit --> Queue[CreateJob: QUEUED]
    Queue --> Cycle[Cycle: refresh policy và queue order]
    Cycle --> Plan[Snapshot mới: filter → score → select]
    Plan --> Commit[CommitAssignment: recheck dưới lock]
    Commit --> Atomic[GPU RESERVED + Job ASSIGNED + START command]
    Atomic --> Agent[Agent kiểm tra rồi triển khai]
~~~

`POST jobs/preview` trả reason, sizing/quota status, necessityLabel và resourceMatch; không có Job ID, reservation, server/UUID hoặc score. `matchedGpuCount` là số GPU match tối đa ở **một** server fresh; model list là tập model match từ các server fresh. `satisfiable` nói snapshot có placement, chưa cam kết phục vụ trước Job khác.

`POST jobs` evaluate lại rồi trả 201 QUEUED kể cả chưa có GPU. Submit không bypass queue để reserve ngay; background/run-once evaluate toàn queue và commit atomic. Final placement có thể khác preview. GET Job trả policy snapshot lúc submit/commit; GET Queue trả evaluation mới. Không thêm Job state SCHEDULING/RESERVED/DISPATCHING.

## Focused tests

`policy/engine_test.go`: levels, reasons, coefficients, waiting boundaries, quota/lanes. `application/allocation_test.go`: validation, preview không mutation, submit re-evaluation, queue winner, commit khi capability đổi. `capability/catalog_test.go`: FP8/unknown model. `httpapi/allocation_test.go`: DTO/JSON type/auth/cấm placement controls. Memory/durable safety tests giữ reserve/release và race nhiều request.

## Thí nghiệm deterministic

Trong backend:

~~~sh
go run ./cmd/aiwm-scheduler-bench
~~~

Fixture tại cmd/aiwm-scheduler-bench/main.go: 5 host, mỗi host 4 A100; free lần lượt 3,4,2,1,4; phần còn lại External. Job lần lượt cần 1,3,4,4,2 GPU. Không random và dùng fixed timestamp.

| Policy | Scheduled / queued | GPU dùng / allocatable | Host phân mảnh cuối |
|---|---|---|---|
| first-fit | 4 / 1 | 10 / 14 (71.43%) | 3 |
| best-fit | 5 / 0 | 14 / 14 (100%) | 0 |
| bin-pack | 5 / 0 | 14 / 14 (100%) | 0 |
| fragmentation-aware | 5 / 0 | 14 / 14 (100%) | 0 |

Utilization trong benchmark là tỷ lệ GPU được cấp phát trên GPU allocatable, không phải NVML compute utilization. FragmentedServers đếm host còn một phần GPU trống. Kết quả chỉ phản ánh fixture này; không dùng benchmark giả trên UI.

