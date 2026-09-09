# AIWM Docker GPU Console

Next.js 16 + React 19 + TypeScript Console cho Docker GPU resource pool. [README workspace](../../README.md) hướng dẫn chạy đầy đủ Control Plane, Agent/NVIDIA mock, frontend, demo và acceptance.

## Chạy

Từ workspace root chạy python scripts/configure.py và docker compose up --build -d control-plane agent-a100 agent-t4. Sau đó tại project frontend này:

~~~powershell
npm.cmd ci
npm.cmd run dev -- --hostname 127.0.0.1
~~~

Mở http://127.0.0.1:3000. Linux dùng npm thay npm.cmd. Dockerfile frontend dùng Node24; máy dev nên dùng cùng major. Có thể dùng root docker compose --profile console up --build -d để chạy frontend trong container thay cho node trên host.

.env.local chứa AIWM_API_BASE_URL, AIWM_API_TOKEN và NEXT_PUBLIC_REFRESH_INTERVAL_MS. Token chỉ đọc ở server; không có frontend mock mode. Nếu API báo Unauthorized, đồng bộ API token với CP rồi restart Next.js.

## Màn hình đã nối API

Dashboard summary/capacity/recent job events; Servers table và detail có hardware/readiness/drain; GPU inventory; External/Managed containers; Jobs; Queue priority/FIFO; Submit form; Job detail với reservation/UUID/score/reason/events và stop/cancel. Scheduler page dùng quyết định từ jobs thật. Onboarding hướng dẫn Agent và inventory; Settings trình bày cấu hình hiện tại.

Route cũ /policies redirect sang /scheduler. Không còn mock benchmark, K1/K2/K3 hoặc checkpoint controls. [Screen → API](docs/SCREEN_API_MAP.md) và [OpenAPI backend](../../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/api/openapi.yaml).

## Boundary và source

Browser → /api/aiwm/* → Next BFF → Control Plane /api/v1/*. API client duy nhất ở src/lib/api/api-client.ts; DTO ở types.ts; hooks/query cache ở src/features. BFF whitelist ở proxy-policy.ts, token/config ở src/config/server.ts; không expose Agent/Docker routes.

Routes trong src/app/(console); components tái sử dụng trong src/components; theme tập trung ở src/app/globals.css, navigation ở src/config/navigation.ts. Không đặt scheduler trong UI. [Architecture](docs/ARCHITECTURE.md).

Console chưa có user login/RBAC; chỉ dành cho operator trên loopback/trusted network hoặc sau authenticated gateway. Backend bearer và Origin check không thay thế user authentication.

## Kiểm tra

~~~powershell
npm.cmd run check
~~~

Gồm lint, typecheck, 9 unit/contract tests và production build. Chạy production preview bằng npm.cmd run start -- --hostname 127.0.0.1.

Khi frontend và root Compose demo đang chạy, không chạy acceptance song song:

~~~powershell
$env:AIWM_BROWSER_CHANNEL="msedge"
npm.cmd run test:e2e
~~~

Dùng Edge đã cài trên Windows. Máy chưa có browser tương thích: npx playwright install chromium và bỏ AIWM_BROWSER_CHANNEL. E2E test live server/GPU/external inventory, create→RUNNING→stop và BFF protection. Test chỉ dừng job do nó tạo, trace/screenshot nếu lỗi nằm trong test-results.

Từ root có thể chạy python scripts/acceptance.py --base-url http://127.0.0.1:3000/api/aiwm để test đầy đủ public BFF path.
