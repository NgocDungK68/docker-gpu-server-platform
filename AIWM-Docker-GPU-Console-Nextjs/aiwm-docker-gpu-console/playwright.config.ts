import { defineConfig } from "@playwright/test";

export default defineConfig({
  testDir: "./tests/e2e",
  workers: 1,
  timeout: 90_000,
  expect: { timeout: 45_000 },
  use: {
    baseURL: process.env.AIWM_CONSOLE_URL ?? "http://127.0.0.1:3000",
    channel: process.env.AIWM_BROWSER_CHANNEL || undefined,
    viewport: { width: 1440, height: 1000 },
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
});
