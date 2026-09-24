# Current Task

Dashboard fleet + capability input tối giản + account context + UI tiếng Việt (23/09/2026).
Checkpoint đầu phiên: 6d7d75e; working tree ban đầu clean.

## DONE

- Header chung: ADMIN dùng username; user đơn vị dùng organization.code; bỏ context VTT mặc định khỏi admin.
- Tổng quan chỉ còn fleet KPIs và bảng theo đơn vị (ADMIN)/máy chủ (user); bỏ toàn bộ Job/events/attention blocks.
- Tổng hợp từ Server[] API đã scope ở backend. Util TB tính từng mẫu GPU hợp lệ, giữ 0%, không có mẫu hiển thị “—”; không thêm lịch sử/analytics.
- Form chỉ có bốn capability controls: số GPU, VRAM GB, AUTO/HIGH_PERFORMANCE, FP8. Giữ Training/Inference, image/command/env, policy, neededAt và duration. Không gửi CPU/RAM.
- Catalog backend tập trung: AUTO không FP8 không ép model, HIGH_PERFORMANCE nhóm H100/H200; FP8 match alias được khai báo. Profile cũ giữ tương thích API/script nhưng không xuất hiện trong options/UI.
- Preview debounce 400 ms, hủy request cũ; input thay đổi không dùng kết quả cũ để submit. Preview vẫn dùng time planner hiện có, không tạo reservation.
- Dọn technical notes trên tất cả console pages; giữ hành động và trạng thái nghiệp vụ, xác nhận dừng/thu hồi. Enum backend không đổi.
- Go tests liên quan PASS, không đổi Agent/NVML/Docker/auth/Policy formula/placement/calendar.

## PARTIAL

- Không còn implementation dở dang trong scope UI/capability này. Browser đã PASS 4/4 cases sau khi sửa liên kết label/control trong form.
- Telemetry hiện không có validity flag riêng từng field: chỉ kiểm số hữu hạn 0–100, GPU Healthy, server không OFFLINE và có inventory timestamp. Không suy ra lịch sử hoặc freshness timeout mới.
- Không chạy toàn Docker fake demo, GPU thật hay production frontend build trong task UI này.

## TODO

- Kiểm tra chấp nhận bằng tài khoản demo trên stack thật sau khi khởi động lại với code mới; chưa chạy full Docker E2E trong task này.

## TESTS

- PASS: go test ./internal/capability ./internal/domain ./internal/application ./internal/httpapi ./internal/store/memory ./internal/store/durable ./internal/policy.
- PASS: npm.cmd run typecheck; ESLint các file frontend liên quan.
- PASS: node --experimental-strip-types --test tests/contracts.test.mjs tests/fleet.test.mjs (11 + 2 tests).
- PASS: Playwright tests/e2e/fleet-capability.spec.ts — 4 cases (3 pass ở lần đầu, case form pass sau sửa accessibility), Edge/Next dev cổng 3100, mock BFF payload. Các màn list/detail/settings/onboarding cũng được duyệt.
- PASS: typecheck sau sửa cuối; ESLint targeted; git diff --check.
- Lỗi đã sửa: label bao select/error khiến accessible name không ổn định; Field dùng useId + htmlFor/id + aria-describedby.
- Authorization thật kiểm tra bằng Go HTTP tests. Không suy diễn UI mock thành live backend E2E.
- Next dev cổng 3100 do phiên này tạo đã dừng; không đụng Docker/container hay dịch vụ khác.

## FILES CHANGED

- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/containers/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/gpus/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/onboarding/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/organizations/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/queue/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/scheduler/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/servers/[serverId]/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/servers/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/settings/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/workloads/[jobId]/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/workloads/new/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/(console)/workloads/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/dashboard/metric-card.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/layout/brand.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/layout/connection-status.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/layout/topbar.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/ui/badge.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/ui/page.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/allocation-preview.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/job-form.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/job-lifecycle.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/config/navigation.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/features/dashboard/fleet.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/features/dashboard/use-dashboard.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/features/identity/organization-context.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/features/identity/session.tsx`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/api-client.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/types.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/jobs/form-schema.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/utils/display.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/tests/contracts.test.mjs`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/tests/e2e/fleet-capability.spec.ts`
- `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/tests/fleet.test.mjs`
- `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml`
- `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/application/capability_test.go`
- `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/capability/catalog.go`
- `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/capability/catalog_test.go`
- `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/domain/allocation.go`
- `docs/ALGORITHMS.md`
- `docs/API_TESTING_POSTMAN.md`
- `docs/IMPLEMENTATION_STATUS.md`
- `docs/SYSTEM_DESIGN.md`

## NEXT STEP

Khởi động lại demo với code mới và kiểm tra chấp nhận giao diện bằng admin/VTT. Không audit lại backend; task UI/capability đã hoàn tất.

---

## Checkpoint trước: Time-based Planning


Time-based Resource Allocation Planning + Workload Execution — tiếp tục implementation dang dở từ checkpoint 40d4a0a (23/09/2026).

CURRENT MILESTONE = WORKING CHECKPOINT: future reservation works and does NOT execute early.
PLANNING / TIME_RESERVATION / POLICY_CONFLICT / EXECUTION_TRIGGER = WORKING qua focused tests.
FRONTEND = DONE ở scope form/list/detail, typecheck/contracts/lint PASS.
LIVE_FAKE_TIME_PLANNING = CHƯA CHẠY. Docker hiện không có container chạy khi kiểm tra; không rebuild/start lại toàn demo trong phiên này.

## DONE

- Recover chỉ status/log/status file và modified/untracked files; giữ phần implementation phiên trước.
- Reuse neededAt + ttlSeconds (requested duration giây), interval [start,end), một helper overlap; giữ fractional seconds. Start tolerance quá khứ 60 giây; interval phải chưa kết thúc; max duration theo config cũ.
- Assignment.StartAt/EndAt và PLANNED; giữ future calendar riêng với GPU.State. Không duplicate Assignment/Reservation entity.
- Greedy Policy ordering + organization hard constraint + resource/capability/time filter + bốn strategy cũ.
- ReplanReservations atomic batch dưới runtime lock; durable rollback nếu disk lỗi, persistence qua restart. Chỉ QUEUED/future PLANNED chưa bắt đầu được đổi lịch; không preempt RUNNING.
- Execution controller nối ScheduleOnce/Reconcile; revalidate start và poll. Đến giờ mới STARTING + GPU/Assignment RESERVED + START atomically. Preview dùng cùng planner nhưng không mutation.
- Hết EndAt: cancel chưa dispatch; execution đã giao/chạy gửi graceful STOP. Release trên observed exit hoặc absence đủ mới sau ACK. Lost START ACK có đường STOP sau expiry, không redeliver START muộn.
- Guard queue expiration kiểm lại status dưới lock: snapshot QUEUED cũ không được FAILED/release Job đã dispatch. Regression TestStaleQueuedExpirationCannotReleaseDispatchedJob PASS.
- External/unknown/unhealthy không được allocate; expiry không stop External. Agent/NVML/Docker/auth/policy/scorer code không đổi.
- API/OpenAPI thêm requestedStartAt/requestedEndAt, assignment.startAt/endAt/PLANNED, preview AVAILABLE/CONFLICT; giữ request ttlSeconds và execution spec.
- Frontend form giữ image/command/env/CPU/RAM, start datetime, duration số với preset; list/detail thể hiện “Đã lên lịch · Chưa chạy”, interval và thời lượng; bỏ score khỏi lịch.
- scripts/demo.py planning: scenario dùng API thật, A/N2 + C/N3 + B/N1 tranh 3 H100 VTT, future/no early → RUNNING → automatic STOP/release. Không sửa inventory/frontend giả, chỉ cleanup Job của scenario.
- Folder Postman Planning mới trong collection cũ: 14 requests; user thường chờ ticker qua GET, không gọi admin run-once.
- Cập nhật đúng sáu canonical docs trong scope, tiếng Việt. Giữ docs/status lịch sử phía dưới; không thêm MD hoặc sửa README.

## PARTIAL

- Scenario Python/Postman mới đã kiểm syntax/contract nhưng CHƯA chạy trên stack Docker thật; chưa có browser E2E cho time planning.
- Không có Linux GPU thật: chưa chứng nhận runtime timing/STOP thực tế trên GPU host.
- EndAt kích hoạt graceful STOP, không bảo đảm deadline realtime khi pull lâu, CP/Agent offline hoặc STOP đang chạy. Giữ claim đến actual confirmation, không coi GPU FREE chỉ do hết giờ.
- Job/Assignment cũ có zero interval giữ legacy execution; không tự diễn giải TTL thành deadline mới. Tạo Job mới để test planning.
- Metadata/policy facts ngoài runtime transaction; quota DEVELOPMENT_CONFIG vẫn tĩnh. Single-writer memory/durable MVP, không HA/interval tree/preemption.
- Không chạy full benchmark, toàn bộ integration, Linux race hoặc production frontend build.

## TODO

- Chạy fake time-planning scenario theo TESTING_RUNBOOK; ghi PASS/FAIL thực tế vào .cache/multi-server-demo/last-planning-check.json và status.
- Sau đó kiểm trên Linux GPU thật khi có host. Không audit/refactor hoặc chạy lại mọi phase cũ.

## FILES CHANGED

Backend: AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/
- api/openapi.yaml.
- internal/domain/model.go, reservation.go, reservation_test.go.
- internal/ports/repository.go.
- internal/application/controlplane.go, allocation.go, allocation_test.go, planning.go, planning_test.go.
- internal/store/memory/store.go, accounting.go, lifecycle.go, planning.go.
- internal/store/durable/repository.go, planning_test.go.
- internal/httpapi/job_view.go, allocation_test.go.

Frontend: AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/
- src/lib/api/types.ts; src/lib/jobs/form-schema.ts.
- src/components/workloads/job-form.tsx, allocation-preview.tsx, job-lifecycle.tsx.
- src/app/(console)/workloads/page.tsx và [jobId]/page.tsx.
- tests/contracts.test.mjs.

Workspace:
- scripts/demo.py; postman/AIWM.postman_collection.json.
- docs/SYSTEM_DESIGN.md, ALGORITHMS.md, DOMAIN_MODEL.md, API_TESTING_POSTMAN.md, TESTING_RUNBOOK.md, IMPLEMENTATION_STATUS.md.
- Binary local ignored: .cache/aiwm-planning-server.exe; không chạy process này.

## TESTS / COMMANDS VERIFIED

PASS tại backend:
- go test ./internal/domain ./internal/application ./internal/httpapi ./internal/store/memory ./internal/store/durable ./internal/policy
- Hai regression bổ sung cuối: go test ./internal/application -run 'TestExpiredBlockedReservation|TestRequestedTimeValidation' -count=1
- Sau guard race cuối: go test ./internal/application ./internal/store/memory ./internal/store/durable ./internal/httpapi — PASS; rebuild Control Plane — PASS.
- go vet ./internal/domain ./internal/application ./internal/httpapi ./internal/store/memory ./internal/store/durable
- go build -o ../../.cache/aiwm-planning-server.exe ./cmd/aiwm-server
- gofmt chỉ các file Go modified/untracked của task.

PASS tại frontend:
- npm.cmd run typecheck
- node --experimental-strip-types --test tests/contracts.test.mjs — 11 tests.
- ESLint targeted bảy source files frontend thay đổi.

PASS static: OpenAPI YAML parse/time fields; Python AST demo.py; JSON collection/14 request planning; git diff --check.
Không reset/revert/xóa resource. Không chạy Docker prune, migrations hoặc network research.

## NEXT_STEP

Từ workspace root, khởi động lab với code mới rồi chạy đúng scenario mới:

~~~powershell
python scripts/demo.py up
python scripts/demo.py planning
~~~

Không dùng --skip-build vì Control Plane đã đổi. Scenario đòi vtt-gpu-01 fresh có 3 H100 FREE; không tự stop Job khác để đạt điều kiện. Xem docs/TESTING_RUNBOOK.md mục “Kiểm thử Resource Allocation Planning theo thời gian”. Sau PASS ghi kết quả thực tế vào status; không sửa lại policy/scorer/Agent.

---

# Lịch sử checkpoint release (18/09/2026)

REAL AGENT RELEASE / DISTRIBUTION — tiếp tục checkpoint đã commit/push, không sửa lại Fake Demo, policy, scheduler hoặc frontend (18/09/2026).

## Last Working Checkpoint

RESUMED_FROM = 53486aa (working tree sạch khi recover).
FAKE_DEMO_STATUS = WORKING — checkpoint 7fd1a79, đã kiểm chứng 17/09; phiên này không chạy lại.
REAL_RELEASE_STATUS = WORKING — production Linux amd64 binary, glibc 2.31, checksum và safe validation PASS.
REAL_INSTALL_STATUS = PARTIAL — installer đã kiểm chứng trong container riêng; còn xác nhận Docker/NVML/permissions trên GPU host thật.
REAL_E2E_RUNBOOK = DONE.
Checkpoint phiên này: wip: verified Linux Agent release and deployment runbook (không push).

## DONE

- Recover đúng thứ tự status/branch/log/AGENTS/status; không scan repo hoặc đọc diff khi clean.
- Giữ ReleaseControlPlaneURL, URL precedence env > build default > localhost, --version, preflight/MachineID đã có trong checkpoint.
- Sửa builder không tồn tại: Go toolchain 1.26.6-bookworm + buildpack-deps:bullseye (GCC/glibc 2.31 có sẵn), CGO=1, Linux amd64, chỉ build cmd/aiwm-agent.
- Build helper dùng Docker Desktop Windows CLI nếu WSL thiếu socket; chuyển path bằng wslpath, không đổi Docker/WSL host config.
- Artifact generic, không organization/token/password. Giữ các output directory của lần build lỗi, không ghi đè/xóa.
- Installer có sẵn đã PASS đường dẫn/mode0600/state0700/account/systemd/không tự start runtime/không ghi đè trong disposable container, probe stub được đánh dấu rõ.
- Binary thật chạy --version trên Debian không có Go; thiếu Docker/NVML trả DEGRADED, unschedulable, exit 1.
- scripts/agent-admin.py: public API, password nhập ẩn, token file riêng mỗi server, không in secret, không ghi đè token. Normal user để backend derive ownership.
- demo/workloads/real-gpu-smoke.json: image CUDA có Linux amd64 manifest, nvidia-smi -L + sleep 300, 1 GPU; không benchmark.
- Runbook: CP host/network bind → stable URL → central build → unique enrollment → transfer/install/check/service → GPU E2E/stop/failure/reinstall.
- SYSTEM_DESIGN có release/deployment diagrams; TECH_STACK phân biệt development Go toolchain với runtime GPU host không cần Go.
- README documentation index chỉ cập nhật mô tả tìm deployment/tech stack; không rewrite README.

## PARTIAL

- Không có Linux NVIDIA GPU host thật được cung cấp: chưa chứng nhận successful NVML discovery, container GPU execution, systemd permissions thực hoặc CUDA.
- Artifact hiện inject http://127.0.0.1:8080 chỉ để validation; không phân phối nguyên artifact này cho GPU server ở máy khác.
- Baseline ABI glibc 2.31, Linux x86_64; Ubuntu/Debian và RHEL/Rocky/Alma cần đáp ứng ABI, Docker/NVML/runtime nvidia. Không claim mọi distro, ARM64/musl hoặc CDI-only.
- Installer positive test dùng probe stub, không thay thế preflight trên server. Các tests thật thiếu Docker/NVML chỉ xác nhận fail-closed.
- Core vẫn dùng organizationId; admin submit theo home organization; không đổi scope này.

## TODO

- Deploy CP trên host trung tâm, chọn URL thật; build artifact với URL đó và onboarding một GPU server.
- Chạy REAL GPU SERVER E2E trong TESTING_RUNBOOK, ghi kết quả thực tế RUNNING → release và failure/reconnect.
- Không cần làm lại Fake Demo hoặc mở rộng UI/organization trong task này.

## Commands Verified

PASS:
- Backend: go test ./internal/agent/config ./internal/agent ./internal/agent/state.
- python -m unittest discover -s scripts -p test_agent_admin.py — 4 tests.
- WSL: bash scripts/build-agent-release.sh --control-plane-url http://127.0.0.1:8080 --output-dir dist/aiwm-agent-0.2.0-linux-amd64-checked.
- bash -n scripts/build-agent-release.sh scripts/install-agent-linux.sh scripts/test-agent-install.sh.
- Tại artifact directory: sha256sum -c SHA256SUMS — 4 files OK.
- Docker Debian bullseye-slim: production aiwm-agent --version; --check không Docker/NVML => DEGRADED/exit 1.
- Docker buildpack-deps:bullseye, network none, release/test script bind read-only: bash /test.sh — installer positive/no-overwrite với probe stub.
- Compose config: CP/Console bind 0.0.0.0 được, PostgreSQL vẫn 127.0.0.1; không start/recreate deployment.
- Docker manifest nvidia/cuda:12.4.1-base-ubuntu22.04 có linux/amd64.
- Các temporary test containers dùng --rm, không mount Docker socket hoặc GPU host.

Lỗi build đã xử lý: tag Go bullseye không tồn tại; apt mirror Bullseye trả 404; builder cuối dùng compiler image đã có dependency. WSL thiếu Docker socket được xử lý bằng Docker Desktop CLI fallback. Không restart Docker/driver hoặc global cleanup.

## Artifact Path

dist/aiwm-agent-0.2.0-linux-amd64-checked/
- aiwm-agent (0.2.0, linux/amd64, CGO)
- install-agent-linux.sh
- aiwm-agent.service
- BUILD_INFO.txt
- SHA256SUMS

dist/ được gitignore; không commit binary/token.

## Control Plane URL Config

AIWM_CONTROL_PLANE_URL > internal/agent/config.ReleaseControlPlaneURL > http://localhost:8080.
Release build inject -X github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/config.ReleaseControlPlaneURL.
Artifact validation dùng loopback; central deployment cần URL reachable từ GPU host.

## Install Command

Trên GPU host, sau nhận release và enrollment riêng:

~~~bash
sudo ./install-agent-linux.sh --binary ./aiwm-agent --enrollment-token-file "$HOME/aiwm-enrollment.token"
sudo /usr/local/bin/aiwm-agent --check
sudo systemctl enable --now aiwm-agent
~~~

URL optional --control-plane-url; có thể bỏ token options để nhập ẩn. Giữ /etc/aiwm-agent/agent.env và /var/lib/aiwm-agent qua restart.

## Demo URLs / Demo Accounts

Fake demo lịch sử: http://127.0.0.1:3000/login; xem DEMO_ACCOUNTS.md. Phiên này không khởi động lại stack đang dừng.

## Files Changed

- Backend Dockerfile.agent-release.
- scripts/build-agent-release.sh; mode executable cho install-agent-linux.sh; agent-admin.py; test_agent_admin.py; test-agent-install.sh; .gitattributes.
- demo/workloads/real-gpu-smoke.json.
- docs/TESTING_RUNBOOK.md, SYSTEM_DESIGN.md, TECH_STACK.md, IMPLEMENTATION_STATUS.md; README documentation index.
- Không sửa policy/scheduler/frontend/runtime ownership.

## NEXT STEP

Trên máy AIWM có URL CP thật, chạy:
bash scripts/build-agent-release.sh --control-plane-url "$CONTROL_PLANE_URL" --output-dir dist/agent-release-real-01
Sau đó theo TESTING_RUNBOOK.md mục “REAL GPU SERVER E2E — VTT-GPU-01” trên Linux NVIDIA host thật. Không clone/build trên GPU host.

---

## Lịch sử trước Phase A

# Trạng thái implementation

## Task hiện tại: Organization boundary + auth + onboarding (2026-09-17)

CURRENT TASK: tiếp tục mentor scope từ WIP checkpoint `fffd420`.

RECOVERY: đã đọc AGENTS/status, chạy git status --short, git diff --stat, git diff và git log -3; working tree sạch khi resume. Checkpoint đã chứa core implementation và phần lớn canonical docs/runbook, không làm lại các lát cắt đó.

DONE:
- Lát cắt 1: domain Organization/User/Principal, PostgreSQL metadata schema/adapter, PBKDF2 password hash, session 8 giờ, login/me/logout, ADMIN quản lý organization/user; CLI migrate/bootstrap đã nối trong aiwm-server.
- Lát cắt 2: enrollment bind organization/machine server-side; ownership bất biến, scope API tại application; Job lấy organization từ Principal; hard filter ở preview/scheduler và kiểm tra lại trong CommitAssignment.
- Lát cắt 3: production preflight báo SUPPORTED/DEGRADED/UNSUPPORTED, đọc OS/architecture/kernel/cgroup, Docker version/API/runtime nvidia, NVML/GPU discovery; không chạy shell probe hoặc test container. Startup/reconnect cần FULL inventory; heartbeat quay lại chưa đủ để lease START. Retry startup/heartbeat/command có backoff+jitter; inventory định kỳ giữ nhịp cũ.
- Lát cắt 4: login nội bộ, BFF cookie HttpOnly/session, current organization/user/role; ADMIN có bộ lọc đơn vị và CRUD metadata tối thiểu; onboarding cấp token, không nhập GPU; query tài nguyên dùng organization filter do backend authorize.
- Compose/configure: PostgreSQL service, credentials sinh local, migrate/bootstrap tường minh và token riêng từng Agent Sim. Không chạy script hoặc Compose trong task.
- SYSTEM_DESIGN/DOMAIN_MODEL/ALGORITHMS/TECH_STACK đã cập nhật ownership; CORPORATE_POLICY và TESTING_RUNBOOK đã có; API_TESTING_POSTMAN đã cập nhật phần auth/tenancy.
- Lát cắt 5 (resume): OpenAPI thêm session/organization/user/enrollment và ownership DTO; Postman có luồng login/enrollment/scoped resources/Job/logout, ADMIN và tenancy negatives. Fixture Agent cũ giữ lifecycle, thêm login/enrollment riêng. README chuyển sang runbook và index canonical.
- Kiểm tra tĩnh cuối: HTTP source/OpenAPI đều có 33 method endpoints. Bổ sung regression cases login/hash/disabled identity, session scope/logout/spoofing, enrollment ownership, organization filter trước scorer, commit conflict không mutation và heartbeat chưa có FULL inventory. Chưa chạy các tests này.
- Fixture scheduler/benchmark/allocation và memory safety đã gắn organization tường minh; không thêm fallback org rỗng trong production. Sửa ký tự onboarding và loại score khỏi recent events của summary. Không sửa công thức policy/placement.
- Canonical docs/traceability đã đồng bộ. ARCHITECTURE.md giữ đường dẫn chuyển tiếp sau khi nội dung hợp nhất vào SYSTEM_DESIGN/DOMAIN_MODEL/ALGORITHMS. Không xóa file hoặc dữ liệu; giữ docs lịch sử còn nội dung riêng.

PARTIAL:
- Tất cả thay đổi mới chỉ được đọc/sửa tĩnh, chưa chạy migration/build/test theo yêu cầu.
- Dependency lib/pq đã khai báo; chưa tải dependency hoặc cập nhật go.sum qua Go tool.
- Dữ liệu runtime cũ thiếu organization được giữ nguyên và không được schedule; không tự chuyển ownership.
- Automation lịch sử (doctor/acceptance/recovery/browser fixtures và các fixture chưa chuyển) chưa chứng nhận session/enrollment mới. Có runbook thủ công ba mức và Postman mới; chưa có script tự provision CP/database riêng cho protocol fixture.
- Chưa có kiểm chứng PostgreSQL transaction/bind concurrency thực tế hoặc GPU Linux thật. Preflight chỉ kiểm tra nền tảng, không chứng nhận CUDA/CDI-only.

TODO:
- Người dùng chạy dependency resolution, format/check/build/tests và migration theo TESTING_RUNBOOK khi cho phép; review go.mod/go.sum sau go mod tidy.
- Kiểm chứng local → fake Agent/NVML → real Linux GPU và failure/reconnect. Không dùng kết quả PASS lịch sử để kết luận scope mới đã đạt.

FILES CHANGED (phiên resume, 21 file; Backend/Frontend dùng hai project ở AGENTS.md):
- Backend: api/openapi.yaml; cmd/aiwm-scheduler-bench/main.go; internal/application/{allocation_test,scheduler_test,scheduler_safety_test,identity_test}.go; internal/httpapi/{server,identity_test}.go; internal/store/memory/safety_test.go.
- Frontend: src/app/(console)/onboarding/page.tsx.
- Workspace: README.md; postman/AIWM.postman_collection.json; postman/AIWM.local.postman_environment.json.
- Docs: ALGORITHMS.md, API_TESTING_POSTMAN.md, ARCHITECTURE.md, DOMAIN_MODEL.md, IMPLEMENTATION_STATUS.md, TECH_STACK.md, TESTING_RUNBOOK.md, TRACEABILITY.md.
- Core mentor implementation còn lại đã nằm trong checkpoint fffd420; không reset/revert.

NEXT STEP: chạy thủ công docs/TESTING_RUNBOOK.md LEVEL 1 (configure → PostgreSQL → go mod tidy → --migrate → --bootstrap-admin → CP/frontend/login), rồi LEVEL 2/3. Phiên này dừng ở implementation/documentation và inspection tĩnh; không chạy build/test/migration/network. Các kết quả test bên dưới thuộc task cũ, không chứng nhận task hiện tại.

---

## Lịch sử task trước

CURRENT TASK: Policy-driven GPU Allocation + Automatic Placement

CURRENT MILESTONE: COMPLETE — implementation và focused verification hoàn tất

DONE:
- Recover bằng AGENTS, status và Git diff; reuse scheduler/atomic reservation/Agent core hiện có.
- AllocationIntent + AllocationResources; enum/field validation backend authoritative, structured error.fields.
- Create DTO không nhận priority/strategy/serverSelector/resources.gpuModel/selected UUID hoặc quota claim.
- policy.Evaluator/FactsProvider: TRAINING/INFERENCE reason catalog, lane → Necessity → auxiliary priority → CreatedAt/ID.
- Importance/quota/waiting chỉ tính ở backend; normalize auxiliary product để FIFO không bị sai số float.
- capability.Resolver: profile/FP8 catalog tập trung; ResolvedModels persist private; Matches dùng chung Plan/commit.
- Strategy từ CP config; injectable SchedulingPolicy qua Options.PlacementPolicy/WithPolicy; không override hard constraints.
- Preview chỉ đọc; Submit validate/evaluate latest facts/resources rồi vào queue chung; cycle re-evaluate trước atomic reserve + Job + command.
- GET jobs/options, POST jobs/preview; OpenAPI/JobView/TypeScript client/BFF đồng bộ.
- Form tiếng Việt, catalog/limits động, panel ĐỐI CHIẾU TỰ ĐỘNG; không K/score/UUID/strategy; sửa input làm preview cũ không còn dùng cho submit.
- Queue/list/detail đổi sang necessityLabel từ backend; không redesign trang.
- Postman migrate các body cũ; thêm folder Allocation - Policy Preview Submit (25 requests); tổng 94 definitions.
- README root/backend, DOMAIN_MODEL, ARCHITECTURE, SCHEDULER, CONFIGURATION, API_TESTING_POSTMAN, TRACEABILITY và frontend-api-map cập nhật.
- Compose forward config policy/limits; demo scripts và browser fixture đổi input mới. Giữ các thay đổi có sẵn trước task, không commit/reset/restore Git.

PARTIAL:
- Không còn implementation dang dở trong scope. Các giới hạn demo và kiểm chứng chưa chạy được ghi ở TESTS/NEXT STEP.

TODO:
- Không còn TODO bắt buộc trong task; không tự mở rộng feature.

FILES CHANGED:
- Backend: domain/{allocation,model,dto}; policy/{engine,profiles,engine_test}; capability/{catalog,catalog_test}; application/{allocation,allocation_test,validation,scheduler,scheduler_safety_test,controlplane}; httpapi/{server,job_view,allocation_test,security_test}; memory/{store,safety_test}; integration/agent_controlplane_test; config/config; cmd/aiwm-server/main; api/openapi.yaml; README; docs/frontend-api-map.
- Frontend: components/workloads/{job-form,allocation-preview}; lib/jobs/form-schema; lib/api/{types,api-client,endpoints,proxy-policy}; workloads/list/detail và queue; tests/contracts.test.mjs; tests/e2e/console.spec.ts.
- Workspace: compose.yaml; scripts/{acceptance,recovery}.py; README; docs/{DOMAIN_MODEL,ARCHITECTURE,SCHEDULER,CONFIGURATION,API_TESTING_POSTMAN,TRACEABILITY,IMPLEMENTATION_STATUS}; postman collection/environment.

TESTS:
- PASS: focused Go application/policy/capability/httpapi/memory/durable/config/integration tests. Integration dùng httptest + fake runtime, không Docker.
- PASS: go vet các packages liên quan và cmd; go build ./cmd/...; go fmt touched packages.
- PASS: benchmark bốn strategy giữ kết quả First Fit 4/5, ba strategy còn lại 5/5.
- PASS: frontend 10 contract tests, npm run lint, focused ESLint sau sửa date validation và typecheck cuối.
- PASS: Newman 6.2.2 folder Allocation trên CP native Windows riêng + inventory fixture: 25 requests, 51 assertions, 0 failures; External vẫn protected. Log kết quả không có secret tại .cache/allocation-newman-result.json.
- PASS: 21 source/OpenAPI/doc routes, valid/negative request schemas, 113 Postman scripts parse, blank credential mẫu, doc links, Python demo syntax; git diff --check.
- Không chạy lại Docker/full acceptance/full browser E2E/production frontend build/Linux race/NVML integration trong task này. Số liệu cũ 195 Newman assertions thuộc contract trước intent.
- Helper scripts/test logs ở .cache là artifact local, không dependency runtime/project.

NEXT STEP:
- Task đã hoàn tất. Người dùng có thể test theo docs/API_TESTING_POSTMAN.md; không cần audit lại hoặc làm lại phần đã có.
- Để test trên laptop: rebuild CP image khi đang dùng Docker cũ; restart frontend; import Postman, set access_token, chạy Health → Security - Token hợp lệ → Allocation 01..07.
- Limitation thực tế: Planning/Quota là DEVELOPMENT_CONFIG tĩnh; catalog compatibility chưa chứng nhận measured performance; NeededAt chưa hẹn start; TTL chưa tự stop/reclaim. Các mục này ngoài scope, không tự triển khai thêm.
