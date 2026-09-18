# Tech stack AIWM

Tài liệu mô tả công nghệ đang được nối vào source hiện tại, dựa trên [go.mod][gomod], [package.json][package] và các integration points; không phải xác nhận đã triển khai production. Chi tiết lifecycle/algorithms nằm trong [SYSTEM_DESIGN](SYSTEM_DESIGN.md) và [ALGORITHMS](ALGORITHMS.md).

## Tech stack theo tầng

`PRODUCTION`: dùng trong luồng ứng dụng/Agent thật, có thể được dùng chung bởi simulator. `SIMULATION`: thay thế hoặc cấu hình phục vụ mô phỏng. `DEVELOPMENT`: công cụ compile/build/check; JavaScript/CSS tạo ra vẫn dùng cho ứng dụng production. Version dưới đây là declaration trong manifest/image, không phải version đã kiểm tra trên máy người dùng.

| Layer | Technology | Role in AIWM | Used by | Runtime status | Evidence |
|---|---|---|---|---|---|
| Frontend | Next.js 16.3.4, React/React DOM 19.2.8 | Render Console; Next.js Route Handler làm BFF cho public API | Browser UI, Next.js server | PRODUCTION | [package.json][package], [layout][layout], [BFF][bff] |
| Frontend server | Node.js | Runtime của Next.js/BFF; frontend Dockerfile dùng image Node 24 | Console server | PRODUCTION | [frontend Dockerfile][fe-docker] |
| Frontend typing | TypeScript 5.x | Types cho UI/API và kiểm tra kiểu trước runtime JavaScript | Frontend source | DEVELOPMENT | [package.json][package], [API client][api-client] |
| UI styling | Tailwind CSS 4, `@tailwindcss/postcss`, CSS nội bộ | Compile utilities và styles cho Console | Frontend build → CSS của UI | DEVELOPMENT | [globals.css][css], [postcss config][postcss] |
| Frontend data | `@tanstack/react-query` | Quản lý queries/mutations, cache và refetch dữ liệu từ API | `AppProviders`, request form | PRODUCTION | [app-providers.tsx][providers], [job-form.tsx][form] |
| Frontend form | React Hook Form, `@hookform/resolvers`, Zod | Quản lý form và kiểm tra input bằng schema trước khi gửi request | Form tạo Job | PRODUCTION | [job-form.tsx][form], [form-schema.ts][schema] |
| UI charts/icons | Recharts, Lucide React | Vẽ biểu đồ inventory và icons giao diện | GPU chart, form/components | PRODUCTION | [gpu-state-chart.tsx][chart], [job-form.tsx][form] |
| Backend language | Go; `go.mod` khai báo `go 1.24.0` | Chạy Control Plane, Agent và các application/domain components | `aiwm-server`, `aiwm-agent`; dùng chung với sim | PRODUCTION | [go.mod][gomod], [server main][server-main], [agent main][agent-main] |
| HTTP/API | Go `net/http`, `http.ServeMux`, `encoding/json`; Fetch API ở BFF/client | HTTP routing và JSON public/Agent protocol; Agent chủ động polling | CP HTTP server, Agent HTTP client, BFF | PRODUCTION | [httpapi/server.go][http], [Agent client][agent-client], [BFF][bff] |
| Logging | Go `log/slog` với JSONHandler | Structured logs của process và các operation | CP, Agent, simulator | PRODUCTION | [server main][server-main], [agent main][agent-main], [sim main][sim-main] |
| Metadata storage | PostgreSQL 17 (Compose), database/sql + github.com/lib/pq v1.10.9 | Organization/User/session/enrollment/server ownership; SQL parameterized và transaction bind | IdentityService / MetadataRepository | PRODUCTION | internal/store/postgres; cmd/aiwm-server/main.go |
| Password/session | crypto/pbkdf2 + SHA-256, crypto/rand; cookie HttpOnly | Hash mật khẩu, opaque session TTL và auth nội bộ | Control Plane, BFF | PRODUCTION | application/identity.go; httpapi/identity.go; BFF |
| CP runtime storage | Go maps, `sync` locks, `encoding/gob` và local filesystem | `memory.Store` giữ state; `durable.Store` ghi snapshot, rollback khi ghi lỗi, một writer | Control Plane repository | PRODUCTION | [memory store][memory], [durable store][durable] |
| Agent storage | `encoding/json` và local file | Lưu identity/token, inventory sequence và processed command results; temp + Sync + Rename | `state.FileStore` | PRODUCTION | [state/file.go][agent-state], [contracts.go][agent-contracts] |
| Container integration | Docker Engine API; Moby Go SDK `moby/moby/client` + `moby/moby/api` | List/inspect/events và start/stop managed containers; truyền exact GPU DeviceIDs | `dockerengine.Engine` trên GPU host | PRODUCTION | [go.mod][gomod], [engine.go][docker] |
| GPU platform | NVIDIA driver / NVML | API local để quan sát GPU hardware và GPU processes | GPU host, NVML adapter | PRODUCTION | [nvml_linux.go][nvml] |
| GPU Go binding | `github.com/NVIDIA/go-nvml/pkg/nvml`, module v0.13.3-1 | Đưa lời gọi NVML vào Go qua binding; adapter hiện yêu cầu Linux/CGO | `gpu.Reader` | PRODUCTION | [go.mod][gomod], [NVML adapter][nvml], [unsupported adapter][unsupported] |
| Mock GPU | NVIDIA mock NVML shared library + YAML profile | Cung cấp NVML responses mô phỏng cho cùng Go adapter | `aiwm-agent-sim` | SIMULATION | [Dockerfile.sim][sim-docker], [compose.yaml][compose] |
| Mock containers | Go + JSON file | `simulator.Runtime` mô phỏng DockerRuntime và events, lưu containers riêng | Agent sim | SIMULATION | [simulator/agent.go][sim], [sim main][sim-main] |
| Demo deployment | Docker images, Docker Compose, named volumes | Chạy CP, hai Agent sim và Console optional profile, giữ state qua restart | Demo workspace | SIMULATION | [compose.yaml][compose] |
| Frontend checks | ESLint, TypeScript compiler, Node test runner, Playwright | Lint/typecheck, contract tests và browser E2E theo scripts đã khai báo | Developer/CI | DEVELOPMENT | [package.json][package] |

