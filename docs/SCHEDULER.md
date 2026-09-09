# Scheduler và queue

Code dùng chung: backend/internal/application/scheduler.go. Binary benchmark import đúng package này; không có bản sao thuật toán trong simulator/frontend.

## Filter → score → select → reserve → release

Filter yêu cầu server online, không drain, heartbeat và inventory received còn trong OfflineAfter. GPU phải healthy, FREE, chưa assigned; model khớp không phân biệt hoa thường, đủ available VRAM từng GPU, count trên cùng một server và labels phù hợp. Không ghép GPU của hai server cho một job.

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

## Queue

Priority giảm dần, createdAt tăng dần, cuối cùng ID tăng dần. FIFO trong cùng priority. Mỗi cycle thử các queued jobs theo thứ tự; job lớn chưa vừa không chặn mọi job nhỏ phía sau. Không có aging/fair-share/preemption; priority thấp có thể chờ lâu.

Pending reason phân biệt no fresh online agent, labels, model, health, occupied External/unknown, reservation, VRAM, insufficient GPU count. CPU/RAM là Docker limits, chưa phải admission filter tổng server.

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

