// Demonstrates api.tools.gh.* — GitHub CLI wrapper. Gracefully degrades when
// `gh` is missing or unauthenticated so this script always passes
// (essential for `make demo` on hosts without a gh login).

api.runtime.log("=== authStatus ===");
const auth = await api.tools.gh.authStatus();
api.runtime.log("authenticated:", auth.authenticated);
api.runtime.log("user:", auth.user || "(none)");
if (!auth.authenticated) {
  api.runtime.log("raw:", auth.raw.slice(0, 160));
  api.runtime.log("");
  api.runtime.log("(skipping prList / repoView — gh is not authenticated)");
} else {
  api.runtime.log("");
  api.runtime.log("=== repoView (public repo) ===");
  // Hit a known-public repo so the demo works regardless of which
  // repo the host happens to be sitting in.
  const repo = await api.tools.gh.repoView("cli/cli");
  api.runtime.log("name:    ", repo.owner + "/" + repo.name);
  api.runtime.log("default: ", repo.defaultBranch);
  api.runtime.log("visible: ", repo.visibility);
  api.runtime.log("desc:    ", (repo.description as string).slice(0, 80));

  api.runtime.log("");
  api.runtime.log("=== prList (last 3 open in cli/cli) ===");
  // gh's --repo flag scopes a single call without affecting global state.
  // We don't expose --repo on api.tools.gh.prList yet (opts.cwd is the only
  // scoping lever), so use runText-style escape via api.tools.exec.shell to
  // showcase the simpler binding form against the host's default repo.
  const prs = await api.tools.gh.prList({ limit: 3 }).catch((e: unknown) => ({
    error: String(e).slice(0, 100),
  }));
  if ("error" in (prs as Record<string, unknown>)) {
    api.runtime.log("(prList errored — likely no repo context here)");
    api.runtime.log("error:", (prs as { error: string }).error);
  } else {
    const list = prs as Array<{ number: number; title: string; author: string; state: string }>;
    if (list.length === 0) {
      api.runtime.log("(no PRs returned from this directory)");
    } else {
      for (const pr of list) {
        api.runtime.log(`  #${pr.number} ${pr.state} ${pr.title.slice(0, 60)} (by ${pr.author})`);
      }
    }
  }
}
