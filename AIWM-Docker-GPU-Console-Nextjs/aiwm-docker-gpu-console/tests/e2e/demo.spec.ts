import { expect, test, type Page } from "@playwright/test";
import { readFileSync } from "node:fs";
import { resolve } from "node:path";

const accounts = JSON.parse(readFileSync(resolve(process.cwd(), "../../config/demo-users.json"), "utf8")).users as {
  username: string; password: string; role: string; organizationCode: string;
}[];

async function login(page: Page, username: string) {
  const account = accounts.find(user => user.username === username)!;
  await page.goto("/login");
  await page.getByLabel("Tên đăng nhập hoặc email").fill(account.username);
  await page.getByLabel("Mật khẩu", { exact: true }).fill(account.password);
  await page.getByRole("button", { name: "Đăng nhập", exact: true }).click();
  await expect(page).toHaveURL(/\/$/);
  await expect(page.locator("h1")).toBeVisible();
}

test("demo ADMIN xem sáu servers và các màn chính", async ({ page }) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  await login(page, "admin");
  const summary = await (await page.request.get("/api/aiwm/system/summary")).json();
  expect(summary.data.serversTotal).toBe(6);
  expect(summary.data.gpusTotal).toBe(26);
  for (const route of ["/servers", "/gpus", "/containers", "/workloads", "/queue", "/onboarding", "/organizations"]) {
    await page.goto(route);
    await expect(page.locator("h1")).toBeVisible();
  }
  await page.setViewportSize({ width: 1024, height: 768 });
  await page.goto("/servers");
  await expect(page.getByRole("link", { name: "vtt-gpu-01", exact: true })).toBeVisible();
  expect(errors).toEqual([]);
  await page.getByRole("button", { name: "Đăng xuất", exact: true }).click();
  await expect(page).toHaveURL(/\/login$/);
});

test("demo VTT tạo workload qua form rồi release GPU", async ({ page }) => {
  await login(page, "vtt");
  const me = await (await page.request.get("/api/aiwm/auth/me")).json();
  const servers = await (await page.request.get("/api/aiwm/servers")).json();
  expect(servers.data).toHaveLength(2);
  expect(servers.data.every((s: { organizationId: string }) => s.organizationId === me.data.user.organizationId)).toBe(true);
  let jobID = "";
  try {
    await page.goto("/workloads/new");
    await page.getByLabel("Tên workload", { exact: true }).fill("browser-demo-" + Date.now());
    await page.getByLabel("Docker image", { exact: true }).fill("alpine:3.21");
    await page.locator('[name="commandLines"]').fill("sh\n-c\nsleep 300");
    await page.getByLabel("Loại workload").selectOption("TRAINING");
    await page.getByLabel("Số GPU", { exact: true }).fill("1");
    await page.getByLabel("Profile hiệu năng").selectOption("general");
    await page.locator('[name="necessityLevel"]').selectOption("NECESSITY_2");
    await page.getByLabel("Mức độ quan trọng hệ thống").selectOption("IMPORTANT");
    await page.locator('[name="necessityReason"]').selectOption("GO_LIVE_90_DAYS");
    await page.locator('[name="neededAt"]').fill(new Date().toISOString().slice(0, 16));
    await page.getByRole("button", { name: "Đối chiếu tự động", exact: true }).click();
    const submit = page.getByRole("button", { name: "Gửi vào hàng đợi", exact: true });
    await expect(submit).toBeEnabled();
    const created = page.waitForResponse(r => r.url().endsWith("/api/aiwm/jobs") && r.request().method() === "POST");
    await submit.click();
    jobID = (await (await created).json()).data.id;
    await expect.poll(async () => (await (await page.request.get("/api/aiwm/jobs/" + jobID)).json()).data.status).toBe("RUNNING");
    await page.request.post("/api/aiwm/jobs/" + jobID + "/stop", { data: {} });
    await expect.poll(async () => (await (await page.request.get("/api/aiwm/jobs/" + jobID)).json()).data.assignment.reservationState).toBe("RELEASED");
  } finally {
    if (jobID) await page.request.post("/api/aiwm/jobs/" + jobID + "/stop", { data: {} });
    await page.request.post("/api/aiwm/auth/logout", { data: {} });
  }
});
