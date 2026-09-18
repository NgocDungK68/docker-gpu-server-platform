#!/usr/bin/env bash
# Chỉ chạy trong disposable container. Stub kiểm tra installer, KHÔNG giả GPU thật.
set -euo pipefail
[[ -f /.dockerenv && "$EUID" == 0 ]] || { echo "Chỉ chạy trong container kiểm thử riêng."; exit 1; }
mkdir -p /fixture /run/systemd/system
printf 'test-machine-identity\n' > /etc/machine-id
groupadd docker
python3 -c 'import socket; s=socket.socket(socket.AF_UNIX); s.bind("/var/run/docker.sock"); s.close()'
cat > /fixture/agent-probe-stub <<'STUB'
#!/bin/sh
test "$1" = "--check"
STUB
chmod 755 /fixture/agent-probe-stub
cat > /usr/local/bin/systemctl <<'STUB'
#!/bin/sh
printf '%s\n' "$*" >> /fixture/systemctl-calls
STUB
chmod 755 /usr/local/bin/systemctl
bash /release/install-agent-linux.sh --binary /fixture/agent-probe-stub --enrollment-token test-only-enrollment
test "$(stat -c '%a:%U' /etc/aiwm-agent/agent.env)" = 600:root
test "$(stat -c '%a:%U' /var/lib/aiwm-agent)" = 700:aiwm-agent
test -x /usr/local/bin/aiwm-agent
cmp /release/aiwm-agent.service /etc/systemd/system/aiwm-agent.service
test "$(cat /fixture/systemctl-calls)" = daemon-reload
! grep -q '^AIWM_CONTROL_PLANE_URL=' /etc/aiwm-agent/agent.env
before="$(sha256sum /etc/aiwm-agent/agent.env)"
if bash /release/install-agent-linux.sh --binary /fixture/agent-probe-stub --enrollment-token different-test-token; then
  echo "FAIL: installer đã ghi đè installation" >&2
  exit 1
fi
test "$before" = "$(sha256sum /etc/aiwm-agent/agent.env)"
echo "PASS installer paths, permissions, service, no implicit runtime start, no overwrite (probe stub only)."
