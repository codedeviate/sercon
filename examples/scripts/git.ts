// Demonstrates services.git.* — read-only ops only against a throwaway repo
// (so the demo doesn't pollute the host's current checkout).

const tmp = runtime.env.get("TMPDIR") ?? "/tmp";
const repo = tmp + "/sercon-git-demo-" + Date.now();

// Bootstrap a tiny repo via runText (the escape hatch).
await services.git.runText(["init", "-q", "-b", "main", repo]);
await services.git.runText(["config", "user.email", "demo@example.com"], { cwd: repo });
await services.git.runText(["config", "user.name", "Sercon Demo"], { cwd: repo });
await services.git.runText(["config", "commit.gpgsign", "false"], { cwd: repo });

// Seed two commits so log / diffStat have something to chew on.
await services.exec.shell(`echo hello > ${repo}/README.md`);
await services.git.add("README.md", { cwd: repo });
await services.git.commit("initial commit", { cwd: repo });
await services.exec.shell(`echo line2 >> ${repo}/README.md`);
await services.git.add("README.md", { cwd: repo });
await services.git.commit("add line2", { cwd: repo });

runtime.log("=== branch ===");
const b = await services.git.branch({ cwd: repo });
runtime.log("current:", b.current, "detached:", b.detached);
runtime.log("all branches:", b.all);

runtime.log("");
runtime.log("=== isClean ===");
runtime.log("clean:", await services.git.isClean({ cwd: repo }));

runtime.log("");
runtime.log("=== revParse ===");
runtime.log("HEAD:", (await services.git.revParse("HEAD", { cwd: repo })).slice(0, 12) + "...");

runtime.log("");
runtime.log("=== log (latest 2) ===");
const commits = await services.git.log({ cwd: repo, limit: 2 });
for (const c of commits) {
  runtime.log("  ", c.shortSha, c.subject, "(" + c.author + ")");
}

runtime.log("");
runtime.log("=== diffStat ===");
const stat = await services.git.diffStat({ cwd: repo, revRange: "HEAD~1..HEAD" });
runtime.log(`files=${stat.files} +${stat.insertions} -${stat.deletions}`);

runtime.log("");
runtime.log("=== status (after a fresh edit) ===");
await services.exec.shell(`echo more >> ${repo}/README.md`);
const status = await services.git.status({ cwd: repo });
for (const e of status) {
  runtime.log(`  [${e.indexStatus}${e.workingStatus}] ${e.path}`);
}

runtime.log("");
runtime.log("=== runText (escape hatch) ===");
const cf = await services.git.runText(["config", "user.email"], { cwd: repo });
runtime.log("user.email:", cf.stdout.trim());

// Clean up the demo repo so repeated `make demo` runs don't pile up dirs.
await services.exec.shell(["rm", "-rf", repo]);
