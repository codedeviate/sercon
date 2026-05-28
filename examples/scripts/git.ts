// Demonstrates api.tools.git.* — read-only ops only against a throwaway repo
// (so the demo doesn't pollute the host's current checkout).

const tmp = api.runtime.env.get("TMPDIR") ?? "/tmp";
const repo = tmp + "/sercon-git-demo-" + Date.now();

// Bootstrap a tiny repo via runText (the escape hatch).
await api.tools.git.runText(["init", "-q", "-b", "main", repo]);
await api.tools.git.runText(["config", "user.email", "demo@example.com"], { cwd: repo });
await api.tools.git.runText(["config", "user.name", "Sercon Demo"], { cwd: repo });
await api.tools.git.runText(["config", "commit.gpgsign", "false"], { cwd: repo });

// Seed two commits so log / diffStat have something to chew on.
await api.tools.exec.shell(`echo hello > ${repo}/README.md`);
await api.tools.git.add("README.md", { cwd: repo });
await api.tools.git.commit("initial commit", { cwd: repo });
await api.tools.exec.shell(`echo line2 >> ${repo}/README.md`);
await api.tools.git.add("README.md", { cwd: repo });
await api.tools.git.commit("add line2", { cwd: repo });

api.runtime.log("=== branch ===");
const b = await api.tools.git.branch({ cwd: repo });
api.runtime.log("current:", b.current, "detached:", b.detached);
api.runtime.log("all branches:", b.all);

api.runtime.log("");
api.runtime.log("=== isClean ===");
api.runtime.log("clean:", await api.tools.git.isClean({ cwd: repo }));

api.runtime.log("");
api.runtime.log("=== revParse ===");
api.runtime.log("HEAD:", (await api.tools.git.revParse("HEAD", { cwd: repo })).slice(0, 12) + "...");

api.runtime.log("");
api.runtime.log("=== log (latest 2) ===");
const commits = await api.tools.git.log({ cwd: repo, limit: 2 });
for (const c of commits) {
  api.runtime.log("  ", c.shortSha, c.subject, "(" + c.author + ")");
}

api.runtime.log("");
api.runtime.log("=== diffStat ===");
const stat = await api.tools.git.diffStat({ cwd: repo, revRange: "HEAD~1..HEAD" });
api.runtime.log(`files=${stat.files} +${stat.insertions} -${stat.deletions}`);

api.runtime.log("");
api.runtime.log("=== status (after a fresh edit) ===");
await api.tools.exec.shell(`echo more >> ${repo}/README.md`);
const status = await api.tools.git.status({ cwd: repo });
for (const e of status) {
  api.runtime.log(`  [${e.indexStatus}${e.workingStatus}] ${e.path}`);
}

api.runtime.log("");
api.runtime.log("=== runText (escape hatch) ===");
const cf = await api.tools.git.runText(["config", "user.email"], { cwd: repo });
api.runtime.log("user.email:", cf.stdout.trim());

// Clean up the demo repo so repeated `make demo` runs don't pile up dirs.
await api.tools.exec.shell(["rm", "-rf", repo]);
