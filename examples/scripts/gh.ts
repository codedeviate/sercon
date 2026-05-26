// Demonstrates api.gh.* — GitHub CLI wrapper. Gracefully degrades when
// `gh` is missing or unauthenticated so this script always passes
// (essential for `make demo` on hosts without a gh login).

api.log("=== authStatus ===");
const auth = await api.gh.authStatus();
api.log("authenticated:", auth.authenticated);
api.log("user:", auth.user || "(none)");
if (!auth.authenticated) {
  api.log("raw:", auth.raw.slice(0, 160));
  api.log("");
  api.log("(skipping prList / repoView — gh is not authenticated)");
} else {
  api.log("");
  api.log("=== repoView (public repo) ===");
  // Hit a known-public repo so the demo works regardless of which
  // repo the host happens to be sitting in.
  const repo = await api.gh.repoView("cli/cli");
  api.log("name:    ", repo.owner + "/" + repo.name);
  api.log("default: ", repo.defaultBranch);
  api.log("visible: ", repo.visibility);
  api.log("desc:    ", (repo.description as string).slice(0, 80));

  api.log("");
  api.log("=== prList (last 3 open in cli/cli) ===");
  // gh's --repo flag scopes a single call without affecting global state.
  // We don't expose --repo on api.gh.prList yet (opts.cwd is the only
  // scoping lever), so use runText-style escape via api.exec.shell to
  // showcase the simpler binding form against the host's default repo.
  const prs = await api.gh.prList({ limit: 3 }).catch((e: unknown) => ({
    error: String(e).slice(0, 100),
  }));
  if ("error" in (prs as Record<string, unknown>)) {
    api.log("(prList errored — likely no repo context here)");
    api.log("error:", (prs as { error: string }).error);
  } else {
    const list = prs as Array<{ number: number; title: string; author: string; state: string }>;
    if (list.length === 0) {
      api.log("(no PRs returned from this directory)");
    } else {
      for (const pr of list) {
        api.log(`  #${pr.number} ${pr.state} ${pr.title.slice(0, 60)} (by ${pr.author})`);
      }
    }
  }
}
