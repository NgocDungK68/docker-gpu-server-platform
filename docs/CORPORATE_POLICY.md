# Policy nghiệp vụ của AIWM

Đây là **project policy baseline được cung cấp cho hệ thống này**, thể hiện trong `internal/policy/engine.go` và `profiles.go`. Tài liệu không xác nhận đây là văn bản chính thức đã được thẩm định của Tập đoàn. Thay đổi Organization không sửa hệ số hoặc comparator hiện có.

## Phân biệt trách nhiệm

- **Policy**: đánh giá eligibility/lane và thứ tự Job trong queue.
- **Placement**: chọn Server/GPU cho Job đang được xét.
- **Organization ownership**: hard constraint `Job.organizationId == Server.organizationId`, cả hai không rỗng. Đây không phải hệ số priority và không thể bị policy override.

## Sizing plan và quota

`FactsProvider` là boundary cho nguồn quy hoạch/hạn mức. Runtime hiện nối `DevelopmentFacts`, trả `Source=DEVELOPMENT_CONFIG`; chưa có quota ledger theo organization hoặc tích hợp hệ thống quy hoạch chính thức.

`InSizingPlan` lấy từ `AIWM_DEV_IN_SIZING_PLAN`. `WithinQuota` được tính:

```text
Job.Resources.GPUCount <= AIWM_DEV_QUOTA_GPUS - AIWM_DEV_QUOTA_USED_GPUS
```

Các env này áp dụng chung trong process; PostgreSQL metadata không biến chúng thành quota riêng từng đơn vị. Trong quy hoạch **và** trong quota → `AUTO_ELIGIBLE`; các trường hợp khác → `COMPETITIVE`. Cả hai lane đều có thể được xếp lịch, tùy tài nguyên. `COMPETITIVE` không phải reject hay chờ phê duyệt thủ công.

## Necessity 1–4

Trong cùng lane: **Necessity 1 > 2 > 3 > 4**. User phải chọn `TRAINING` hoặc `INFERENCE` và reason đúng profile. Backend validate reason; không dùng NLP để đoán mức cần thiết.

| Workload | Mức | Reason ID và ý nghĩa |
|---|---|---|
| TRAINING | N1 | `EXECUTIVE_DIRECTION`: chỉ đạo cấp cao; `CONTRACT_PENALTY`: cam kết có phạt; `INCIDENT_RETRAINING`: huấn luyện lại khẩn sau sự cố; `LEGAL_DEADLINE`: pháp lý gần hạn. |
| TRAINING | N2 | `GO_LIVE_90_DAYS`: go-live dưới 90 ngày; `BUSINESS_VALUE`: doanh thu/tiết kiệm lớn; `APPROVED_PLAN`: kế hoạch/phương án đã duyệt. |
| TRAINING | N3 | `PERIODIC_RETRAINING`: huấn luyện lại định kỳ; `INTERNAL_IMPROVEMENT`: cải tiến nội bộ. |
| TRAINING | N4 | `EARLY_RESEARCH`: nghiên cứu ban đầu; `MODEL_EXPERIMENT`: mô hình mới. |
| INFERENCE | N1 | `EXECUTIVE_DIRECTION`: chỉ đạo cấp cao; `BROAD_EMERGENCY`: khẩn cấp/ảnh hưởng rộng; `INCIDENT_MITIGATION`: khắc phục sự cố; `LEGAL_REQUIREMENT`: pháp lý bắt buộc. |
| INFERENCE | N2 | `CUSTOMER_SLA`: sản phẩm phục vụ khách hàng có SLA chặt. Backend hiện gộp hai tiêu chí này trong một reason. |
| INFERENCE | N3 | `INTERNAL_PROCESS`, `BATCH_PROCESSING`, `NON_REALTIME`: quy trình nội bộ, batch, không yêu cầu độ trễ tức thời. |
| INFERENCE | N4 | `RESEARCH`, `EXPERIMENT`, `DEMO`, `DEV_UAT`: R&D, thử nghiệm, demo, Dev/UAT. |

Ngoài reason định nghĩa sẵn, chọn `CUSTOM` và nhập `necessityExplanation`. Source hiện bắt buộc nội dung khác rỗng sau trim, tối đa 4096 byte; chưa đánh giá chất lượng hoặc độ chi tiết của giải trình. Nhãn deadline/90 ngày là tiêu chí do người yêu cầu khai báo, chưa được xác minh với hệ thống bên ngoài.

## Auxiliary priority

\[
AuxPriority = \frac{\operatorname{round}(1000 K_{system\_importance}K_{quota}K_{waiting\_time})}{1000}
\]

Làm tròn ba chữ số thập phân đúng `Engine.Evaluate`, tránh phá FIFO do sai số float.

| `systemImportance` | Hệ số backend |
|---|---:|
| `CRITICAL_SPECIAL` | 1.4 |
| `VERY_IMPORTANT` | 1.2 |
| `IMPORTANT` | 1.0 |
| `NORMAL` | 0.8 |

`K_quota=1.0` khi trong quota, `0.8` khi vượt quota.

\[
w=\min(4,\max(0,\lfloor(now-Job.CreatedAt)/(7\text{ ngày})\rfloor)),\qquad K_{waiting\_time}=1+w/10
\]

Chờ dưới 7 ngày: 1.0; từ 7/14/21/28 ngày: 1.1/1.2/1.3/1.4. Thời gian tính từ `Job.CreatedAt` của Control Plane, không từ `neededAt`, heartbeat hay lần retry. `Order` tính lại khi đọc queue/scheduling cycle. Cap này không bảo đảm chống starvation tuyệt đối.

## Comparator thực tế

`Before(a,b)` so lexicographic, dừng tại tiêu chí đầu tiên khác nhau:

1. `AUTO_ELIGIBLE` trước `COMPETITIVE`.
2. Necessity rank tăng dần 1 → 4; Job cũ thiếu intent có rank 5.
3. `AuxiliaryScore` giảm dần, chỉ khi cùng lane và Necessity.
4. `CreatedAt` tăng dần.
5. `ID` theo thứ tự chuỗi tăng dần.

Ví dụ cùng lane: A có N1/score 0.64 đứng trước B có N2/score 1.96. Khác lane, lane được so trước Necessity; đây là behavior đang có, không phải một weighted sum.

## Giới hạn

Preview không reserve. Submit đánh giá lại; scheduler tiếp tục đánh giá theo state mới. `neededAt` và TTL hiện là field được validate/lưu, chưa là delayed start hay auto-stop. Preemption, reclaim, checkpoint, MIG, time-sharing và chia sẻ GPU đều ngoài scope. User không nhập hệ số/score/strategy/Server/GPU; frontend hiển thị nhãn và kết quả đối chiếu.

## Nguồn

Các path dưới đây thuộc `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`:

| Quyết định | File / function |
|---|---|
| Sizing/quota và lane | `internal/policy/engine.go`: `DevelopmentFacts.Facts`, `Engine.Evaluate` |
| Hệ số | Cùng file: `ImportanceCoefficient`, `WaitingCoefficient` |
| Thứ tự queue | Cùng file: `Order`, `Before`, `necessityRank` |
| Reason và validation | `internal/policy/profiles.go`: `Profiles`; `engine.go`: `Validate` |
| Context organization | `internal/application/allocation.go`: `prepareJob`; `internal/domain/organization.go`: `SameOrganization` |

Chi tiết resource filtering/placement/reservation: [ALGORITHMS.md](ALGORITHMS.md).
