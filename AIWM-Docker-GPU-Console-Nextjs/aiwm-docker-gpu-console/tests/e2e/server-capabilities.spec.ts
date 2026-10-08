import { expect, test, type Page } from "@playwright/test";
import type { Server, ServerCapabilities } from "../../src/lib/api/types";

const organization = { id: "vtt", code: "VTT", name: "Viettel Telecom", enabled: true };
const full: ServerCapabilities = {
  agentOperational: true, operatingMode: "FULL", os: "linux", architecture: "amd64",
  kernelVersion: "6.8.0", cgroupMode: "v2", machineIdAvailable: true,
  dockerAvailable: true, dockerVersion: "28.0.0", dockerApiVersion: "1.52", dockerOs: "linux",
  gpuInventoryAvailable: true, nvmlAvailable: true, gpuCount: 1,
  nvidiaDriverVersion: "570.0", cudaDriverVersion: "12.8",
  nvidiaContainerSupport: true, managedExecutionReady: true, reasons: [],
};

async function setup(page: Page, admin: boolean, capabilities = full) {
  const now = new Date().toISOString();
  const server: Server = {
    id: "s1", organizationId: "vtt", machineId: "machine-private", name: "GPU VTT 01",
    status: "ONLINE", drained: false, schedulable: capabilities.managedExecutionReady,
    schedulingReason: "developer diagnostics", inventoryVersion: 1, inventoryReceivedAt: now, lastHeartbeatAt: now,
    host: { hostname: "host-01", os: "linux", architecture: "amd64", cpuCount: 32, memoryTotalMiB: 131072 },
    capabilities, agentVersion: "0.2.0",
    gpus: [{ uuid: "GPU-private-uuid", index: 0, model: "H100", memoryUsedMiB: 1024, memoryTotalMiB: 81920, utilizationPct: 20, temperatureC: 45, healthy: true, state: "OCCUPIED_UNKNOWN" }],
    containers: [{ id: "private-container-id", name: "Existing Training", origin: "LEGACY", state: "running", image: "training:demo", gpuUuids: ["GPU-private-uuid"] }],
  };
  await page.route("**/api/**", async route => {
    const path = new URL(route.request().url()).pathname;
    let data: unknown;
    if (path.endsWith("/aiwm-health")) return route.fulfill({ json: { status: "ok" } });
    if (path.endsWith("/auth/me") || path.endsWith("/auth/login")) data = { user: { username: admin ? "admin" : "vtt", role: admin ? "ADMIN" : "ORGANIZATION_USER", organizationId: "vtt" }, organization };
    else if (path.endsWith("/organizations")) {
      expect(admin, "normal user must not query cross-org metadata").toBe(true);
      data = [organization];
    }
    else if (path.endsWith("/servers")) data = [server];
    else if (path.endsWith("/servers/s1")) data = server;
    else return route.fulfill({ status: 404, json: { error: { message: "Không tìm thấy dữ liệu" } } });
    return route.fulfill({ json: { data } });
  });
  await page.goto("/servers");
  await page.getByRole("link", { name: "GPU VTT 01", exact: true }).click();
  await expect(page.getByRole("heading", { name: "GPU VTT 01", exact: true })).toBeVisible();
  return server;
}

for (const admin of [true, false]) {
  test("FULL server detail with organization context " + (admin ? "admin" : "user"), async ({ page }) => {
    await setup(page, admin);
    await expect(page.getByLabel("Tài khoản hiện tại")).toHaveText(admin ? "admin" : "VTT");
    await expect(page.locator(".page-header")).toContainText("Viettel Telecom");
    await expect(page.locator(".page-header")).toContainText("VTT");
    await expect(page.getByText("Đầy đủ chức năng", { exact: true })).toBeVisible();
    await expect(page.getByText("Existing Training", { exact: true })).toBeVisible();
    await expect(page.getByRole("cell", { name: "H100", exact: false })).toBeVisible();
    await expect(page.getByText("6.8.0", { exact: true })).toBeHidden();
    await page.getByText("Thông tin nền tảng", { exact: true }).click();
    await expect(page.getByText("6.8.0", { exact: true })).toBeVisible();
    await expect(page.getByText("CUDA Driver", { exact: true })).toBeVisible();
    await expect(page.getByText("12.8", { exact: true })).toBeVisible();
    await expect(page.locator("main")).not.toContainText(/NVML|source validity|reconciliation|GPU-private|private-container|developer diagnostics|aiwm.managed/);
    if (admin) await page.screenshot({ path: "test-results/server-capability-full.png" });
    await page.setViewportSize({ width: 390, height: 844 });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth)).toBe(true);
  });
}

