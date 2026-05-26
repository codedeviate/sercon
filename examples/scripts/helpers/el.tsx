/** @jsx h */

// TSX fixture. The pragma on the very first line tells esbuild to compile
// `<div>` etc. into `h(...)` calls instead of the default
// `React.createElement`, so no React runtime needs to be in scope — we
// just define h ourselves below. Avoid putting that pragma token anywhere
// else in the file; esbuild scans every comment for it and the last one
// wins.

function h(tag: string, props: any, ...children: any[]) {
  return { tag, props: props ?? {}, children };
}

export function makeBox(label: string) {
  return <div className="box">{label}</div>;
}
