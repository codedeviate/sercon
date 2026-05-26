// Demonstrates api.jq.* — query / queryAll over a JS-side data structure.
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

api.log("first user name:    ", await api.jq.query(data, ".users[0].name"));
api.log("meta source:        ", await api.jq.query(data, ".meta.source"));
api.log("all names:          ", await api.jq.queryAll(data, ".users[].name"));
api.log("admin names:        ", await api.jq.queryAll(data,
  ".users[] | select(.admin) | .name"));
api.log("ages sum (sync):    ", await api.jq.query(data, "[.users[].age] | add"));
api.log("group by admin:     ", await api.jq.query(data,
  "[.users[] | {key: (.admin | tostring), value: .name}] | group_by(.key)"));

// Optional access returns null on missing paths instead of throwing.
api.log("missing (optional): ", await api.jq.query(data, ".does.not.exist?"));

// Parse errors throw — wrap in try/catch if your filter is user-supplied.
try {
  await api.jq.query(data, "this is not valid jq");
} catch (e) {
  api.log("parse error caught: ", String(e).slice(0, 80) + "…");
}
