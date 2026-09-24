import { expect, test, type Page } from "@playwright/test";

const now = Date.parse("2026-09-24T12:00:00Z");
const hour = 3600000;
const iso = (n: number) => new Date(n).toISOString();
const base = {
  id: "running", name: "OCR giấy tờ", organizationId: "vtt", workloadType: "TRAINING", status: "RUNNING",
  image: "demo", resources: { gpuCount: 2, performanceProfile: "HIGH_PERFORMANCE" },
  requestedStartAt: iso(now-hour), requestedEndAt: iso(now+hour), createdAt: iso(now-hour), updatedAt: iso(now),
  training: { checkpointStatus:"NONE", artifactStatus:"NONE" }, events: [],
  assignment: { serverId:"s1",gpuUuids:["g1","g2"],startAt:iso(now-hour),endAt:iso(now+hour),reservationState:"ALLOCATED" },
  statusReason: "ObjectStore S3 MinIO scheduler invariant reconciliation NVML aiwm.managed=true",
};
const jobs = [
  base,
  {...base,id:"future",name:"LLM Fine-tuning",status:"ASSIGNED",requestedStartAt:iso(now+3*24*hour),requestedEndAt:iso(now+4*24*hour),assignment:{...base.assignment,startAt:iso(now+3*24*hour),endAt:iso(now+4*24*hour),reservationState:"PLANNED"}},
  {...base,id:"warning",name:"Cảnh báo thời gian",training:{...base.training,checkpointWarningAt:iso(now-60000)}},
  {...base,id:"resume",name:"Fraud Detection",status:"STOPPED",terminationReason:"TIME_LIMIT",resumable:true,requestedStartAt:iso(now-2*hour),requestedEndAt:iso(now-hour),assignment:{...base.assignment,startAt:iso(now-2*hour),endAt:iso(now-hour),reservationState:"RELEASED"},training:{...base.training,checkpointStatus:"AVAILABLE",latestCheckpointURI:"s3://hidden/checkpoint",checkpointCreatedAt:iso(now-hour),checkpointStep:25}},
  {...base,id:"expired",name:"Hết thời gian",status:"STOPPED",terminationReason:"TIME_LIMIT",resumable:false,assignment:{...base.assignment,startAt:iso(now-2*hour),endAt:iso(now-hour),reservationState:"RELEASED"}},
  {...base,id:"model",name:"Recommendation Training",status:"SUCCEEDED",terminationReason:"COMPLETED",training:{...base.training,artifactStatus:"READY",finalArtifactURI:"s3://hidden/model",artifactCreatedAt:iso(now-60000)}},
  {...base,id:"saving",name:"Lưu model",status:"SUCCEEDED"},
];
const session = {user:{username:"vtt",role:"ORGANIZATION_USER",organizationId:"vtt"},organization:{id:"vtt",code:"VTT",name:"Viettel Telecom",enabled:true}};
const server = {id:"s1",organizationId:"vtt",name:"vtt-gpu-01",status:"ONLINE",gpus:[{uuid:"g1",model:"H100"},{uuid:"g2",model:"H100"}]};
const technical = /ObjectStore|\bS3\b|MinIO|scheduler|reconciliation|NVML|heartbeat|aiwm\.managed|SIGTERM|presigned|TIME_LIMIT|GPU UUID/;
async function setup(page: Page) {
  await page.clock.setFixedTime(new Date(now));
  const continuations: Array<{neededAt:string;ttlSeconds:number}> = [];
  let rejectContinuation = true;
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown;
    if (path.endsWith("/health")) return route.fulfill({json:{status:"ok"}});
    if (path.endsWith("/auth/me")) data=session;
    else if (path.endsWith("/organizations")) data=[session.organization];
    else if (path.endsWith("/servers")) data=[server];
    else if (path.endsWith("/jobs/options")) data={limits:{maxGpuCount:64,maxTtlSeconds:7200}};
    else if (path.endsWith("/jobs/resume/continue")) {
      continuations.push(route.request().postDataJSON());
      if (rejectContinuation) {rejectContinuation=false;return route.fulfill({status:409,json:{error:{message:"internal debug should not be shown"}}});}
      data={...jobs[1],id:"new-allocation",name:"Fraud Detection"};
    } else if (path.endsWith("/jobs/model/artifact")) data={url:new URL("/test-model.bin",route.request().url()).toString(),expiresInSeconds:300};
    else if (path.endsWith("/jobs/new-allocation")) data={...jobs[1],id:"new-allocation",name:"Fraud Detection"};
    else if (path.endsWith("/jobs")) data=jobs;
    else {
      data=jobs.find(job => path.endsWith("/jobs/"+job.id));
      if (!data) return route.fulfill({status:404,json:{error:{message:"Fixture not found"}}});
    }
    return route.fulfill({json:{data}});
  });
  await page.route("**/test-model.bin",route=>route.fulfill({status:200,headers:{"content-type":"application/octet-stream","content-disposition":'attachment; filename="model-demo.bin"'},body:"demo output"}));
  return continuations;
}

