"""Acceptance A-E/H against the live Control Plane or the Next.js public proxy."""
import argparse
from datetime import datetime, timezone
from concurrent.futures import ThreadPoolExecutor
import json
from pathlib import Path
import time
from urllib.request import Request, urlopen
from urllib.request import build_opener, HTTPCookieProcessor
from urllib.parse import urlsplit
from http.cookiejar import CookieJar
from urllib.error import HTTPError
from configure import ROOT, read_env

class SessionAPI:
    """Session thật cho CP trực tiếp hoặc cookie HttpOnly của BFF; không in secret."""
    def __init__(self, base, username, password):
        self.base = base.rstrip("/")
        self.token = ""
        self.opener = build_opener(HTTPCookieProcessor(CookieJar()))
        self.identity = self("/auth/login", {"username": username, "password": password})
        self.token = self.identity.get("token", "")

    def __call__(self, path, body=None):
        headers = {"Content-Type": "application/json"}
        if self.token:
            headers["Authorization"] = "Bearer " + self.token
        origin = urlsplit(self.base)
        headers["Origin"] = origin.scheme + "://" + origin.netloc
        request = Request(self.base + path, data=None if body is None else json.dumps(body).encode(), headers=headers)
        try:
            with self.opener.open(request, timeout=15) as response:
                return json.load(response)["data"]
        except HTTPError as error:
            # Không đưa response login/enrollment hoặc credentials vào output.
            raise RuntimeError(f"HTTP {error.code}: {path.split('?')[0]}") from None

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--base-url", default="http://localhost:8080/api/v1")
    args = parser.parse_args()
    token = read_env(ROOT / ".env").get("AIWM_API_TOKEN", "")
    def api(path, body=None):
        data = None if body is None else json.dumps(body).encode()
        request = Request(args.base_url + path, data=data,
                          headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
        try:
            with urlopen(request, timeout=10) as response:
                return json.load(response)["data"]
        except HTTPError as error:
            raise RuntimeError(str(error.code) + ": " + error.read().decode()) from error
    def wait(check, message, timeout=45):
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            value = check()
            if value:
                return value
            time.sleep(0.3)
        raise AssertionError(message)
    created = []
    stamp = str(time.time_ns())
    def submit(name, count=1, profile="a100-equivalent"):
        job = api("/jobs", {"name": "acceptance-" + name + "-" + stamp, "image": "alpine:3.21",
                           "command": ["sh", "-c", "sleep 300"], "resources": {"gpuCount": count, "minVramMiB": 1024, "performanceProfile": profile, "fp8Required": False},
                           "workloadType": "TRAINING", "necessityLevel": "NECESSITY_2", "necessityReason": "GO_LIVE_90_DAYS",
                           "systemImportance": "IMPORTANT", "neededAt": datetime.now(timezone.utc).isoformat(), "ttlSeconds": 3600})
        created.append(job["id"])
        return job["id"]
    def job(id):
        return api("/jobs/" + id)
    def running(id):
        return wait(lambda: (value if (value := job(id))["status"] == "RUNNING" else None), "Job failed to run: " + id)
    def stop(id):
        api("/jobs/" + id + "/stop", {})
        wait(lambda: job(id)["status"] in ["STOPPED", "CANCELLED", "SUCCEEDED", "FAILED"], "Job failed to stop: " + id)
    servers = wait(lambda: (items if len(items := api("/servers")) >= 2 else None), "Two agents did not register")
    external_before = [item for item in api("/containers?origin=LEGACY") if item["container"]["state"] == "running"]
    assert external_before, "No external workload discovered"
    a100 = next(server for server in servers if server["machineId"] == "sim-a100")
    t4 = next(server for server in servers if server["machineId"] == "sim-t4")
    assert len(a100["gpus"]) == 4 and len(t4["gpus"]) == 2
    assert a100["gpus"][0]["state"] == "OCCUPIED_LEGACY"
    protected = {uuid for item in external_before for uuid in item["container"].get("gpuUuids", [])}
    print("PASS A/B: two NVML servers discovered; external GPU protected", flush=True)
    try:
        holder = submit("holder", 3)
        assigned = running(holder)
        assert not protected.intersection(assigned["assignment"]["gpuUuids"])
        print("PASS C: job RUNNING on three free A100 GPUs", flush=True)
        queued = submit("waiting")
        wait(lambda: "insufficient" in job(queued).get("statusReason", "") or "occupied" in job(queued).get("statusReason", "") or "reserved" in job(queued).get("statusReason", ""), "Pending reason absent")
        assert job(queued)["status"] == "QUEUED"
        print("PASS D: insufficient resources remain QUEUED with a reason", flush=True)
        stop(holder)
        running(queued)
        assert job(holder)["assignment"]["reservationState"] == "RELEASED"
        print("PASS E: stopped job releases GPU and waiting job runs", flush=True)
        with ThreadPoolExecutor(max_workers=2) as executor:
            concurrent = list(executor.map(lambda index: submit("concurrent-" + str(index), profile="general"), range(2)))
        placements = [running(id)["assignment"]["gpuUuids"] for id in concurrent]
        assert set(placements[0]).isdisjoint(placements[1])
        print("PASS H: concurrent jobs have distinct physical GPU UUIDs", flush=True)
        external_after = api("/containers?origin=LEGACY")
        for previous in external_before:
            current = next(item for item in external_after if item["serverId"] == previous["serverId"] and item["container"]["id"] == previous["container"]["id"])
            assert current["container"]["state"] == "running"
            assert current["container"].get("startedAt") == previous["container"].get("startedAt")
        print("PASS: external container identity and start time unchanged", flush=True)
    finally:
        for id in created:
            stop(id)
        print("Only jobs created by this acceptance run were stopped/cancelled.", flush=True)

if __name__ == "__main__":
    main()
