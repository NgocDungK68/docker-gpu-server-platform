# Runbook kiểm thử AIWM

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

GPU server production sẽ nhận generic Agent release + installer + enrollment riêng, không clone repo và không cần Go compiler. Release builder/installer đang là bước kế tiếp của Phase B. Phần LEVEL 3 build từ source phía dưới chỉ là DEVELOPMENT/TROUBLESHOOTING, chưa phải quy trình production đã kiểm chứng. Chưa có GPU NVIDIA vật lý để chứng nhận real E2E.

---
Tài liệu dành cho người chạy thủ công. **Task implementation này không chạy build, test, migration, Docker, npm hoặc network.** Các lệnh dưới đây dựa trên entrypoint, Compose, Makefile, script và routes hiện có; kết quả mong đợi chưa phải kết quả đã kiểm chứng.

## Chuẩn bị chung và bảo toàn dữ liệu

- Workspace root: thư mục chứa README, `compose.yaml`, `scripts/`.
- Backend: `AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane`.
- Frontend: `AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console`.
- Windows: PowerShell, Python 3.10+, Go ≥1.24, Node.js 24; PostgreSQL local hoặc Docker Desktop Linux containers. Agent production chỉ chạy trên Linux. Fake Agent dùng Linux container; không cần GPU vật lý trên laptop.
- Dùng project Compose riêng `aiwm-org-demo` để không đụng volume demo cũ. Port 5432/8080/3000 phải trống; không chạy CP native và CP container cùng port.
- `scripts/configure.py` sinh credentials local nếu thiếu, giữ giá trị cũ, không in secret. Đọc username/password bootstrap trong `.env` bằng editor, không commit/export credentials. Có thể đổi seed `AIWM_BOOTSTRAP_ORGANIZATION_CODE/NAME` trước khi bootstrap.
- VTT/Viettel Telecom là seed mặc định của script; VTNET/Viettel Network và VDS/Viettel Digital Services có thể tạo qua UI/API. Không có mã đơn vị trong scheduler.
- Runtime snapshot cũ thiếu `organizationId` không tự được chuyển ownership; Job cũ không nhận placement. Không xóa snapshot/state cũ. Với native demo mới, chọn `AIWM_STATE_FILE=.local/control-plane-org.gob`. Việc chuyển ownership dữ liệu cũ đang chạy cần quy trình riêng; chưa có migration tự động.
- `go.mod` thêm `lib/pq`. Chưa tải dependency trong task; chạy `go mod tidy` thủ công trước build và review `go.mod/go.sum`.

## LEVEL 1 — Backend/frontend local

### 1. Cấu hình và PostgreSQL

Tại workspace root, PowerShell:

```powershell
python scripts/configure.py
docker compose -p aiwm-org-demo up -d postgres
docker compose -p aiwm-org-demo logs --tail 30 postgres
```

Chờ PostgreSQL báo sẵn sàng nhận kết nối. Script tạo `AIWM_DATABASE_URL` dùng `localhost:5432` cho backend native; Compose CP dùng `AIWM_COMPOSE_DATABASE_URL` với hostname `postgres`. `sslmode=disable` chỉ cho lab local; môi trường nội bộ qua mạng cần cấu hình TLS phù hợp.

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

## LEVEL 3 — GPU server thật (Linux)

### 1. Prerequisites

Control Plane/PostgreSQL đã chạy và có Organization/User/Enrollment. GPU host cần Linux, Go ≥1.24 nếu build tại host, CGO/GCC, Docker Engine, NVIDIA Driver/NVML và NVIDIA Container Toolkit cấu hình runtime `nvidia`. Quyền Docker socket cho account chạy Agent và quyền đọc NVIDIA devices/NVML.

Repo không có script cài driver/toolkit theo distro hoặc copy source sang host: **`TODO: command not defined by current repository`**. Chuẩn bị bằng quy trình của đơn vị. CP Compose mặc định chỉ bind 127.0.0.1; GPU server từ xa cần endpoint CP có thể truy cập, TLS/firewall theo deployment nội bộ. Với CP native, `AIWM_HTTP_ADDR`, `AIWM_TLS_CERT_FILE`, `AIWM_TLS_KEY_FILE` là config thực có.

### 2. Build/cấu hình/preflight

Trên Linux GPU host, tại backend root, theo Makefile và `docs/agent-deployment.md` của backend:

```bash
go mod tidy
CGO_ENABLED=1 go build -trimpath -o bin/aiwm-agent ./cmd/aiwm-agent
cp .env.agent.example .env.agent
chmod 600 .env.agent
```

Chỉ copy example nếu chưa có `.env.agent`; giữ cấu hình hiện có. Điền URL CP thực, enrollment token của đúng server, name và state path. `MachineID` mặc định `/etc/machine-id`; không clone state giữa host. `AIWM_AGENT_LABELS` không đổi ownership; display name/labels authoritative từ enrollment.

```bash
sudo ./bin/aiwm-agent --check
sudo ./bin/aiwm-agent
```

