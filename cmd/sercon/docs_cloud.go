package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// Inline handle-type strings, mirroring sqlHandleType/respHandleType in
// docs_db.go. cloud.google(...)'s per-service handles (storage()/compute()/
// iam()/secrets()) and the call() escape hatch are all built at script-run
// time — see googleHandle in cloud.go and googleStorage/googleCompute/
// googleIAM/googleSecrets in cloud_google_*.go — so the d.ts emitter's
// reflection has no static shape to recover for them (a Go
// `func(goja.FunctionCall) goja.Value` carries no type information). These
// constants hand the emitter the real shape via the "google" MemberDoc's
// ReturnType, which is spliced in verbatim (see asyncReturnType/
// writeMemberObject in pkg/scriptengine/dts.go) and therefore must be valid
// TypeScript on its own.
const (
	googleStorageHandleType = "{ listBuckets(opts: { project: string }): Promise<{ items?: Array<Record<string, unknown>> }>; getBucket(opts: { bucket: string }): Promise<Record<string, unknown>>; createBucket(opts: { project: string; bucket: string }): Promise<Record<string, unknown>>; deleteBucket(opts: { bucket: string }): Promise<Record<string, unknown>>; listObjects(opts: { bucket: string; prefix?: string }): Promise<{ items?: Array<Record<string, unknown>> }>; statObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>>; readObject(opts: { bucket: string; key: string }): Promise<{ bytes: number[] }>; putObject(opts: { bucket: string; key: string; body: string | Uint8Array | ArrayBuffer }): Promise<Record<string, unknown>>; deleteObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>> }"

	googleComputeHandleType = "{ listInstances(opts: { project: string; zone: string }): Promise<{ items?: Array<Record<string, unknown>> }>; getInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>; createInstance(opts: { project: string; zone: string; instance: Record<string, unknown> }): Promise<Record<string, unknown>>; deleteInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>; startInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>; stopInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>; listZones(opts: { project: string }): Promise<{ items?: Array<Record<string, unknown>> }>; listDisks(opts: { project: string; zone: string }): Promise<{ items?: Array<Record<string, unknown>> }> }"

	googleIAMHandleType = "{ listServiceAccounts(opts: { project: string }): Promise<{ accounts?: Array<Record<string, unknown>> }>; getServiceAccount(opts: { project: string; email: string }): Promise<Record<string, unknown>>; createServiceAccount(opts: { project: string; accountId: string; displayName?: string }): Promise<Record<string, unknown>>; deleteServiceAccount(opts: { project: string; email: string }): Promise<Record<string, unknown>>; listKeys(opts: { project: string; email: string }): Promise<{ keys?: Array<Record<string, unknown>> }>; createKey(opts: { project: string; email: string }): Promise<Record<string, unknown>>; getIamPolicy(opts: { resource: string }): Promise<Record<string, unknown>>; setIamPolicy(opts: { resource: string; policy: Record<string, unknown> }): Promise<Record<string, unknown>> }"

	googleSecretsHandleType = "{ listSecrets(opts: { project: string }): Promise<{ secrets?: Array<Record<string, unknown>> }>; getSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>>; createSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>>; addSecretVersion(opts: { project: string; name: string; payload: string }): Promise<Record<string, unknown>>; accessSecretVersion(opts: { project: string; name: string; version?: string }): Promise<{ value: string }>; deleteSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>> }"

	googleCallSig = "call(opts: { api: string; version?: string; httpMethod?: string; path: string; params?: Record<string, string>; body?: unknown }): Promise<unknown>"

	// googleHandleType is the object cloud.google(...) itself resolves to —
	// spliced verbatim into the "google" MemberDoc's ReturnType.
	googleHandleType = "{ storage(): " + googleStorageHandleType + "; compute(): " + googleComputeHandleType + "; iam(): " + googleIAMHandleType + "; secrets(): " + googleSecretsHandleType + "; " + googleCallSig + " }"

	googleErrors = "Rejects with a structured Error { code, status, message, details } on API or transport failure. code/status are 0/\"TRANSPORT\" for non-API errors (DNS, TLS, timeout, connection refused)."
)

