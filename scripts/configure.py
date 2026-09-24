"""Tạo cấu hình local, giữ giá trị đã có và không in credentials."""
from pathlib import Path
import argparse
import secrets
import sys
from urllib.parse import quote, urlsplit, urlunsplit

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

def set_values(path, values):
    """Chỉ thay các key được yêu cầu tường minh, giữ các cấu hình khác."""
    lines = path.read_text(encoding="utf-8-sig").splitlines() if path.exists() else []
    remaining = dict(values)
    for index, line in enumerate(lines):
        key = line.split("=", 1)[0]
        if not line.lstrip().startswith("#") and key in values:
            lines[index] = key + "=" + values[key]
            remaining.pop(key, None)
    lines.extend(key + "=" + value for key, value in remaining.items())
    path.write_text("\n".join(lines) + "\n", encoding="utf-8")
    path.chmod(0o600)

def local_database_port(url, port):
    parts = urlsplit(url)
    if parts.hostname not in ("localhost", "127.0.0.1"):
        raise ValueError("--postgres-port chỉ đổi DSN local; giữ nguyên DSN tùy chỉnh/remote")
    credentials = parts.netloc.rsplit("@", 1)[0] + "@" if "@" in parts.netloc else ""
    return urlunsplit(parts._replace(netloc=credentials + parts.hostname + ":" + str(port)))

if __name__ == "__main__":
    sys.stdout.reconfigure(encoding="utf-8")
    sys.stderr.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--postgres-port", type=int, help="Đổi host port PostgreSQL demo và DSN local, giữ credentials")
    args = parser.parse_args()
    if args.postgres_port is not None and not 1 <= args.postgres_port <= 65535:
        parser.error("postgres-port phải trong khoảng 1–65535")
    config = read_env(ROOT / ".env")
    port = args.postgres_port or int(config.get("AIWM_POSTGRES_PORT") or 15432)
    # Kiểm tra mọi DSN trước khi ghi, tránh cập nhật một nửa khi có DSN remote.
    port_updates = {}
    if args.postgres_port is not None:
        for path in [ROOT / ".env", BACKEND / ".env"]:
            current = read_env(path)
            updates = {"AIWM_POSTGRES_PORT": str(port)}
            if current.get("AIWM_DATABASE_URL", "").strip():
                try:
                    updates["AIWM_DATABASE_URL"] = local_database_port(current["AIWM_DATABASE_URL"], port)
                except ValueError as exc:
                    parser.error(str(exc))
            port_updates[path] = updates
        config.update(port_updates[ROOT / ".env"])
    for key in ["AIWM_POSTGRES_PASSWORD", "AIWM_BOOTSTRAP_PASSWORD"]:
        if not config.get(key, "").strip():
            config[key] = secrets.token_hex(32)
    password = quote(config["AIWM_POSTGRES_PASSWORD"], safe="")
    defaults = {
        "AIWM_POSTGRES_PORT": str(port),
        "AIWM_DATABASE_URL": "postgres://aiwm:" + password + "@localhost:" + str(port) + "/aiwm?sslmode=disable",
        "AIWM_COMPOSE_DATABASE_URL": "postgres://aiwm:" + password + "@postgres:5432/aiwm?sslmode=disable",
        # Metadata demo có thể đổi; không đi vào scheduler/policy.
        "AIWM_BOOTSTRAP_ORGANIZATION_CODE": "VTT",
        "AIWM_BOOTSTRAP_ORGANIZATION_NAME": "Viettel Telecom",
        "AIWM_BOOTSTRAP_USERNAME": "admin",
        "AIWM_A100_ENROLLMENT_TOKEN": "",
        "AIWM_T4_ENROLLMENT_TOKEN": "",
        "AIWM_SESSION_COOKIE_SECURE": "false",
    }
    for key,value in defaults.items():
        if not config.get(key, "").strip():
            config[key] = value
    for path, values in port_updates.items():
        set_values(path, values)
    ensure(ROOT / ".env", config)
    ensure(BACKEND / ".env", {
        **config, "AIWM_HTTP_ADDR": "127.0.0.1:8080",
        "AIWM_STATE_FILE": ".local/control-plane.gob",
        "AIWM_SCHEDULER_STRATEGY": "best-fit",
        "AIWM_AGENT_OFFLINE_AFTER": "20s",
    })
    ensure(BACKEND / ".env.agent", {
        "AIWM_ENROLLMENT_TOKEN": "",
        "AIWM_CONTROL_PLANE_URL": "http://localhost:8080",
    })
    ensure(FRONTEND / ".env.local", {
        "AIWM_API_BASE_URL": "http://localhost:8080",
        "AIWM_SESSION_COOKIE_SECURE": config["AIWM_SESSION_COOKIE_SECURE"],
        "NEXT_PUBLIC_REFRESH_INTERVAL_MS": "2000",
    })
    print("Cấu hình đã lưu, không in secret. Xem docs/TESTING_RUNBOOK.md: PostgreSQL -> migrate -> bootstrap -> login -> enrollment -> Agent.")
