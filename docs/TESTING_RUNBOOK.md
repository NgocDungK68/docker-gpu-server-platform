# Runbook kiểm thử AIWM


## Kiểm thử Resource Allocation Planning theo thời gian (23/09/2026)

**Checkpoint đã kiểm chứng:** backend focused tests với clock giả PASS; frontend typecheck + contract tests PASS. Docker stack đang dừng khi kiểm tra phiên này; chưa chạy lại scenario time-planning trên Docker/NVML. Không dùng PASS Fake Demo lịch sử để khẳng định feature mới đã E2E PASS.

### Backend — không cần Docker/GPU và không chờ nhiều giờ

Từ workspace root:

~~~powershell
cd AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane
go test ./internal/domain ./internal/application ./internal/httpapi ./internal/store/memory ./internal/store/durable ./internal/policy
~~~

Các test trong application/planning_test.go cover: future/no early, touching/overlap, N1/N2, auxiliary cùng Necessity, isolation, dispatch đúng start, stale Agent/external/unknown/unhealthy, không preempt RUNNING, hết duration/STOP/release, delayed poll, lost ACK, concurrency, preview read-only. durable/planning_test.go kiểm atomic batch/disk rollback và restart giữ calendar.

Không expose API đổi clock. Test tự tăng c.now; production dùng clock thật. neededAt phải là RFC3339 có timezone; TTLSeconds là duration tính từ requested start, không tính từ RUNNING.

### Fake nhiều server — scenario thực qua public API

Từ workspace root, Docker Desktop Linux containers/WSL sẵn sàng:

~~~bash
python scripts/demo.py up
python scripts/demo.py planning
~~~

Trong WSL dùng python3 thay python nếu cần. up dùng helper đã có: build/start demo, metadata seed, enrollment và Agent Sim; không dùng --skip-build khi source CP thay đổi. Không chạy up nếu chỉ muốn unit tests.

planning yêu cầu vtt-gpu-01 fresh, 3 H100 FREE còn lại sau một existing container. Không tự dừng Job khác, drain host hoặc sửa GPU state để ép demo. Scenario dùng credentials DEMO từ config/demo-users.json.

| Mốc | Kết quả cần thấy |
|---|---|
| Tạo A/N2, start now+45s, duration 30s | ASSIGNED, assignment PLANNED; commandId rỗng; physical GPU có thể vẫn FREE. |
| Thêm C/N3 và B/N1 cùng interval | B giữ 3 GPU, A/C QUEUED; assigned server cùng Organization VTT. |
| Trước start | Không STARTING/RUNNING hoặc START command cho ba Job. |
| Đến start | B → STARTING → Agent Sim/runtime → RUNNING/ALLOCATED. |
| Đến end | Graceful STOP → STOPPED/RELEASED → GPU FREE; external identity/state giữ nguyên. |

Script in từng PASS và lưu .cache/multi-server-demo/last-planning-check.json. Finally chỉ stop/cancel ba Job do lần chạy tạo, không cleanup Docker resource khác. Tổng khoảng 75–100 giây; lỗi assertion là FAIL thực, không ghi WORKING. Nếu không có đủ 3 H100 FREE, script dừng với reason; xử lý Job của chính bạn bằng UI nếu phù hợp rồi chạy lại.

UI: mở Workloads, lọc “Đã lên lịch”, xem start/end/thời lượng. Trạng thái “Đã lên lịch · Chưa chạy” không có nghĩa container đã chạy. Form giữ execution fields và các mức 0.5/1/2/4/8 giờ hoặc duration số hợp lệ.

### Linux GPU thật

Giữ flow release/install/enrollment bên dưới. Khi submit real-gpu-smoke qua UI, chọn start trong vài phút tới và duration phù hợp. Trước start: ASSIGNED/PLANNED, chưa có container mới của Job. Đến start: kiểm docker ps, nvidia-smi và API/UI. EndAt gửi STOP; pull/start/stop có latency nên không bảo đảm runtime kết thúc chính xác tại EndAt.

Nếu Agent/CP offline: workload vẫn chạy và GPU không tự FREE. Khi reconnect/full inventory, CP revalidate phần interval còn lại hoặc request STOP nếu hết. Job khác có reservation chạm endpoint phải chờ GPU thực tế an toàn. Không stop existing/legacy workload để đáp ứng lịch.

Lệnh agent-admin.py smoke hiện đặt neededAt=now; muốn minh họa future dùng UI hoặc folder Postman Planning. Job cũ thiếu Assignment.StartAt/EndAt không tự được áp deadline mới; tạo Job mới để test time planning.

## FAKE multi-server — đường chạy đã kiểm chứng (17/09/2026)

Dùng mục này cho demo hiện tại; các bước thủ công phía dưới chỉ để đối chiếu/development. Phase A đã chạy thật trên Windows + Docker Desktop và Ubuntu WSL2.

### Khởi động tại workspace root

PowerShell:

~~~powershell
python scripts/demo.py up
~~~