CP runtime persistence vẫn là **memory + durable gob snapshot**; business metadata dùng PostgreSQL riêng, không migrate Job/Assignment/Command/inventory. BFF giữ token từng session trong cookie HttpOnly và chỉ forward public routes; Agent dùng enrollment/Agent token riêng. HTTP server có tùy chọn TLS, Agent client có cấu hình CA/client certificate.

## NVIDIA NVML và go-nvml

**NVML** là NVIDIA Management Library: API quản lý/quan sát GPU được Agent truy cập tại GPU host thông qua NVIDIA driver/library. **go-nvml** là Go binding để gọi API đó, **không phải GPU driver**, không phải scheduler và không phải thư viện thực thi model AI.

Chuỗi gọi local: `Agent → InventoryCollector → gpu.Reader → go-nvml → NVML/driver → NVIDIA GPU`. Thông tin được trả ngược về collector rồi gửi CP trong inventory.

[`Reader.Snapshot`][nvml] hiện đọc:

- GPU index, UUID và model/name.
- VRAM total/used, GPU utilization và temperature khi đọc được.
- Compute/graphics running processes: PID, GPU UUID và used GPU memory; process name bổ sung từ `/proc/<pid>/comm`.
- MIG mode và uncorrected volatile ECC count để đánh giá `Healthy`; việc đọc MIG mode không có nghĩa AIWM cấp phát MIG.

