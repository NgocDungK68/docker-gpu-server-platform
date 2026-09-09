"""Acceptance F/G and Control Plane restart for the isolated root Compose demo."""
import json
import subprocess
import time
from urllib.request import Request, urlopen
from configure import ROOT, read_env

TOKEN = read_env(ROOT / ".env")["AIWM_API_TOKEN"]
BASE = "http://localhost:8080/api/v1"

def api(path, body=None):
    request = Request(BASE + path, data=None if body is None else json.dumps(body).encode(),
                      headers={"Authorization": "Bearer " + TOKEN, "Content-Type": "application/json"})
    with urlopen(request, timeout=10) as response:
        return json.load(response)["data"]

def wait(check, message, timeout=60):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            value = check()
            if value:
                return value
        except (OSError, KeyError):
            pass
        time.sleep(0.5)
    raise AssertionError(message)

def compose(*args):
    return subprocess.check_output(["docker", "compose", *args], cwd=ROOT, text=True)

def runtime():
    return json.loads(compose("exec", "-T", "agent-a100", "cat", "/state/agent.json.docker.json"))

def main():
    server = next(s for s in api("/servers") if s["machineId"] == "sim-a100")
    assert server["schedulable"], "Start the root Compose demo and wait for inventory first"
    created = []
    def submit(name):
        job = api("/jobs", {"name": name + "-" + str(time.time_ns()), "image": "alpine:3.21",
                           "resources": {"gpuCount": 1}, "serverSelector": {"site": "hanoi"}})
        created.append(job["id"])
        return job["id"]
    def job(id):
        return api("/jobs/" + id)
    try:
        active = submit("recovery-running")
        wait(lambda: job(active)["status"] == "RUNNING", "Managed job did not run")
        before = runtime()
        active_containers = {id: c for id, c in before.items() if c["State"] == "running"}
        compose("stop", "agent-a100")
        wait(lambda: api("/servers/" + server["id"])["status"] == "OFFLINE", "Agent never marked offline")
        blocked = submit("recovery-queued")
        time.sleep(4)
        assert job(blocked)["status"] == "QUEUED"
        assert job(active)["status"] == "RUNNING", "Connectivity loss changed workload desired state"
        assert not api("/servers/" + server["id"])["schedulable"]
        print("PASS F: offline agent receives no new placement; active job remains reserved", flush=True)
        compose("start", "agent-a100")
        wait(lambda: api("/servers/" + server["id"])["schedulable"], "Agent did not reconcile fresh inventory")
        wait(lambda: job(blocked)["status"] == "RUNNING", "Queued job did not run after reconnect")
        after = runtime()
        for id, previous in active_containers.items():
            assert after[id]["State"] == "running" and after[id]["StartedAt"] == previous["StartedAt"]
        assert sum(c.get("Labels", {}).get("aiwm.job-id") == active for c in after.values() if c.get("Labels")) == 1
        print("PASS G: agent restart preserves external and managed identities; no duplicate container", flush=True)
        before_cp = runtime()
        compose("stop", "control-plane")
        time.sleep(4)
        assert runtime() == before_cp, "Control Plane loss mutated simulated Docker runtime"
        compose("start", "control-plane")
        wait(lambda: api("/servers/" + server["id"])["schedulable"], "Control Plane did not recover inventory")
        assert job(active)["status"] == "RUNNING" and job(blocked)["status"] == "RUNNING"
        assert runtime() == before_cp
        print("PASS: Control Plane restart restores jobs/reservations and leaves runtime unchanged", flush=True)
    finally:
        compose("start", "control-plane", "agent-a100")
        wait(lambda: api("/servers/" + server["id"])["schedulable"], "Demo recovery failed")
        for id in created:
            api("/jobs/" + id + "/stop", {})
            wait(lambda: job(id)["status"] in ["STOPPED", "CANCELLED", "FAILED", "SUCCEEDED"], "Cleanup failed")
        print("Demo services restored; only this script's jobs were stopped.", flush=True)

if __name__ == "__main__":
    main()
