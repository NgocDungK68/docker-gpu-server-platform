# Test scenarios

[README root](../../../README.md) chứa lệnh chạy backend/Agent/frontend. Chạy các acceptance lần lượt trên root Compose demo, vì chúng dùng chung GPU pool.

| Scenario | Lệnh tại workspace root | Assertion chính |
|---|---|---|
| A Empty T4 | python scripts/acceptance.py | 2 NVML GPU, không External |
| B Existing A100 | Cùng script | 4 GPU, External GPU0 protected, ID/start time không đổi |
| C Managed job | Cùng script | 3 A100 UUID trống → RUNNING |
| D Insufficient resource | Cùng script | Job chờ có pending reason |
| E Release | Cùng script | Stop → RELEASED → pending job chạy |
| F Agent lost | python scripts/recovery.py | Offline không nhận placement; giữ reservation |
| G Reconnect | Cùng script | Fresh inventory, không duplicate, runtime state giữ nguyên |
| H Concurrent jobs | python scripts/acceptance.py | 2 T4 jobs không trùng physical UUID |
| CP restart | python scripts/recovery.py | Jobs/reservations restore, runtime không đổi |
| BFF | python scripts/acceptance.py --base-url http://127.0.0.1:3000/api/aiwm | Cùng A–E/H qua Next proxy |
| Browser | npm.cmd run test:e2e trong frontend | Inventory + form/create/run/stop + BFF policy |

## Unit/integration

Go vet/test/build cả cmd; go test -race ./... trên Linux có GCC. Dockerfile.test chạy cùng checks và production NVML integration với bốn profile. Các test safety bao phủ atomic all-or-none, concurrency, rollback diskfail, stale inventory, ACK trùng, cancellation, queue priority/FIFO, failed/exited/missing job, token/validation/redaction.

[Traceability](../../../docs/TRACEABILITY.md) chỉ ra file test cho từng scope. [Simulation](../../../docs/SIMULATION.md) có profile unknown/unhealthy và failure injection.

Simulation không thực thi Docker/CUDA payload. Real Docker SDK được kiểm tra qua HTTP adapter tests; CUDA/Toolkit và runtime process continuity trên GPU thật chưa chạy trong môi trường này.
