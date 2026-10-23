"""Ứng dụng demo contract AIWM; JSON minh họa state/output, không phải model ML thật."""
import argparse
import datetime as dt
import json
import os
import signal
import sys

sys.stdout.reconfigure(encoding='utf-8')
sys.stderr.reconfigure(encoding='utf-8')
import time
import urllib.error
import urllib.request

parser = argparse.ArgumentParser()
parser.add_argument("--steps", type=int, default=120)
parser.add_argument("--step-seconds", type=float, default=1)
parser.add_argument("--checkpoint-every", type=int, default=5)
args = parser.parse_args()
if args.steps < 1 or args.step_seconds <= 0 or args.checkpoint_every < 1:
    parser.error("steps, step-seconds và checkpoint-every phải > 0")

base = os.environ["AIWM_TRAINING_URL"]
token = os.environ["AIWM_TRAINING_TOKEN"]
end = dt.datetime.fromisoformat(os.environ["AIWM_ALLOCATION_END_AT"].replace("Z", "+00:00")).timestamp()
stopping = False
signal.signal(signal.SIGTERM, lambda *_: globals().__setitem__("stopping", True))
signal.signal(signal.SIGINT, lambda *_: globals().__setitem__("stopping", True))


def request(path="", body=None):
    req = urllib.request.Request(
        base + path, data=body, method="PUT" if body is not None else "GET",
        headers={"Authorization": "Bearer " + token, "Content-Type": "application/octet-stream"},
    )
    with urllib.request.urlopen(req, timeout=20) as response:
        return json.load(response)["data"]


step = 0
contract = request()
if contract.get("resumeURL"):
    with urllib.request.urlopen(contract["resumeURL"], timeout=20) as response:
        step = int(json.load(response)["step"])
    print("Đã tiếp tục từ checkpoint, step =", step, flush=True)


def checkpoint():
    data = json.dumps({"step": step, "demoState": "opaque-training-state"}).encode()
    request("/checkpoint?step=" + str(step), data)
    print("Checkpoint đã lưu, step =", step, flush=True)


while step < args.steps and not stopping and time.time() < end:
    time.sleep(min(args.step_seconds, max(0, end - time.time())))
    if stopping or time.time() >= end:
        break
    step += 1
    try:
        contract = request()
        if step % args.checkpoint_every == 0 or contract["checkpointRequested"]:
            checkpoint()
    except (OSError, urllib.error.HTTPError):
        # Không in URL/token; lần kế tiếp thử lại. Không giả vờ save thành công.
        print("Chưa lưu được checkpoint; sẽ thử lại.", flush=True)

if step >= args.steps and not stopping and time.time() < end:
    request("/artifact", json.dumps({"demoModel": True, "trainedSteps": step}).encode())
    print("Model demo đã sẵn sàng.", flush=True)
else:
    checkpoint()
    print("Đã lưu state để xin cấp phát tiếp.", flush=True)
    # Dừng tính toán tại EndAt, chờ STOP graceful; không giả vờ training hoàn thành.
    while not stopping and time.time() < end + 20:
        time.sleep(0.2)
        try:
            stopping = request().get("stopRequested", False)
        except (OSError, urllib.error.HTTPError):
            pass
    if not stopping:
        raise SystemExit(75)