`InventoryCollector` kết hợp GPU/process data với Docker grants/container observations để xác định occupancy; riêng một con số utilization không đủ kết luận GPU được phép cấp phát. **FP8 capability hiện đến từ backend [capability catalog][catalog] theo model aliases**, không được adapter này query trực tiếp từ NVML.

`nvml_linux.go` là Linux adapter; platform khác trả lỗi trong `nvml_unsupported.go`. Dockerfile.sim build với `CGO_ENABLED=1` và nạp mock `libnvidia-ml.so.1` qua `LD_LIBRARY_PATH`; `MOCK_NVML_CONFIG` chọn YAML profile. Production và sim dùng cùng `gpu.New/Reader`, nhưng sim nhận dữ liệu từ mock library. Source mock nằm trong NVIDIA `k8s-test-infra/pkg/gpu/mocknvml`; tên repository này không có nghĩa AIWM dùng Kubernetes runtime.

## Sơ đồ công nghệ

Số 1–5 là thứ tự thuyết trình, không phải thứ tự Agent khởi tạo kết nối. Nét đứt chỉ nhánh simulation; Agent nằm local trên GPU host.

~~~mermaid
flowchart LR
    U["1. User / Browser"] --> UI["2. Next.js / React UI"]
    UI -->|Fetch HTTP| BFF["Next.js BFF / Node.js"]
    BFF -->|Public HTTP JSON| CP["3. Go Control Plane<br/>net/http + ServeMux<br/>Policy / Scheduler"]
    CP --> PG[("PostgreSQL / metadata")]
    CP --> STORE[("memory.Store<br/>durable.Store + gob")]
    CP -->|HTTP poll response: command| AG["4. Go Agent<br/>InventoryCollector / Executor"]
    AG -->|HTTP register / inventory<br/>heartbeat / poll / ACK| CP
    AG --> SDK["5. Moby Go SDK"]
    SDK -->|Local Engine API| DOCKER["Docker Engine"]
    DOCKER -->|NVIDIA GPU DeviceRequests| GPU["NVIDIA GPU"]
    AG --> GN["5. go-nvml"]
    GN --> NV["NVML / NVIDIA driver"]
    NV --> GPU
    AG -.->|SIMULATION: Docker boundary| SIM["simulator.Runtime<br/>JSON containers"]
    GN -.->|SIMULATION: mock library| MOCK["NVIDIA mock NVML<br/>YAML profile"]
~~~

Luồng công nghệ: Browser gọi BFF, BFF gọi public CP API; Agent tự gửi register/inventory/heartbeat và poll CP bằng HTTP/JSON. Command đi về trong **response của poll**, sau đó Agent gọi Docker adapter và gửi ACK/inventory. Không có CP push vào Agent hoặc message broker trong đường giao tiếp này. Docker điều khiển containers; NVML là nhánh observation GPU local, không đi qua HTTP tới CP.

Trong simulation, `simulator.Runtime` thay Docker boundary và mock library thay nguồn NVML responses; cùng Agent core vẫn xử lý inventory/commands. Simulated containers là records JSON, **không thực thi Docker image/command hoặc CUDA thật**. Trình tự lifecycle chi tiết đã có trong [SYSTEM_DESIGN](SYSTEM_DESIGN.md), nên tài liệu này không lặp sequence diagram.

## Technology khác application component

`Control Plane`, `Agent`, `Scheduler`, `InventoryCollector` và `Policy Engine` là **components do project tổ chức/implement**. `Go`, `Next.js`, `Docker Engine API`, `NVML` và `go-nvml` là **ngôn ngữ/framework/API/library/platform** dùng để xây dựng chúng. Ví dụ khi trình bày: “Scheduler được implement bằng Go trong Control Plane”, không liệt kê Scheduler như một framework bên cạnh Next.js.

## Tóm tắt tech stack cho thuyết trình

