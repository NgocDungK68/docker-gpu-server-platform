# Test trên laptop Windows và WSL

## Đường chạy đã kiểm chứng trên máy này

Windows PowerShell + Docker Desktop **Linux containers** chạy đủ demo, không cần GPU thật và không cần tự chuyển code sang WSL. Python, Go, Node/npm.cmd có sẵn trên Windows; Microsoft Edge dùng cho Playwright. Docker Desktop đã dùng WSL2; Ubuntu WSL2 cũng tồn tại.

Agent NVML dùng Linux shared library nên không chạy native bằng go run cmd/aiwm-agent-sim trên Windows. Windows vẫn chạy được Control Plane, benchmark, Go business tests và frontend. Dockerfile.sim/test cung cấp Linux/CGO/GCC cho NVML và race tests.

## A. Đơn giản nhất: cả backend, Agent Sim và frontend trong Docker

PowerShell tại đúng workspace:

~~~powershell
cd F:\Viettel\VDT\demo-project
python scripts/configure.py
python scripts/doctor.py
docker compose --profile console up --build -d
docker compose --profile console ps
~~~

Mở http://127.0.0.1:3000. Sau khi hai Agent có inventory:

~~~powershell
python scripts/acceptance.py
python scripts/recovery.py
python scripts/acceptance.py --base-url http://127.0.0.1:3000/api/aiwm
~~~

Các script dùng Python standard library, không cần pip install. Doctor không in token. Configure điền cấu hình còn thiếu/trống, giữ giá trị đã tự đặt; nếu Doctor báo mismatch, đồng bộ đúng token giữa các file được nêu, không dán secret vào issue/log.

Acceptance nên bắt đầu khi không có job demo khác đang chiếm GPU; các scripts không dừng workload của người dùng để giành GPU. Chạy lần lượt. Recovery chủ động stop/start **dịch vụ demo** rồi khôi phục. State volumes giữ nguyên qua restart.

## B. Sửa/test frontend trên Windows

Nếu Console Docker đang dùng port 3000, dừng riêng nó trước khi chạy Next trên host:

~~~powershell
docker compose --profile console stop console
cd AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console
npm.cmd ci
npm.cmd run check
npm.cmd run dev -- --hostname 127.0.0.1
~~~

Terminal PowerShell khác tại frontend:

~~~powershell
$env:AIWM_BROWSER_CHANNEL="msedge"
npm.cmd run test:e2e
~~~

Dùng npm.cmd để tránh lỗi execution policy của npm.ps1. Không cần đổi execution policy toàn máy. E2E cũng chạy được khi frontend là Docker ở phương án A, miễn localhost:3000 đã sẵn sàng.

## C. Test backend/Agent trên Windows

Trong backend:

~~~powershell
cd F:\Viettel\VDT\demo-project\AIWM-Docker-GPU-Scheduler-with-Agent\aiwm-docker-control-plane
go vet ./...
go test ./...
go build ./cmd/...
go run ./cmd/aiwm-scheduler-bench
docker build -f Dockerfile.sim -t aiwm-agent-sim:local .
docker build -f Dockerfile.test -t aiwm-tests:local .
~~~

Go native build compile cả unsupported-platform stub cho Agent. Dockerfile.test mới là bước compile Linux production Agent và kiểm chứng NVML mock + race. Không cần cài GCC vào Windows để chạy đường này.

Nếu muốn CP native, dừng riêng CP Compose trước (tránh port 8080), chạy go run ./cmd/aiwm-server trong backend. Dữ liệu CP native và volume Compose là hai state riêng; để demo nhất quán nên giữ CP/Agents cùng root Compose.

## D. Chạy từ Ubuntu WSL nếu muốn

Trong Docker Desktop, bật WSL integration cho Ubuntu nếu docker info ở WSL không tới được engine. Dùng cùng Docker Desktop daemon; không cần dựng Docker daemon thứ hai.

~~~sh
cd /mnt/f/Viettel/VDT/demo-project
python3 scripts/configure.py
python3 scripts/doctor.py
docker compose --profile console up --build -d
python3 scripts/acceptance.py
python3 scripts/recovery.py
~~~

Toàn bộ binaries và Next build chạy trong container, nên đường này không cần cài Node/Go vào Ubuntu. Windows và WSL nhìn cùng thư mục và volume Docker; không chạy hai bản demo cạnh tranh cùng port/machine ID.

Nếu phát triển native trong WSL, cần Go >=1.24 + GCC và Node24 Linux. Dùng checkout riêng dưới filesystem Linux để tránh chậm; không dùng chung node_modules/.next tạo bởi Windows với Linux. README không yêu cầu bạn chuyển repo đang chạy.

## Lỗi thường gặp

| Hiện tượng | Cách kiểm tra/xử lý |
|---|---|
| Cannot connect Docker daemon | Mở Docker Desktop, đợi engine Running, chạy docker info |
| WSL có docker nhưng không kết nối | Bật Ubuntu WSL integration và dùng đúng Docker Desktop context |
| npm.ps1 cannot be loaded | Dùng npm.cmd trong PowerShell |
| Port 3000 đã dùng | Chọn Console Docker hoặc Next host; dừng đúng tiến trình/service bạn đã mở |
| HTTP401 từ public API | Doctor kiểm tra token mismatch; sửa config, restart CP/Next tương ứng |
| Agent không xuất hiện | docker compose logs --tail 50 agent-a100 agent-t4; kiểm tra enrollment, MOCK_NVML_CONFIG, volume |
| Job QUEUED | Đọc statusReason: labels/model/VRAM/count, Offline/Drain hoặc GPU đang bận |
| NVML unsupported trên Windows | Chạy Dockerfile.sim/test; không thay NVML bằng fake Go |
| Build lần đầu chậm | Tải Go modules, Node deps và NVIDIA library; lần sau Docker layer cache được tái dùng |

Không dùng nvidia-smi trên Windows làm tiêu chí pass cho NVML mock. GPU thật/CUDA là đường test khác cần Linux NVIDIA server và Container Toolkit; simulator không chạy lệnh training thực.