test("GPU observe only retains GPU and recovers without identity change", async ({ page }) => {
  const server = await setup(page, true, { ...full, operatingMode: "GPU_OBSERVE_ONLY", dockerAvailable: false, managedExecutionReady: false, reasons: ["DOCKER_UNAVAILABLE"] });
  await expect(page.getByText("Chỉ giám sát GPU", { exact: true })).toBeVisible();
  await expect(page.getByText("Không có dữ liệu container.", { exact: true })).toBeVisible();
  await expect(page.getByText("Existing Training", { exact: true })).toHaveCount(0);
  await expect(page.getByRole("cell", { name: "H100", exact: false })).toBeVisible();
  await page.screenshot({ path: "test-results/server-capability-observe.png" });
  server.capabilities = full; server.schedulable = true;
  await page.getByRole("button", { name: "Làm mới", exact: true }).click();
  await expect(page.getByText("Đầy đủ chức năng", { exact: true })).toBeVisible();
  await expect(page.getByText("Existing Training", { exact: true })).toBeVisible();
});

test("Docker observe only marks GPUs as last known without hiding containers", async ({ page }) => {
  await setup(page, false, { ...full, operatingMode: "DOCKER_OBSERVE_ONLY", nvmlAvailable: false, gpuInventoryAvailable: false, managedExecutionReady: false, reasons: ["NVML_UNAVAILABLE"] });
  await expect(page.getByText("Chỉ giám sát container", { exact: true })).toBeVisible();
  await expect(page.getByText("Hiển thị lần ghi nhận gần nhất", { exact: true })).toBeVisible();
  await expect(page.getByText("Existing Training", { exact: true })).toBeVisible();
});

test("missing NVIDIA runtime remains observable but cannot accept workloads", async ({ page }) => {
  await setup(page, true, { ...full, operatingMode: "DEGRADED", nvidiaContainerSupport: false, managedExecutionReady: false, reasons: ["NVIDIA_CONTAINER_UNAVAILABLE"] });
  await expect(page.getByText("Giới hạn chức năng", { exact: true })).toBeVisible();
  const row = page.locator("dl > div").filter({ has: page.getByText("Nhận workload mới", { exact: true }) });
  await expect(row).toContainText("Không khả dụng");
  await expect(page.getByText("Existing Training", { exact: true })).toBeVisible();
});

test("missing capability data is not shown as FULL", async ({ page }) => {
  const server = await setup(page, true);
  server.capabilities = undefined;
  await page.getByRole("button", { name: "Làm mới", exact: true }).click();
  await expect(page.getByText("Chưa xác định chức năng", { exact: true })).toBeVisible();
  await expect(page.getByText("Đầy đủ chức năng", { exact: true })).toHaveCount(0);
});

test("server errors show actionable feedback without backend exceptions", async ({ page }) => {
  await setup(page, true);
  await page.route("**/api/**/servers/s1", route => route.fulfill({
    status: 500, json: { error: { message: "NVML probe exception: /var/run/docker.sock private trace" } },
  }));
  await page.getByRole("button", { name: "Làm mới", exact: true }).click();
  await expect(page.getByText("Không lấy được thông tin máy chủ. Vui lòng thử lại.", { exact: true })).toBeVisible();
  await expect(page.locator("main")).not.toContainText(/NVML|docker.sock|private trace|probe/);
});
