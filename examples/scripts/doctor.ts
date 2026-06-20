// Demonstrates services.doctor — report on external tool requirements and
// assert the ones a script needs. Offline; deterministic on git (always
// present in CI). Optional tools just show in the report.

const report = await services.doctor();
const installed = report.tools.filter((t: any) => t.installed);
runtime.log(`doctor: ${installed.length}/${report.tools.length} tools installed; ok=${report.ok}`);
for (const t of installed) {
  runtime.log(`  ${t.category}/${t.name} ${t.version ?? ""}${t.detail ? " [" + t.detail + "]" : ""}`);
}

// Assert a prerequisite the host reliably has.
const need = await services.doctor(["git"]);
runtime.assert.ok(need.satisfied, "git requirement satisfied");
runtime.assert.equal(need.unmet.length, 0, "no unmet requirements for git");

// Feature requirements use the same names as the *.available gates; an unmet
// one lands in `unmet` (not an error) so a script can self-skip:
const opt = await services.doctor(["typst", "webdriver"]);
runtime.log("optional features satisfied:", opt.satisfied, "unmet:", JSON.stringify(opt.unmet));

runtime.log("doctor demo OK");
