import test from "node:test";
import assert from "node:assert/strict";
import { jobFormSchema, parseKeyValueLines } from "../src/lib/jobs/form-schema.ts";
import { allowedMutationOrigin, allowedPublicRoute } from "../src/lib/api/proxy-policy.ts";

const valid = { name: "gpu-demo", image: "alpine:3.21", commandLines: "", environmentLines: "", gpuCount: 1, gpuModel: "", priority: 50, strategy: "best-fit", selectorLines: "" };
test("same-origin mutation works for inbound localhost and IP hosts, rejects foreign origins", () => {
 assert.equal(allowedMutationOrigin("http://127.0.0.1:3000", "127.0.0.1:3000"), true);
 assert.equal(allowedMutationOrigin("http://localhost:3000", "localhost:3000"), true);
 assert.equal(allowedMutationOrigin("https://console.example", "console.example"), true);
 for (const origin of ["https://untrusted.example", "http://localhost:3001", "null", "http://localhost:3000/path", "http://user@localhost:3000"]) {
   assert.equal(allowedMutationOrigin(origin, "localhost:3000"), false);
 }
});
test("accept all four strategies and preserve empty environment values", () => {
 for (const strategy of ["first-fit", "best-fit", "bin-pack", "fragmentation-aware"]) assert.equal(jobFormSchema.safeParse({ ...valid, strategy }).success, true);
 assert.deepEqual(parseKeyValueLines("EMPTY=\nURL=a=b"), { EMPTY: "", URL: "a=b" });
});
for (const environmentLines of ["missing-equals", "A=1\nA=2", "NVIDIA_VISIBLE_DEVICES=all", "CUDA_VISIBLE_DEVICES=0", "BAD KEY=x"]) {
 test("reject malformed or unsafe environment: " + environmentLines, () => assert.equal(jobFormSchema.safeParse({ ...valid, environmentLines }).success, false));
}
test("validate counts and resource limits", () => {
 for (const fields of [{gpuCount: 0}, {gpuCount: 65}, {priority: -1}, {memoryMiB: -1}, {image: "bad image"}]) assert.equal(jobFormSchema.safeParse({...valid, ...fields}).success, false);
});
test("proxy allows public routes but rejects Agent, Docker, traversal and unsupported verbs", () => {
 assert.equal(allowedPublicRoute("GET", ["servers", "srv_123"]), true);
 assert.equal(allowedPublicRoute("POST", ["jobs", "job_123", "stop"]), true);
 for (const path of [["agents", "srv_123", "commands"], ["docker"], ["..", "agents"], ["jobs%2f.."], ["jobs", ".", "stop"]]) assert.equal(allowedPublicRoute("GET", path), false);
 assert.equal(allowedPublicRoute("DELETE", ["jobs", "job_123"]), false);
});
