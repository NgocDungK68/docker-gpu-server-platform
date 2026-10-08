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
docker_command=docker
windows_cli=false
# WSL có thể chưa bật integration; dùng Docker Desktop CLI của Windows nếu có.
if ! docker version --format '{{.Server.Version}}' >/dev/null 2>&1; then
  if command -v docker.exe >/dev/null && command -v wslpath >/dev/null && docker.exe version --format '{{.Server.Version}}' >/dev/null 2>&1; then
    docker_command=docker.exe
    windows_cli=true
  else
    echo "Máy build cần Docker Engine/BuildKit đang chạy; kiểm tra Docker Desktop WSL integration." >&2
    exit 1
  fi
fi
version="$(sed -n 's/^const agentVersion = "\([^"]*\)".*/\1/p' "$backend/cmd/aiwm-agent/main.go")"
[[ "$version" =~ ^[0-9A-Za-z._-]+$ ]] || { echo "Không xác định được Agent version." >&2; exit 1; }
output="${output:-$root/dist/aiwm-agent-$version-linux-amd64}"
[[ ! -e "$output" ]] || { echo "Output đã tồn tại; giữ nguyên. Dùng --output-dir khác." >&2; exit 1; }
mkdir -p "$output"
output="$(cd "$output" && pwd)"
context="$backend"
dockerfile="$backend/Dockerfile.agent-release"
destination="$output"
if "$windows_cli"; then
  context="$(wslpath -w "$context")"
  dockerfile="$(wslpath -w "$dockerfile")"
  destination="$(wslpath -w "$destination")"
fi
"$docker_command" build --platform linux/amd64 --target artifact \
  --build-arg "CONTROL_PLANE_URL=$url" \
  --output "type=local,dest=$destination" \
  -f "$dockerfile" "$context"
cp "$root/scripts/install-agent-linux.sh" "$output/install-agent-linux.sh"
cp "$backend/deploy/agent/aiwm-agent.service" "$output/aiwm-agent.service"
chmod 755 "$output/aiwm-agent" "$output/install-agent-linux.sh"
(
  cd "$output"
  sha256sum aiwm-agent install-agent-linux.sh aiwm-agent.service BUILD_INFO.txt > SHA256SUMS
)
printf 'Release: %s\nChuyển toàn bộ directory + enrollment riêng cho từng server.\n' "$output"
[[ -n "$url" ]] || echo "Chưa inject URL: release dùng development fallback localhost; production cần URL thật."
