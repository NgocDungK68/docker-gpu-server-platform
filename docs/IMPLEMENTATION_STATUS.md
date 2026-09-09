# Trạng thái implementation

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
