"""Lab AIWM nhiều server: cấu hình, seed, enrollment, Agent Sim và kiểm thử thật."""
import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import subprocess
import sys
import time
import uuid

from acceptance import SessionAPI
from configure import ROOT, read_env, set_values

PROJECT = "aiwm-org-demo"
BASE = "http://127.0.0.1:8080/api/v1"
CACHE = ROOT / ".cache" / "multi-server-demo"
SCENARIO = ROOT / "demo/scenarios/multi-server.json"
ACCOUNTS = ROOT / "config/demo-users.json"


def run(*args, capture=False):
    result = subprocess.run(list(args), cwd=ROOT, check=True, text=True,
                            encoding="utf-8", stdout=subprocess.PIPE if capture else None)
    return result.stdout.strip() if capture else None


def compose(*args, capture=False):
    return run("docker", "compose", "-p", PROJECT, "-f", "compose.yaml", "-f",
               str(CACHE / "compose.json"), *args, capture=capture)


def wait(check, message, timeout=90):
    deadline = time.monotonic() + timeout
    while time.monotonic() < deadline:
        try:
            value = check()
            if value:
                return value
        except (OSError, RuntimeError, KeyError):
            pass
        time.sleep(1)
    raise RuntimeError(message)


def token_key(machine):
    return "AIWM_DEMO_ENROLLMENT_" + machine.upper().replace("-", "_")


def generate(servers):
    """Chỉ sinh input cho NVIDIA mock library và Compose, không tạo inventory API giả."""
    profiles = CACHE / "profiles"
    profiles.mkdir(parents=True, exist_ok=True)
    services, volumes = {}, {}
    for server in servers:
        machine = server["machineId"]
        # MachineID đồng thời dùng làm tên service/file do scenario của project quản lý.
        if not machine or any(c not in "abcdefghijklmnopqrstuvwxyz0123456789-" for c in machine):
            raise ValueError("machineId scenario không hợp lệ")
        memory = server["vramGiB"] * 1024 ** 3
        profile = {"version": "1.0", "system": {"driver_version": "550.163.01", "nvml_version": "12.550.163.01", "cuda_version": "12.4"},
                   "device_defaults": {"name": server["model"], "brand": "nvidia", "architecture": server["architecture"],
                                       "memory": {"total_bytes": memory, "free_bytes": memory, "used_bytes": 0},
                                       "utilization": {"gpu": 0, "memory": 0}, "thermal": {"temperature_gpu_c": 35},
                                       "mig": {"mode_current": "disabled", "mode_pending": "disabled"}}, "devices": []}
        for index in range(server["gpuCount"]):
            device = {"index": index, "uuid": "GPU-" + str(uuid.uuid5(uuid.NAMESPACE_URL, "aiwm-demo/" + machine + "/" + str(index)))}
            if index in server.get("externalGpuIndexes", []):
                device["processes"] = [{"pid": 10000 + index, "type": "C", "name": "existing-inference", "used_memory_mib": 1024}]
            if index in server.get("unknownGpuIndexes", []):
                device["processes"] = [{"pid": 424242, "type": "C", "name": "unattributed-process", "used_memory_mib": 1024}]
            if index in server.get("unhealthyGpuIndexes", []):
                device["failure"] = {"mode": "ecc_uncorrectable"}
            profile["devices"].append(device)
        # JSON là cú pháp YAML hợp lệ, được mock NVML đọc trực tiếp.
        (profiles / (machine + ".yaml")).write_text(json.dumps(profile, indent=2), encoding="utf-8")
        volumes[machine + "-state"] = {}
        services[machine] = {"image": "aiwm-agent-sim:local", "profiles": ["multi"], "depends_on": ["control-plane"],
            "environment": {"AIWM_CONTROL_PLANE_URL": "http://control-plane:8080", "AIWM_ENROLLMENT_TOKEN": "${" + token_key(machine) + ":-}",
                "AIWM_AGENT_MACHINE_ID": machine, "AIWM_AGENT_NAME": machine, "AIWM_AGENT_STATE_FILE": "/state/agent.json",
                "AIWM_AGENT_HEARTBEAT_INTERVAL": "2s", "AIWM_AGENT_INVENTORY_INTERVAL": "2s", "AIWM_AGENT_COMMAND_POLL_INTERVAL": "1s",
                "AIWM_SIM_EXTERNAL_GPU_INDEXES": ",".join(map(str, server.get("externalGpuIndexes", []))),
                "MOCK_NVML_CONFIG": "/profiles/" + machine + ".yaml"},
            "volumes": [machine + "-state:/state", {"type": "bind", "source": str(profiles.resolve()), "target": "/profiles", "read_only": True}]}
    (CACHE / "compose.json").write_text(json.dumps({"services": services, "volumes": volumes}, indent=2), encoding="utf-8")


