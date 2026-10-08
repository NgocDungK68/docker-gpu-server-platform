"""Thao tác deployment qua public API: enrollment riêng và workload smoke GPU thật."""
import argparse
from datetime import datetime, timezone
import getpass
import json
import os
from pathlib import Path
import sys

from acceptance import SessionAPI
from configure import ROOT


def main():
    sys.stdout.reconfigure(encoding="utf-8")
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", required=True, help="Control Plane origin, ví dụ URL của host nội bộ")
    parser.add_argument("--username", required=True)
    sub = parser.add_subparsers(dest="action", required=True)
    enrollment = sub.add_parser("enroll")
    enrollment.add_argument("--organization", required=True, help="Organization code đã tồn tại")
    enrollment.add_argument("--name", required=True)
    enrollment.add_argument("--token-file", type=Path, required=True)
    smoke = sub.add_parser("smoke")
    smoke.add_argument("--image", help="Override CUDA image đã được đơn vị phê duyệt")
    for action in ["status", "stop"]:
        command = sub.add_parser(action)
        command.add_argument("--job-id", required=True)
    sub.add_parser("servers")
    args = parser.parse_args()
    # Không truyền password/token qua command line hoặc in response enrollment/session.
    api = SessionAPI(args.base_url.rstrip("/") + "/api/v1", args.username, getpass.getpass("Password: "))
    try:
        if args.action == "enroll":
            org = next((o for o in api("/organizations") if o["code"] == args.organization), None)
            if not org:
                parser.error("Không thấy organization trong quyền truy cập của account.")
            if any(e["displayName"] == args.name and e["organizationId"] == org["id"] and not e["revoked"]
                   for e in api("/enrollments")):
                parser.error("Enrollment cùng tên đã có; dùng token cũ hoặc revoke qua API trước khi cấp lại.")
            body = {"displayName": args.name}
            if api.identity["user"]["role"] == "ADMIN":
                body["organizationId"] = org["id"]
            args.token_file.parent.mkdir(parents=True, exist_ok=True)
            fd = os.open(args.token_file, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
            with os.fdopen(fd, "w", encoding="utf-8") as output:
                result = api("/enrollments", body)
                output.write(result["enrollmentToken"] + "\n")
            print("Enrollment:", result["id"], "| Server:", result["serverId"], "| Token file:", args.token_file)
        elif args.action == "smoke":
            body = json.loads((ROOT / "demo/workloads/real-gpu-smoke.json").read_text(encoding="utf-8"))
            body["neededAt"] = datetime.now(timezone.utc).isoformat()
            if args.image:
                body["image"] = args.image
            result = api("/jobs", body)
            print("Job:", result["id"], "| Status:", result["status"])
        elif args.action == "servers":
            for server in api("/servers"):
                print(server["id"], server["name"], "org=" + server["organizationId"],
                      server["status"], "schedulable=" + str(server["schedulable"]), "GPUs=" + str(len(server["gpus"])))
        else:
            path = "/jobs/" + args.job_id
            result = api(path + "/stop", {}) if args.action == "stop" else api(path)
            # Chỉ in state/placement, không in execution environment.
            print(json.dumps({key: result.get(key) for key in ("id", "status", "statusReason", "organizationId", "assignment")}, ensure_ascii=False, indent=2))
    finally:
        api("/auth/logout", {})


if __name__ == "__main__":
    main()
