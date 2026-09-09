"""Read-only checks for the Windows/WSL Compose demo; never prints credentials."""
import shutil
import subprocess
import sys
from configure import ROOT, BACKEND, FRONTEND, read_env

def check(command):
    try:
        result = subprocess.run(command, capture_output=True, text=True, timeout=20, cwd=ROOT)
        return result.returncode == 0, result.stdout.strip()
    except (OSError, subprocess.TimeoutExpired):
        return False, ""

def main():
    failed = False
    print("Python:", sys.version.split()[0])
    for project in [BACKEND, FRONTEND]:
        exists = project.is_dir()
        print(("PASS" if exists else "FAIL") + ": project " + str(project.relative_to(ROOT)))
        failed |= not exists
    docker = shutil.which("docker")
    if docker:
        ok, output = check([docker, "info", "--format", "{{.OSType}}"])
        linux = ok and output == "linux"
        print(("PASS" if linux else "FAIL") + ": Docker Desktop/Linux engine")
        failed |= not linux
        ok, _ = check([docker, "compose", "version"])
        print(("PASS" if ok else "FAIL") + ": Docker Compose")
        failed |= not ok
    else:
        print("FAIL: docker not found; start/install Docker Desktop and enable Linux containers")
        failed = True
    root_env = read_env(ROOT / ".env")
    for key in ["AIWM_API_TOKEN", "AIWM_ENROLLMENT_TOKEN"]:
        ok = bool(root_env.get(key, "").strip())
        print(("PASS" if ok else "FAIL") + ": " + key + " configured at root (value hidden)")
        failed |= not ok
    for path, keys in [(BACKEND/".env", ["AIWM_API_TOKEN","AIWM_ENROLLMENT_TOKEN"]),
                       (BACKEND/".env.agent", ["AIWM_ENROLLMENT_TOKEN"]),
                       (FRONTEND/".env.local", ["AIWM_API_TOKEN"])]:
        configured = read_env(path)
        for key in keys:
            same = configured.get(key) == root_env.get(key) and bool(root_env.get(key))
            print(("PASS" if same else "FAIL") + ": " + str(path.relative_to(ROOT)) + " / " + key + " matches root")
            failed |= not same
    for tool in ["go", "node"]:
        print(("AVAILABLE" if shutil.which(tool) else "OPTIONAL MISSING") + ": " + tool + " (not required for all-Docker demo)")
    if failed:
        print("Fix FAIL entries; run python scripts/configure.py for missing/blank local settings.")
        return 1
    print("Ready: docker compose --profile console up --build -d")
    return 0

if __name__ == "__main__":
    sys.exit(main())