def login(user, base=BASE):
    return SessionAPI(base, user["username"], user["password"])


def seed(catalog, servers, set_demo_passwords=False):
    config = read_env(ROOT / ".env")
    api = SessionAPI(BASE, config["AIWM_BOOTSTRAP_USERNAME"], config["AIWM_BOOTSTRAP_PASSWORD"])
    try:
        organizations = {o["code"]: o for o in api("/organizations")}
        for item in catalog["organizations"]:
            if item["code"] not in organizations:
                organizations[item["code"]] = api("/organizations", {**item, "enabled": True})
            if not organizations[item["code"]]["enabled"]:
                raise RuntimeError("Không tự bật lại organization disabled: " + item["code"])
        users = {u["username"]: u for u in api("/users")}
        for item in catalog["users"]:
            org = organizations[item["organizationCode"]]
            if item["username"] not in users:
                api("/users", {"username": item["username"], "password": item["password"], "role": item["role"], "organizationId": org["id"], "enabled": True})
            else:
                existing = users[item["username"]]
                if existing["role"] != item["role"] or existing["organizationId"] != org["id"] or not existing["enabled"]:
                    raise RuntimeError("Account trùng nhưng khác ownership/role; không ghi đè: " + item["username"])
            # Account đã tồn tại phải dùng đúng mật khẩu demo; không reset ngầm.
            try:
                session = login(item)
            except RuntimeError as error:
                if not set_demo_passwords or "HTTP 401" not in str(error):
                    raise RuntimeError("Account có mật khẩu khác seed: " + item["username"] + "; chỉ lab được phép đổi bằng --set-demo-passwords") from None
                existing = users[item["username"]]
                api("/users/" + existing["id"], {"username": item["username"], "password": item["password"], "role": item["role"], "organizationId": org["id"], "enabled": True})
                if item["username"] == config["AIWM_BOOTSTRAP_USERNAME"]:
                    set_values(ROOT / ".env", {"AIWM_BOOTSTRAP_PASSWORD": item["password"]})
                    api = login(item)
                session = login(item)
            session("/auth/logout", {})
        enrollments = {e["id"]: e for e in api("/enrollments")}
        known = {s["machineId"]: s for s in api("/servers")}
        for server in servers:
            machine = server["machineId"]
            key = token_key(machine)
            org = organizations[server["organizationCode"]]
            existing = enrollments.get(config.get(key + "_ID"))
            if existing and config.get(key):
                if existing["revoked"] or existing["organizationId"] != org["id"] or existing.get("machineId") not in (None, "", machine):
                    raise RuntimeError("Enrollment không còn hợp lệ: " + machine)
                if not existing.get("machineId") and datetime.fromisoformat(existing["expiresAt"].replace("Z", "+00:00")) <= datetime.now(timezone.utc):
                    raise RuntimeError("Enrollment chưa bind đã hết hạn: " + machine)
                continue
            if machine in known or config.get(key):
                raise RuntimeError("Thiếu metadata/token local khớp server cũ; không đổi ownership: " + machine)
            e = api("/enrollments", {"displayName": machine, "organizationId": org["id"], "labels": {"pool": "multi-server-demo"}})
            set_values(ROOT / ".env", {key: e["enrollmentToken"], key + "_ID": e["id"]})
        print("Đã chuẩn bị accounts/enrollments; không in token.", flush=True)
    finally:
        api("/auth/logout", {})