`--check` không cần enrollment token, không register, không pull/start/stop container. Báo JSON gồm OS/architecture/kernel/cgroup, Docker version/API/OS/runtime nvidia, NVML availability/GPU count; sau đó thử FULL inventory. `SUPPORTED` chỉ là điều kiện nền tảng, không cam kết GPU FREE. `DEGRADED`/`UNSUPPORTED` hoặc inventory lỗi → exit khác 0; sửa môi trường rồi chạy lại. Startup production cũng dùng preflight.

Socket permission được kiểm tra bằng truy cập Docker API với account hiện tại, không sửa quyền. Runtime `nvidia` trong Docker Info là bằng chứng cấu hình, **không chứng minh mọi CUDA image chạy được**; triển khai CDI-only có thể bị đánh giá DEGRADED vì MVP chưa xác minh đường này. Non-Linux không giả thành công.

Có thể đối chiếu bằng `docker ps` và `nvidia-smi` như hướng dẫn Agent hiện có; chạy bằng account có quyền. Không thêm container probe hoặc `--gpus all` vào server đang dùng.

### 3. Register → real workload → release

1. UI/API: server đúng Organization, ONLINE **và** schedulable; NVML UUID/model/VRAM trùng GPU host. Kiểm tra External/unknown trước khi cấp phát.
2. Submit Job nhỏ như LEVEL 2, `gpuCount=1`, profile phù hợp GPU thật, FP8=false; không nhập UUID/server/strategy. Image phải được phép trong môi trường; `alpine:3.21` + `sleep 300` kiểm tra container/GPU grant, không chứng minh CUDA computation.
3. CP tạo START_CONTAINER; Agent kiểm tra lại local occupancy, inspect image và pull nếu chưa có, create/start Docker với DeviceRequests driver `nvidia` và exact UUID.
4. API/UI: Assignment server/UUID đúng đơn vị, Job RUNNING sau inventory. Trên host `docker ps` thấy container `aiwm-<jobID>`; `nvidia-smi` hiển thị trạng thái GPU thực. Workload sleep có thể không tạo GPU process/utilization; đó không phải lỗi allocation.
5. Stop từ UI/API; inventory thấy terminal → Job STOPPED, Assignment RELEASED. Chỉ FREE nếu không còn External/unknown/unhealthy/managed blocker.
6. Image pull/create/start lỗi → ACK failed, Command FAILED, Job FAILED và release logical reservation. Không có command-status public endpoint riêng; dùng Job status/events/Assignment. Nếu ACK chưa tới vì network, giữ reservation tới khi biết kết quả; không release chỉ dựa timeout Agent.

### 4. Failure tests trên môi trường được phép

**A. Dừng riêng Agent:** giữ Job đang running lâu đủ quan sát. Với Agent foreground, Ctrl+C ở terminal Agent; đây là stop graceful, không phải SIGKILL. Container Docker tiếp tục running. Sau ngưỡng offline (mặc định 20 giây, cộng nhịp Reconcile 5 giây), UI báo OFFLINE/schedulable=false. Submit mới cùng đơn vị không lấy GPU trên server đó. Không đánh dấu GPU FREE vì mất heartbeat.

Nếu đã cài service theo `deploy/agent/aiwm-agent.service` và hướng dẫn backend:

```bash
sudo systemctl stop aiwm-agent
sudo systemctl start aiwm-agent
sudo systemctl status aiwm-agent
```

Thử crash cưỡng bức bằng PID-specific kill chưa có command/script được repo định nghĩa: `TODO: command not defined by current repository`. Không dùng kill theo tên rộng trên host dùng chung.

**B. Khởi động lại Agent:** chạy lại `sudo ./bin/aiwm-agent` hoặc start service. Agent load state, verify MachineID, re-register cùng enrollment/machine, lưu credential, tăng sequence, gửi FULL inventory. CP giữ last-known assignment/inventory và chờ snapshot mới trước placement. Reconciliation nhận lại container đang chạy, không start bản sao.

**C. CP tạm ngừng:** nếu CP dùng Compose project demo:

```powershell
docker compose -p aiwm-org-demo stop control-plane
docker compose -p aiwm-org-demo start control-plane
docker compose -p aiwm-org-demo logs --tail 30 control-plane
```

Container trên GPU host tiếp tục chạy. Agent startup/heartbeat/command poll retry exponential backoff + jitter, cap 60 giây; inventory giữ interval định kỳ. Sau phục hồi, heartbeat không đủ mở scheduling: phải có FULL inventory fresh. Không dừng PostgreSQL/Docker daemon/GPU host trong phép thử mất CP.

Chưa có script fault injection network partition trong repo: `TODO: command not defined by current repository`. Chỉ heartbeat không thể phân biệt Agent crash, host chết và network partition. Không có out-of-band monitor.

## Các kiểm tra thủ công sau khi review code

Không có lệnh nào dưới đây đã chạy trong task. Sau khi tải dependency, người dùng có thể chạy trong backend:

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