WSL (Docker Desktop bật integration cho Ubuntu, có Python 3):

~~~bash
cd /mnt/f/Viettel/VDT/demo-project
bash scripts/demo-up.sh
~~~

Lần đầu script build tuần tự Control Plane, Agent Sim và Console; migrate metadata, seed demo accounts, tạo enrollment riêng từng server rồi khởi động sáu Agent. Lần sau khi source không đổi dùng `--skip-build`. Không cần Go/Node trên máy chỉ chạy Compose demo.

Mở **http://127.0.0.1:3000/login**. Các user `admin`, `vtt`, `vds`, `vtnet`, `vtit` dùng password demo `AIWM-Demo-2026!`; xem [DEMO_ACCOUNTS.md](DEMO_ACCOUNTS.md). Chỉ dùng trên lab, không production. Nếu account cũ khác mật khẩu, script dừng; chỉ dùng `--set-demo-passwords` khi chủ động đổi các account demo có cùng role/ownership. Không xóa database để làm lại demo.

### Lỗi bind PostgreSQL 5432 trên máy Windows này

Đã xác định port 5432 có PostgreSQL Windows đang lắng nghe. Giữ nguyên service đó; Compose demo dùng **127.0.0.1:15432 → postgres:5432**. Script up tự đồng bộ host port trong root/backend env. Muốn chỉ sửa cấu hình local:

~~~powershell
python scripts/configure.py --postgres-port 15432
docker compose -p aiwm-org-demo up -d --wait postgres
~~~

Nếu 15432 cũng bận, chọn port trống khác bằng `python scripts/demo.py up --postgres-port 25432`. Không tắt dịch vụ lạ hoặc sửa Windows reserved ports. Native backend kết nối localhost:15432; container CP vẫn dùng postgres:5432. Không in DSN chứa password để debug.

### Dữ liệu và luồng demo

Nguồn: `config/demo-users.json` và `demo/scenarios/multi-server.json`; organization chỉ là metadata.

| Organization | Server | GPU |
|---|---|---|
| VTT | vtt-gpu-01; vtt-gpu-02 | 4 H100 80GB; 8 A100 80GB |
| VDS | vds-gpu-01; vds-gpu-02 | 4 H100 80GB; 4 A100 80GB |
| VTNET | vtnet-gpu-01 | 4 H100 80GB |
| VTIT | vtit-gpu-01 | 2 A100 80GB |

Tổng **6 servers, 26 GPUs**. Dùng model đã có trong capability catalog, chưa thêm H200/L40S. Mock NVML tạo process external/unknown và lỗi ECC; SAME Runner + InventoryCollector phân loại LEGACY/UNKNOWN/UNHEALTHY. RESERVED xuất hiện khi scheduler commit trước Agent thực thi; ALLOCATED khi workload chạy. Đây không phải state gán giả trong frontend.

Login vtt → Tạo workload: image `alpine:3.21`, command mỗi dòng một đối số `sh`, `-c`, `sleep 300`; TRAINING, 1 GPU, VRAM 1024 MiB, profile general, Cần thiết 2, lý do go-live, mức Quan trọng, điền thời điểm cần. Đối chiếu → Gửi → RUNNING trên VTT → Stop → reservation RELEASED → GPU FREE nếu không còn blocker. Lặp với vds: chỉ thấy/nhận tài nguyên VDS.

Simulation không chạy CUDA/image thật: simulator.Runtime mô phỏng Docker lifecycle, cùng Agent core quan sát mock NVML. GPU utilization mock không đại diện benchmark. Quy tắc placement/policy/reservation/auth đều chạy qua Control Plane thật.

### Kiểm thử có thể chạy lại

Từ workspace root (WSL thay `python` bằng `python3`):

~~~powershell
python scripts/demo.py check
python scripts/demo.py check --base-url http://127.0.0.1:3000/api/aiwm
python scripts/demo.py failure
~~~

`failure` tạm pause/stop đúng Agent VTIT và Control Plane của project demo, rồi phục hồi; không chạy đồng thời khi đang thuyết trình. Fake runtime state được persist trong volume; phép thử này không chứng minh process CUDA thật sống sót.

Browser smoke trên Windows có Edge và frontend dependencies đã cài:

~~~powershell
cd AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console
$env:AIWM_BROWSER_CHANNEL = "msedge"
npm.cmd run test:e2e -- tests/e2e/demo.spec.ts
~~~

| Kiểm tra thực tế | Kết quả |
|---|---|
| CP API và BFF: ADMIN thấy toàn bộ; bốn user scope đúng organization | PASS |
| Submit → reservation → command → Agent Sim → RUNNING; stop → RELEASED/FREE | PASS |
| Request quá lớn QUEUED; không mượn GPU đơn vị khác | PASS |
| LEGACY/UNKNOWN/UNHEALTHY không được cấp phát; external giữ identity/start time | PASS |
| RESERVED trước execution; OFFLINE giữ allocation; Agent/CP restart phục hồi | PASS |
| Edge: các màn admin; VTT tạo workload qua form và release | PASS — 2 tests |
| Go application/httpapi/memory; frontend typecheck; Compose images | PASS |

