"""Kiểm tra CLI deployment, dùng API stub; không kết nối Control Plane/GPU thật."""
import contextlib
import importlib.util
import io
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("agent_admin", Path(__file__).with_name("agent-admin.py"))
cli = importlib.util.module_from_spec(spec)
spec.loader.exec_module(cli)


class Output(io.StringIO):
    def reconfigure(self, **kwargs):
        pass


class API:
    def __init__(self, role="ADMIN"):
        self.identity = {"user": {"role": role}}
        self.calls = []
        self.created = []

    def __call__(self, path, body=None):
        self.calls.append((path, body))
        if path == "/organizations":
            return [{"code": "VTT", "id": "org-a"}]
        if path == "/enrollments" and body is None:
            return self.created
        if path == "/enrollments":
            result = {**body, "id": "enr-" + str(len(self.created)), "serverId": "server-" + str(len(self.created)),
                      "organizationId": body.get("organizationId", "org-a"), "revoked": False,
                      "enrollmentToken": "test-token-" + str(len(self.created))}
            self.created.append(result)
            return result
        if path == "/jobs":
            return {"id": "job-smoke", "status": "QUEUED"}
        return {}


class DeploymentCLI(unittest.TestCase):
    def invoke(self, api, *args):
        output = Output()
        argv = ["agent-admin.py", "--base-url", "http://cp.example.test", "--username", "operator", *map(str, args)]
        with patch.object(sys, "argv", argv), patch.object(cli, "SessionAPI", return_value=api), \
             patch.object(cli.getpass, "getpass", return_value="test-only-password"), contextlib.redirect_stdout(output):
            cli.main()
        self.assertNotIn("test-token-", output.getvalue())
        self.assertEqual(api.calls[-1][0], "/auth/logout")

    def test_distinct_enrollments_for_two_servers(self):
        api = API()
        with tempfile.TemporaryDirectory() as directory:
            for name in ["server-a", "server-b"]:
                self.invoke(api, "enroll", "--organization", "VTT", "--name", name,
                            "--token-file", Path(directory) / name)
            self.assertNotEqual((Path(directory) / "server-a").read_text(), (Path(directory) / "server-b").read_text())
        self.assertTrue(all(e["organizationId"] == "org-a" for e in api.created))

    def test_organization_user_leaves_ownership_to_backend(self):
        api = API("ORGANIZATION_USER")
        with tempfile.TemporaryDirectory() as directory:
            self.invoke(api, "enroll", "--organization", "VTT", "--name", "server-a", "--token-file", Path(directory) / "token")
        body = next(body for path, body in api.calls if path == "/enrollments" and body is not None)
        self.assertNotIn("organizationId", body)

    def test_existing_token_file_is_never_overwritten(self):
        api = API()
        with tempfile.TemporaryDirectory() as directory:
            target = Path(directory) / "token"
            target.write_text("keep-existing")
            with self.assertRaises(FileExistsError):
                self.invoke(api, "enroll", "--organization", "VTT", "--name", "server-a", "--token-file", target)
            self.assertEqual(target.read_text(), "keep-existing")
            self.assertEqual(api.created, [])

    def test_smoke_has_no_placement_or_ownership_override(self):
        api = API("ORGANIZATION_USER")
        self.invoke(api, "smoke")
        body = next(body for path, body in api.calls if path == "/jobs")
        self.assertEqual(body["resources"]["gpuCount"], 1)
        self.assertTrue(body["neededAt"])
        self.assertEqual(body["command"], ["sh", "-c", "nvidia-smi -L && sleep 300"])
        self.assertFalse({"organizationId", "serverSelector", "priority", "strategy"}.intersection(body))


if __name__ == "__main__":
    unittest.main()
