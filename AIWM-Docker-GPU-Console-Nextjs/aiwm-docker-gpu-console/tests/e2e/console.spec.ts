import { expect, test } from "@playwright/test";

test("live inventory, submit, placement, stop and protected external container", async ({ page, request }) => {
  const errors: string[] = [];
  page.on("pageerror", error => errors.push(error.message));
  const before = await (await request.get("/api/aiwm/containers?origin=LEGACY")).json();
  expect(before.data.length).toBeGreaterThan(0);
  await page.goto("/servers");
  await expect(page.getByRole("link", { name: "fake-server-a100", exact: true })).toBeVisible();
  await expect(page.getByRole("link", { name: "fake-server-t4", exact: true })).toBeVisible();
  await page.getByRole("link", { name: "fake-server-a100", exact: true }).click();
  await expect(page.getByText("GPU 0 · NVIDIA A100-SXM4-40GB", { exact: true })).toBeVisible();
  await expect(page.getByText("existing-inference-external-0", { exact: true })).toBeVisible();
  let jobID = "";
  try {
    await page.goto("/workloads/new");
    await page.getByLabel("Tên workload").fill("browser-demo-" + Date.now());
    await page.getByLabel("Placement strategy").selectOption("fragmentation-aware");
    await page.getByRole("button", { name: "Gửi vào hàng đợi" }).click();
    await expect(page).toHaveURL(/\/workloads\/job_/);
    jobID = page.url().split("/").pop()!;
    await expect.poll(async () => (await (await request.get("/api/aiwm/jobs/" + jobID)).json()).data.status).toBe("RUNNING");
    await expect(page.getByText("managed container observed active", { exact: true }).first()).toBeVisible();
    await page.getByRole("button", { name: "Dừng job", exact: true }).click();
    await page.getByRole("dialog").getByRole("button", { name: "Dừng workload", exact: true }).click();
    await expect.poll(async () => (await (await request.get("/api/aiwm/jobs/" + jobID)).json()).data.status).toBe("STOPPED");
    for (const route of ["/", "/gpus", "/containers", "/workloads", "/queue", "/scheduler"]) {
      await page.goto(route);
      await expect(page.locator("h1")).toBeVisible();
    }
    await page.setViewportSize({ width: 1024, height: 768 });
    await page.goto("/servers");
    await expect(page.getByRole("link", { name: "fake-server-a100", exact: true })).toBeVisible();
    const after = await (await request.get("/api/aiwm/containers?origin=LEGACY")).json();
    expect(after.data).toEqual(before.data);
    expect(errors).toEqual([]);
  } finally {
    if (jobID) await request.post("/api/aiwm/jobs/" + jobID + "/stop", { data: {} });
  }
});

test("BFF rejects Agent routes and cross-origin mutation", async ({ request }) => {
  expect((await request.get("/api/aiwm/agents/register")).status()).toBe(404);
  expect((await request.post("/api/aiwm/jobs", { headers: { Origin: "https://untrusted.example" }, data: {} })).status()).toBe(403);
});
