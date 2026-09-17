# Current Task

Đơn giản hóa AIWM theo thứ tự: fake demo → real Agent release → organization đơn giản → UI tiếng Việt → docs.

## Last Working Checkpoint

FAKE_DEMO_STATUS = WORKING (17/09/2026, Docker Desktop + Windows + Ubuntu WSL2).
Checkpoint Phase A: `wip: working multi-server fake GPU demo`; nền trước đó `fffd420`.
REAL_RELEASE_STATUS = PARTIAL — production Agent/preflight có sẵn; chưa có release builder/installer đơn giản.

## DONE

- Sửa xung đột port: native PostgreSQL Windows giữ 5432, demo dùng 15432; synchronize env local, không xóa dữ liệu.
- Một lệnh up/down; 4 organizations, 5 demo accounts, 6 Agent Sim, 26 GPUs. Scenario configurable; token enrollment riêng từng server, lưu local.
- Cùng Runner/InventoryCollector/GPU reader và NVIDIA mock NVML; chỉ Docker runtime là simulator.
- Kiểm chứng backend/BFF/browser và failure/reconnect; external không bị stop; GPU được reserve/release qua state thật.
- Policy/công thức/placement không thay đổi. OrganizationId hard filter có sẵn được giữ và kiểm chứng; không rewrite model.
- Không đụng PostgreSQL Windows, Docker resource ngoài project hoặc xóa volume.

## PARTIAL

- Real GPU E2E chưa chạy: laptop không có GPU NVIDIA dành cho kiểm thử.
- REAL DEPLOYMENT cần generic binary build tập trung + default URL/override + installer/systemd, không cần source/Go trên GPU host.
- Core dùng organizationId bất biến; organizationCode là metadata/seed và hiển thị. Chưa đổi schema sang code; ADMIN submit hiện theo home organization.
- UI chạy được nhưng chưa hoàn tất giản lược/Việt hóa toàn bộ thuật ngữ và bỏ developer notes.
- Các docs/fixtures lịch sử không chứng nhận flow mới; mục fake mới trong runbook là authoritative.

## TODO

- Phase B: build-agent-release, install-agent-linux, hướng dẫn Control Plane network URL/enrollment riêng/release distribution và real E2E.
- Sau B: cân nhắc tối thiểu nhu cầu orgCode/admin submit; UI Việt hóa; diagrams/docs canonical tương ứng.
- Chỉ sửa phần cần thiết, không migrate/rewrite runtime store.

## Commands Verified

- `python scripts/configure.py --postgres-port 15432`; Compose postgres healthy.
- Build Compose control-plane, agent-a100 (image dùng chung), console.
- `python scripts/demo.py up --skip-build --set-demo-passwords` (chuyển account lab cũ một lần).
- `wsl -d Ubuntu --cd /mnt/f/Viettel/VDT/demo-project -- bash scripts/demo-up.sh --skip-build`.
- `wsl -d Ubuntu --cd /mnt/f/Viettel/VDT/demo-project -- python3 scripts/demo.py check`: PASS.
- `python scripts/demo.py check`: PASS 7 nhóm.
- `python scripts/demo.py check --base-url http://127.0.0.1:3000/api/aiwm`: PASS 7 nhóm.
- `python scripts/demo.py failure`: PASS RESERVED, OFFLINE giữ allocation, Agent/CP restart.
- Frontend `npm.cmd run typecheck`: PASS; Edge `npm.cmd run test:e2e -- tests/e2e/demo.spec.ts`: 2 PASS.
- Backend `go test ./internal/application ./internal/httpapi ./internal/store/memory`: PASS.
- Không chạy benchmark/stress/global cleanup.

## Demo URLs

Console http://127.0.0.1:3000/login; CP http://127.0.0.1:8080; health /healthz; PostgreSQL localhost:15432.

## Demo Accounts

admin/vtt/vds/vtnet/vtit — password mẫu công khai `AIWM-Demo-2026!`.
Xem [DEMO_ACCOUNTS.md](DEMO_ACCOUNTS.md). Không dùng cho production.

## Files Changed

Phase A: compose.yaml; scripts/configure.py, acceptance.py (SessionAPI), demo.py, demo-up.sh, demo-down.sh; config/demo-users.json; demo/scenarios/multi-server.json; backend go.mod/go.sum và memory/store_test.go; frontend tests/e2e/demo.spec.ts; docs/DEMO_ACCOUNTS.md, TESTING_RUNBOOK.md, IMPLEMENTATION_STATUS.md; README.md.
Giữ nguyên thay đổi có sẵn từ phiên resume trước; checkpoint chỉ stage phần của Phase A.

## NEXT STEP

NEXT STEP = real GPU server support / central Agent release.
Đọc trực tiếp `internal/agent/config/config.go`, `cmd/aiwm-agent/main.go`, `deploy/agent/aiwm-agent.service` trong backend, rồi thêm `scripts/build-agent-release.sh`, `scripts/install-agent-linux.sh`. URL: env override > build default > localhost. Không sửa lại fake demo đã PASS.

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
