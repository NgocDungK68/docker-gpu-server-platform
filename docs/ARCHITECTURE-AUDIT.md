# Architecture audit và implementation plan

## Trạng thái ban đầu

Workspace không có Git metadata. Backend dùng Go 1.24, net/http, NVIDIA go-nvml và Moby Go SDK; repository in-memory có mutex và schema PostgreSQL dự kiến, chưa có DB adapter thực tế. Frontend dùng Next.js 16.3.4, React 19, TypeScript, React Query, React Hook Form/Zod, Tailwind và Recharts. Các route/API client/theme đã có thể tái sử dụng.

HTML đính kèm là tham chiếu bố cục/sidebar/card/table, màu Viettel. Những ý tưởng Kubernetes, checkpoint, quota hay preemption trong HTML không phải yêu cầu triển khai.

## Gap đã xác nhận từ source

1. Simulator tự tạo GPU trong Go và duplicate Agent lifecycle; không dùng NVIDIA nvml-mock.
2. Control Plane restart mất jobs/reservations/identity. Reconciliation chỉ đánh dấu offline và xác nhận running, chưa xử lý container missing/exited.
3. ACK lặp có thể làm lùi job; reserve chưa kiểm tra đủ tập UUID và server trong critical section, cập nhật slice trước khi validate xong; release chưa atomic.
4. Thiếu fragmentation strategy, pending reason cụ thể, state transition guard và cancellation cho queued jobs.
5. NVML memory/utilization chưa chặn unknown occupancy; Docker index mapping chặn cả server; retry có thể restart container đã kết thúc.
6. Public API chưa có authentication, default enrollment secret hardcode, proxy frontend chuyển tiếp Agent routes, lỗi nội bộ có thể lộ ra response.
7. Frontend có màn hình/mock benchmark ngoài scope; form ví dụ trỏ tới train.py không có sẵn trong image.
8. Coverage ban đầu ít (13 test), chưa có frontend tests và hướng dẫn root.

## Quyết định và thứ tự triển khai

1. Giữ bốn binaries, Agent outbound HTTP, SDK/NVML adapters và UI framework hiện tại.
2. Sửa domain, atomic reservation/ACK/stop, reconciliation và freshness trước; thêm durable single-process snapshot adapter để MVP khôi phục sau crash, không thay database đang chạy (hiện không có). Schema PostgreSQL vẫn là mục tiêu adapter tương lai.
3. Tách filter/score/select, thêm fragmentation-aware và benchmark dùng production scheduler.
4. Dùng NVIDIA mock libnvidia-ml.so build từ upstream pin commit, cùng production NVML reader và Agent runner. Docker runtime mô phỏng lưu state; production Docker adapter được kiểm tra riêng qua HTTP adapter tests.
5. Thêm auth/config/validation, đồng bộ public DTO/OpenAPI và UI; bỏ các control ngoài scope.
6. Chạy backend/frontend/build/integration khả thi; ghi rõ giới hạn GPU/CUDA thật. Viết README tiếng Việt, API/config/security/scope mapping và demo A–H.

## Baseline

`go test ./...` đã pass trước khi sửa. Máy host là Windows; có Ubuntu WSL/GCC. Docker Desktop ban đầu chưa chạy. Việc test production NVML cần Linux/CGO; CUDA computation cần GPU thật và không thể được chứng minh bằng mock NVML.
