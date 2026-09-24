import { expect, test, type Page } from "@playwright/test";

const organizations = [
  { id: "vtt", code: "VTT", name: "Viettel Telecom", enabled: true },
  { id: "vds", code: "VDS", name: "Viettel Digital Services", enabled: true },
];
const gpu = (uuid: string, utilizationPct: number, state = "FREE") => ({
  uuid, index: 0, model: "H100", utilizationPct, state, healthy: true, memoryTotalMiB: 81920, memoryUsedMiB: 0,
});
const servers = [
  { id: "s1", organizationId: "vtt", name: "vtt-gpu-01", gpus: [gpu("g1", 20), gpu("g2", 60, "ALLOCATED")] },
  { id: "s2", organizationId: "vds", name: "vds-gpu-01", gpus: [gpu("g3", 80)] },
].map(s => ({ ...s, status: "ONLINE", schedulable: true, inventoryReceivedAt: new Date().toISOString(), lastHeartbeatAt: new Date().toISOString(), containers: [], host: {}, labels: {} }));
const job = {
  id: "job-test", name: "Training demo", organizationId: "vtt", status: "ASSIGNED",
  image: "alpine:3.21", resources: { gpuCount: 1, minVramMiB: 1024, performanceProfile: "AUTO" },
  necessityLabel: "Cần thiết 2", neededAt: new Date().toISOString(), ttlSeconds: 3600,
  requestedStartAt: new Date().toISOString(), requestedEndAt: new Date(Date.now()+3600000).toISOString(),
  createdAt: new Date().toISOString(), updatedAt: new Date().toISOString(), events: [],
  assignment: { serverId: "s1", gpuUuids: ["g1"], reservationState: "PLANNED", startAt: new Date().toISOString(), endAt: new Date(Date.now()+3600000).toISOString() },
  statusReason: "scheduler: GPU UUID; reconciliation; aiwm.managed=true",
};
const options = {
  limits: { maxGpuCount: 64, maxTtlSeconds: 2592000 },
  workloadTypes: [{ id: "TRAINING", label: "Training" }, { id: "INFERENCE", label: "Inference" }],
  performanceProfiles: [{ id: "AUTO", label: "Auto" }, { id: "HIGH_PERFORMANCE", label: "Hiệu năng cao" }],
  systemImportance: [{ id: "IMPORTANT", label: "Quan trọng" }],
  customReason: { id: "CUSTOM", label: "Khác" },
  necessityProfiles: [{ workloadType: "TRAINING", level: "NECESSITY_2", label: "Cần thiết 2", reasons: [{ id: "GO_LIVE_90_DAYS", label: "Sắp triển khai" }] }],
};

async function setup(page: Page, username: "admin" | "vtt") {
  const previews: Record<string, unknown>[] = [];
  const admin = username === "admin";
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown;
    if (path.endsWith("/health")) return route.fulfill({ json: { status: "ok" } });
    if (path.endsWith("/auth/login") || path.endsWith("/auth/me")) data = { user: { username, role: admin ? "ADMIN" : "ORGANIZATION_USER", organizationId: "vtt" }, organization: organizations[0] };
    else if (path.endsWith("/organizations")) data = organizations;
    else if (path.endsWith("/users") || path.endsWith("/enrollments")) data = [];
    else if (path.endsWith("/servers")) {
      const filter = new URL(route.request().url()).searchParams.get("organizationId");
      data = servers.filter(s => (admin || s.organizationId === "vtt") && (!filter || filter === s.organizationId));
    } else if (path.endsWith("/servers/s1")) data = servers[0];
    else if (path.endsWith("/gpus")) data = servers.filter(s => admin || s.organizationId === "vtt").flatMap(s => s.gpus.map(gpu => ({ organizationId: s.organizationId, serverId: s.id, serverName: s.name, serverStatus: s.status, gpu })));
    else if (path.endsWith("/containers")) data = [];
    else if (path.endsWith("/jobs/options")) data = options;
    else if (path.endsWith("/jobs/preview")) {
      const input = route.request().postDataJSON();
      previews.push(input);
      data = { requestedStartAt: input.neededAt, requestedEndAt: new Date(Date.parse(input.neededAt)+input.ttlSeconds*1000).toISOString(), planningStatus: "AVAILABLE", resourceMatch: { satisfiable: true, recommendedGpuModels: ["H100"], matchedGpuCount: input.resources.gpuCount } };
    } else if (path.endsWith("/jobs/job-test")) data = job;
    else if (path.endsWith("/jobs")) data = [job];
    else if (path.endsWith("/queue")) data = [{ ...job, status: "QUEUED" }];
    else if (path.endsWith("/auth/logout")) data = { loggedOut: true };
    else return route.fulfill({ status: 404, json: { error: { message: "Unknown mock endpoint: " + path } } });
    await route.fulfill({ json: { data } });
  });
  await page.goto("/login");
  await page.getByLabel("Tên đăng nhập hoặc email").fill(username);
  await page.getByLabel("Mật khẩu", { exact: true }).fill("UI-fixture-only-password");
  await page.getByRole("button", { name: "Đăng nhập", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.getByLabel("Tài khoản hiện tại")).toHaveText(username === "admin" ? "admin" : "VTT");
  return previews;
}

