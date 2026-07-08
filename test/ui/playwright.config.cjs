const { defineConfig, devices } = require("@playwright/test");

const baseURL = process.env.UI_PROFILE_BASE_URL || "http://127.0.0.1:3000";

module.exports = defineConfig({
  testDir: __dirname,
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  workers: 1,
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL,
    trace: "on-first-retry",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
  ],
});
