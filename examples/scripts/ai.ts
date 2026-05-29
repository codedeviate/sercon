// Demonstrates services.ai.* — run a one-shot prompt through an AI CLI
// (claude / codex / copilot / gemini). Gracefully degrades when none
// is on PATH, so it stays green in `make demo`.

const providers = services.ai.providers();
runtime.log("detected providers:", providers.length ? providers.join(", ") : "(none on PATH)");

if (providers.length === 0) {
  runtime.log("install one of claude / codex / copilot / gemini to see it live");
} else {
  runtime.log("");
  runtime.log("sending a prompt via", providers[0], "...");
  try {
    const r = await services.ai.send({
      system: "Answer in exactly one word.",
      prompt: "What is the capital of France?",
      timeout: 60000,
    });
    runtime.log(`[${r.provider}] exit ${r.exitCode}:`, r.output.slice(0, 200));
  } catch (e) {
    runtime.log("send failed:", String(e).slice(0, 80));
  }
}