def up(catalog, servers, skip_build, set_demo_passwords, postgres_port):
    run(sys.executable, "scripts/configure.py", "--postgres-port", str(postgres_port))
    compose("up", "-d", "--wait", "postgres")
    if not skip_build:
        # Build nối tiếp để giảm áp lực RAM cho Docker Desktop trên laptop.
        for service in ["control-plane", "agent-a100", "console"]:
            compose("build", service)
    compose("run", "--rm", "--no-deps", "control-plane", "--migrate")
    count = compose("exec", "-T", "postgres", "psql", "-U", "aiwm", "-d", "aiwm", "-tAc", "SELECT count(*) FROM users", capture=True)
    if count == "0":
        admin = next(u for u in catalog["users"] if u["role"] == "ADMIN")
        org = next(o for o in catalog["organizations"] if o["code"] == admin["organizationCode"])
        set_values(ROOT / ".env", {"AIWM_BOOTSTRAP_USERNAME": admin["username"], "AIWM_BOOTSTRAP_PASSWORD": admin["password"],
                                    "AIWM_BOOTSTRAP_ORGANIZATION_CODE": org["code"], "AIWM_BOOTSTRAP_ORGANIZATION_NAME": org["name"]})
        compose("run", "--rm", "--no-deps", "control-plane", "--bootstrap-admin")
    compose("up", "-d", "control-plane", "console")
    from urllib.request import urlopen
    wait(lambda: urlopen("http://127.0.0.1:8080/healthz", timeout=5).status == 200, "Control Plane chưa sẵn sàng")
    seed(catalog, servers, set_demo_passwords)
    compose("up", "-d", *[s["machineId"] for s in servers])
    api = login(next(u for u in catalog["users"] if u["role"] == "ADMIN"))
    try:
        wanted = {s["machineId"] for s in servers}
        wait(lambda: wanted.issubset({s["machineId"] for s in api("/servers") if s["schedulable"]}), "Agent chưa có inventory hợp lệ; xem docker compose logs")
    finally:
        api("/auth/logout", {})
    print("Demo sẵn sàng: http://127.0.0.1:3000/login — accounts: docs/DEMO_ACCOUNTS.md", flush=True)


