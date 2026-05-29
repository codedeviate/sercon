// Demonstrates services.gh.* — GitHub CLI wrapper. Gracefully degrades when
// `gh` is missing or unauthenticated so this script always passes
// (essential for `make demo` on hosts without a gh login).

runtime.log("=== authStatus ===");
const auth = await services.gh.authStatus();
runtime.log("authenticated:", auth.authenticated);
runtime.log("user:", auth.user || "(none)");
if (!auth.authenticated) {
  runtime.log("raw:", auth.raw.slice(0, 160));
  runtime.log("");
  runtime.log("(skipping prList / repoView — gh is not authenticated)");
} else {
  runtime.log("");
  runtime.log("=== repoView (public repo) ===");
  // Hit a known-public repo so the demo works regardless of which
  // repo the host happens to be sitting in.
  const repo = await services.gh.repoView("cli/cli");
  runtime.log("name:    ", repo.owner + "/" + repo.name);
  runtime.log("default: ", repo.defaultBranch);
  runtime.log("visible: ", repo.visibility);
  runtime.log("desc:    ", (repo.description as string).slice(0, 80));

  runtime.log("");
  runtime.log("=== prList (last 3 open in cli/cli) ===");
  // gh's --repo flag scopes a single call without affecting global state.
  // We don't expose --repo on services.gh.prList yet (opts.cwd is the only
  // scoping lever), so use runText-style escape via services.exec.shell to
  // showcase the simpler binding form against the host's default repo.
  const prs = await services.gh.prList({ limit: 3 }).catch((e: unknown) => ({
    error: String(e).slice(0, 100),
  }));
  if ("error" in (prs as Record<string, unknown>)) {
    runtime.log("(prList errored — likely no repo context here)");
    runtime.log("error:", (prs as { error: string }).error);
  } else {
    const list = prs as Array<{ number: number; title: string; author: string; state: string }>;
    if (list.length === 0) {
      runtime.log("(no PRs returned from this directory)");
    } else {
      for (const pr of list) {
        runtime.log(`  #${pr.number} ${pr.state} ${pr.title.slice(0, 60)} (by ${pr.author})`);
      }
    }
  }
}
