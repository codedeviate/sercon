// Shared network-tolerance helper for the network-dependent demo set.
//
// netSkip(e) returns true for failure signatures that mean "the network or
// endpoint is unusable here" — transport timeouts/deadlines, DNS failures,
// connection refused, TLS errors, connection resets, and the case where a
// proxy returns an HTML error page instead of JSON ("invalid character '<'").
// Demos that hit external hosts wrap their body in a try/catch and, on a
// matching error, log a skip message and exit 0 instead of failing the run;
// any non-matching (i.e. genuine) error is re-thrown so real regressions still
// surface when the network is healthy.
export function netSkip(e: unknown): boolean {
  return /deadline|time(?:d)? ?out|connection refused|no such host|lookup |server misbehaving|nxdomain|no answer|dial |i\/o timeout|tls|handshake|invalid character '<'|unexpected end of|eof|reset by peer|network is unreachable|no route to host/i
    .test(String(e));
}