def check(catalog, servers, base):
    sessions = {u["username"]: login(u, base) for u in catalog["users"]}
    admin = sessions["admin"]
    created = []
    passed = []
    def ok(message):
        passed.append(message)
        print("PASS " + message, flush=True)
    def submit(api, name, count=1, profile="general", min_vram=1024):
        j = api("/jobs", {"name": "demo-check-" + name + "-" + str(time.time_ns()), "image": "alpine:3.21", "command": ["sh", "-c", "sleep 300"],
            "resources": {"gpuCount": count, "minVramMiB": min_vram, "performanceProfile": profile, "fp8Required": False}, "workloadType": "TRAINING",
            "necessityLevel": "NECESSITY_2", "necessityReason": "GO_LIVE_90_DAYS", "systemImportance": "IMPORTANT", "neededAt": datetime.now(timezone.utc).isoformat(), "ttlSeconds": 3600})
        created.append((api, j["id"]))
        return j["id"]
    def running(api, jid):
        return wait(lambda: j if (j := api("/jobs/" + jid))["status"] == "RUNNING" else None, "Job không RUNNING: " + jid)
    def stop(api, jid):
        api("/jobs/" + jid + "/stop", {})
        return wait(lambda: j if (j := api("/jobs/" + jid))["status"] in ["STOPPED", "SUCCEEDED", "FAILED", "CANCELLED"] else None, "Job không terminal: " + jid)
    try:
        inventory = admin("/servers")
        expected = {s["machineId"] for s in servers}
        actual = {s["machineId"]: s for s in inventory if s["machineId"] in expected}
        assert set(actual) == expected and all(s["schedulable"] for s in actual.values())
        assert sum(len(s["gpus"]) for s in actual.values()) == sum(s["gpuCount"] for s in servers)
        ok("C: ADMIN thấy 6 servers / 26 GPUs / 4 organizations")
        states = {g["state"] for s in actual.values() for g in s["gpus"]}
        assert {"FREE", "OCCUPIED_LEGACY", "OCCUPIED_UNKNOWN", "UNHEALTHY"}.issubset(states), states
        external = admin("/containers?origin=LEGACY")
        protected = {g["uuid"] for s in actual.values() for g in s["gpus"] if g["state"] != "FREE"}
        for username in ["vtt", "vds", "vtnet", "vtit"]:
            api = sessions[username]
            org = api.identity["user"]["organizationId"]
            assert all(s["organizationId"] == org for s in api("/servers"))
            assert all(g["organizationId"] == org for g in api("/gpus"))
            foreign = next(s for s in inventory if s["organizationId"] != org)
            try:
                api("/servers/" + foreign["id"])
                raise AssertionError("Lộ Server khác đơn vị")
            except RuntimeError as error:
                assert "HTTP 404" in str(error)
            jid = submit(api, username)
            j = running(api, jid)
            assigned = admin("/servers/" + j["assignment"]["serverId"])
            assert assigned["organizationId"] == org == j["organizationId"]
            assert not protected.intersection(j["assignment"]["gpuUuids"])
            assert j["assignment"]["reservationState"] == "ALLOCATED"
            stopped = stop(api, jid)
            assert stopped["assignment"]["reservationState"] == "RELEASED"
            selected = set(j["assignment"]["gpuUuids"])
            wait(lambda: all(g["state"] == "FREE" for g in admin("/servers/" + assigned["id"])["gpus"] if g["uuid"] in selected), "GPU chưa FREE")
            ok("A/B/F/G: " + username + " isolation → RUNNING → stop → RELEASED/FREE")
        pending = submit(sessions["vtt"], "too-large", 64)
        time.sleep(3)
        assert sessions["vtt"]("/jobs/" + pending)["status"] == "QUEUED"
        stop(sessions["vtt"], pending)
        ok("E: thiếu tài nguyên → QUEUED, không lấy GPU của đơn vị khác")
        after = admin("/containers?origin=LEGACY")
        for item in external:
            current = next(x for x in after if x["serverId"] == item["serverId"] and x["container"]["id"] == item["container"]["id"])
            assert current["container"]["state"] == item["container"]["state"]
            assert current["container"].get("startedAt") == item["container"].get("startedAt")
        ok("D: LEGACY/UNKNOWN/UNHEALTHY bị loại; External không đổi identity/start time")
        (CACHE / "last-check.json").write_text(json.dumps({"at": datetime.now(timezone.utc).isoformat(), "base": base, "passed": passed}, ensure_ascii=False, indent=2), encoding="utf-8")
    finally:
        for api, jid in created:
            stop(api, jid)
        for api in sessions.values():
            api("/auth/logout", {})