test("admin has global fleet, organization utilization and shared account context", async ({ page }) => {
  await setup(page, "admin");
  await expect(page.getByRole("heading", { name: "Hiệu suất theo đơn vị" })).toBeVisible();
  await expect(page.getByRole("row").filter({ hasText: "VTT" })).toContainText("40%");
  await expect(page.getByRole("row").filter({ hasText: "VDS" })).toContainText("80%");
  await expect(page.getByText("53,3%", { exact: true })).toBeVisible();
  for (const text of ["Đơn vị của tài khoản", "admin · ADMIN", "Workload gần đây", "Sự kiện job gần đây", "Bộ lọc chỉ áp dụng"]) await expect(page.getByText(text, { exact: false })).toHaveCount(0);
  await page.goto("/servers");
  await expect(page.getByLabel("Tài khoản hiện tại")).toHaveText("admin");
  await expect(page.getByRole("link", { name: "vds-gpu-01", exact: true })).toBeVisible();
});

test("organization dashboard has only its fleet and a fixed organization identity", async ({ page }) => {
  await setup(page, "vtt");
  await expect(page.getByRole("heading", { name: "Tài nguyên theo máy chủ" })).toBeVisible();
  await expect(page.getByRole("link", { name: "vtt-gpu-01", exact: true })).toBeVisible();
  await expect(page.getByText("40%", { exact: true }).first()).toBeVisible();
  await expect(page.getByText(/VDS|VTNET|vds-gpu/)).toHaveCount(0);
  await expect(page.getByRole("heading", { name: "Hiệu suất theo đơn vị" })).toHaveCount(0);
  await page.goto("/gpus");
  await expect(page.getByLabel("Tài khoản hiện tại")).toHaveText("VTT");
  await expect(page.getByText("vds-gpu-01")).toHaveCount(0);
});

test("four capability inputs auto-preview, debounce and keep execution/time fields", async ({ page }) => {
  const previews = await setup(page, "vtt");
  await page.goto("/workloads/new");
  await expect(page.getByLabel("Docker image")).toBeVisible();
  await expect(page.getByLabel("Command — mỗi đối số một dòng")).toBeVisible();
  await expect(page.getByLabel("Environment — KEY=value mỗi dòng")).toBeVisible();
  await expect(page.getByLabel(/CPU|RAM \(MiB\)|GPU model|strategy|UUID/i)).toHaveCount(0);
  await expect(page.getByRole("button", { name: /Đối chiếu|Kiểm tra khả năng|Preview/ })).toHaveCount(0);
  await expect(page.getByLabel("Profile hiệu năng").locator("option")).toHaveText(["Chọn…", "Auto", "Hiệu năng cao"]);
  await page.getByLabel("Loại workload").selectOption("TRAINING");
  await page.getByLabel("Tính cần thiết", { exact: true }).selectOption("NECESSITY_2");
  await page.getByLabel("Mức độ quan trọng hệ thống").selectOption("IMPORTANT");
  await page.getByLabel("Lý do tính cần thiết").selectOption("GO_LIVE_90_DAYS");
  const start = new Date(Date.now() + 3600000);
  const local = new Date(start.getTime() - start.getTimezoneOffset()*60000).toISOString().slice(0,16);
  await page.getByLabel("Thời điểm bắt đầu", { exact: false }).fill(local);
  await expect(page.getByText("Có tài nguyên phù hợp", { exact: true })).toBeVisible();
  const before = previews.length;
  await page.getByLabel("Số GPU", { exact: true }).fill("2");
  await page.getByLabel("Số GPU", { exact: true }).fill("3");
  await expect(page.getByRole("button", { name: "Gửi vào hàng đợi" })).toBeDisabled();
  await expect(page.getByText("3 GPU theo yêu cầu")).toBeVisible();
  expect(previews.length - before).toBe(1);
  await page.getByLabel("VRAM tối thiểu / GPU (GB)").fill("80");
  await page.getByLabel("Profile hiệu năng").selectOption("HIGH_PERFORMANCE");
  await page.getByLabel("Yêu cầu FP8", { exact: false }).check();
  await expect.poll(() => previews.at(-1)?.resources).toEqual({ gpuCount: 3, minVramMiB: 81920, performanceProfile: "HIGH_PERFORMANCE", fp8Required: true });
  await expect(page.getByRole("button", { name: "Gửi vào hàng đợi" })).toBeEnabled();
  await page.getByLabel("Số GPU", { exact: true }).fill("0");
  await expect(page.getByRole("button", { name: "Gửi vào hàng đợi" })).toBeDisabled();
  await expect(page.getByText("Có tài nguyên phù hợp", { exact: true })).toHaveCount(0);
});

test("operator pages omit developer notes while preserving business actions", async ({ page }) => {
  await setup(page, "admin");
  const errors: string[] = [];
  page.on("pageerror", e => errors.push(e.message));
  for (const path of ["/servers", "/servers/s1", "/gpus", "/containers", "/workloads", "/workloads/job-test", "/queue", "/scheduler", "/settings", "/onboarding", "/organizations"]) {
    await page.goto(path);
    await expect(page.locator("h1")).toBeVisible();
    await expect(page.getByLabel("Tài khoản hiện tại")).toHaveText("admin");
    await expect(page.locator("main")).not.toContainText(/heartbeat|NVML|aiwm\.managed|reconciliation|scheduler|Control Plane|Brownfield|Commit nguyên tử|DeviceRequests/i);
  }
  expect(errors).toEqual([]);
});
