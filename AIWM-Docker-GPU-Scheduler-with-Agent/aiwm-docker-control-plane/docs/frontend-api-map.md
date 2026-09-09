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

Successful response là data; failure là error.code/message. /stop hủy queued/unleased job hoặc stop managed, idempotent. Không có thao tác External. Job response gồm events, containerId, reservationState, release time, strategy score/reason; environment values che [redacted].

Server có host hardware, dockerVersion, inventoryReceivedAt, schedulable/schedulingReason. UI không tính GPU FREE ở host offline/stale/drained là sẵn sàng.

Contract đầy đủ: backend/api/openapi.yaml. Frontend types/client: src/lib/api. [README root](../../../README.md) có bảng method/endpoint/caller/request/response và demo.
