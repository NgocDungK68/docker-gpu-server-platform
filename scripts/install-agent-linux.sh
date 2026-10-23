#!/usr/bin/env bash
# Installer độc lập: gửi cùng binary + service template, không cần source/Go.
set -euo pipefail
umask 077
package="$(cd "$(dirname "$0")" && pwd)"
binary="$package/aiwm-agent"
token=""
token_file=""
url=""
fail() { echo "$*" >&2; exit 1; }
while (($#)); do
  case "$1" in
    --binary) binary="${2:?Thiếu binary}"; shift 2 ;;
    --enrollment-token) token="${2:?Thiếu token}"; shift 2 ;;
    --enrollment-token-file) token_file="${2:?Thiếu token file}"; shift 2 ;;
    --control-plane-url) url="${2:?Thiếu URL}"; shift 2 ;;
    --help) echo "sudo bash install-agent-linux.sh --binary ./aiwm-agent [--enrollment-token-file FILE | --enrollment-token TOKEN] [--control-plane-url URL]"; exit 0 ;;
    *) fail "Tham số không hợp lệ" ;;
  esac
done
[[ "$(uname -s)" == Linux ]] || fail "UNSUPPORTED: yêu cầu Linux."
[[ "$(uname -m)" == x86_64 ]] || fail "UNSUPPORTED: release này chỉ xác nhận Linux amd64."
[[ "$EUID" == 0 ]] || fail "Cần sudo để cài service và config bảo vệ."
[[ -x "$binary" ]] || fail "Không thấy binary executable; kiểm tra artifact/chmod."
binary="$(readlink -f "$binary")"
[[ -r "$package/aiwm-agent.service" ]] || fail "Thiếu aiwm-agent.service trong release package."
[[ -s /etc/machine-id ]] || fail "DEGRADED: thiếu /etc/machine-id, cần host identity ổn định."
for tool in getent groupadd useradd runuser install; do
  command -v "$tool" >/dev/null || fail "Thiếu công cụ Linux: $tool"
done
getent group docker >/dev/null || fail "Thiếu group docker; chuẩn bị Docker socket permissions theo quy trình đơn vị."
[[ -S /var/run/docker.sock ]] || fail "DEGRADED: thiếu local Docker socket; installer không cài/start Docker."
if [[ -n "$token_file" ]]; then
  [[ -z "$token" ]] || fail "Chỉ dùng một nguồn enrollment token."
  token="$(cat -- "$token_file")"
fi
if [[ -z "$token" ]]; then
  read -r -s -p "Enrollment token riêng của server: " token
  printf '\n'
fi
[[ "$token" =~ ^[A-Za-z0-9._-]+$ ]] || fail "Enrollment token không hợp lệ."
if [[ -n "$url" && ! "$url" =~ ^https?://[a-zA-Z0-9.-]+(:[0-9]+)?/?$ ]]; then
  fail "URL override phải là HTTP(S) origin, không credentials/query/path."
fi
for path in /usr/local/bin/aiwm-agent /etc/aiwm-agent/agent.env /etc/systemd/system/aiwm-agent.service; do
  [[ ! -e "$path" ]] || fail "Đã có $path; giữ nguyên config/state, dùng quy trình upgrade trong runbook."
done
# Chỉ đọc Docker/NVML, không register/pull/start/stop container.
(cd / && "$binary" --check) || fail "DEGRADED: preflight thất bại; chuẩn bị Docker, NVIDIA Driver/NVML và runtime nvidia rồi thử lại."
getent group aiwm-agent >/dev/null || groupadd --system aiwm-agent
id aiwm-agent >/dev/null 2>&1 || useradd --system --gid aiwm-agent --no-create-home --shell /usr/sbin/nologin aiwm-agent
install -d -m 0755 /etc/aiwm-agent
install -d -m 0700 -o aiwm-agent -g aiwm-agent /var/lib/aiwm-agent
install -m 0755 "$binary" /usr/local/bin/aiwm-agent
{
  printf 'AIWM_ENROLLMENT_TOKEN=%s\n' "$token"
  printf 'AIWM_AGENT_STATE_FILE=/var/lib/aiwm-agent/state.json\n'
  [[ -z "$url" ]] || printf 'AIWM_CONTROL_PLANE_URL=%s\n' "$url"
} > /etc/aiwm-agent/agent.env
chmod 0600 /etc/aiwm-agent/agent.env
unset token
# Giống User/Group/SupplementaryGroups của systemd unit, kiểm tra quyền thật.
(cd / && runuser -u aiwm-agent -g aiwm-agent -G docker -- /usr/local/bin/aiwm-agent --check) \
  || fail "DEGRADED: account service chưa truy cập được Docker/NVML; sửa quyền trước khi start."
if command -v systemctl >/dev/null && [[ -d /run/systemd/system ]]; then
  install -m 0644 "$package/aiwm-agent.service" /etc/systemd/system/aiwm-agent.service
  systemctl daemon-reload
  echo "Đã cài. Khởi động: sudo systemctl enable --now aiwm-agent"
else
  echo "Không có systemd; chạy foreground bằng sudo sh -c 'set -a; . /etc/aiwm-agent/agent.env; exec /usr/local/bin/aiwm-agent'"
fi
echo "Config: /etc/aiwm-agent/agent.env (0600); state: /var/lib/aiwm-agent. Không đổi Docker/driver/container."
