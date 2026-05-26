// Demonstrates api.git.* — read-only ops only against a throwaway repo
// (so the demo doesn't pollute the host's current checkout).

const tmp = api.env.get("TMPDIR") ?? "/tmp";
const repo = tmp + "/sercon-git-demo-" + Date.now();

// Bootstrap a tiny repo via runText (the escape hatch).
await api.git.runText(["init", "-q", "-b", "main", repo]);
await api.git.runText(["config", "user.email", "demo@example.com"], { cwd: repo });
await api.git.runText(["config", "user.name", "Sercon Demo"], { cwd: repo });
await api.git.runText(["config", "commit.gpgsign", "false"], { cwd: repo });

// Seed two commits so log / diffStat have something to chew on.
await api.exec.shell(`echo hello > ${repo}/README.md`);
await api.git.add("README.md", { cwd: repo });
await api.git.commit("initial commit", { cwd: repo });
await api.exec.shell(`echo line2 >> ${repo}/README.md`);
await api.git.add("README.md", { cwd: repo });
await api.git.commit("add line2", { cwd: repo });

api.log("=== branch ===");
const b = await api.git.branch({ cwd: repo });
api.log("current:", b.current, "detached:", b.detached);
api.log("all branches:", b.all);

api.log("");
api.log("=== isClean ===");
api.log("clean:", await api.git.isClean({ cwd: repo }));

api.log("");
api.log("=== revParse ===");
api.log("HEAD:", (await api.git.revParse("HEAD", { cwd: repo })).slice(0, 12) + "...");

api.log("");
api.log("=== log (latest 2) ===");
const commits = await api.git.log({ cwd: repo, limit: 2 });
for (const c of commits) {
  api.log("  ", c.shortSha, c.subject, "(" + c.author + ")");
}

api.log("");
api.log("=== diffStat ===");
const stat = await api.git.diffStat({ cwd: repo, revRange: "HEAD~1..HEAD" });
api.log(`files=${stat.files} +${stat.insertions} -${stat.deletions}`);

api.log("");
api.log("=== status (after a fresh edit) ===");
await api.exec.shell(`echo more >> ${repo}/README.md`);
const status = await api.git.status({ cwd: repo });
for (const e of status) {
  api.log(`  [${e.indexStatus}${e.workingStatus}] ${e.path}`);
}

api.log("");
api.log("=== runText (escape hatch) ===");
const cf = await api.git.runText(["config", "user.email"], { cwd: repo });
api.log("user.email:", cf.stdout.trim());

// Clean up the demo repo so repeated `make demo` runs don't pile up dirs.
await api.exec.shell(["rm", "-rf", repo]);
