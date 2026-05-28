// Demonstrates api.text.jq.* — query / queryAll over a JS-side data structure.
// gojq is a pure-Go re-implementation of jq; the filter syntax is the same
// you'd type at the shell. Data passes through goja's Export as
// map[string]any / []any so JSON.parse output works directly.

const data = {
  users: [
    { name: "alice", age: 30, admin: true },
    { name: "bob",   age: 25, admin: false },
    { name: "carol", age: 35, admin: true },
  ],
  meta: { count: 3, source: "demo" },
};

api.runtime.log("first user name:    ", await api.text.jq.query(data, ".users[0].name"));
api.runtime.log("meta source:        ", await api.text.jq.query(data, ".meta.source"));
api.runtime.log("all names:          ", await api.text.jq.queryAll(data, ".users[].name"));
api.runtime.log("admin names:        ", await api.text.jq.queryAll(data,
  ".users[] | select(.admin) | .name"));
api.runtime.log("ages sum (sync):    ", await api.text.jq.query(data, "[.users[].age] | add"));
api.runtime.log("group by admin:     ", await api.text.jq.query(data,
  "[.users[] | {key: (.admin | tostring), value: .name}] | group_by(.key)"));

// Optional access returns null on missing paths instead of throwing.
api.runtime.log("missing (optional): ", await api.text.jq.query(data, ".does.not.exist?"));

// Parse errors throw — wrap in try/catch if your filter is user-supplied.
try {
  await api.text.jq.query(data, "this is not valid jq");
} catch (e) {
  api.runtime.log("parse error caught: ", String(e).slice(0, 80) + "…");
}
