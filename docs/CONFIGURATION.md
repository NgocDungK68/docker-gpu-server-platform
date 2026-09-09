# Configuration

Không đọc env rải rác trong UI/business logic. Backend/internal/config/config.go đọc .env; backend/internal/agent/config/config.go đọc .env.agent; environment của process ưu tiên hơn dotenv. Frontend dùng chuẩn Next.js (.env.local) và src/config/{server,app}.ts.

Từ root chạy python scripts/configure.py để tạo token ngẫu nhiên cho demo, ghi root .env, backend .env/.env.agent, frontend .env.local. Không commit các file secret; .example chỉ có placeholder/giá trị trống. Script giữ giá trị cũ nên khi đã tự sửa cần đồng bộ credential thủ công.

## Control Plane

| Env | Default | Ý nghĩa |
|---|---|---|
| AIWM_HTTP_ADDR | 127.0.0.1:8080 | Bind; root Compose dùng :8080 bên trong, publish loopback |
| AIWM_STATE_FILE | .local/control-plane.gob | Snapshot v1, một process writer |
| AIWM_API_TOKEN | Bắt buộc | Operator/public API bearer |
| AIWM_ENROLLMENT_TOKEN | Bắt buộc | Đăng ký Agent |
| AIWM_TLS_CERT_FILE / AIWM_TLS_KEY_FILE | Trống | Phải đặt cả cặp để bật HTTPS trực tiếp |
| AIWM_AGENT_HEARTBEAT_INTERVAL | 5s | Khoảng heartbeat trả khi enroll |
| AIWM_AGENT_OFFLINE_AFTER | 20s | Giới hạn heartbeat/inventory freshness, > heartbeat |
| AIWM_COMMAND_LEASE | 15s | Delivery retry lease |
| AIWM_SCHEDULER_INTERVAL | 2s | Queue scheduling cycle |
| AIWM_RECONCILE_INTERVAL | 5s | Mark offline cycle |
| AIWM_SCHEDULER_STRATEGY | best-fit | first-fit, best-fit, bin-pack, fragmentation-aware |
| AIWM_CORS_ORIGINS | http://localhost:5173,http://localhost:3000 | Browser origin allowlist; BFF dùng same-origin |
| AIWM_LOG_LEVEL | info | debug/info/warn/error |

Duration dùng cú pháp Go như 500ms, 2s, 1m, phải dương. OfflineAfter phải lớn hơn heartbeat; inventory interval cũng nên nhỏ hơn OfflineAfter để tránh liên tục unschedulable.

## Agent production và simulator

| Env | Default | Ý nghĩa |
|---|---|---|
| AIWM_CONTROL_PLANE_URL | http://localhost:8080 | URL root HTTP/HTTPS |
| AIWM_ENROLLMENT_TOKEN | Bắt buộc | Khớp CP |
| AIWM_AGENT_MACHINE_ID | /etc/machine-id, fallback hostname | Identity ổn định và duy nhất |
| AIWM_AGENT_NAME | hostname | Tên inventory |
| AIWM_AGENT_LABELS | Trống | CSV key=value, ví dụ site=hanoi,pool=training |
| AIWM_AGENT_STATE_FILE | /var/lib/aiwm-agent/state.json | Token/sequence/results; file riêng từng Agent |
| AIWM_AGENT_HEARTBEAT_INTERVAL | 5s | Outbound heartbeat |
| AIWM_AGENT_INVENTORY_INTERVAL | 15s | Full Docker/NVML snapshot |
| AIWM_AGENT_COMMAND_POLL_INTERVAL | 2s | Poll typed commands |
| AIWM_AGENT_REQUEST_TIMEOUT | 30s | Timeout mỗi request |
| AIWM_DOCKER_ENDPOINT | unix:///var/run/docker.sock | Chỉ local unix/npipe; production Agent cần Linux NVML |
| AIWM_UNKNOWN_MEMORY_MIB | 256 | Usage không attribution đạt ngưỡng bị chặn |
| AIWM_UNKNOWN_UTILIZATION_PCT | 5 | Util không attribution đạt ngưỡng bị chặn |
| AIWM_AGENT_TLS_CA_FILE | Trống | CA thêm ngoài system roots |
| AIWM_AGENT_TLS_CERT_FILE / AIWM_AGENT_TLS_KEY_FILE | Trống | Cặp client cert cho gateway mTLS |
| AIWM_LOG_LEVEL | info | Structured logger |

Agent command batch 10, history tối đa 1000 và event debounce 500ms là internal bounded defaults trong composition/core; chưa expose env. Agent không tự retry tạo job terminal.

## NVIDIA mock / Docker runtime simulation

| Env | Giá trị / default | Ý nghĩa |
|---|---|---|
| MOCK_NVML_CONFIG | Bắt buộc | YAML của NVIDIA nvml-mock |
| LD_LIBRARY_PATH | /opt/nvml trong image | Chọn mock libnvidia-ml.so.1 |
| AIWM_SIM_EXTERNAL_GPU_INDEXES | Trống | Seed external Docker grant từ UUID đã đọc NVML |
| AIWM_SIM_RUNTIME_STATE_FILE | AgentStateFile + .docker.json | State riêng cho simulated Docker |
| AIWM_SIM_REJECT_STARTS | 0 | Số lần start mới bị adapter từ chối, phục vụ test release |

Mỗi fake host cần machine ID, Agent state, runtime state và bộ UUID riêng. Không dùng cùng physical UUID cho hai server.

## Frontend

| Env | Default | Boundary |
|---|---|---|
| AIWM_API_BASE_URL | http://localhost:8080 | Next server; không kèm /api/v1 |
| AIWM_API_TOKEN | Trống, cần khớp CP để API hoạt động | Next server; không dùng NEXT_PUBLIC cho token |
| AIWM_API_TIMEOUT_MS | 30000 | Request/health timeout dương |
| NEXT_PUBLIC_REFRESH_INTERVAL_MS | 5000 | Client polling, >=500, giá trị đóng tại build |
| AIWM_CONSOLE_URL | http://127.0.0.1:3000 | Chỉ Playwright |
| AIWM_BROWSER_CHANNEL | Chromium của Playwright | msedge để dùng Edge đã cài |

Không còn mock frontend data mode. Đổi server env cần restart Next; đổi NEXT_PUBLIC cần rebuild production bundle.