Kết quả JSON local ở `.cache/multi-server-demo/`; không commit enrollment/session. Test chỉ stop/cancel Job do chính test tạo. Các script acceptance/recovery/doctor lịch sử không phải entrypoint chứng nhận demo mới.

### Dừng demo

~~~powershell
python scripts/demo.py down
~~~

WSL: `bash scripts/demo-down.sh`. Chỉ project `aiwm-org-demo`, giữ named volumes/metadata/Agent identity. Không `down -v`, không global prune. Bật lại bằng up --skip-build. Khi đổi qua lại PowerShell/WSL, Compose có thể recreate Agent vì cách biểu diễn bind path khác; volume giữ nguyên.

### Ranh giới với REAL DEPLOYMENT

GPU server production nhận generic Agent release + installer + enrollment riêng, không clone repo và không cần Go compiler. Xem Real Deployment Demo và REAL GPU SERVER E2E bên dưới. Laptop chưa có GPU NVIDIA để chứng nhận real E2E.

---
Các phần local dưới đây là lựa chọn chạy thủ công. Kết quả đã kiểm chứng được ghi riêng ở mục FAKE và IMPLEMENTATION_STATUS; real GPU cần kiểm tra trên host NVIDIA thật.

## Chuẩn bị chung và bảo toàn dữ liệu

- Workspace root: thư mục chứa README, `compose.yaml`, `scripts/`.
- Backend: `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`.
- Frontend: `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console`.
- Windows: PowerShell, Python 3.10+, Go ≥1.24, Node.js 24; PostgreSQL local hoặc Docker Desktop Linux containers. Agent production chỉ chạy trên Linux. Fake Agent dùng Linux container; không cần GPU vật lý trên laptop.
- Dùng project Compose riêng `aiwm-org-demo` để không đụng volume demo cũ. Port 15432/8080/3000 phải trống; không chạy CP native và CP container cùng port.
- `scripts/configure.py` sinh credentials local nếu thiếu, giữ giá trị cũ, không in secret. Đọc username/password bootstrap trong `.env` bằng editor, không commit/export credentials. Có thể đổi seed `AIWM_BOOTSTRAP_ORGANIZATION_CODE/NAME` trước khi bootstrap.
- VTT/Viettel Telecom là seed mặc định của script; VTNET/Viettel Network và VDS/Viettel Digital Services có thể tạo qua UI/API. Không có mã đơn vị trong scheduler.
- Runtime snapshot cũ thiếu `organizationId` không tự được chuyển ownership; Job cũ không nhận placement. Không xóa snapshot/state cũ. Với native demo mới, chọn `AIWM_STATE_FILE=.local/control-plane-org.gob`. Việc chuyển ownership dữ liệu cũ đang chạy cần quy trình riêng; chưa có migration tự động.
- Dependencies và checksum đã có trong go.mod/go.sum. Chỉ development mới cần Go toolchain; Compose/release dùng builder image.

## LEVEL 1 — Backend/frontend local

### 1. Cấu hình và PostgreSQL

Tại workspace root, PowerShell:

```powershell
python scripts/configure.py --postgres-port 15432
docker compose -p aiwm-org-demo up -d postgres
docker compose -p aiwm-org-demo logs --tail 30 postgres
```

Chờ PostgreSQL báo sẵn sàng nhận kết nối. Script tạo `AIWM_DATABASE_URL` dùng `localhost:15432` cho backend native; Compose CP dùng `AIWM_COMPOSE_DATABASE_URL` với hostname `postgres`. `sslmode=disable` chỉ cho lab local; môi trường nội bộ qua mạng cần cấu hình TLS phù hợp.

### 2. Schema và admin đầu tiên

```powershell
cd AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane
go mod tidy
$env:AIWM_STATE_FILE = ".local/control-plane-org.gob"
go run ./cmd/aiwm-server --migrate
go run ./cmd/aiwm-server --bootstrap-admin
go run ./cmd/aiwm-server
```

`--migrate` chạy schema `internal/store/postgres/001_metadata.sql` trong transaction rồi thoát. `--bootstrap-admin` tạo Organization/User cùng transaction, chỉ khi chưa có user; chạy lại khi đã có user trả conflict. Không chạy các migration runtime cũ trong `migrations/` cho metadata mới. Không cần `AIWM_API_TOKEN` hay enrollment token chung cho CP.

### 3. Frontend

Terminal khác, tại workspace root:

```powershell
cd AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console
npm.cmd ci
npm.cmd run dev -- --hostname 127.0.0.1
```

`AIWM_API_BASE_URL` trong `.env.local` trỏ CP. Local HTTP đặt `AIWM_SESSION_COOKIE_SECURE=false`; khi phục vụ HTTPS đặt `true`. Không cần token upstream dùng chung. Trên WSL/Linux dùng `npm` thay `npm.cmd`, `python3` thay `python`.

