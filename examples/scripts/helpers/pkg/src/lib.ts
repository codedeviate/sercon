// Source-of-truth TS module for the demo-pkg fixture. Selected over
// dist/index.js by the engine's package.json `source` preference.

export const v = "from-source";

export function greet(name: string): string {
  return "hello " + name + " (from src/lib.ts)";
}
