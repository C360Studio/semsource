const { defineConfig, devices } = require("@playwright/test");

const baseURL = process.env.UI_PROFILE_BASE_URL || "http://127.0.0.1:3000";

module.exports = defineConfig({
  testDir: __dirname,
  timeout: 300_000,
  expect: {
    timeout: 10_000,
  },
  fullyParallel: false,
  workers: 1,
  outputDir: "./test-results",
  reporter: process.env.CI ? "github" : "list",
  use: {
    baseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
    video: "retain-on-failure",
  },
  projects: [
    {
      name: "chromium",
      use: { ...devices["Desktop Chrome"] },
    },
    {
      name: "narrow-chromium",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 390, height: 844 },
      },
    },
  ],
});
