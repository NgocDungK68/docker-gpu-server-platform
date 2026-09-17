# Trạng thái implementation

## Task hiện tại: Organization boundary + auth + onboarding (2026-09-17)

DONE:
- Lát cắt 1: domain Organization/User/Principal, PostgreSQL metadata schema/adapter, PBKDF2 password hash, session 8 giờ, login/me/logout, ADMIN quản lý organization/user; CLI migrate/bootstrap đã nối trong aiwm-server.
- Lát cắt 2: enrollment bind organization/machine server-side; ownership bất biến, scope API tại application; Job lấy organization từ Principal; hard filter ở preview/scheduler và kiểm tra lại trong CommitAssignment.
- Lát cắt 3: production preflight báo SUPPORTED/DEGRADED/UNSUPPORTED, đọc OS/architecture/kernel/cgroup, Docker version/API/runtime nvidia, NVML/GPU discovery; không chạy shell probe hoặc test container. Startup/reconnect cần FULL inventory; heartbeat quay lại chưa đủ để lease START. Retry startup/heartbeat/command có backoff+jitter; inventory định kỳ giữ nhịp cũ.
- Lát cắt 4: login nội bộ, BFF cookie HttpOnly/session, current organization/user/role; ADMIN có bộ lọc đơn vị và CRUD metadata tối thiểu; onboarding cấp token, không nhập GPU; query tài nguyên dùng organization filter do backend authorize.
- Compose/configure: PostgreSQL service, credentials sinh local, migrate/bootstrap tường minh và token riêng từng Agent Sim. Không chạy script hoặc Compose trong task.

PARTIAL:
- Tất cả thay đổi mới chỉ được đọc/sửa tĩnh, chưa chạy migration/build/test theo yêu cầu.
- Dependency lib/pq đã khai báo; chưa tải dependency hoặc cập nhật go.sum qua Go tool.
- Dữ liệu runtime cũ thiếu organization được giữ nguyên và không được schedule; không tự chuyển ownership.

TODO:
- Docs canonical, OpenAPI/Postman, TESTING_RUNBOOK, kiểm tra tĩnh thay đổi.

FILES CHANGED:
- Backend: domain/{organization,model,errors}; ports/metadata; store/postgres/{store,001_metadata.sql}; application/{identity,tenancy,controlplane,allocation,scheduler}; store/memory/store; httpapi/{identity,server,job_view}; config/config; cmd/aiwm-server/main; go.mod.
- docs/IMPLEMENTATION_STATUS.md.
- Pre-existing: README.md modified; docs/ALGORITHMS.md và docs/TECH_STACK.md untracked. Không reset/revert.

NEXT STEP: docs/Postman/runbook và kiểm tra tĩnh. Các kết quả test bên dưới thuộc task cũ, không chứng nhận task hiện tại.

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
