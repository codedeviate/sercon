// Demonstrates api.ai.* — run a one-shot prompt through an AI CLI
// (claude / codex / copilot / gemini). Gracefully degrades when none
// is on PATH, so it stays green in `make demo`.

const providers = api.ai.providers();
api.log("detected providers:", providers.length ? providers.join(", ") : "(none on PATH)");

if (providers.length === 0) {
  api.log("install one of claude / codex / copilot / gemini to see it live");
} else {
  api.log("");
  api.log("sending a prompt via", providers[0], "...");
  try {
    const r = await api.ai.send({
      system: "Answer in exactly one word.",
      prompt: "What is the capital of France?",
      timeout: 60000,
    });
    api.log(`[${r.provider}] exit ${r.exitCode}:`, r.output.slice(0, 200));
  } catch (e) {
    api.log("send failed:", String(e).slice(0, 80));
  }
}
