// Demonstrates api.path.* and api.time.format. Path semantics are POSIX
// (forward slashes); time.format takes Unix milliseconds (matching
// api.time.nowMs) plus a strftime-style layout and an optional IANA zone.

api.log("=== api.path.* ===");
api.log("dirname:       ", api.path.dirname("/var/log/sys.log"));
api.log("basename:      ", api.path.basename("/var/log/sys.log"));
api.log("basename strip:", api.path.basename("/var/log/sys.log", ".log"));

api.log("=== api.time.format ===");
const now = api.time.nowMs();
api.log("UTC short: ", api.time.format(now, "%F %T",     "UTC"));
api.log("UTC zone:  ", api.time.format(now, "%FT%T%z",   "UTC"));
api.log("local:     ", api.time.format(now, "%F %T %Z"));
api.log("weekday:   ", api.time.format(now, "%A %B %d",  "UTC"));
