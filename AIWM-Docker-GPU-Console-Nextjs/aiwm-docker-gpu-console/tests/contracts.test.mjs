import test from "node:test";
import assert from "node:assert/strict";
import { jobFormSchema, parseKeyValueLines, allocationFormSchema, allocationInput } from "../src/lib/jobs/form-schema.ts";
import { allowedMutationOrigin, allowedPublicRoute } from "../src/lib/api/proxy-policy.ts";

const valid = { name: "gpu-demo", image: "alpine:3.21", commandLines: "", environmentLines: "", gpuCount: 1, minVramGB:1, performanceProfile:"AUTO", fp8Required:false, workloadType:"TRAINING",necessityLevel:"NECESSITY_2",necessityReason:"GO_LIVE_90_DAYS",necessityExplanation:"",systemImportance:"IMPORTANT",neededAt:new Date(Date.now() + 3600000 - new Date().getTimezoneOffset()*60000).toISOString().slice(0,16),ttlHours:1 };

test("same-origin mutation works for inbound localhost and IP hosts, rejects foreign origins", () => {
 assert.equal(allowedMutationOrigin("http://127.0.0.1:3000", "127.0.0.1:3000"), true);
 assert.equal(allowedMutationOrigin("http://localhost:3000", "localhost:3000"), true);
 assert.equal(allowedMutationOrigin("https://console.example", "console.example"), true);
 for (const origin of ["https://untrusted.example", "http://localhost:3001", "null", "http://localhost:3000/path", "http://user@localhost:3000"]) {
   assert.equal(allowedMutationOrigin(origin, "localhost:3000"), false);
 }
});
test("intent input preserves empty environment and omits placement controls", () => {
 assert.equal(jobFormSchema.safeParse(valid).success, true);
 assert.deepEqual(parseKeyValueLines("EMPTY=\nURL=a=b"), { EMPTY: "", URL: "a=b" });
 const input=allocationInput({...valid,ttlHours:1.5});
 assert.equal(input.ttlSeconds,5400);
 assert.match(input.neededAt,/Z$/);
 for (const field of ["priority","strategy","serverSelector"]) {
   assert.equal(field in input,false);
   assert.equal(jobFormSchema.safeParse({...valid,[field]:"forbidden"}).success,false);
 }
 assert.equal("gpuModel" in input.resources,false);
 assert.equal(input.resources.minVramMiB,1024);
 assert.equal("cpuMilli" in input.resources,false);
 assert.equal("memoryMiB" in input.resources,false);
});
test("catalog supplies limits and workload-specific reasons", () => {
 const options={limits:{maxGpuCount:8,maxTtlSeconds:7200},performanceProfiles:[{id:"AUTO",label:"Tổng quát"}],
 customReason:{id:"CUSTOM",label:"Khác"},necessityProfiles:[{workloadType:"TRAINING",level:"NECESSITY_2",reasons:[{id:"GO_LIVE_90_DAYS",label:"Go-live"}]}]};
 const schema=allocationFormSchema(options);
 assert.equal(schema.safeParse(valid).success,true);
 for (const change of [{gpuCount:9},{ttlHours:3},{performanceProfile:"bad"},{workloadType:"INFERENCE"},{necessityLevel:"NECESSITY_1"},{necessityReason:"CUSTOM"}]) {
   assert.equal(schema.safeParse({...valid,...change}).success,false);
 }
 assert.equal(schema.safeParse({...valid,necessityReason:"CUSTOM",necessityExplanation:"Lý do thực tế"}).success,true);
 assert.equal(allocationFormSchema({...options,limits:{...options.limits,maxGpuCount:100}}).safeParse({...valid,gpuCount:65}).success,true);
});
for (const environmentLines of ["missing-equals", "A=1\nA=2", "NVIDIA_VISIBLE_DEVICES=all", "CUDA_VISIBLE_DEVICES=0", "BAD KEY=x"]) {
 test("reject malformed or unsafe environment: " + environmentLines, () => assert.equal(jobFormSchema.safeParse({ ...valid, environmentLines }).success, false));
}
test("validate counts and resource limits", () => {
 for (const fields of [{gpuCount: 0}, {gpuCount: -1}, {gpuCount: 1.5}, {minVramGB:0}, {minVramGB:-1}, {workloadType:"OTHER"}, {systemImportance:"OTHER"}, {necessityLevel:"NECESSITY_5"}, {neededAt:"invalid"}, {neededAt:"2026-02-30T10:00"}, {ttlHours:0}, {memoryMiB: -1}, {image: "bad image"}]) assert.equal(jobFormSchema.safeParse({...valid, ...fields}).success, false);
});
test("proxy allows public routes but rejects Agent, Docker, traversal and unsupported verbs", () => {
 assert.equal(allowedPublicRoute("GET", ["servers", "srv_123"]), true);
 assert.equal(allowedPublicRoute("GET", ["jobs", "options"]), true);
 assert.equal(allowedPublicRoute("POST", ["jobs", "preview"]), true);
 assert.equal(allowedPublicRoute("POST", ["jobs", "job_123", "stop"]), true);
 for (const path of [["agents", "srv_123", "commands"], ["docker"], ["..", "agents"], ["jobs%2f.."], ["jobs", ".", "stop"]]) assert.equal(allowedPublicRoute("GET", path), false);
 assert.equal(allowedPublicRoute("DELETE", ["jobs", "job_123"]), false);
});

test("time planning rejects stale start and keeps half-hour duration", () => {
 assert.equal(jobFormSchema.safeParse({...valid,neededAt:"2020-01-01T08:00"}).success,false);
 assert.equal(allocationInput({...valid,ttlHours:0.5}).ttlSeconds,1800);
});

test("training proxy only exposes authenticated continuation and artifact download", () => {
  assert.equal(allowedPublicRoute("POST", ["jobs", "job_123", "continue"]), true);
  assert.equal(allowedPublicRoute("GET", ["jobs", "job_123", "artifact"]), true);
  assert.equal(allowedPublicRoute("GET", ["training", "job_123"]), false);
  assert.equal(allowedPublicRoute("PUT", ["training", "job_123", "artifact"]), false);
});
