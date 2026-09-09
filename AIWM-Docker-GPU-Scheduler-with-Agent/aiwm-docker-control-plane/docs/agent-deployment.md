# Triển khai Agent production trên Linux

Agent chỉ cài thêm binary, env và identity state. Docker/NVIDIA driver/Container Toolkit phải có sẵn. Không restart Docker daemon hay container production trong quy trình này.

## Build, cấu hình, preflight

Trong Go project trên Linux có Go >=1.24 và GCC:

~~~sh
CGO_ENABLED=1 go build -trimpath -o bin/aiwm-agent ./cmd/aiwm-agent
cp .env.agent.example .env.agent
chmod 600 .env.agent
~~~

Sửa .env.agent: URL CP, enrollment token tương ứng, tên/labels, state path. Machine ID mặc định /etc/machine-id; không clone identity/state giữa server. --check vẫn load configuration nên cần điền enrollment trước.

~~~sh
sudo ./bin/aiwm-agent --check
sudo ./bin/aiwm-agent
~~~

--check đọc Docker/NVML và log tổng GPU/container/process; không register hoặc đổi container. NVML cần quyền đọc NVIDIA devices; Docker socket cần quyền truy cập. Foreground Ctrl+C chỉ dừng Agent.

Đối chiếu Docker ps và nvidia-smi với Server detail trước khi submit job. Nếu cần kiểm tra NVIDIA Toolkit bằng container mới, chỉ dùng GPU đã xác minh trống, UUID cụ thể và CUDA image được phép trong môi trường; không dùng --gpus all trên server đang có workload.

## systemd

Các lệnh sau chỉ dùng khi triển khai lên host được quản trị:

~~~sh
sudo useradd --system --home /var/lib/aiwm-agent --shell /usr/sbin/nologin aiwm-agent
sudo usermod -aG docker aiwm-agent
sudo install -m 0755 bin/aiwm-agent /usr/local/bin/aiwm-agent
sudo install -d -o root -g aiwm-agent -m 0750 /etc/aiwm-agent
sudo install -o root -g aiwm-agent -m 0640 deploy/agent/agent.env.example /etc/aiwm-agent/agent.env
sudo install -m 0644 deploy/agent/aiwm-agent.service /etc/systemd/system/aiwm-agent.service
~~~

Sửa /etc/aiwm-agent/agent.env (URL/token/labels/CA) trước start. User service cần được phép đọc NVIDIA devices. Docker group là quyền tương đương root; state chứa token phải giữ riêng.

~~~sh
sudo systemctl daemon-reload
sudo systemctl enable --now aiwm-agent
sudo systemctl status aiwm-agent
sudo journalctl -u aiwm-agent -f
~~~

Không copy .env.agent vào journal hoặc terminal output để demo. [Config](../../../docs/CONFIGURATION.md) và [security](../../../docs/SECURITY.md) giải thích TLS/client cert. Client cert cần gateway xác thực mTLS; CP trực tiếp hiện hỗ trợ server TLS, chưa client-CA enforcement.

## Đưa server vào pool

1. Đảm bảo chưa có queued job nhắm host mới trong lúc kiểm chứng inventory.
2. Chạy preflight, đối chiếu existing containers/GPU grants.
3. Start Agent; trên Console kiểm tra External GPU/unknown usage được loại khỏi pool.
4. Drain host trong lúc đối chiếu; bỏ drain khi inventory đúng và mới.
5. Submit job nhỏ vào GPU trống bằng selector riêng; kiểm tra exact UUID trong Docker inspect.
6. Dừng managed job và xác nhận External container ID/StartedAt không đổi.

Agent mất kết nối không kill container. systemctl stop aiwm-agent chỉ stop service; CP mark offline sau timeout. Giữ /var/lib/aiwm-agent/state.json để reconnect và replay command idempotent. Có một khoảng giữa local occupancy check và Docker start; operator phải phối hợp việc cấp GPU trực tiếp ngoài AIWM.

Môi trường phát triển hiện chỉ kiểm chứng production adapter qua tests và NVML mock. Real-GPU/Toolkit/CUDA validation phải thực hiện trên host phù hợp trước rollout.