Mở `http://127.0.0.1:3000/login`, đăng nhập bằng account bootstrap. Kiểm tra organization name/code, username và role ADMIN. Dùng UI tạo một `ORGANIZATION_USER` cùng đơn vị; mật khẩu mới 12–256 byte theo backend. Không có public registration.

### 4. Health và API

Import hai file trong `postman/`, đặt `base_url=http://localhost:8080`. Chạy folder `Organization - Luồng chính`: Health → Login → Current user. Điền `username/password` trong environment cá nhân; Login lưu `access_token` từ response. Không export environment đã có token/password.

Kỳ vọng: health 200, current user có organization, dashboard rỗng nếu chưa có Agent. Preview chưa đủ GPU trả `satisfiable=false`; Submit hợp lệ vẫn tạo `QUEUED`. PostgreSQL không thay durable store của Job/Assignment/Command.

## LEVEL 2 — Fake GPU E2E đầy đủ

Chọn đường Compose này thay CP native ở LEVEL 1. Dừng process CP/frontend native bằng Ctrl+C trước khi dùng cùng ports. Dùng cùng PostgreSQL đã tạo thì bỏ qua bootstrap nếu đã có user.

### 1. Build và mở Control Plane

Tại workspace root, sau configure và `go mod tidy` ở backend:

```powershell
docker compose -p aiwm-org-demo --profile simulation --profile console build control-plane agent-a100 console
docker compose -p aiwm-org-demo up -d postgres
docker compose -p aiwm-org-demo logs --tail 30 postgres
docker compose -p aiwm-org-demo run --rm --no-deps control-plane --migrate
docker compose -p aiwm-org-demo run --rm --no-deps control-plane --bootstrap-admin
docker compose -p aiwm-org-demo --profile console up -d control-plane console
```

Chỉ chạy bootstrap trên database chưa có user. PostgreSQL volume `metadata-data` và CP volume `control-plane-data` riêng. Không chạy `down -v` để reset.

### 2. Tạo ownership và enrollment

1. Login ADMIN; kiểm tra VTT có sẵn từ bootstrap. Tạo VDS (hoặc mã demo khác) và account VTT user.
2. Logout ADMIN, login VTT user; mở **Kết nối GPU Server**. Organization tự điền VTT, không có selector.
3. Tạo enrollment cho A100. Copy token một lần vào `AIWM_A100_ENROLLMENT_TOKEN` trong `.env` root.
4. Login ADMIN, chọn VDS khi tạo enrollment cho T4. Copy token vào `AIWM_T4_ENROLLMENT_TOKEN`.
5. Mỗi enrollment dành cho một MachineID. Không dùng chung token cho A100/T4 hoặc host khác. Token bind lần đầu trong 24 giờ; sau bind chỉ cùng machine được đăng ký lại cho tới khi revoke.

```powershell
docker compose -p aiwm-org-demo --profile simulation up -d agent-a100 agent-t4
docker compose -p aiwm-org-demo logs --tail 30 agent-a100 agent-t4 control-plane
```

### 3. Điều kiện và kết quả mong đợi

| Bước | Thực hiện | Kết quả cần quan sát |
|---|---|---|
| Register | Agent Sim tự register với enrollment token và `sim-a100`/`sim-t4` | CP resolve Server/Organization từ metadata; không nhận org từ Agent. |
| Discovery | Agent core gọi `gpu.Reader` → mock NVML, ghép `simulator.Runtime` | VTT có 4 A100; VDS có 2 T4 theo YAML. ADMIN xem tất cả hoặc lọc; user VTT chỉ thấy VTT. |
| External | A100 seed `AIWM_SIM_EXTERNAL_GPU_INDEXES=0` với profile `a100-external.yaml` | GPU 0 `OCCUPIED_LEGACY`; workload External vẫn running. Không có nút stop External. |
| Preview | VTT user khai báo TRAINING, 3 GPU, 1024 MiB/GPU, chọn profile tương đương A100 trong options, FP8=false; Necessity/reason hợp lệ | `satisfiable=true` nếu GPU 1–3 đang trống; preview không tạo Job/reservation. |
| Submit | Gửi cùng request với image `alpine:3.21`, command `sh`, `-c`, `sleep 300`, neededAt hợp lệ, TTL 3600 | Job.organizationId bằng account VTT, status ban đầu QUEUED. |
| Policy/placement | Scheduler ticker chạy tự động | Policy lane/Necessity/auxiliary được giữ; loại T4 của VDS **trước** resource filtering; chọn A100 của VTT. |
| Reservation | CP commit Job + GPUs + command | Job ASSIGNED, Assignment RESERVED, START_CONTAINER gắn đúng Agent. |
| Execution | Agent poll và CommandExecutor gọi simulated runtime | Container mô phỏng running; ACK và FULL inventory cập nhật Job RUNNING, reservation ALLOCATED. Có thể không thấy STARTING trung gian. |
| Stop | Bấm Dừng workload, xác nhận | STOP_CONTAINER → simulated exited → inventory/reconcile → STOPPED + RELEASED. GPU 1–3 FREE nếu không blocker khác; GPU 0 vẫn LEGACY. |

