// The scale claim: a year of one-second samples (31,536,000 points) drawn as 2,000 points in
// under 400ms, with no spike dropped. Drives the real built console (console/dist) with
// Playwright's own bundled Chromium, headless, launched and closed by this script (never a real
// installed browser, never chromedriver), calling the same lttb()+D3 render path the live app
// uses (see app.component.ts's runFullScaleRenderBenchmark, exposed on window for exactly this).
import { chromium } from "playwright";
import { createServer } from "node:http";
import { readFile, writeFileSync, mkdirSync } from "node:fs";
import { extname, join } from "node:path";
import { fileURLToPath } from "node:url";

const __dirname = fileURLToPath(new URL(".", import.meta.url));
const DIST_DIR = join(__dirname, "..", "console", "dist", "console", "browser");
const DOCS_DIR = join(__dirname, "..", "docs");

const CONTENT_TYPES = { ".html": "text/html", ".js": "text/javascript", ".css": "text/css" };

function startStaticServer() {
  return new Promise((resolve) => {
    const server = createServer((req, res) => {
      let path = req.url.split("?")[0];
      if (path === "/") path = "/index.html";
      readFile(join(DIST_DIR, path), (err, data) => {
        if (err) {
          res.writeHead(404);
          res.end();
          return;
        }
        res.writeHead(200, { "Content-Type": CONTENT_TYPES[extname(path)] ?? "application/octet-stream" });
        res.end(data);
      });
    });
    server.listen(0, "127.0.0.1", () => resolve(server));
  });
}

function percentile(sorted, p) {
  const idx = Math.min(sorted.length - 1, Math.ceil((p / 100) * sorted.length) - 1);
  return sorted[idx];
}

async function main() {
  const totalPoints = 31_536_000; // 365 days x 86,400 seconds/day of 1Hz samples
  const threshold = 2000;
  const spikeIndex = 12_345_678;
  const repeats = Number(process.argv[2] ?? 5);

  const staticServer = await startStaticServer();
  const staticPort = staticServer.address().port;

  const browser = await chromium.launch({ headless: true });
  const page = await browser.newPage();
  page.on("pageerror", (err) => console.log("[pageerror]", err.message));
  await page.goto(`http://127.0.0.1:${staticPort}/`, { waitUntil: "load" });
  await page.waitForFunction(() => typeof window.runFullScaleRenderBenchmark === "function", null, { timeout: 15000 });

  const results = [];
  for (let i = 0; i < repeats; i++) {
    const result = await page.evaluate(
      ({ totalPoints, threshold, spikeIndex }) => window.runFullScaleRenderBenchmark(totalPoints, threshold, spikeIndex),
      { totalPoints, threshold, spikeIndex },
    );
    results.push(result);
  }

  await browser.close();
  staticServer.close();

  const elapsed = results.map((r) => r.elapsedMs).sort((a, b) => a - b);
  const allSpikesSurvived = results.every((r) => r.spikeSurvived);
  const allOutputCorrectSize = results.every((r) => r.outputPoints === threshold);

  const report = {
    totalPoints, threshold, repeats,
    elapsedMs: { p50: percentile(elapsed, 50), min: elapsed[0], max: elapsed[elapsed.length - 1] },
    allSpikesSurvived, allOutputCorrectSize,
    rawResults: results,
  };

  mkdirSync(DOCS_DIR, { recursive: true });
  writeFileSync(join(DOCS_DIR, "render_benchmark_output.json"), JSON.stringify(report, null, 2));
  writeFileSync(join(DOCS_DIR, "render_benchmark_output.txt"),
    `Full-scale time-series render benchmark (LTTB downsample + D3 draw, real headless Chromium)\n` +
    `Series: ${totalPoints.toLocaleString()} points (a year of 1Hz samples) -> ${threshold} points\n` +
    `Repeats: ${repeats}\n` +
    `Elapsed: p50=${report.elapsedMs.p50.toFixed(1)}ms min=${report.elapsedMs.min.toFixed(1)}ms max=${report.elapsedMs.max.toFixed(1)}ms\n` +
    `Claim: under 400ms\n` +
    `Spike at index ${spikeIndex} survived on every repeat: ${allSpikesSurvived}\n` +
    `Output was exactly ${threshold} points on every repeat: ${allOutputCorrectSize}\n`);

  console.log(`p50=${report.elapsedMs.p50.toFixed(1)}ms max=${report.elapsedMs.max.toFixed(1)}ms spikeSurvived=${allSpikesSurvived}`);
  if (report.elapsedMs.max >= 400 || !allSpikesSurvived || !allOutputCorrectSize) {
    process.exitCode = 1;
  }
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