| Technology | Vai trò ngắn gọn |
|---|---|
| PostgreSQL / lib/pq | Lưu metadata tổ chức, người dùng, session và onboarding. |
| Go | Implement Control Plane và Agent backend. |
| Next.js / React / TypeScript / Node.js | Xây dựng Console và chạy BFF cho public API. |
| Tailwind CSS | Tạo styling cho Console. |
| TanStack Query | Quản lý dữ liệu API, cache và refresh phía UI. |
| React Hook Form / Zod | Quản lý form và validate input. |
| Recharts / Lucide React | Hiển thị biểu đồ và icons. |
| HTTP/JSON — net/http / Fetch | Kết nối UI/BFF, Control Plane và outbound Agent protocol. |
| Docker Engine API / Moby SDK | Inventory và điều khiển managed containers trên GPU host. |
| NVML / go-nvml | Quan sát GPU hardware và GPU processes. |
| Go maps + gob/JSON files | Lưu CP snapshot và Agent state trên local filesystem. |
| NVIDIA mock NVML / Docker Compose | Chạy môi trường simulation/demo. |

## Vì sao phù hợp với AIWM hiện tại?

| Technology | Lý do gắn với implementation |
|---|---|
| Go | Cho phép CP và production/sim Agent chia sẻ cùng domain, core và adapter interfaces trong một module. |
| Next.js / React / Node.js | Kết hợp UI với BFF phía server để browser dùng public API mà không giữ upstream token. |
| TanStack Query; React Hook Form/Zod | Khớp với Console đọc inventory định kỳ và gửi allocation request có schema. |
| Tailwind CSS; Recharts/Lucide | Phục vụ trực tiếp các màn hình form, trạng thái và biểu đồ inventory hiện có. |
| HTTP/JSON | Khớp mô hình Agent outbound polling và các public endpoints đang được implement. |
| Docker Engine API / Moby SDK | Cho Agent thao tác trực tiếp local Docker trên standalone GPU server. |
| NVML / go-nvml | Cho Agent lấy telemetry GPU local bằng cùng Go adapter trên driver thật hoặc mock. |
| PostgreSQL / lib/pq | Lưu account/organization/enrollment với ràng buộc unique, FK và transaction, độc lập runtime snapshot. |
| Gob/JSON files | Giữ CP và Agent state qua process restart theo mô hình local persistence hiện tại. |
| Mock NVML / Compose | Cho demo luồng inventory/control với NVIDIA shared-library mock mà không cần GPU vật lý. |

## Ánh xạ source

Paths `internal/` và `cmd/` dưới đây thuộc backend; `src/` thuộc frontend. Links trỏ đúng integration files, không liệt kê toàn bộ source.

| Technology | Main integration file/package |
|---|---|
| PostgreSQL / database/sql / lib/pq | [internal/store/postgres/store.go](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/postgres/store.go); 001_metadata.sql; cmd/aiwm-server/main.go |
| Go / server composition | [go.mod][gomod]; [cmd/aiwm-server/main.go][server-main]; [cmd/aiwm-agent/main.go][agent-main] |
| Next.js / React / Node.js / TypeScript | [package.json][package]; [src/app/layout.tsx][layout]; [frontend Dockerfile][fe-docker] |
| BFF / Fetch | [src/app/api/aiwm/[...path]/route.ts][bff]; [src/lib/api/api-client.ts][api-client] |
| TanStack Query | [src/providers/app-providers.tsx][providers] |
| Form / schema libraries | [src/components/workloads/job-form.tsx][form]; [src/lib/jobs/form-schema.ts][schema] |
| Tailwind / charts / icons | [src/app/globals.css][css]; [postcss.config.mjs][postcss]; [gpu-state-chart.tsx][chart]; [job-form.tsx][form] |
| net/http / Agent HTTP client | [internal/httpapi/server.go][http]; [internal/agent/controlplane/client.go][agent-client] |
| Docker Engine / Moby SDK | [internal/agent/dockerengine/engine.go][docker] |
| NVML / go-nvml | [internal/agent/gpu/nvml_linux.go][nvml]; [nvml_unsupported.go][unsupported] |
| Gob / memory state | [internal/store/memory/store.go][memory]; [internal/store/durable/store.go][durable] |
| Agent JSON state | [internal/agent/state/file.go][agent-state]; [contracts.go][agent-contracts] |
| Mock NVML / simulation / Compose | [cmd/aiwm-agent-sim/main.go][sim-main]; [internal/simulator/agent.go][sim]; [Dockerfile.sim][sim-docker]; [compose.yaml][compose] |