def failure(catalog):
    """Chỉ tạm dừng đúng fake Agent/CP của lab; không đụng Docker daemon hay workload ngoài test."""
    api = login(next(u for u in catalog["users"] if u["username"] == "vtit"))
    machine = "vtit-gpu-01"
    server = next(s for s in api("/servers") if s["machineId"] == machine)
    jid = None
    def runtime():
        return json.loads(compose("exec", "-T", machine, "cat", "/state/agent.json.docker.json", capture=True))
    try:
        compose("pause", machine)
        j = api("/jobs", {"name": "demo-failure-" + str(time.time_ns()), "image": "alpine:3.21", "command": ["sh", "-c", "sleep 300"],
            "resources": {"gpuCount": 1, "minVramMiB": 1024, "performanceProfile": "general", "fp8Required": False}, "workloadType": "TRAINING",
            "necessityLevel": "NECESSITY_2", "necessityReason": "GO_LIVE_90_DAYS", "systemImportance": "IMPORTANT", "neededAt": datetime.now(timezone.utc).isoformat(), "ttlSeconds": 3600})
        jid = j["id"]
        wait(lambda: api("/jobs/" + jid)["status"] == "ASSIGNED", "Không quan sát được reservation", timeout=15)
        assert any(g["state"] == "RESERVED" for g in api("/servers/" + server["id"])["gpus"])
        print("PASS reservation: ASSIGNED + RESERVED qua scheduler/commit thật", flush=True)
        compose("unpause", machine)
        wait(lambda: api("/jobs/" + jid)["status"] == "RUNNING", "Không RUNNING sau unpause")
        before = runtime()
        compose("stop", machine)
        wait(lambda: api("/servers/" + server["id"])["status"] == "OFFLINE", "Không chuyển OFFLINE sau mất heartbeat")
        offline = api("/servers/" + server["id"])
        assert not offline["schedulable"] and any(g["state"] == "ALLOCATED" for g in offline["gpus"])
        assert api("/jobs/" + jid)["assignment"]["reservationState"] == "ALLOCATED"
        compose("start", machine)
        wait(lambda: api("/servers/" + server["id"])["schedulable"], "Reconnect không có inventory fresh")
        assert runtime() == before
        print("PASS Agent restart: OFFLINE chặn placement, giữ reservation; FULL inventory phục hồi, runtime không đổi", flush=True)
        compose("stop", "control-plane")
        time.sleep(4)
        assert runtime() == before
        compose("start", "control-plane")
        wait(lambda: api("/servers/" + server["id"])["schedulable"], "CP không phục hồi inventory")
        assert api("/jobs/" + jid)["status"] == "RUNNING" and runtime() == before
        print("PASS CP disconnect/restart: fake Docker state và identity container giữ nguyên", flush=True)
        (CACHE / "last-failure.json").write_text(json.dumps({"at": datetime.now(timezone.utc).isoformat(), "passed": True}), encoding="utf-8")
    finally:
        # Khôi phục đúng services của lab ngay cả khi assertion thất bại.
        container = compose("ps", "--all", "--quiet", machine, capture=True)
        if container and run("docker", "inspect", "--format", "{{.State.Paused}}", container, capture=True) == "true":
            compose("unpause", machine)
        compose("start", "control-plane", machine)
        if jid:
            wait(lambda: api("/jobs/" + jid), "CP chưa phục hồi để cleanup job test")
            api("/jobs/" + jid + "/stop", {})
            wait(lambda: api("/jobs/" + jid)["status"] in ["STOPPED", "CANCELLED", "FAILED", "SUCCEEDED"], "Chưa stop được job test")
        api("/auth/logout", {})


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=["up", "down", "check", "failure"])
    parser.add_argument("--skip-build", action="store_true", help="Tái dùng image đã build khi source không đổi")
    parser.add_argument("--postgres-port", type=int, default=15432, help="Host port PostgreSQL demo; không đổi port nội bộ 5432")
    parser.add_argument("--set-demo-passwords", action="store_true", help="Chỉ lab: cho phép đổi password account cùng role/ownership sang password DEMO công khai")
    parser.add_argument("--base-url", default=BASE, help="check qua CP hoặc http://127.0.0.1:3000/api/aiwm")
    args = parser.parse_args()
    catalog = json.loads(ACCOUNTS.read_text(encoding="utf-8"))
    servers = json.loads(SCENARIO.read_text(encoding="utf-8"))["servers"]
    generate(servers)
    if args.action == "up":
        up(catalog, servers, args.skip_build, args.set_demo_passwords, args.postgres_port)
    elif args.action == "down":
        compose("down")  # Chỉ project này; giữ toàn bộ volumes; không prune/remove-orphans.
    elif args.action == "failure":
        failure(catalog)
    else:
        check(catalog, servers, args.base_url)


if __name__ == "__main__":
    main()
