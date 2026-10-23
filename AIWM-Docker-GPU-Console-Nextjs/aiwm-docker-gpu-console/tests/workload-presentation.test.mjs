import test from "node:test";
import assert from "node:assert/strict";
import { allocationProgress, allocationTimeLabel, canContinue, workloadDisplay, workloadGPU, workloadWarning } from "../src/lib/jobs/presentation.ts";

const now = Date.parse("2026-09-24T12:00:00Z");
const iso = n => new Date(n).toISOString();
const hour = 3600000;
function job(patch = {}) {
  return { id: "j", name: "OCR", organizationId: "vtt", workloadType: "TRAINING", status: "RUNNING",
    requestedStartAt: iso(now - hour), requestedEndAt: iso(now + hour),
    resources: { gpuCount: 4, performanceProfile: "HIGH_PERFORMANCE" }, training: { checkpointStatus: "NONE", artifactStatus: "NONE" }, ...patch };
}

test("allocation progress uses time only, clamps and keeps expired red", () => {
  assert.deepEqual(allocationProgress(job(), now), { percent: 50, tone: "success" });
  const future = job({ requestedStartAt: iso(now + hour), requestedEndAt: iso(now + 2*hour) });
  assert.deepEqual(allocationProgress(future, now), { percent: 0, tone: "success" });
  const expired = job({ requestedStartAt: iso(now - 2*hour), requestedEndAt: iso(now - hour), status: "STOPPED", terminationReason: "TIME_LIMIT" });
  assert.deepEqual(allocationProgress(expired, now), { percent: 100, tone: "danger" });
  for (const [elapsed, tone] of [[69,"success"],[70,"warning"],[89,"warning"],[90,"danger"],[100,"danger"]]) {
    assert.deepEqual(allocationProgress(job({ requestedStartAt: iso(now), requestedEndAt: iso(now+100000) }), now+elapsed*1000), { percent: elapsed, tone });
  }
  assert.equal(allocationProgress(job({ requestedEndAt: "bad" }), now), null);
});

test("business status covers planned, running, warning, saving and expiry", () => {
  const cases = [
    [{ status:"ASSIGNED" }, "Đã lên lịch"], [{}, "Đang chạy"],
    [{ training:{checkpointWarningAt:iso(now),checkpointStatus:"NONE"} }, "Cảnh báo"],
    [{ training:{checkpointStatus:"SAVING"} }, "Đang lưu checkpoint"],
    [{ status:"STOPPING",terminationReason:"TIME_LIMIT" }, "Đang thu hồi"],
    [{ status:"STOPPED",terminationReason:"TIME_LIMIT",training:{checkpointStatus:"AVAILABLE",latestCheckpointURI:"s3://test/checkpoint"},resumable:true }, "Có thể tiếp tục"],
    [{ status:"STOPPED",terminationReason:"TIME_LIMIT" }, "Hết thời gian"],
    [{ status:"SUCCEEDED",training:{artifactStatus:"READY"} }, "Hoàn thành"],
    [{ status:"SUCCEEDED",training:{artifactStatus:"NONE"} }, "Đang lưu model"],
    [{ status:"SUCCEEDED",training:{artifactStatus:"FAILED"} }, "Lỗi lưu model"],
    [{ status:"SUCCEEDED",workloadType:"INFERENCE" }, "Hoàn thành"],
    [{ status:"FAILED" }, "Thất bại"], [{ status:"CANCELLED" }, "Đã hủy"],
  ];
  for (const [patch,label] of cases) assert.equal(workloadDisplay(job(patch),now).label,label);
  assert.equal(workloadWarning(job({ requestedEndAt:iso(now+1000), training:{checkpointWarningAt:iso(now+500)} }),now),false);
  assert.equal(workloadWarning(job({ training:{checkpointWarningAt:"0001-01-01T00:00:00Z"} }),now),false);
  assert.equal(canContinue(job({status:"STOPPED",terminationReason:"TIME_LIMIT",resumable:false,training:{checkpointStatus:"AVAILABLE",latestCheckpointURI:"s3://x"}})),false);
});

test("relative labels remain business-friendly and completed is distinct", () => {
  assert.equal(allocationTimeLabel(job({ requestedStartAt:iso(now+3*24*hour),requestedEndAt:iso(now+4*24*hour) }),now),"Bắt đầu sau 3 ngày");
  assert.equal(allocationTimeLabel(job({ requestedEndAt:iso(now+3*24*hour) }),now),"Còn 3 ngày");
  assert.equal(allocationTimeLabel(job({ status:"STOPPED",terminationReason:"TIME_LIMIT",requestedStartAt:iso(now-4*24*hour),requestedEndAt:iso(now-3*24*hour) }),now),"Hết hạn 3 ngày trước");
  assert.equal(allocationTimeLabel(job({status:"SUCCEEDED"}),now),"Đã hoàn thành");
});

test("resolved model follows selected inventory and organization; never exposes UUID", () => {
  const servers=[{id:"s",organizationId:"vtt",gpus:[{uuid:"a",model:"H100"},{uuid:"b",model:"L40S"},{uuid:"c",model:"H100"}]}];
  assert.equal(workloadGPU(job()),"4 GPU · Hiệu năng cao");
  const allocated=job({assignment:{serverId:"s",gpuUuids:["a","c"]}});
  assert.equal(workloadGPU(allocated,servers),"2 × H100");
  assert.equal(workloadGPU(allocated,[{...servers[0],organizationId:"vds"}]),"2 GPU");
  assert.equal(workloadGPU(allocated,[]),"2 GPU");
  assert.equal(workloadGPU(job({assignment:{serverId:"s",gpuUuids:["a","b"]}}),servers),"1 × H100 · 1 × L40S");
});