Simulator **không chạy payload/image/CUDA thật**, không tự tạo tải GPU theo Job. NVML adapter là cùng code production, nạp mock shared library bằng `LD_LIBRARY_PATH` và `MOCK_NVML_CONFIG`; chỉ DockerRuntime được thay.

### 4. Kiểm thử cách ly đơn vị và failure

- Đang login VTT: yêu cầu profile chỉ VDS có hoặc yêu cầu nhiều hơn 3 GPU khi A100 GPU0 bị External chiếm → QUEUED; không chọn VDS dù VDS rảnh.
- Postman VTT: GET Server/Job ID của VDS → 404; stop Job VDS → 404; query `organizationId=VDS_ID` → 403; Submit body thêm `organizationId` → 400 strict DTO; tạo enrollment có organizationId dù cùng đơn vị → 403 với non-admin.
- ADMIN xem VDS được; Job do ADMIN submit vẫn thuộc **organization của account ADMIN**, bộ lọc chỉ để xem. Muốn submit cho VDS, đăng nhập account VDS.
- Muốn mô phỏng start thất bại: `simulator.Config` hỗ trợ `AIWM_SIM_REJECT_STARTS`; dùng một Agent Sim riêng/token/machine/profile không trùng UUID và setting này. Chưa có command/script tenant-aware tự dựng scenario đó: `TODO: command not defined by current repository`.
- Các scripts `acceptance.py`, `recovery.py`, `doctor.py` cũ còn dựa token chung; chưa được migrate sang session/enrollment mới. Không dùng kết quả lịch sử của chúng để chứng nhận task này.

## Real Deployment Demo — Control Plane Host

Máy trung tâm chạy Control Plane/frontend/storage. Chỉ máy AIWM/CP host có checkout; **GPU host không clone repo, không cần Go compiler**.

### A. Deploy Control Plane

Trên CP host Linux có Docker/Compose và Python 3, tại workspace root:

~~~bash
python3 scripts/configure.py --postgres-port 15432
~~~

Trong .env đặt các giá trị sau (hoặc IP interface cụ thể thay 0.0.0.0):

~~~dotenv
AIWM_CP_BIND_HOST=0.0.0.0
AIWM_CONSOLE_BIND_HOST=0.0.0.0
AIWM_SESSION_COOKIE_SECURE=false
~~~

Giữ bootstrap credentials do configure sinh, không seed demo-users.json trên deployment này. PostgreSQL chỉ publish loopback:15432; Agent không kết nối database.

~~~bash
docker compose -p aiwm-real --profile console build control-plane console
docker compose -p aiwm-real up -d --wait postgres
docker compose -p aiwm-real run --rm --no-deps control-plane --migrate
# Chỉ lần đầu, khi database chưa có user:
docker compose -p aiwm-real run --rm --no-deps control-plane --bootstrap-admin
docker compose -p aiwm-real up -d control-plane console
curl -fsS http://127.0.0.1:8080/healthz
~~~

Login tại http://IP-hoặc-DNS-thật-của-CP:3000/login bằng bootstrap account. ADMIN tạo Organization VTT/VDS/... và user tương ứng trên màn quản lý. Không chạy aiwm-real cùng ports 8080/3000 với fake demo trên một host.

Lab mạng nội bộ có thể dùng HTTP:8080. HTTPS cần TLS termination theo hạ tầng đơn vị; repo chưa có reverse-proxy/certificate automation. Frontend qua HTTPS đặt AIWM_SESSION_COOKIE_SECURE=true rồi recreate console. Chỉ mở cổng cần thiết cho các GPU host.

Trên máy development, nhập URL ổn định mà GPU servers truy cập được, không dùng localhost:

~~~bash
read -r -p "Control Plane URL thực tế (http(s)://host[:port]): " CONTROL_PLANE_URL
export CONTROL_PLANE_URL
curl -fsS "$CONTROL_PLANE_URL/healthz"
~~~

### B. AIWM team build một lần

Development cần Bash + Docker/BuildKit; Go/GCC nằm trong builder. Windows chạy từ WSL. Script dùng Docker CLI Linux; nếu thiếu socket mà có Docker Desktop Windows CLI thì dùng docker.exe và chuyển path bằng wslpath, không sửa cấu hình host.

~~~bash
bash scripts/build-agent-release.sh --control-plane-url "$CONTROL_PLANE_URL"
~~~

