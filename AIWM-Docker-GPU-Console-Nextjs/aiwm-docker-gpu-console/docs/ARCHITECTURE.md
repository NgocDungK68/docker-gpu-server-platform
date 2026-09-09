# Frontend architecture

Next App Router trong src/app; Console routes dưới (console). UI components ở src/components; feature hooks giữ query/mutation/cache trong src/features; centralized client/DTO ở src/lib/api.

Browser không biết CP secret/base URL. Same-origin /api/aiwm BFF kiểm tra route/method whitelist rồi đọc src/config/server.ts để forward public API. Origin check cho mutation và body/timeout bounds; generic gateway errors. Agent routes và Docker không qua proxy.

Không còn frontend mock data hoặc fake benchmark. Scheduler page đọc Assignment từ jobs thật. Server/GPU availability dùng freshness do CP tính; UI không tự lập kế hoạch scheduling.

Theme/font/radius tại globals.css, logo public/brand, navigation tại src/config/navigation.ts. Loading/error/empty/badges là components dùng lại. Form schema RHF/Zod ở src/lib/jobs/form-schema.ts; backend vẫn là bên validation có thẩm quyền.

Test gồm Node contract tests và Playwright dùng backend thật. [README frontend](../README.md), [Screen API mapping](SCREEN_API_MAP.md) và [Architecture toàn hệ thống](../../../docs/ARCHITECTURE.md).
