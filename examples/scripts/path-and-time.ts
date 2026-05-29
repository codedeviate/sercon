// Demonstrates fs.path.* and runtime.time.format. Path semantics are POSIX
// (forward slashes); time.format takes Unix milliseconds (matching
// runtime.time.nowMs) plus a strftime-style layout and an optional IANA zone.

runtime.log("=== fs.path.* ===");
runtime.log("dirname:       ", fs.path.dirname("/var/log/sys.log"));
runtime.log("basename:      ", fs.path.basename("/var/log/sys.log"));
runtime.log("basename strip:", fs.path.basename("/var/log/sys.log", ".log"));

runtime.log("=== runtime.time.format ===");
const now = runtime.time.nowMs();
runtime.log("UTC short: ", runtime.time.format(now, "%F %T",     "UTC"));
runtime.log("UTC zone:  ", runtime.time.format(now, "%FT%T%z",   "UTC"));
runtime.log("local:     ", runtime.time.format(now, "%F %T %Z"));
runtime.log("weekday:   ", runtime.time.format(now, "%A %B %d",  "UTC"));
