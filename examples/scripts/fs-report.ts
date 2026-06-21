// Build an illustrated step-by-step run report, exercising the fs.* file API
// (writeText / writeBytes / readText / readBytes / mkdir / exists / remove /
// stat). Captures a real screenshot per step when a WebDriver is available;
// otherwise records the step without an image. Self-contained: writes a report
// folder under the CWD and cleans it up at the end.

const dir = "fs-report-demo";
await fs.remove(dir); // clean slate (no error if absent)
await fs.mkdir(dir);

const driver = services.webdriver.available
  ? await services.webdriver.connect({ browser: "chrome", headless: true })
  : null;

let srv: any = null;
if (driver) {
  srv = await server.http.listen({
    port: 38260,
    routes: {
      "GET /": (q: any, r: any) =>
        r.html(`<h1 id="t">Step demo</h1>` +
          `<button id="b" onclick="document.getElementById('t').textContent='clicked'">Go</button>`),
    },
  });
  await driver.get("http://127.0.0.1:38260/");
}

const steps: { label: string; act?: () => Promise<void> }[] = [
  { label: "Open the page" },
  { label: "Click the button", act: async () => { if (driver) await driver.clickWhenReady("id", "b", { timeout: 3000 }); } },
];

const rows: string[] = [];
let n = 0;
for (const step of steps) {
  if (step.act) await step.act();
  n++;
  let img: string;
  if (driver) {
    const shot: any = await driver.screenshot();            // { bytes, format:"png" }
    const file = `step-${String(n).padStart(2, "0")}.png`;
    await fs.writeBytes(`${dir}/${file}`, new Uint8Array(shot.bytes));
    img = `<img src="${file}" alt="${step.label}">`;
  } else {
    img = `<em>(no driver — image skipped)</em>`;
  }
  rows.push(`<figure><figcaption>${n}. ${step.label}</figcaption>${img}</figure>`);
}

const html = `<!doctype html><meta charset="utf-8"><title>Run report</title>
<style>body{font:15px system-ui;margin:2rem;max-width:60rem}figure{margin:0 0 2rem}` +
  `img{max-width:100%;border:1px solid #ccc;border-radius:4px}figcaption{font-weight:600;margin:.4rem 0}</style>
<h1>Run report</h1>
${rows.join("\n")}`;

const idx = `${dir}/index.html`;
const w: any = await fs.writeText(idx, html);
runtime.log("wrote", w.path, `(${w.bytes} bytes)`);

// Verify with the read side of the API.
runtime.assert.ok(await fs.exists(idx), "report exists");
const st: any = await fs.stat(idx);
runtime.assert.ok(st.size > 0 && !st.isDir, "report is a non-empty file");
const back = await fs.readText(idx);
runtime.assert.ok(back.includes("Run report"), "report round-trips");
if (driver) {
  const bytes: Uint8Array = await fs.readBytes(`${dir}/step-01.png`);
  runtime.assert.ok(bytes.length > 0, "screenshot bytes round-trip");
}

runtime.log(`fs-report OK -> ${idx} (open it in a browser)`);

// Clean up so `make demo` leaves no artifacts. Delete this line to keep the report.
await fs.remove(dir);

if (driver) { await driver.quit(); await srv.close(); }
