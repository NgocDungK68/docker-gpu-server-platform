# Screen → API

Browser gọi /api/aiwm cùng origin; BFF chuyển sang /api/v1 và thêm server-side API token. Không proxy /agents hoặc Docker/NVML.

| Screen | Public endpoint |
|---|---|
| Dashboard | GET /system/summary, /servers, /gpus, /jobs |
| Servers / Onboarding | GET /servers |
| Server detail | GET /servers/{id}; POST /servers/{id}/drain |
| GPU inventory | GET /gpus |
| Containers | GET /containers?origin=MANAGED hoặc LEGACY hoặc UNKNOWN |
| Jobs / Scheduler | GET /jobs |
| Submit Job | POST /jobs |
| Job detail | GET /jobs/{id}; POST /jobs/{id}/stop |
| Queue | GET /queue; POST /scheduler/run-once |
| Connection status | GET /api/aiwm-health ở Next → /healthz ở CP |

Successful response là data; failure là error.code/message và error.fields cho validation. /stop hủy queued/unleased job hoặc stop managed, idempotent. Không có thao tác External. Job response gồm events, containerId, reservationState, release time, strategy score/reason; environment values che [redacted].

Server có host hardware, dockerVersion, inventoryReceivedAt, schedulable/schedulingReason. CP Summary.gpusFree dùng freshness/readiness của Server; trang GPU inventory hiện chỉ đếm State=FREE cho ô “Sẵn sàng”, chưa xét Server offline/stale/drained. Đây là khác biệt presentation đã ghi trong [API audit](../../../docs/API_TESTING_POSTMAN.md); không dùng số đếm trang GPU thay quyết định scheduler.

Contract đầy đủ: backend/api/openapi.yaml. Frontend types/client: src/lib/api. [README root](../../../README.md) có bảng method/endpoint/caller/request/response và demo.

Create form gọi `GET /api/v1/jobs/options`, `POST /api/v1/jobs/preview` qua BFF tương ứng. Backend cung cấp labels/reasons/profiles/limits, frontend không tính K/score. `POST jobs` nhận intent + constraints, re-evaluate rồi trả QUEUED; không nhận `strategy/priority/serverSelector/resources.gpuModel`. Xem [Scheduler/policy](../../../docs/SCHEDULER.md).