Output mặc định **dist/aiwm-agent-0.2.0-linux-amd64/** gồm aiwm-agent, install-agent-linux.sh, aiwm-agent.service, BUILD_INFO.txt và SHA256SUMS. BUILD_INFO ghi version/platform, glibc của builder và default URL. Checksum giúp đối chiếu file, không thay chữ ký release. Artifact không chứa enrollment/password.

Script giữ nguyên output đã có, kể cả thư mục còn từ build lỗi; chọn --output-dir dist/agent-release-lan-01 khi cần. Artifact validation trên laptop dùng URL localhost: **build với URL CP thật trước khi phân phối**.

URL precedence:

1. AIWM_CONTROL_PLANE_URL từ process environment hoặc /etc/aiwm-agent/agent.env;
2. config.ReleaseControlPlaneURL inject bằng -X github.com/VDT-AI-2026/aiwm-docker-control-plane/internal/agent/config.ReleaseControlPlaneURL=...;
3. http://localhost:8080 cho development.

Helpers nhận HTTP(S) origin hostname/IPv4 và port tùy chọn, không credentials/query/path prefix. Thay CP URL bằng runtime override rồi restart Agent, không cần rebuild theo organization.

### C. Enrollment riêng từng server

UI: ADMIN → Server onboarding → chọn organization → đặt tên → tạo enrollment. Normal user dùng đơn vị của account. Hoặc từ máy AIWM, password nhập ẩn:

~~~bash
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username admin enroll \
  --organization VTT --name VTT-GPU-01 --token-file .cache/enrollments/vtt-01.token
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username admin enroll \
  --organization VTT --name VTT-GPU-02 --token-file .cache/enrollments/vtt-02.token
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username admin enroll \
  --organization VDS --name VDS-GPU-01 --token-file .cache/enrollments/vds-01.token
~~~

Helper không in token, tạo file mới mode 0600 trên Linux và không ghi đè file/enrollment đang tồn tại. Nếu API/write file thất bại, xem enrollment theo tên trên UI/API; revoke cái chưa dùng qua POST /api/v1/enrollments/{enrollmentID}/revoke trước khi cấp lại. Không revoke enrollment của Agent đang hoạt động để thử lệnh này.

Cả ba server nhận **cùng release**, token khác nhau. CP bind token → ServerID → OrganizationID; register bind thêm MachineID. Agent không nhận organizationCode. Enrollment chưa bind hết hạn sau 24 giờ; sau bind chỉ cùng machine được re-register đến khi revoke.

## REAL GPU SERVER E2E — VTT-GPU-01

### 1. Baseline và kiểm tra chỉ đọc

Target release: **Linux x86_64, glibc từ 2.31**, Docker standalone, NVIDIA Driver/NVML, GPU NVIDIA, Container Toolkit đã cấu hình runtime nvidia. Nhắm Ubuntu/Debian và RHEL/Rocky/Alma khi ABI/dependencies phù hợp; không chứng nhận mọi phiên bản distro. ARM64, musl/Alpine và glibc thấp hơn baseline chưa được chứng nhận.

Trên GPU host, account có quyền Docker:

~~~bash
uname -s
uname -m
getconf GNU_LIBC_VERSION
test -s /etc/machine-id
docker version
docker info --format '{{json .Runtimes}}'
ls -l /var/run/docker.sock
nvidia-smi -L
nvidia-smi
~~~

Cần Linux/x86_64, Docker đáp ứng, runtime nvidia và GPU thật. Không probe --gpus all hoặc stop existing containers. Driver phải tương thích image workload; preflight chưa chứng nhận CUDA execution. CDI-only chưa được chấp nhận nếu Docker Info không công bố runtime nvidia.

### 2. Chuyển release; không clone repo

Từ máy AIWM, dùng output directory thật nếu đã thay bằng --output-dir:

~~~bash
read -r -p "SSH target của VTT-01 (user@host): " GPU_SSH
scp -r dist/aiwm-agent-0.2.0-linux-amd64 "$GPU_SSH:~/aiwm-agent-release"
scp .cache/enrollments/vtt-01.token "$GPU_SSH:~/aiwm-enrollment.token"
ssh "$GPU_SSH"
~~~

Dùng thư mục đích mới để tránh lồng directory từ lần copy trước. Trên GPU server:

~~~bash
cd ~/aiwm-agent-release
sha256sum -c SHA256SUMS
chmod 600 ~/aiwm-enrollment.token
chmod +x aiwm-agent install-agent-linux.sh
./aiwm-agent --version
sudo ./aiwm-agent --check
# URL trong BUILD_INFO là public; kiểm tra từ chính GPU host:
CP_URL="$(sed -n 's/^ControlPlaneURL=//p' BUILD_INFO.txt)"
curl -fsS "$CP_URL/healthz"
sudo ./install-agent-linux.sh --binary ./aiwm-agent \
  --enrollment-token-file "$HOME/aiwm-enrollment.token"
~~~

Có thể bỏ token options để nhập token ẩn, hoặc dùng --enrollment-token "$TOKEN"; file/prompt tránh lộ token trong process arguments. URL thường không cần nhập. Nếu cần override, thêm --control-plane-url "$CONTROL_PLANE_URL" vào lệnh install; biến URL phải được nhập lại trên SSH terminal.

Installer kiểm tra Linux/amd64, MachineID, Docker socket/group, preflight thật rồi tạo account aiwm-agent, binary /usr/local/bin/aiwm-agent, config /etc/aiwm-agent/agent.env (root:0600), state /var/lib/aiwm-agent (aiwm-agent:0700), kiểm tra quyền service và cài unit. Không cài/restart Docker/driver hoặc thao tác containers.

Thiếu dependency thì sửa theo quy trình đơn vị rồi check lại. Installer từ chối ghi đè installation có sẵn. Nếu check quyền service thất bại sau khi copy binary/config, giữ các file và sửa quyền trước khi start; không xóa state. Sau khi runuser --check PASS, có thể cài unit từ release bằng:

~~~bash
sudo install -m 0644 ./aiwm-agent.service /etc/systemd/system/aiwm-agent.service
sudo systemctl daemon-reload
~~~

### 3. Preflight và start

~~~bash
sudo /usr/local/bin/aiwm-agent --check
sudo runuser -u aiwm-agent -g aiwm-agent -G docker -- /usr/local/bin/aiwm-agent --check
sudo systemctl enable --now aiwm-agent
sudo systemctl status aiwm-agent --no-pager
sudo journalctl -u aiwm-agent -n 50 --no-pager
~~~

--check không register/pull/start/stop container. JSON báo OS/architecture/kernel/cgroup, MachineID availability, Docker reachable/version/API/runtime nvidia, NVML/GPU count; sau đó thử inventory. SUPPORTED + exit 0 là điều kiện nền tảng; DEGRADED/UNSUPPORTED + exit khác 0 phải xử lý trước. Inventory health/free/occupancy vẫn quyết định scheduling.

Unit nạp config, restart on failure, chỉ quản lý Agent process. Nếu không có systemd:

~~~bash
sudo sh -c 'set -a; . /etc/aiwm-agent/agent.env; exec /usr/local/bin/aiwm-agent'
~~~

Foreground cần terminal/supervisor của đơn vị. Token vẫn cần để re-register/re-auth nên giữ config bảo vệ.

### 4. Verify ownership/inventory và workload nhẹ

Trên máy AIWM:

~~~bash
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username vtt servers
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username vtt smoke
~~~

Trước submit: server ONLINE + schedulable=true, đúng đơn vị; đối chiếu UUID/model/VRAM với nvidia-smi. GPU existing/unknown không được FREE.

Smoke dùng **demo/workloads/real-gpu-smoke.json**: 1 GPU/general/FP8=false, INFERENCE/N4; nvidia/cuda:12.4.1-base-ubuntu22.04 chạy nvidia-smi -L rồi sleep 300; helper điền neededAt. Có thể --image cho mirror/image được phê duyệt và có cùng command. Manifest có Linux amd64 đã được xác nhận; chưa chạy trên GPU thật trong phiên này.

User submit lấy organization từ identity. ADMIN hiện submit theo home organization: muốn test VTT hãy dùng vtt. Nếu có nhiều VTT servers, scheduler chọn bất kỳ server phù hợp trong VTT; không ép VTT-01. Model ngoài catalog T4/A100/H100 có thể inventory được nhưng profile general chưa chấp nhận; task này không sửa catalog.

~~~bash
read -r -p "Job ID vừa tạo: " JOB_ID
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username vtt status --job-id "$JOB_ID"
~~~

Theo dõi Job đến RUNNING, Assignment chỉ đúng đơn vị và GPU UUID. Agent pull image nếu thiếu, create/start Docker bằng exact DeviceRequests. Thiếu GPU thì QUEUED.

Trên **server được assignment**, nhập cùng Job ID:

~~~bash
read -r -p "Job ID cần kiểm tra: " JOB_ID
docker ps --filter "label=aiwm.job-id=$JOB_ID"
CONTAINER_ID="$(docker ps -q --filter "label=aiwm.job-id=$JOB_ID")"
test -n "$CONTAINER_ID"
docker inspect --format '{{json .HostConfig.DeviceRequests}}' "$CONTAINER_ID"
docker logs --tail 20 "$CONTAINER_ID"
nvidia-smi
~~~

Log in GPU được cấp; sleep giữ container RUNNING đủ quan sát. Đây là GPU visibility/runtime smoke, không benchmark/training; utilization có thể bằng 0.

### 5. Stop/finish và release

Trên máy AIWM (hoặc Stop trên UI):

~~~bash
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username vtt stop --job-id "$JOB_ID"
python3 scripts/agent-admin.py --base-url "$CONTROL_PLANE_URL" --username vtt status --job-id "$JOB_ID"
~~~

Chờ terminal + Assignment.reservationState=RELEASED. Nếu không stop, command kết thúc sau 300 giây rồi được reconcile. FREE chỉ khi không còn blocker. Pull/create/start lỗi phản ánh failure; khi CP nhận ACK/reconciliation kết quả mới release. Mất Agent không tự release.

### 6. Failure, restart và reinstall

Chỉ thử trên server được phép, Job chạy đủ lâu và đã ghi CONTAINER_ID. Không dừng Docker daemon:

~~~bash
sudo systemctl stop aiwm-agent
docker inspect --format '{{.State.Running}} {{.State.StartedAt}}' "$CONTAINER_ID"
# Chờ quá offline timeout + nhịp reconcile rồi đối chiếu UI/API.
sudo systemctl start aiwm-agent
sudo journalctl -u aiwm-agent -n 50 --no-pager
~~~

Container tiếp tục chạy; CP OFFLINE/unschedulable, giữ assignment/reservation/inventory. Restart load state, verify MachineID, FULL inventory/reconcile, fresh/healthy mới được placement.

Crash riêng Agent (service tự restart, có thể nhanh hơn offline timeout):

~~~bash
sudo systemctl kill --kill-whom=main --signal=SIGKILL aiwm-agent
sudo systemctl status aiwm-agent --no-pager
~~~

Trên CP host, unavailable riêng CP:

~~~bash
docker compose -p aiwm-real stop control-plane
# Đối chiếu container trên GPU host vẫn chạy và Agent log retry.
docker compose -p aiwm-real start control-plane
docker compose -p aiwm-real logs --tail 30 control-plane
~~~

Agent retry/backoff+jitter, resync sau kết nối lại. Chỉ heartbeat không phân biệt Agent chết, host chết và network partition. Không mở rộng HA/out-of-band monitoring.

Upgrade cùng host: giữ config/state/MachineID, verify checksum release mới rồi:

~~~bash
sudo systemctl stop aiwm-agent
sudo install -m 0755 ./aiwm-agent /usr/local/bin/aiwm-agent
sudo runuser -u aiwm-agent -g aiwm-agent -G docker -- /usr/local/bin/aiwm-agent --check
sudo systemctl start aiwm-agent
~~~

Nếu check thất bại, giữ Agent stopped để xử lý; containers vẫn độc lập. MachineID đổi sau reinstall thì Runner từ chối state của máy khác: cấp enrollment mới, archive state cũ theo quy trình đơn vị rồi onboarding; không silently reuse token/state hoặc xóa assignment cũ.

### DEVELOPMENT / SOURCE TEST — chỉ máy AIWM

GPU host production không cần Go compiler. Development/troubleshoot tại checkout backend Linux có Go/GCC:

~~~bash
CGO_ENABLED=1 go build -trimpath -o bin/aiwm-agent ./cmd/aiwm-agent
./bin/aiwm-agent --check
~~~

Đây không phải quy trình cài cho từng đơn vị. Real GPU E2E chỉ PASS sau khi chạy trên NVIDIA host thật; mock không thay kiểm chứng này.


## Các kiểm tra thủ công sau khi review code

Các lệnh rộng dưới đây là tùy chọn development; không phải prerequisite trên GPU server production. Kết quả targeted release tests nằm trong IMPLEMENTATION_STATUS. Chạy trong backend:

```powershell
go fmt ./...
go vet ./...
go test ./...
go build ./cmd/...
```

Trong frontend:

```powershell
npm.cmd run lint
npm.cmd run typecheck
npm.cmd test
npm.cmd run build
```

Fixture scheduler/benchmark/allocation và memory safety đã thêm ownership; regression tests HTTP identity/application login cũng đã viết nhưng chưa chạy. Các fixtures/automation khác dùng bearer chung hoặc ownership rỗng vẫn có thể cần chuyển sang Organization/session/enrollment trước khi full suite đạt. Các test và số liệu PASS của task trước không chứng minh phiên bản này đã chạy đạt.

## Fixture Postman protocol tùy chọn

Folder Organization - Protocol nội bộ cần CP/state/database kiểm thử riêng và account ADMIN riêng; không gửi fake inventory vào Agent đang chạy. Đặt internal_base_url khác base_url, internal_username/internal_password, enable_agent_internal=true. Folder tự login, tạo enrollment và sinh MachineID/UUID độc lập rồi thực hiện protocol cũ. Không dùng nó để chứng minh Docker/NVML execution. Repo chưa có script dựng CP/database fixture độc lập theo tenancy: **TODO: command not defined by current repository**. Dùng LEVEL 2 để kiểm thử fake Agent đầy đủ mà không cần tự gửi inventory.

## Nguồn và phạm vi

| Nội dung | Source |
|---|---|
| Lab PostgreSQL/CP/Agents/Console | `compose.yaml`, `compose.scenarios.yaml`, `scripts/configure.py` ở workspace root |
| Binary và migration/bootstrap | Backend `Makefile`, `cmd/aiwm-server/main.go`, `internal/store/postgres/001_metadata.sql` |
| Agent config/preflight/service | Backend `.env.agent.example`, `internal/agent/config/config.go`, `preflight.go`, `deploy/agent/aiwm-agent.service`, `docs/agent-deployment.md` |
| Fake GPU | Backend `Dockerfile.sim`, `internal/simulator/config.go`, `deploy/simulation/profiles/` |
| API fixture và auth | [API_TESTING_POSTMAN.md](API_TESTING_POSTMAN.md), `postman/AIWM.postman_collection.json` |