// cloudDocs documents the `cloud` global (cloud.google(...) and its
// storage/compute/iam/secrets services). Keys are relative to "cloud" (no
// "cloud." prefix — SetMemberDocsStructured prepends it), matching the
// convention in docs_fs.go/docs_net.go/docs_db.go. Because the storage()/
// compute()/iam()/secrets() handles and call() are all built at script-run
// time (see cloud.go, cloud_google_storage.go, cloud_google_compute.go,
// cloud_google_iam.go, cloud_google_secrets.go), only the top-level "google"
// entry is reachable by the automatic .d.ts / markdown-reference walkers
// (which recurse only into literal map[string]any namespace members, and
// "google" is an opaque Go func). The per-service/per-method entries below
// are still written out in full as the documentation source of truth (and in
// case a future emitter upgrade recurses into runtime-built handles); the
// deep typing that DOES reach the emitted .d.ts today comes from the
// "google" entry's ReturnType (googleHandleType above), not from these.
func cloudDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"google": {
			Summary: "Google Cloud provider handle. Pure-Go, CGO-free (google.golang.org/api); reuses Application Default Credentials unless credentials is given. Returns an object exposing storage(), compute(), iam(), secrets(), and the generic call() REST escape hatch.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project?: string; location?: string; credentials?: string | Record<string, unknown>; scopes?: string[]; quotaProject?: string }", Optional: true, Desc: "project/location: default GCP project id / region-or-zone threaded into the service calls that accept them. credentials: a file path (string) to a service-account JSON key, or an inline service-account JSON object; omitted ⇒ Application Default Credentials (gcloud auth application-default login, GOOGLE_APPLICATION_CREDENTIALS, or attached-metadata identity). scopes: OAuth scopes to request; omitted ⇒ each service's default scope. quotaProject: billing/quota project override (X-Goog-User-Project)."},
			},
			ReturnType: googleHandleType,
			Returns:    "The provider handle: { storage(), compute(), iam(), secrets(), call(opts) }. Each of storage()/compute()/iam()/secrets() returns a fresh service handle bound to this call's config; call() is the generic path-based REST escape hatch for APIs without a typed service above.",
			Errors:     "Throws synchronously (not a rejected promise) if opts is provided but is not an object, or credentials is neither a string path nor a plain object.",
			Example: `const g = cloud.google({ project: "my-proj" });
const gcs = g.storage();
const r = await gcs.listBuckets({ project: "my-proj" });
runtime.log(r.items?.length ?? 0);`,
		},
		"google.call": {
			Summary: "Generic path-based REST escape hatch onto any {api}.googleapis.com endpoint — for APIs without a typed service above (storage/compute/iam/secrets). Authenticates the same way as the parent cloud.google(...) handle.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ api: string; version?: string; httpMethod?: string; path: string; params?: Record<string, string>; body?: unknown }", Desc: "api: the API's subdomain, e.g. \"compute\" for compute.googleapis.com. version: URL-versioning hint some paths embed directly; defaults to \"v1\" (informational — path is used as given). httpMethod: HTTP verb, defaults to \"GET\". path: request path, e.g. \"/compute/v1/projects/p/zones\" — appended to https://{api}.googleapis.com as-is. params: query-string parameters. body: JSON-serialisable request body; sent with Content-Type: application/json when present."},
			},
			ReturnType: "Promise<unknown>",
			Returns:    "A promise resolving to the decoded JSON response body ({} when the response body is empty).",
			Errors:     googleErrors + " Also rejects if `api` or `path` is missing/empty, or if body is not JSON-serialisable.",
			Example: `const g = cloud.google({ project: "my-proj" });
const zones = await g.call({ api: "compute", path: "/compute/v1/projects/my-proj/zones" });
runtime.log(zones.items?.length ?? 0);`,
		},

		// --- storage() — Cloud Storage (google.golang.org/api/storage/v1) ---

		"google.storage.listBuckets": {
			Summary:    "List the Cloud Storage buckets in a project.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ project: string }", Desc: "project: the GCP project id."}},
			ReturnType: "Promise<{ items?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Storage buckets.list response.",
			Errors:     googleErrors,
			Example:    `const gcs = cloud.google({ project: "p" }).storage(); const r = await gcs.listBuckets({ project: "p" });`,
		},
		"google.storage.getBucket": {
			Summary:    "Get a Cloud Storage bucket's metadata.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ bucket: string }", Desc: "bucket: the bucket name (not the gs:// URI)."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Storage buckets.get response.",
			Errors:     googleErrors,
			Example:    `const b = await cloud.google({ project: "p" }).storage().getBucket({ bucket: "my-bucket" });`,
		},
		"google.storage.createBucket": {
			Summary: "Create a Cloud Storage bucket.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; bucket: string }", Desc: "project: owning GCP project id. bucket: the new bucket's globally-unique name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Storage buckets.insert response.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).storage().createBucket({ project: "p", bucket: "my-new-bucket" });`,
		},
		"google.storage.deleteBucket": {
			Summary:    "Delete a Cloud Storage bucket. The bucket must be empty.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ bucket: string }", Desc: "bucket: the bucket name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).storage().deleteBucket({ bucket: "my-bucket" });`,
		},
		"google.storage.listObjects": {
			Summary: "List objects in a bucket, optionally filtered by key prefix.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; prefix?: string }", Desc: "bucket: the bucket name. prefix: only list objects whose key starts with this string."},
			},
			ReturnType: "Promise<{ items?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Storage objects.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).storage().listObjects({ bucket: "my-bucket", prefix: "logs/" });`,
		},
		"google.storage.statObject": {
			Summary: "Get an object's metadata without downloading its content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key (path within the bucket)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Storage objects.get response.",
			Errors:     googleErrors,
			Example:    `const meta = await cloud.google({ project: "p" }).storage().statObject({ bucket: "my-bucket", key: "logs/a.txt" });`,
		},
		"google.storage.readObject": {
			Summary: "Download an object's content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key."},
			},
			ReturnType: "Promise<{ bytes: number[] }>",
			Returns:    "A promise resolving to { bytes } where bytes is a plain JS number[] (byte-value array), NOT a real Uint8Array — wrap it with new Uint8Array(res.bytes) before treating it as binary data (e.g. before fs.writeBytes or further decoding).",
			Errors:     googleErrors,
			Example: `const res = await cloud.google({ project: "p" }).storage().readObject({ bucket: "my-bucket", key: "logs/a.txt" });
const bytes = new Uint8Array(res.bytes);
runtime.log(bytes.length);`,
		},
		"google.storage.putObject": {
			Summary: "Upload/overwrite an object's content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string; body: string | Uint8Array | ArrayBuffer }", Desc: "bucket: the bucket name. key: the object's key. body: a string (encoded as UTF-8) or raw bytes (Uint8Array/ArrayBuffer) to upload as the object's content."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Storage objects.insert response.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).storage().putObject({ bucket: "my-bucket", key: "logs/a.txt", body: "hello" });`,
		},
		"google.storage.deleteObject": {
			Summary: "Delete an object.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).storage().deleteObject({ bucket: "my-bucket", key: "logs/a.txt" });`,
		},

		// --- compute() — Compute Engine (google.golang.org/api/compute/v1) ---

		"google.compute.listInstances": {
			Summary: "List Compute Engine VM instances in a zone.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string }", Desc: "project: the GCP project id. zone: e.g. \"europe-north1-a\"."},
			},
			ReturnType: "Promise<{ items?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Compute instances.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).compute().listInstances({ project: "p", zone: "europe-north1-a" });`,
		},
		"google.compute.getInstance": {
			Summary: "Get a Compute Engine VM instance's metadata.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string; name: string }", Desc: "project: the GCP project id. zone: the instance's zone. name: the instance name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Compute instances.get response.",
			Errors:     googleErrors,
			Example:    `const inst = await cloud.google({ project: "p" }).compute().getInstance({ project: "p", zone: "europe-north1-a", name: "web-1" });`,
		},
		"google.compute.createInstance": {
			Summary: "Create a Compute Engine VM instance.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string; instance: Record<string, unknown> }", Desc: "project: the GCP project id. zone: the target zone. instance: a Compute Engine Instance resource body (machineType, disks, networkInterfaces, etc. — see the Compute Engine instances.insert API reference)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the (typically long-running) Compute instances.insert operation resource.",
			Errors:     googleErrors,
			Example: `const op = await cloud.google({ project: "p" }).compute().createInstance({
  project: "p", zone: "europe-north1-a",
  instance: { name: "web-1", machineType: "zones/europe-north1-a/machineTypes/e2-micro" },
});`,
		},
		"google.compute.deleteInstance": {
			Summary: "Delete a Compute Engine VM instance.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string; name: string }", Desc: "project: the GCP project id. zone: the instance's zone. name: the instance name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Compute instances.delete operation resource.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).compute().deleteInstance({ project: "p", zone: "europe-north1-a", name: "web-1" });`,
		},
		"google.compute.startInstance": {
			Summary: "Start a stopped Compute Engine VM instance.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string; name: string }", Desc: "project: the GCP project id. zone: the instance's zone. name: the instance name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Compute instances.start operation resource.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).compute().startInstance({ project: "p", zone: "europe-north1-a", name: "web-1" });`,
		},
		"google.compute.stopInstance": {
			Summary: "Stop a running Compute Engine VM instance.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string; name: string }", Desc: "project: the GCP project id. zone: the instance's zone. name: the instance name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Compute instances.stop operation resource.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).compute().stopInstance({ project: "p", zone: "europe-north1-a", name: "web-1" });`,
		},
		"google.compute.listZones": {
			Summary:    "List the Compute Engine zones available to a project.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ project: string }", Desc: "project: the GCP project id."}},
			ReturnType: "Promise<{ items?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Compute zones.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).compute().listZones({ project: "p" });`,
		},
		"google.compute.listDisks": {
			Summary: "List Compute Engine persistent disks in a zone.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; zone: string }", Desc: "project: the GCP project id. zone: e.g. \"europe-north1-a\"."},
			},
			ReturnType: "Promise<{ items?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Compute disks.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).compute().listDisks({ project: "p", zone: "europe-north1-a" });`,
		},

		// --- iam() — Cloud IAM service accounts (google.golang.org/api/iam/v1) ---

		"google.iam.listServiceAccounts": {
			Summary:    "List the IAM service accounts in a project.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ project: string }", Desc: "project: the GCP project id."}},
			ReturnType: "Promise<{ accounts?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the IAM serviceAccounts.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).iam().listServiceAccounts({ project: "p" });`,
		},
		"google.iam.getServiceAccount": {
			Summary: "Get an IAM service account's metadata.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; email: string }", Desc: "project: the GCP project id. email: the service account's email address."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM serviceAccounts.get response.",
			Errors:     googleErrors,
			Example:    `const sa = await cloud.google({ project: "p" }).iam().getServiceAccount({ project: "p", email: "sa@p.iam.gserviceaccount.com" });`,
		},
		"google.iam.createServiceAccount": {
			Summary: "Create an IAM service account.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; accountId: string; displayName?: string }", Desc: "project: the GCP project id. accountId: the service account id (the part before @ in its email). displayName: an optional human-readable name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM serviceAccounts.create response.",
			Errors:     googleErrors,
			Example:    `const sa = await cloud.google({ project: "p" }).iam().createServiceAccount({ project: "p", accountId: "my-sa", displayName: "My SA" });`,
		},
		"google.iam.deleteServiceAccount": {
			Summary: "Delete an IAM service account.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; email: string }", Desc: "project: the GCP project id. email: the service account's email address."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).iam().deleteServiceAccount({ project: "p", email: "sa@p.iam.gserviceaccount.com" });`,
		},
		"google.iam.listKeys": {
			Summary: "List a service account's IAM keys.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; email: string }", Desc: "project: the GCP project id. email: the service account's email address."},
			},
			ReturnType: "Promise<{ keys?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the IAM serviceAccounts.keys.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).iam().listKeys({ project: "p", email: "sa@p.iam.gserviceaccount.com" });`,
		},
		"google.iam.createKey": {
			Summary: "Create a new IAM key for a service account. The private key material is only ever returned by this call — store it immediately.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; email: string }", Desc: "project: the GCP project id. email: the service account's email address."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM serviceAccounts.keys.create response, including the base64-encoded private key material.",
			Errors:     googleErrors,
			Example:    `const key = await cloud.google({ project: "p" }).iam().createKey({ project: "p", email: "sa@p.iam.gserviceaccount.com" });`,
		},
		"google.iam.getIamPolicy": {
			Summary: "Get the IAM policy attached to a resource (e.g. a service account).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resource: string }", Desc: "resource: the full resource name, e.g. \"projects/p/serviceAccounts/sa@p.iam.gserviceaccount.com\"."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM getIamPolicy response.",
			Errors:     googleErrors,
			Example:    `const policy = await cloud.google({ project: "p" }).iam().getIamPolicy({ resource: "projects/p/serviceAccounts/sa@p.iam.gserviceaccount.com" });`,
		},
		"google.iam.setIamPolicy": {
			Summary: "Replace the IAM policy attached to a resource. This is a full replace, not a merge — read-modify-write via getIamPolicy first to avoid clobbering existing bindings.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resource: string; policy: Record<string, unknown> }", Desc: "resource: the full resource name. policy: the complete IAM Policy body ({ bindings: [{ role, members }], etag?, version? })."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM setIamPolicy response (the policy as stored).",
			Errors:     googleErrors,
			Example: `const resource = "projects/p/serviceAccounts/sa@p.iam.gserviceaccount.com";
const current = await cloud.google({ project: "p" }).iam().getIamPolicy({ resource });
current.bindings = [...(current.bindings ?? []), { role: "roles/iam.serviceAccountUser", members: ["user:me@example.com"] }];
await cloud.google({ project: "p" }).iam().setIamPolicy({ resource, policy: current });`,
		},

		// --- secrets() — Secret Manager (google.golang.org/api/secretmanager/v1) ---

		"google.secrets.listSecrets": {
			Summary:    "List the secrets in a project (metadata only — not their values).",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ project: string }", Desc: "project: the GCP project id."}},
			ReturnType: "Promise<{ secrets?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to the Secret Manager secrets.list response.",
			Errors:     googleErrors,
			Example:    `const r = await cloud.google({ project: "p" }).secrets().listSecrets({ project: "p" });`,
		},
		"google.secrets.getSecret": {
			Summary: "Get a secret's metadata (not its value — use accessSecretVersion for that).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; name: string }", Desc: "project: the GCP project id. name: the secret id."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secret Manager secrets.get response.",
			Errors:     googleErrors,
			Example:    `const secret = await cloud.google({ project: "p" }).secrets().getSecret({ project: "p", name: "db-password" });`,
		},
		"google.secrets.createSecret": {
			Summary: "Create a secret container (automatic replication). Does not set a value — call addSecretVersion to add one.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; name: string }", Desc: "project: the GCP project id. name: the new secret's id."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secret Manager secrets.create response.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).secrets().createSecret({ project: "p", name: "db-password" });`,
		},
		"google.secrets.addSecretVersion": {
			Summary: "Add a new version (value) to an existing secret. The payload is base64-encoded on the wire automatically.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; name: string; payload: string }", Desc: "project: the GCP project id. name: the secret id. payload: the plaintext secret value."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secret Manager secrets.addVersion response.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).secrets().addSecretVersion({ project: "p", name: "db-password", payload: "s3cr3t" });`,
		},
		"google.secrets.accessSecretVersion": {
			Summary: "Access (decrypt) a secret version's plaintext value.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; name: string; version?: string }", Desc: "project: the GCP project id. name: the secret id. version: a version number as a string, or \"latest\"; defaults to \"latest\" when omitted."},
			},
			ReturnType: "Promise<{ value: string }>",
			Returns:    "A promise resolving to { value } — the decoded plaintext secret value (already base64-decoded; never the raw wire-format base64).",
			Errors:     googleErrors,
			Example: `const { value } = await cloud.google({ project: "p" }).secrets().accessSecretVersion({ project: "p", name: "db-password" });
runtime.log(value);`,
		},
		"google.secrets.deleteSecret": {
			Summary: "Delete a secret and all of its versions.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project: string; name: string }", Desc: "project: the GCP project id. name: the secret id."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     googleErrors,
			Example:    `await cloud.google({ project: "p" }).secrets().deleteSecret({ project: "p", name: "db-password" });`,
		},
	}
}
