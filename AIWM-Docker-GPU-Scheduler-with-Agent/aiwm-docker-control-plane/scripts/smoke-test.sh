#!/usr/bin/env sh
set -eu
script_dir="$(CDPATH='' cd -- "$(dirname -- "$0")" && pwd)"
exec python3 "$script_dir/../../../scripts/acceptance.py" --base-url "${AIWM_BASE_URL:-http://localhost:8080}/api/v1"
