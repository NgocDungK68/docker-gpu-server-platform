$ErrorActionPreference = "Stop"
# Compatibility entry point for the workspace's two-Agent demo acceptance.
$acceptanceScript = Join-Path $PSScriptRoot "../../../scripts/acceptance.py"
$apiBase = if ($env:AIWM_BASE_URL) { $env:AIWM_BASE_URL.TrimEnd("/") + "/api/v1" } else { "http://localhost:8080/api/v1" }
python $acceptanceScript --base-url $apiBase
exit $LASTEXITCODE
