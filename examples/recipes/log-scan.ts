// Recipe: scan server.log and tally HTTP status codes + collect client IPs.
const dir = fs.path.dirname(runtime.argv[1]);
const data = (n: string) => `${dir}/../data/${n}`; // fs.path has no join(); concat (OS resolves "..")
const text = await fs.readText(data("server.log"));
const lines = text.split("\n").filter((l) => l.trim());
const status: Record<string, number> = {};
const ips = new Set<string>();
for (const line of lines) {
  const m = line.match(/^\S+\s+(\S+)\s+\S+\s+\S+\s+(\d{3})/);
  if (!m) continue;
  ips.add(m[1]);
  status[m[2]] = (status[m[2]] ?? 0) + 1;
}
runtime.assert.ok((status["200"] ?? 0) > 0, "saw some 200s");
runtime.log("log-scan:", lines.length, "lines,", ips.size, "client IPs, status tally", JSON.stringify(status));
