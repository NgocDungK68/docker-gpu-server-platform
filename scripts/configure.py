"""Create local AIWM configuration with random secrets, preserving existing files."""
from pathlib import Path
import secrets

ROOT = Path(__file__).resolve().parents[1]
BACKEND = ROOT / "AIWM-Docker-GPU-Scheduler-with-Agent/aiwm-docker-control-plane"
FRONTEND = ROOT / "AIWM-Docker-GPU-Console-Nextjs/aiwm-docker-gpu-console"

def read_env(path):
    if not path.exists():
        return {}
    return dict(line.split("=", 1) for line in path.read_text(encoding="utf-8-sig").splitlines()
                if "=" in line and not line.lstrip().startswith("#"))

def ensure(path, values):
    existing = read_env(path)
    additions = {key: value for key, value in values.items() if key not in existing}
    # Replace blank placeholders in copied .example files without duplicating keys.
    blanks = {key: value for key, value in values.items() if key in existing and not existing[key].strip() and value}
    if blanks:
        lines = path.read_text(encoding="utf-8-sig").splitlines()
        lines = [line.split("=", 1)[0] + "=" + blanks[line.split("=", 1)[0]]
                 if "=" in line and line.split("=", 1)[0] in blanks else line for line in lines]
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")
        path.chmod(0o600)
    if additions:
        with path.open("a", encoding="utf-8", newline="\n") as stream:
            stream.write("\n" + "".join(key + "=" + value + "\n" for key, value in additions.items()))
        path.chmod(0o600)
    print("Configured " + str(path.relative_to(ROOT)) + " (existing values preserved)")

if __name__ == "__main__":
    config = read_env(ROOT / ".env")
    for key in ["AIWM_ENROLLMENT_TOKEN", "AIWM_API_TOKEN"]:
        if not config.get(key, "").strip():
            config[key] = secrets.token_hex(32)
    ensure(ROOT / ".env", config)
    ensure(BACKEND / ".env", {
        **config, "AIWM_HTTP_ADDR": "127.0.0.1:8080",
        "AIWM_STATE_FILE": ".local/control-plane.gob",
        "AIWM_SCHEDULER_STRATEGY": "best-fit",
        "AIWM_AGENT_OFFLINE_AFTER": "20s",
    })
    ensure(BACKEND / ".env.agent", {
        "AIWM_ENROLLMENT_TOKEN": config["AIWM_ENROLLMENT_TOKEN"],
        "AIWM_CONTROL_PLANE_URL": "http://localhost:8080",
    })
    ensure(FRONTEND / ".env.local", {
        "AIWM_API_BASE_URL": "http://localhost:8080",
        "AIWM_API_TOKEN": config["AIWM_API_TOKEN"],
        "NEXT_PUBLIC_REFRESH_INTERVAL_MS": "2000",
    })
    print("Secrets were generated locally and were not printed. Start with docker compose up --build -d.")
