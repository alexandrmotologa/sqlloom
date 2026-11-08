const fs = require('fs');
const path = require('path');
const puppeteer = require('B:/workgit/node_modules/puppeteer-core');
const { execSync } = require('child_process');

const OUTPUT_DIR = path.resolve(__dirname, '../docs/images');
const FRAMES_DIR = path.resolve(__dirname, '../frames');
const BASE_URL = 'http://127.0.0.1:8089';

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function waitForServer(url, timeoutMs = 15000) {
  const start = Date.now();
  while (Date.now() - start < timeoutMs) {
    try {
      const res = await fetch(`${url}/healthz`);
      if (res.ok) return true;
    } catch {
      // Retry
    }
    await sleep(200);
  }
  throw new Error(`Server at ${url} did not become ready in ${timeoutMs}ms`);
}

async function clickByText(page, selector, text) {
  const elements = await page.$$(selector);
  for (const el of elements) {
    const content = await page.evaluate((node) => node.textContent, el);
    if (content && content.includes(text)) {
      await el.click();
      return true;
    }
  }
  return false;
}

async function run() {
  if (!fs.existsSync(OUTPUT_DIR)) fs.mkdirSync(OUTPUT_DIR, { recursive: true });
  if (!fs.existsSync(FRAMES_DIR)) fs.mkdirSync(FRAMES_DIR, { recursive: true });

  console.log('Waiting for sqlloom dashboard...');
  await waitForServer(BASE_URL);
  console.log('sqlloom dashboard is reachable!');

  console.log('Launching Chrome via puppeteer-core...');
  const browser = await puppeteer.launch({
    executablePath: 'C:/Program Files/Google/Chrome/Application/chrome.exe',
    headless: 'new',
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--window-size=1440,900'],
  });

  const page = await browser.newPage();
  page.on('console', (msg) => console.log('BROWSER LOG:', msg.text()));
  page.on('pageerror', (err) => console.error('BROWSER ERROR:', err.message));

  // 1. Desktop Dashboard Screenshot (1440x900)
  console.log('1. Capturing Desktop Dashboard (sqlloom_dashboard.png)...');
  await page.setViewport({ width: 1440, height: 900, deviceScaleFactor: 2 });
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await sleep(1500);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_dashboard.png') });
  console.log('✓ sqlloom_dashboard.png captured');

  // 2. Table Dependency Map (1440x900)
  console.log('2. Capturing Table Map (sqlloom_table_map.png)...');
  await clickByText(page, 'button', 'Table Map');
  await sleep(800);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_table_map.png') });
  console.log('✓ sqlloom_table_map.png captured');

  // 3. Rules & Policies (1440x900)
  console.log('3. Capturing Rules & Policies (sqlloom_rules.png)...');
  await clickByText(page, 'button', 'Rules & Policies');
  await sleep(800);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_rules.png') });
  console.log('✓ sqlloom_rules.png captured');

  // 4. Invalidations & Churn (1440x900)
  console.log('4. Capturing Invalidations & Churn (sqlloom_invalidations.png)...');
  await clickByText(page, 'button', 'Invalidations & Churn');
  await sleep(800);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_invalidations.png') });
  console.log('✓ sqlloom_invalidations.png captured');

  // 5. Smoke Benchmark Modal (1440x900)
  console.log('5. Capturing Smoke Benchmark Modal (sqlloom_benchmark.png)...');
  await clickByText(page, 'button', 'Smoke Benchmark');
  await sleep(800);
  // Click Run Smoke Benchmark
  await clickByText(page, 'button', 'Run Smoke Benchmark');
  await sleep(2500); // Wait for benchmark to complete
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_benchmark.png') });
  console.log('✓ sqlloom_benchmark.png captured');

  // Close modal
  const closeBtn = await page.$("button[aria-label='Close'], .fixed button");
  if (closeBtn) await closeBtn.click();
  await sleep(500);

  // 6. Tablet Responsive View (768x1024)
  console.log('6. Capturing Tablet View (sqlloom_tablet.png)...');
  await page.setViewport({ width: 768, height: 1024, deviceScaleFactor: 2 });
  await clickByText(page, 'button', 'Query Stream');
  await sleep(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_tablet.png') });
  console.log('✓ sqlloom_tablet.png captured');

  // 7. Mobile Responsive View (390x844 - iPhone)
  console.log('7. Capturing Mobile View (sqlloom_mobile.png)...');
  await page.setViewport({ width: 390, height: 844, isMobile: true, deviceScaleFactor: 2 });
  await sleep(1000);
  await page.screenshot({ path: path.join(OUTPUT_DIR, 'sqlloom_mobile.png') });
  console.log('✓ sqlloom_mobile.png captured');

  // 8. Interactive Demo GIF (960x600)
  console.log('8. Recording frames for sqlloom_demo.gif...');
  await page.setViewport({ width: 1024, height: 640, deviceScaleFactor: 1 });
  await page.goto(BASE_URL, { waitUntil: 'domcontentloaded' });
  await sleep(1500);

  let frameIdx = 0;
  const captureFrame = async (count = 1) => {
    for (let i = 0; i < count; i++) {
      const p = path.join(FRAMES_DIR, `frame_${String(frameIdx).padStart(3, '0')}.png`);
      await page.screenshot({ path: p });
      frameIdx++;
      await sleep(100);
    }
  };

  // 1. Initial live stream pulses
  await captureFrame(10);

  // 2. Click Table Map
  await clickByText(page, 'button', 'Table Map');
  await sleep(300);
  await captureFrame(8);

  // 3. Click Rules
  await clickByText(page, 'button', 'Rules & Policies');
  await sleep(300);
  await captureFrame(8);

  // 4. Click Invalidations
  await clickByText(page, 'button', 'Invalidations & Churn');
  await sleep(300);
  await captureFrame(8);

  // 5. Back to Stream & Open Benchmark
  await clickByText(page, 'button', 'Query Stream');
  await sleep(300);
  await captureFrame(5);

  await clickByText(page, 'button', 'Smoke Benchmark');
  await sleep(400);
  await captureFrame(6);

  // Click Run in benchmark
  await clickByText(page, 'button', 'Run Smoke Benchmark');
  await captureFrame(18);

  // Close benchmark modal
  const modalClose = await page.$("button[aria-label='Close'], .fixed button");
  if (modalClose) await modalClose.click();
  await sleep(300);
  await captureFrame(8);

  // Select a query row to inspect QueryDetail
  const queryRow = await page.$('.divide-y > div');
  if (queryRow) {
    await queryRow.click();
    await sleep(400);
    await captureFrame(12);
  }

  await browser.close();
  console.log(`✓ Captured ${frameIdx} frames. Assembling GIF with ffmpeg...`);

  const gifPath = path.join(OUTPUT_DIR, 'sqlloom_demo.gif');
  const ffmpegCmd = `ffmpeg -y -framerate 10 -i "${path.join(FRAMES_DIR, 'frame_%03d.png')}" -vf "fps=10,scale=960:-1:flags=lanczos,split[s0][s1];[s0]palettegen=max_colors=128[p];[s1][p]paletteuse=dither=bayer" "${gifPath}"`;
  execSync(ffmpegCmd, { stdio: 'inherit' });

  // Clean frames
  fs.rmSync(FRAMES_DIR, { recursive: true, force: true });
  console.log('✓ docs/images/sqlloom_demo.gif generated successfully!');
}

run().catch((err) => {
  console.error('Asset capture failed:', err);
  process.exit(1);
});
