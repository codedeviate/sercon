// Demonstrates api.fs.path.* and api.runtime.time.format. Path semantics are POSIX
// (forward slashes); time.format takes Unix milliseconds (matching
// api.runtime.time.nowMs) plus a strftime-style layout and an optional IANA zone.

api.runtime.log("=== api.fs.path.* ===");
api.runtime.log("dirname:       ", api.fs.path.dirname("/var/log/sys.log"));
api.runtime.log("basename:      ", api.fs.path.basename("/var/log/sys.log"));
api.runtime.log("basename strip:", api.fs.path.basename("/var/log/sys.log", ".log"));

api.runtime.log("=== api.runtime.time.format ===");
const now = api.runtime.time.nowMs();
api.runtime.log("UTC short: ", api.runtime.time.format(now, "%F %T",     "UTC"));
api.runtime.log("UTC zone:  ", api.runtime.time.format(now, "%FT%T%z",   "UTC"));
api.runtime.log("local:     ", api.runtime.time.format(now, "%F %T %Z"));
api.runtime.log("weekday:   ", api.runtime.time.format(now, "%A %B %d",  "UTC"));
