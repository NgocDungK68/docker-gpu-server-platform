# NVIDIA NVML simulation

## Cách tích hợp

Dockerfile.sim build mock libnvidia-ml.so từ [NVIDIA k8s-test-infra](https://github.com/NVIDIA/k8s-test-infra/tree/3b7f630a1da54dd3124a12e3babe66c9a3ac4f13/pkg/gpu/mocknvml), pin commit 3b7f630a1da54dd3124a12e3babe66c9a3ac4f13. Chỉ dùng thư viện mock NVML của repository này; demo không chạy Kubernetes.

Build dùng Makefile của upstream (NO_PADDING=1), copy thư viện thành /opt/nvml/libnvidia-ml.so.1. LD_LIBRARY_PATH chọn thư viện này; MOCK_NVML_CONFIG trỏ profile YAML. Agent Sim gọi gpu.New và cùng Snapshot API như production, qua go-nvml/dlopen. Tham khảo [upstream configuration](https://github.com/NVIDIA/k8s-test-infra/blob/3b7f630a1da54dd3124a12e3babe66c9a3ac4f13/docs/configuration.md).

JSON trong internal/simulator chỉ lưu simulated Docker containers. Nó không cung cấp GPU count/model/memory và không thay NVML reader.

## Chạy một hoặc nhiều fake server

Tại workspace root:

~~~sh
python scripts/configure.py
docker compose up --build -d control-plane agent-a100
docker compose up -d agent-t4
python scripts/acceptance.py
~~~

Mỗi host có mount state độc lập và UUID profile riêng. Server A100 có External GPU0, server T4 trống. AIWM_SIM_EXTERNAL_GPU_INDEXES seed Docker grants sau khi đọc NVML nhưng trước Agent register. PID external 10000 tương ứng cấu hình process trong profile A100. Managed job mới tạo simulated Docker container, phát event, Collector inventory lại và scheduler nhận actual state.

Để demo host unknown và unhealthy thêm, root có compose.scenarios.yaml:

~~~sh
docker compose -f compose.yaml -f compose.scenarios.yaml up -d agent-unknown agent-unhealthy
~~~

T4 còn lại vẫn có thể nhận jobs. Hãy selector site=unknown hoặc site=unhealthy nếu muốn quan sát đúng host thử nghiệm. Unknown GPU1 và unhealthy GPU0 không được chọn. Các profile không tăng/giảm VRAM theo job; Docker grant đủ để reserve nguyên GPU dù mock util bằng 0.

Dừng riêng hai host phụ khi xong để trở lại pool A/B:

~~~sh
docker compose -f compose.yaml -f compose.scenarios.yaml stop agent-unknown agent-unhealthy
~~~

Các server cũ vẫn được giữ last-known trong inventory rồi offline; Summary tổng GPU bao gồm last-known inventory. Không có delete server API trong MVP.

## Profile

Trong backend/deploy/simulation/profiles:

| YAML | Physical GPU | Process / condition |
|---|---|---|
| a100-external.yaml | 4 NVIDIA A100-SXM4-40GB, host1 UUID | PID10000 GPU0 map External |
| t4-empty.yaml | 2 Tesla T4, host2 UUID | Không có consumer |
| a100-unknown.yaml | 4 A100, host3 UUID | PID424242 GPU1 không map được owner |
| a100-unhealthy.yaml | 4 A100, host4 UUID | GPU0 có uncorrectable ECC |

Nếu cần đổi số GPU/model/VRAM, sửa YAML theo schema NVIDIA và đổi UUID cho host mới. Không dùng flag sinh GPU trong Go. Giữ deviceCount và mảng devices nhất quán. Restart fake host để library đọc config mới; dùng state file mới khi thay topology để tránh Docker grants của topology cũ.

## Kiểm thử và failure injection

Dockerfile.test chạy production NVML adapter trên cả bốn profile. Agent inventory unit tests kiểm tra numeric mapping, unknown memory/util và External protection. Acceptance HTTP A–E/H và recovery F/G dùng hai fake server thực chạy trong Compose.

AIWM_SIM_REJECT_STARTS=1 khiến Docker adapter từ chối launch đầu tiên sau process startup: job phải FAILED, assignment RELEASED; External không bị ảnh hưởng. Dùng host riêng với state/labels riêng nếu demo failure, không thay enrollment hoặc token runtime trong log.

## Phần mô phỏng và phần thật

Thật: CP HTTP/auth, scheduler, durable transaction, Agent core, NVML API call/shared library, discovery/accounting, queue/ACK/reconciliation, Next BFF và browser.

Mô phỏng: NVIDIA hardware/telemetry qua upstream lib; DockerRuntime không gọi Docker Engine, không pull image hoặc chạy CUDA. Agent restart test chứng minh identity/state preservation trong simulator; không đo continuity hay throughput của CUDA process thật. Production Docker SDK có adapter tests; test GPU thật cần máy Linux NVIDIA riêng.

