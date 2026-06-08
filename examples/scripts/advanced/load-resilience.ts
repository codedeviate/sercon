// Advanced example — resilience / load self-test.
//
// Drives a service at rising concurrency and reports latency percentiles,
// error rate, and throughput per level, then asserts the service stayed
// healthy and is still responsive after the burst. This is the pattern for
// testing YOUR OWN service's resilience to request floods.
//
// Authorized, defensive use only: as shipped this starts a server it controls
// on loopback and load-tests that, so it's safe to run anywhere (and in CI).
// To test a real endpoint you operate (or are authorized to test), point
// TARGET at its URL and delete the `server.http.listen` block + `srv.close()`.
// Do not point it at systems you don't own or aren't allowed to test.

const PORT = 38090;
const TARGET = `http://127.0.0.1:${PORT}/work`;

// ---- target under test (remove this block when using a real endpoint) ----
const srv = await server.http.listen({
  port: PORT,
  routes: {
    // A little CPU per request so latencies are measurable.
    "GET /work": (_req: any, res: any) => {
      let x = 0;
      for (let i = 0; i < 5000; i++) x += i;
      res.json({ ok: true, x });
    },
  },
});
runtime.log("target up at", srv.address);

// ---- load driver ----------------------------------------------------------
interface Sample {
  ms: number;
  ok: boolean;
}

async function fireOnce(): Promise<Sample> {
  const t0 = runtime.time.nowMs();
  try {
    const r = await net.http.get(TARGET);
    return { ms: runtime.time.nowMs() - t0, ok: r.status === 200 };
  } catch {
    return { ms: runtime.time.nowMs() - t0, ok: false };
  }
}

// Run `total` requests with at most `concurrency` in flight (worker pool).
async function runLevel(concurrency: number, total: number): Promise<Sample[]> {
  const samples: Sample[] = [];
  let launched = 0;
  const worker = async () => {
    while (launched < total) {
      launched++;
      samples.push(await fireOnce());
    }
  };
  await Promise.all(Array.from({ length: concurrency }, () => worker()));
  return samples;
}

function pct(sorted: number[], p: number): number {
  if (sorted.length === 0) return 0;
  return sorted[Math.min(sorted.length - 1, Math.floor((p / 100) * sorted.length))];
}

function report(level: number, samples: Sample[], wallMs: number): number {
  const lat = samples.map((s) => s.ms).sort((a, b) => a - b);
  const errors = samples.filter((s) => !s.ok).length;
  const rps = wallMs > 0 ? Math.round((samples.length / wallMs) * 1000) : 0;
  runtime.log(
    `concurrency ${String(level).padStart(3)}  n=${samples.length}  ` +
      `errors=${errors}  p50=${pct(lat, 50)}ms  p95=${pct(lat, 95)}ms  ` +
      `max=${lat[lat.length - 1]}ms  ~${rps} req/s`,
  );
  return errors;
}

// ---- ramp ------------------------------------------------------------------
const LEVELS = [4, 16, 32];
const PER_LEVEL = 60;
let totalErrors = 0;

runtime.log("ramping load against", TARGET);
for (const level of LEVELS) {
  const t0 = runtime.time.nowMs();
  const samples = await runLevel(level, PER_LEVEL);
  totalErrors += report(level, samples, runtime.time.nowMs() - t0);
}

// ---- resilience assertions -------------------------------------------------
runtime.assert.equal(totalErrors, 0, "no request errors under load");
const after = await net.http.get(TARGET); // still answering after the burst?
runtime.assert.equal(after.status, 200, "service healthy after burst");
runtime.log("resilience check passed: 0 errors, service healthy post-load");

await srv.close();
runtime.log("done");
