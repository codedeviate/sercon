package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

func serverDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"http.listen": {
			Summary: "Bind an HTTP listener: server.http.listen({port, host?, routes, use?}) → handle with .address, .close(), .stopped Promise. routes is a map of stdlib http.ServeMux patterns ('GET /users/{id}') to handlers (req, res) => res.json({...}) or {use: [...], handler: fn} for per-route middleware. Handlers can call res.upgradeWebSocket(opts?) to hijack the connection and return an AsyncIterable<WSMessage> with .send / .close — `for await (const msg of ws)` walks frames; msg is {type:'text',text} or {type:'binary',bytes:Uint8Array}.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; routes: Record<string, ((req: Request, res: Response) => unknown) | { use?: ((req: Request, res: Response, next: () => Promise<void>) => unknown)[]; handler: (req: Request, res: Response) => unknown }>; use?: ((req: Request, res: Response, next: () => Promise<void>) => unknown)[] }", Desc: "Listener config. port is required; host defaults to \"0.0.0.0\". routes maps Go 1.22+ ServeMux patterns ('GET /', 'POST /users/{id}', 'GET /assets/{rest...}') to a handler function or a {use, handler} object for per-route middleware. use is a global middleware chain run before every route. Under `sercon serve`, --port-override replaces port."},
			},
			ReturnType: "{ address: string; stopped: Promise<void>; close(): Promise<void> }",
			Returns:    "A server handle (returned synchronously): address is 'tcp/host:port' (resolved, so a port:0 ephemeral bind reports its OS-chosen port); stopped resolves when the server stops (rejects if Serve fails with a non-close error); close() begins a graceful 30s shutdown and resolves with the same stopped Promise.",
			Errors:     "Throws synchronously if opts is missing, port is 0/absent, routes is missing, a use[] entry or route value is not a function/valid {use, handler}, or the bind fails (e.g. address already in use).",
			Example: `const srv = server.http.listen({
  port: 8080,
  routes: { "GET /": (req, res) => res.json({ ok: true }) },
});
runtime.log(srv.address);
await srv.close();`,
		},
		"https.listen": {
			Summary: "Like server.http.listen plus required cert/key (file paths OR inline PEM strings). No autocert; no self-signed magic.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; cert: string; key: string; routes: Record<string, ((req: Request, res: Response) => unknown) | { use?: ((req: Request, res: Response, next: () => Promise<void>) => unknown)[]; handler: (req: Request, res: Response) => unknown }>; use?: ((req: Request, res: Response, next: () => Promise<void>) => unknown)[] }", Desc: "Same shape as server.http.listen plus cert and key. Each is either a filesystem path or an inline PEM string (detected by a leading '-----BEGIN'). TLS is pinned to a minimum of TLS 1.2."},
			},
			ReturnType: "{ address: string; stopped: Promise<void>; close(): Promise<void> }",
			Returns:    "Same handle shape as server.http.listen; address is 'tcp/host:port'.",
			Errors:     "Throws synchronously on the same conditions as server.http.listen, plus if cert/key are missing or the key pair fails to load/parse.",
			Example: `const srv = server.https.listen({
  port: 8443,
  cert: "/etc/ssl/cert.pem",
  key: "/etc/ssl/key.pem",
  routes: { "GET /": (req, res) => res.text("secure") },
});`,
		},
		"http.static": {
			Summary: "Static-file mount: server.http.static({dir, stripPrefix, index?, etag?}) → handler. Assign to a wildcard route (GET /assets/{rest...}). Internally stdlib http.FileServer with stripPrefix; ETag/Last-Modified/range requests work; no directory listing.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ dir: string; stripPrefix?: string; index?: string; etag?: boolean }", Desc: "dir is the filesystem root to serve. stripPrefix is removed from the request path before lookup (set it to the route's static prefix). index and etag are accepted but currently unused — http.FileServer already serves index.html and emits ETag/Last-Modified by default."},
			},
			ReturnType: "(req: Request, res: Response) => void",
			Returns:    "A route handler marker (returned synchronously). Assign it as a routes entry, typically under a wildcard pattern like 'GET /assets/{rest...}'. The route compiler unwraps it to a stdlib http.FileServer mounted under http.StripPrefix.",
			Errors:     "The call itself does not throw; an invalid dir surfaces as 404s at request time.",
			Example: `server.http.listen({
  port: 8080,
  routes: { "GET /assets/{rest...}": server.http.static({ dir: "./public", stripPrefix: "/assets/" }) },
});`,
		},
		"https.static": {
			Summary: "Like server.http.static; same options.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ dir: string; stripPrefix?: string; index?: string; etag?: boolean }", Desc: "Identical to server.http.static — dir is the root, stripPrefix is removed from the path before lookup, index/etag are accepted but unused."},
			},
			ReturnType: "(req: Request, res: Response) => void",
			Returns:    "A route handler marker (returned synchronously); assign it to a wildcard route on an https listener.",
			Errors:     "The call itself does not throw; an invalid dir surfaces as 404s at request time.",
			Example: `server.https.listen({
  port: 8443, cert, key,
  routes: { "GET /assets/{rest...}": server.https.static({ dir: "./public", stripPrefix: "/assets/" }) },
});`,
		},
		"smtp.listen": {
			Summary: "Bind an SMTP listener: server.smtp.listen({port, hostname?, handlers: {onMail, onRcpt, onData}, auth?, starttls?, allowInsecureAuth?, maxMessageBytes?, maxRecipients?, sessionTimeout?}) → handle with .address, .close(), .stopped Promise. Handlers receive (envelope, …) per stage; return true/undefined to accept, false to reject, a string for a 550 reason, throw for 451 temp-fail. onData receives a parsed Message with text/html bodies, attachments, and raw bytes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; hostname?: string; handlers: { onMail: (env: Envelope) => boolean | string | void | Promise<boolean | string | void>; onRcpt: (env: Envelope, to: string) => boolean | string | void | Promise<boolean | string | void>; onData: (env: Envelope, msg: Message) => boolean | string | void | Promise<boolean | string | void> }; auth?: (user: string, pass: string, env: Envelope) => boolean | Promise<boolean>; starttls?: { cert: string; key: string }; allowInsecureAuth?: boolean; maxMessageBytes?: number; maxRecipients?: number; sessionTimeout?: number }", Desc: "port is required; host (bind interface) defaults to \"0.0.0.0\"; hostname (advertised EHLO domain) defaults to the OS hostname. handlers.onMail/onRcpt/onData are all required and run per protocol stage — each returns true/undefined to accept, false for a 550 reject, a string for '550 <string>', or throws for a 451 temp-fail. auth (optional) enables PLAIN+LOGIN SASL; return truthy to accept. starttls {cert, key} (paths or inline PEM) enables STARTTLS. allowInsecureAuth permits AUTH without TLS. maxMessageBytes defaults to 10 MiB (non-positive values ignored), maxRecipients to 100, sessionTimeout to 30000 (milliseconds)."},
			},
			ReturnType: "{ address: string; stopped: Promise<void>; close(): Promise<void> }",
			Returns:    "A server handle (returned synchronously): address is 'tcp/host:port'; stopped resolves when the server stops (rejects on a non-close Serve error); close() shuts the listener down and resolves with the stopped Promise. The Envelope passed to handlers is { from, recipients, remote, helo, authenticatedUser?, tls?: { version, cipher } }; the Message passed to onData is { from, to, cc, subject, headers, body: { text, html }, attachments: { filename, contentType, bytes }[], raw: Uint8Array }.",
			Errors:     "Throws synchronously if opts is missing, port is 0/absent, handlers is missing, any of onMail/onRcpt/onData is absent or not a function, auth is present but not a function, the starttls key pair fails to load, or the bind fails.",
			Example: `const srv = server.smtp.listen({
  port: 2525,
  handlers: {
    onMail: (env) => true,
    onRcpt: (env, to) => to.endsWith("@example.com"),
    onData: (env, msg) => { runtime.log(msg.subject); },
  },
});`,
		},
		"tcp.listen": {
			Summary: "Bind a raw TCP server: server.tcp.listen({port, host?, readBuffer?}, conn => {…}) → handle { address: 'tcp/host:port', close() }. The connection handler runs once per accepted socket; conn is the SAME handle shape as net.tcp.connect — onData(cb) (cb gets {bytes, text}), onClose(cb), onError(cb), write(data) (string or Uint8Array), close(), and remote/local addresses. Synchronous bind (throws on bind error); port:0 binds an OS-chosen ephemeral port. Emits a READY line under `sercon serve` and joins graceful shutdown.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string; readBuffer?: number }", Desc: "port is the listen port (0 binds an OS-chosen ephemeral port). host defaults to all interfaces. readBuffer is the per-connection inbound channel capacity (frames buffered before backpressure), default 64."},
				{Name: "handler", Type: "(conn: { remote: string; local: string; write(data: string | Uint8Array): Promise<void>; onData(cb: (msg: { bytes: Uint8Array; text: string }) => void): void; onClose(cb: () => void): void; onError(cb: (err: unknown) => void): void; close(): void }) => void", Desc: "Invoked once per accepted connection. conn matches net.tcp.connect's handle: register onData/onClose/onError callbacks, write() to send (returns a Promise), close() to tear down. remote/local are 'host:port' strings."},
			},
			ReturnType: "{ address: string; close(): Promise<void> }",
			Returns:    "A server handle (returned synchronously): address is 'tcp/host:port' (the resolved bind address, so port:0 reports its ephemeral port). close() closes the listener and all accepted connections, then resolves.",
			Errors:     "Throws synchronously if opts is missing, the handler is not a function, or the bind fails (e.g. address already in use).",
			Example: `const srv = server.tcp.listen({ port: 0 }, (conn) => {
  conn.onData((msg) => conn.write("echo: " + msg.text));
});
runtime.log(srv.address);
await srv.close();`,
		},
		"udp.listen": {
			Summary: "Bind a raw UDP server: server.udp.listen({port, host?}, (msg, reply) => {…}) → handle { address: 'udp/host:port', close() }. The handler runs once per inbound datagram; msg is {bytes, text, address, port} (the sender) and reply(data) (string or Uint8Array) sends a datagram back to that sender, returning a Promise. Synchronous bind (throws on bind error); port:0 binds an OS-chosen ephemeral port. Emits a READY line under `sercon serve` and joins graceful shutdown.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ port: number; host?: string }", Desc: "port is the listen port (0 binds an OS-chosen ephemeral port). host defaults to all interfaces."},
				{Name: "handler", Type: "(msg: { bytes: Uint8Array; text: string; address: string; port: number }, reply: (data: string | Uint8Array) => Promise<void>) => void", Desc: "Invoked once per inbound datagram. msg carries the payload (bytes/text) and the sender's address/port. reply(data) sends a datagram back to that sender and returns a Promise that resolves once written (rejects on a write error)."},
			},
			ReturnType: "{ address: string; close(): Promise<void> }",
			Returns:    "A server handle (returned synchronously): address is 'udp/host:port' (resolved, so port:0 reports its ephemeral port). close() closes the socket and resolves. There is no per-connection handle — UDP is connectionless, so reply is bound to the originating datagram's sender.",
			Errors:     "Throws synchronously if opts is missing, the handler is not a function, the address fails to resolve, or the bind fails.",
			Example: `const srv = server.udp.listen({ port: 0 }, (msg, reply) => {
  reply("pong: " + msg.text);
});
runtime.log(srv.address);
await srv.close();`,
		},
		"icmp.listen": {
			Summary: "Bind a raw ICMP listener: server.icmp.listen(opts?, (msg, reply) => {…}) → handle { address: 'icmp/<addr>', close() }. Raw ICMP has no ports — the socket receives ALL host ICMP traffic — and needs root / CAP_NET_RAW (synchronous bind throws otherwise). opts { network?: 'ip4'|'ip6' (default 'ip4'), readBuffer? }. The handler runs once per received packet; msg is { bytes, text, address, type, code } (the sender + parsed ICMP header) and reply(opts?) sends an ICMP message back to the sender (Echo by default, or a raw body), returning a Promise. Emits a READY line under `sercon serve` and joins graceful shutdown.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ network?: \"ip4\" | \"ip6\", readBuffer?: number }", Optional: true, Desc: "network selects the IP version (default 'ip4'); readBuffer is the inbound buffer size (default 64). There is no host/port — raw ICMP binds to all addresses."},
				{Name: "handler", Type: "(msg: { bytes: Uint8Array; text: string; address: string; type: number; code: number }, reply: (opts?: { to?: string; type?: number; code?: number; id?: number; seq?: number; payload?: string | Uint8Array; body?: string | Uint8Array }) => Promise<void>) => void", Desc: "Invoked once per received ICMP packet. msg carries the marshalled body (bytes/text), the sender address, and the parsed type/code. reply(opts?) sends an ICMP message back to the sender (or opts.to): Echo mode { type?, code?, id?, seq?, payload? } or raw mode { type, code?, body } (body marshalled verbatim); it returns a Promise that resolves once written."},
			},
			ReturnType: "{ address: string; close(): Promise<void> }",
			Returns:    "A server handle (returned synchronously): address is 'icmp/<local-addr>'; close() closes the socket and resolves. There is no per-connection handle (ICMP is connectionless) — reply is bound to the received packet's sender.",
			Errors:     "Throws synchronously if the handler is not a function, or if the raw socket can't be opened (typically because it needs root / CAP_NET_RAW). reply rejects on resolve/marshal/write errors and throws synchronously for the raw-body validation rules (a raw body requires type; body is mutually exclusive with id/seq/payload).",
			Example: `// Needs root / CAP_NET_RAW. Reply to every echo request with an echo reply:
const srv = server.icmp.listen({}, (msg, reply) => {
  if (msg.type === 8) reply({ type: 0, payload: msg.bytes });
});
runtime.log(srv.address);
await srv.close();`,
		},
	}
}
