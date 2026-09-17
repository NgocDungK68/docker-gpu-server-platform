#!/usr/bin/env bash
# Chỉ chạy tại development/CI; artifact không chứa enrollment/password.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
backend="$root/AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane"
url=""
output=""
while (($#)); do
  case "$1" in
    --control-plane-url) url="${2:?Thiếu URL}"; shift 2 ;;
    --output-dir) output="${2:?Thiếu output directory}"; shift 2 ;;
    --help) echo "bash scripts/build-agent-release.sh [--control-plane-url URL] [--output-dir DIR]"; exit 0 ;;
    *) echo "Tham số không hợp lệ: $1" >&2; exit 2 ;;
  esac
done
if [[ -n "$url" && ! "$url" =~ ^https?://[a-zA-Z0-9.-]+(:[0-9]+)?/?$ ]]; then
  echo "URL phải là HTTP(S) origin, không credentials/query/path; dùng hostname hoặc IPv4." >&2
  exit 2
fi
command -v docker >/dev/null || { echo "Máy build cần Docker Engine/BuildKit." >&2; exit 1; }
version="$(sed -n 's/^const agentVersion = "\([^"]*\)".*/\1/p' "$backend/cmd/aiwm-agent/main.go")"
[[ "$version" =~ ^[0-9A-Za-z._-]+$ ]] || { echo "Không xác định được Agent version." >&2; exit 1; }
output="${output:-$root/dist/aiwm-agent-$version-linux-amd64}"
[[ ! -e "$output" ]] || { echo "Output đã tồn tại; giữ nguyên. Dùng --output-dir khác." >&2; exit 1; }
mkdir -p "$output"
output="$(cd "$output" && pwd)"
docker build --platform linux/amd64 --target artifact \
  --build-arg "CONTROL_PLANE_URL=$url" \
  --output "type=local,dest=$output" \
  -f "$backend/Dockerfile.agent-release" "$backend"
cp "$root/scripts/install-agent-linux.sh" "$output/install-agent-linux.sh"
cp "$backend/deploy/agent/aiwm-agent.service" "$output/aiwm-agent.service"
chmod 755 "$output/aiwm-agent" "$output/install-agent-linux.sh"
(
  cd "$output"
  sha256sum aiwm-agent install-agent-linux.sh aiwm-agent.service BUILD_INFO.txt > SHA256SUMS
)
printf 'Release: %s\nChuyển toàn bộ directory + enrollment riêng cho từng server.\n' "$output"
[[ -n "$url" ]] || echo "Chưa inject URL: release dùng development fallback localhost; production cần URL thật."
