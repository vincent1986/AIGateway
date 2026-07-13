/**
 * Create first AIGateway wiki page via Chrome DevTools Protocol.
 * Usage: node scripts/create-wiki-home.mjs
 */
import { chromium } from "playwright-core";
import fs from "fs";
import path from "path";
import { fileURLToPath } from "url";

const __dirname = path.dirname(fileURLToPath(import.meta.url));
const root = path.resolve(__dirname, "..");
const homeMd = fs.readFileSync(path.join(root, "docs/wiki/Home.md"), "utf8");

const chromePath =
  "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome";
const userDataDir = path.join(
  process.env.HOME,
  "Library/Application Support/Google/Chrome",
);
// Prefer Profile 1 if exists, else Default
const profile = fs.existsSync(path.join(userDataDir, "Profile 1"))
  ? "Profile 1"
  : "Default";

async function main() {
  // Connect over CDP: start chrome with debugging if needed
  const port = 9333;
  let browser;
  try {
    browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
    console.log("Connected to existing Chrome CDP on", port);
  } catch {
    console.log("Launching Chrome with remote debugging...");
    // Launch detached chrome - may fail if profile locked
    const { spawn } = await import("child_process");
    const tmpProfile = `/tmp/chrome-wiki-profile-${Date.now()}`;
    fs.mkdirSync(tmpProfile, { recursive: true });
    const child = spawn(
      chromePath,
      [
        `--remote-debugging-port=${port}`,
        `--user-data-dir=${tmpProfile}`,
        "--no-first-run",
        "--no-default-browser-check",
        "https://github.com/login",
      ],
      { detached: true, stdio: "ignore" },
    );
    child.unref();
    // Wait for CDP
    for (let i = 0; i < 30; i++) {
      await new Promise((r) => setTimeout(r, 500));
      try {
        browser = await chromium.connectOverCDP(`http://127.0.0.1:${port}`);
        break;
      } catch {
        /* retry */
      }
    }
    if (!browser) throw new Error("Could not connect to Chrome CDP");
    console.log(
      "Launched fresh Chrome profile. If not logged in, wiki create will fail.",
    );
  }

  const context = browser.contexts()[0] || (await browser.newContext());
  const page = await context.newPage();
  await page.goto("https://github.com/vincent1986/AIGateway/wiki/_new", {
    waitUntil: "domcontentloaded",
    timeout: 60000,
  });
  await page.waitForTimeout(2000);
  console.log("URL:", page.url());
  console.log("Title:", await page.title());

  // Detect login
  if (page.url().includes("/login")) {
    console.error("NOT_LOGGED_IN: Please log into GitHub in that Chrome window, then re-run.");
    process.exit(2);
  }

  // Fill form - try multiple selectors used by Gollum / new UI
  const nameSelectors = [
    'input[name="wiki[name]"]',
    "#wiki_name",
    "#gollum-editor-page-name",
    'input[name="wiki_title"]',
    'input[aria-label*="Page name" i]',
    'input[placeholder*="Name" i]',
  ];
  const bodySelectors = [
    'textarea[name="wiki[body]"]',
    "#wiki_body",
    "#gollum-editor-body",
    'textarea[name="wiki_contents"]',
    "textarea",
  ];

  let nameEl = null;
  for (const s of nameSelectors) {
    const el = page.locator(s).first();
    if (await el.count()) {
      nameEl = el;
      console.log("name selector", s);
      break;
    }
  }
  let bodyEl = null;
  for (const s of bodySelectors) {
    const el = page.locator(s).first();
    if (await el.count()) {
      bodyEl = el;
      console.log("body selector", s);
      break;
    }
  }

  if (!nameEl || !bodyEl) {
    const html = await page.content();
    fs.writeFileSync("/tmp/wiki-new-page.html", html);
    console.error("FORM_NOT_FOUND; dumped /tmp/wiki-new-page.html");
    // print visible text
    console.log((await page.locator("body").innerText()).slice(0, 1000));
    process.exit(3);
  }

  await nameEl.fill("Home");
  await bodyEl.fill(homeMd);

  // Submit
  const submit = page
    .locator(
      'button[type="submit"], input[type="submit"], button:has-text("Save Page"), button:has-text("Save"), button:has-text("Create")',
    )
    .first();
  if (await submit.count()) {
    await submit.click();
  } else {
    await bodyEl.press("Meta+Enter");
  }

  await page.waitForTimeout(4000);
  console.log("After save URL:", page.url());
  console.log("After save title:", await page.title());
  // keep browser if connected; if we launched temp, leave it
  process.exit(0);
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