[gomod]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/go.mod
[server-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-server/main.go
[agent-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-agent/main.go
[sim-main]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/cmd/aiwm-agent-sim/main.go
[http]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/httpapi/server.go
[agent-client]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/controlplane/client.go
[docker]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/dockerengine/engine.go
[nvml]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_linux.go
[unsupported]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/gpu/nvml_unsupported.go
[memory]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/memory/store.go
[durable]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/store/durable/store.go
[agent-state]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/state/file.go
[agent-contracts]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/agent/contracts.go
[catalog]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/capability/catalog.go
[sim]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/internal/simulator/agent.go
[sim-docker]: ../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/Dockerfile.sim
[compose]: ../compose.yaml
[package]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/package.json
[layout]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/layout.tsx
[fe-docker]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/Dockerfile
[bff]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/api/aiwm/[...path]/route.ts
[api-client]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/api/api-client.ts
[providers]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/providers/app-providers.tsx
[form]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/workloads/job-form.tsx
[schema]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/lib/jobs/form-schema.ts
[css]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/app/globals.css
[postcss]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/postcss.config.mjs
[chart]: ../AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console/src/components/charts/gpu-state-chart.tsx

PostgreSQL phù hợp cho metadata có unique key/foreign key và enrollment transaction; runtime accounting vẫn dùng store đang có. go-nvml là Go binding, không phải driver. Preflight dùng API/file reads; không chạy CUDA probe. Release build đã được kiểm chứng; CUDA execution trên GPU host thật còn cần kiểm tra theo runbook.

## Phát hành Agent và yêu cầu trên GPU host

| Nơi chạy | Công nghệ/thành phần cần có | Vai trò |
|---|---|---|
| Development/CI | Bash, Docker/BuildKit; Go 1.26.6 + GCC trong builder | Build generic production Agent với CGO; inject default Control Plane URL. |
| Builder ABI | buildpack-deps:bullseye, glibc 2.31 | Tạo Linux amd64 artifact; không mang mock NVML vào release. |
| GPU server runtime | Prebuilt aiwm-agent, glibc tương thích, CA trust cho HTTPS | Chạy Agent; **không cần Go compiler, source hoặc npm**. |
| GPU server runtime | Docker Engine, NVIDIA Driver/NVML, GPU, NVIDIA Container Toolkit/runtime nvidia | Inventory và thực thi container được cấp GPU. |
| Linux service | systemd nếu có; account aiwm-agent + group docker | Nạp config bảo vệ và restart riêng Agent khi process lỗi. |
| Máy quản trị | Python standard library + public HTTP API | Cấp enrollment riêng và gửi/quan sát workload smoke qua scripts/agent-admin.py. |

Đã build và chạy --version trên baseline Debian/glibc 2.31. Ubuntu/Debian và RHEL/Rocky/Alma là target có điều kiện ABI/dependencies phù hợp; chưa chứng nhận GPU thật hoặc mọi distro. ARM64/musl chưa được chứng nhận.

Nguồn: [build-agent-release.sh](../scripts/build-agent-release.sh), [install-agent-linux.sh](../scripts/install-agent-linux.sh), [Dockerfile.agent-release](../AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane/Dockerfile.agent-release), internal/agent/config/config.go (ReleaseControlPlaneURL), cmd/aiwm-agent/main.go (--check/--version). Lệnh triển khai: [TESTING_RUNBOOK](TESTING_RUNBOOK.md).