test("six workload columns, resolved GPU and allocation states/progress",async({page})=>{
  await setup(page);
  await page.goto("/workloads");
  await expect(page.getByRole("columnheader")).toHaveText(["Dự án","Loại workload","GPU","Thời gian","Tiến độ cấp phát","Trạng thái"]);
  const row=(name:string)=>page.getByRole("row").filter({has:page.getByRole("link",{name,exact:true})});
  await expect(row("OCR giấy tờ")).toContainText("2 × H100");
  await expect(row("OCR giấy tờ")).toContainText("Đang chạy");
  await expect(row("OCR giấy tờ").getByRole("progressbar")).toHaveAttribute("aria-valuenow","50");
  await expect(row("LLM Fine-tuning")).toContainText("Đã lên lịch");
  await expect(row("LLM Fine-tuning")).toContainText("Bắt đầu sau 3 ngày");
  await expect(row("LLM Fine-tuning").getByRole("progressbar")).toHaveAttribute("aria-valuenow","0");
  await expect(row("Cảnh báo thời gian")).toContainText("Cảnh báo");
  await expect(row("Fraud Detection")).toContainText("Có thể tiếp tục");
  await expect(row("Fraud Detection").getByRole("progressbar")).toHaveAttribute("aria-valuenow","100");
  await expect(row("Fraud Detection").locator('[data-tone="danger"]')).toBeVisible();
  await expect(row("Hết thời gian")).toContainText("Hết thời gian");
  await expect(row("Recommendation Training")).toContainText("Hoàn thành");
  await expect(row("Lưu model")).toContainText("Đang lưu model");
  await expect(page.locator("main")).not.toContainText(technical);
  await expect(page.getByText("g1",{exact:true})).toHaveCount(0);
  await expect(page.getByText("g2",{exact:true})).toHaveCount(0);
  await expect(page.locator("main")).not.toContainText(/s3:\/\//);
  await page.getByLabel("Lọc trạng thái").selectOption("resumable");
  await expect(page.locator("tbody tr")).toHaveCount(1);
  await page.setViewportSize({width:390,height:844});
  await expect(row("Fraud Detection").getByRole("link")).toBeVisible();
  expect(await page.evaluate(()=>document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
});

test("detail warning, retained expired bar and continuation form uses current API",async({page})=>{
  const sent=await setup(page);
  await page.goto("/workloads/warning");
  await expect(page.getByText("Thời gian cấp phát sắp hết",{exact:true})).toBeVisible();
  await page.goto("/workloads/resume");
  await expect(page.getByRole("progressbar")).toHaveAttribute("aria-valuenow","100");
  await expect(page.locator('[data-tone="danger"]')).toBeVisible();
  await expect(page.getByText("Bước đã lưu")).toBeVisible();
  await expect(page.locator("main")).not.toContainText(technical);
  await page.getByRole("button",{name:"Xin cấp phát tiếp",exact:true}).click();
  const dialog=page.getByRole("dialog");
  await expect(dialog).toBeVisible();
  await dialog.getByLabel("Thời lượng sử dụng (giờ)").fill("0.5");
  await dialog.getByRole("button",{name:"Gửi yêu cầu"}).click();
  await expect(dialog.getByRole("alert")).toContainText("Chưa gửi được yêu cầu");
  await expect(dialog).not.toContainText("internal debug");
  await dialog.getByLabel("Thời lượng sử dụng (giờ)").fill("1.11");
  await dialog.getByRole("button",{name:"Gửi yêu cầu"}).click();
  await expect(page).toHaveURL(/\/workloads\/new-allocation$/);
  expect(sent).toHaveLength(2);
  expect(Object.keys(sent[1]).sort()).toEqual(["neededAt","ttlSeconds"]);
  expect(sent[0].ttlSeconds).toBe(1800);
  expect(sent[1].ttlSeconds).toBe(3996);
  expect(Date.parse(sent[1].neededAt)).toBeGreaterThan(now);
  await expect(page.getByText("Đã lên lịch",{exact:true})).toBeVisible();
});

test("download is offered only for READY; no checkpoint/protocol internals exposed",async({page})=>{
  await setup(page);
  await page.goto("/workloads/expired");
  await expect(page.getByRole("button",{name:"Xin cấp phát tiếp",exact:true})).toHaveCount(0);
  await expect(page.getByRole("button",{name:"Tải model",exact:true})).toHaveCount(0);
  await page.goto("/workloads/saving");
  await expect(page.getByRole("button",{name:"Tải model",exact:true})).toHaveCount(0);
  await page.goto("/workloads/model");
  await expect(page.getByText("Model đã sẵn sàng",{exact:true})).toBeVisible();
  await expect(page.getByText("Hoàn thành",{exact:true})).toBeVisible();
  await expect(page.locator("main")).not.toContainText(technical);
  const pending=page.waitForEvent("download");
  await page.getByRole("button",{name:"Tải model",exact:true}).click();
  const file=await pending;
  expect(file.suggestedFilename()).toBe("model-demo.bin");
});
