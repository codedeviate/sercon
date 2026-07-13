package main

import "github.com/codedeviate/sercon/pkg/scriptengine"

// Inline handle-type strings, mirroring sqlHandleType/respHandleType in
// docs_db.go. cloud.google(...)'s per-service handles (storage()/compute()/
// iam()/secrets()) and the call() escape hatch, cloud.aws(...)'s per-service
// handles (s3()/ec2()/iam()/secretsmanager()/sts()/lambda()/sqs()/
// cloudwatch()/cloudwatchlogs()), and cloud.azure(...)'s per-service handles
// (resourceGroups()/compute()/resources()/blob()/keyvaultSecrets()) plus its
// ARM call() escape hatch, are all built at script-run time — see
// googleHandle in cloud.go and googleStorage/googleCompute/googleIAM/
// googleSecrets in cloud_google_*.go, awsHandle in cloud_aws.go and
// awsS3/awsEC2/awsIAM/awsSecretsManager/awsSTS/awsLambda/awsSQS/awsCloudWatch/
// awsCloudWatchLogs in cloud_aws_*.go, and azureHandle in cloud_azure.go and
// azureResourceGroups/azureCompute/azureResources/azureBlob/
// azureKeyvaultSecrets in cloud_azure_*.go — so the d.ts emitter's reflection
// has no static shape to recover for them (a Go
// `func(goja.FunctionCall) goja.Value` carries no type information). These
// constants hand the emitter the real shape via the "google"/"aws"/"azure"
// MemberDoc entries' ReturnType, which is spliced in verbatim (see
// asyncReturnType/writeMemberObject in pkg/scriptengine/dts.go) and therefore
// must be valid TypeScript on its own.
const (
	// googleHandleType is the object cloud.google(...) resolves to — spliced
	// verbatim into the "google" MemberDoc's ReturnType, so it must be valid
	// TypeScript on its own. Formatted multi-line for readable rendering in the
	// d.ts and MANUAL §17 reference; a single-line form is unreadable.
	googleHandleType = `{
  storage(): {
    listBuckets(opts: { project: string }): Promise<{ items?: Array<Record<string, unknown>> }>;
    getBucket(opts: { bucket: string }): Promise<Record<string, unknown>>;
    createBucket(opts: { project: string; bucket: string }): Promise<Record<string, unknown>>;
    deleteBucket(opts: { bucket: string }): Promise<Record<string, unknown>>;
    listObjects(opts: { bucket: string; prefix?: string }): Promise<{ items?: Array<Record<string, unknown>> }>;
    statObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>>;
    readObject(opts: { bucket: string; key: string }): Promise<{ bytes: number[] }>;
    putObject(opts: { bucket: string; key: string; body: string | Uint8Array | ArrayBuffer }): Promise<Record<string, unknown>>;
    deleteObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>>;
  };
  compute(): {
    listInstances(opts: { project: string; zone: string }): Promise<{ items?: Array<Record<string, unknown>> }>;
    getInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>;
    createInstance(opts: { project: string; zone: string; instance: Record<string, unknown> }): Promise<Record<string, unknown>>;
    deleteInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>;
    startInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>;
    stopInstance(opts: { project: string; zone: string; name: string }): Promise<Record<string, unknown>>;
    listZones(opts: { project: string }): Promise<{ items?: Array<Record<string, unknown>> }>;
    listDisks(opts: { project: string; zone: string }): Promise<{ items?: Array<Record<string, unknown>> }>;
  };
  iam(): {
    listServiceAccounts(opts: { project: string }): Promise<{ accounts?: Array<Record<string, unknown>> }>;
    getServiceAccount(opts: { project: string; email: string }): Promise<Record<string, unknown>>;
    createServiceAccount(opts: { project: string; accountId: string; displayName?: string }): Promise<Record<string, unknown>>;
    deleteServiceAccount(opts: { project: string; email: string }): Promise<Record<string, unknown>>;
    listKeys(opts: { project: string; email: string }): Promise<{ keys?: Array<Record<string, unknown>> }>;
    createKey(opts: { project: string; email: string }): Promise<Record<string, unknown>>;
    getIamPolicy(opts: { resource: string }): Promise<Record<string, unknown>>;
    setIamPolicy(opts: { resource: string; policy: Record<string, unknown> }): Promise<Record<string, unknown>>;
  };
  secrets(): {
    listSecrets(opts: { project: string }): Promise<{ secrets?: Array<Record<string, unknown>> }>;
    getSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>>;
    createSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>>;
    addSecretVersion(opts: { project: string; name: string; payload: string }): Promise<Record<string, unknown>>;
    accessSecretVersion(opts: { project: string; name: string; version?: string }): Promise<{ value: string }>;
    deleteSecret(opts: { project: string; name: string }): Promise<Record<string, unknown>>;
  };
  call(opts: { api: string; version?: string; httpMethod?: string; path: string; params?: Record<string, string>; body?: unknown }): Promise<unknown>;
}`

	googleErrors = "Rejects with a structured Error { code, status, message, details } on API or transport failure. code/status are 0/\"TRANSPORT\" for non-API errors (DNS, TLS, timeout, connection refused)."

	// awsErrors / azureErrors are the shared per-method Errors text for the AWS
	// and Azure flat entries below, mirroring googleErrors. AWS and Azure both
	// map non-API failures to code ""/status 0 (see mapAWSError in cloud_aws.go
	// and mapAzureError in cloud_azure.go), unlike google's 0/"TRANSPORT".
	awsErrors = "Rejects with a structured Error { code, status, message, details } on API or transport failure. code is the AWS/smithy error code and status the HTTP status; both are \"\"/0 for non-API errors (DNS, TLS, timeout, connection refused)."

	azureErrors = "Rejects with a structured Error { code, status, message, details } on API or transport failure. code/status are \"\"/0 for non-API errors (DNS, TLS, timeout, connection refused, credential/token acquisition failure)."

	// awsHandleType is the object cloud.aws(...) resolves to — spliced verbatim
	// into the "aws" MemberDoc's ReturnType, so it must be valid TypeScript on
	// its own (same rationale as googleHandleType above). Formatted multi-line
	// for readable rendering in the d.ts and MANUAL §17 reference: one line per
	// service, one line per method, covering all nine typed services
	// (s3/ec2/iam/secretsmanager/sts/lambda/sqs/cloudwatch/cloudwatchlogs — see
	// cloud_aws_*.go for the implementations these signatures are derived
	// from). getMetricData/getMetricStatistics/putMetricData on cloudwatch()
	// take an SDK-shaped pass-through object (PascalCase keys matching the AWS
	// SDK's Go input struct field names, e.g. Namespace/MetricData/StartTime)
	// rather than a hand-mapped opts shape — see awsCloudWatchGetMetricData's
	// doc comment in cloud_aws_cloudwatch.go.
	awsHandleType = `{
  s3(): {
    listBuckets(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    createBucket(opts: { bucket: string }): Promise<Record<string, unknown>>;
    deleteBucket(opts: { bucket: string }): Promise<Record<string, unknown>>;
    listObjects(opts: { bucket: string; prefix?: string }): Promise<Record<string, unknown>>;
    headObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>>;
    getObject(opts: { bucket: string; key: string }): Promise<{ bytes: number[] }>;
    putObject(opts: { bucket: string; key: string; body: string | Uint8Array | ArrayBuffer }): Promise<Record<string, unknown>>;
    deleteObject(opts: { bucket: string; key: string }): Promise<Record<string, unknown>>;
  };
  ec2(): {
    describeInstances(opts?: { instanceIds?: string[] }): Promise<Record<string, unknown>>;
    runInstances(opts: { imageId: string; instanceType: string; minCount?: number; maxCount?: number }): Promise<Record<string, unknown>>;
    terminateInstances(opts: { instanceIds: string[] }): Promise<Record<string, unknown>>;
    startInstances(opts: { instanceIds: string[] }): Promise<Record<string, unknown>>;
    stopInstances(opts: { instanceIds: string[] }): Promise<Record<string, unknown>>;
    describeVolumes(opts?: { volumeIds?: string[] }): Promise<Record<string, unknown>>;
    describeAvailabilityZones(opts?: Record<string, never>): Promise<Record<string, unknown>>;
  };
  iam(): {
    listUsers(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    getUser(opts?: { userName?: string }): Promise<Record<string, unknown>>;
    listRoles(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    getRole(opts: { roleName: string }): Promise<Record<string, unknown>>;
    listPolicies(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    createUser(opts: { userName: string }): Promise<Record<string, unknown>>;
    deleteUser(opts: { userName: string }): Promise<Record<string, unknown>>;
    attachUserPolicy(opts: { userName: string; policyArn: string }): Promise<Record<string, unknown>>;
  };
  secretsmanager(): {
    listSecrets(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    describeSecret(opts: { secretId: string }): Promise<Record<string, unknown>>;
    createSecret(opts: { name: string; secretString?: string }): Promise<Record<string, unknown>>;
    getSecretValue(opts: { secretId: string }): Promise<{ value: string }>;
    putSecretValue(opts: { secretId: string; secretString: string }): Promise<Record<string, unknown>>;
    deleteSecret(opts: { secretId: string }): Promise<Record<string, unknown>>;
  };
  sts(): {
    getCallerIdentity(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    assumeRole(opts: { roleArn: string; roleSessionName: string; durationSeconds?: number }): Promise<Record<string, unknown>>;
    getSessionToken(opts?: { durationSeconds?: number }): Promise<Record<string, unknown>>;
  };
  lambda(): {
    listFunctions(opts?: Record<string, never>): Promise<Record<string, unknown>>;
    getFunction(opts: { functionName: string }): Promise<Record<string, unknown>>;
    invoke(opts: { functionName: string; payload?: string | Record<string, unknown> }): Promise<{ statusCode: number; payload: string; functionError?: string; executedVersion?: string }>;
    createFunction(opts: { functionName: string; role: string; runtime: string; handler: string; zipFile?: string | Uint8Array | ArrayBuffer; s3Bucket?: string; s3Key?: string }): Promise<Record<string, unknown>>;
    deleteFunction(opts: { functionName: string }): Promise<Record<string, unknown>>;
  };
  sqs(): {
    listQueues(opts?: { prefix?: string }): Promise<Record<string, unknown>>;
    createQueue(opts: { queueName: string }): Promise<Record<string, unknown>>;
    deleteQueue(opts: { queueUrl: string }): Promise<Record<string, unknown>>;
    sendMessage(opts: { queueUrl: string; messageBody: string }): Promise<Record<string, unknown>>;
    receiveMessage(opts: { queueUrl: string; maxMessages?: number }): Promise<Record<string, unknown>>;
    deleteMessage(opts: { queueUrl: string; receiptHandle: string }): Promise<Record<string, unknown>>;
    getQueueAttributes(opts: { queueUrl: string; attributeNames?: string[] }): Promise<Record<string, unknown>>;
  };
  cloudwatch(): {
    listMetrics(opts?: { namespace?: string; metricName?: string }): Promise<Record<string, unknown>>;
    getMetricData(opts: Record<string, unknown>): Promise<Record<string, unknown>>;
    getMetricStatistics(opts: Record<string, unknown>): Promise<Record<string, unknown>>;
    describeAlarms(opts?: { alarmNames?: string[] }): Promise<Record<string, unknown>>;
    putMetricData(opts: Record<string, unknown>): Promise<Record<string, unknown>>;
  };
  cloudwatchlogs(): {
    describeLogGroups(opts?: { prefix?: string }): Promise<Record<string, unknown>>;
    describeLogStreams(opts: { logGroupName: string }): Promise<Record<string, unknown>>;
    getLogEvents(opts: { logGroupName: string; logStreamName: string; limit?: number }): Promise<Record<string, unknown>>;
    filterLogEvents(opts: { logGroupName: string; filterPattern?: string }): Promise<Record<string, unknown>>;
    startQuery(opts: { logGroupName: string; queryString: string; startTime: number; endTime: number }): Promise<Record<string, unknown>>;
    getQueryResults(opts: { queryId: string }): Promise<Record<string, unknown>>;
  };
}`

	// azureHandleType is the object cloud.azure(...) resolves to — spliced
	// verbatim into the "azure" MemberDoc's ReturnType, so it must be valid
	// TypeScript on its own (same rationale as googleHandleType/awsHandleType
	// above). Formatted multi-line for readable rendering in the d.ts and
	// MANUAL §17 reference. Covers exactly the implemented surface (Tasks 1-8
	// of the cloud.azure feature — see cloud_azure.go and cloud_azure_*.go for
	// the source these signatures are derived from):
	//
	//   - resourceGroups()/compute()/resources() are ARM (subscription-scoped)
	//     services — list()/getVirtualMachine()/getById() etc. return the ARM
	//     SDK's own JSON shape (lowercase-camelCase keys like
	//     id/name/location/properties — the generated SDK types' own
	//     MarshalJSON, not Go struct field names) via toPlain, round-tripped
	//     as Record<string, unknown> (or { value: [...] } for list-style
	//     methods, matching the ARM list response envelope).
	//   - call(opts) is the generic ARM REST escape hatch (top-level on the
	//     handle, not nested under any one ARM service — see azureHandle in
	//     cloud_azure.go) for ARM APIs without a typed service above.
	//   - blob(accountUrl)/keyvaultSecrets(vaultUrl) are data-plane services:
	//     the accessor takes the target endpoint URL directly rather than
	//     resolving one from the subscription. blob's list/download results
	//     come from the Storage Blob SDK's un-marshalled Go structs, which
	//     carry no json tags and no custom MarshalJSON (the Storage Blob wire
	//     protocol is XML, not JSON) — toPlain's JSON round-trip therefore
	//     falls back to the exported Go field names verbatim, i.e. PascalCase
	//     keys (Name, Properties, Deleted, ...). keyvaultSecrets' results, by
	//     contrast, use the SDK's own MarshalJSON and so come back
	//     lowercase-camelCase (id, attributes, contentType, managed, tags),
	//     same convention as the ARM services.
	azureHandleType = `{
  resourceGroups(): {
    list(opts?: Record<string, never>): Promise<{ value?: Array<Record<string, unknown>> }>;
    get(opts: { name: string }): Promise<Record<string, unknown>>;
    create(opts: { name: string; location: string }): Promise<Record<string, unknown>>;
    delete(opts: { name: string }): Promise<Record<string, unknown>>;
  };
  compute(): {
    listVirtualMachines(opts?: { resourceGroup?: string }): Promise<{ value?: Array<Record<string, unknown>> }>;
    getVirtualMachine(opts: { resourceGroup: string; name: string }): Promise<Record<string, unknown>>;
    start(opts: { resourceGroup: string; name: string }): Promise<Record<string, unknown>>;
    powerOff(opts: { resourceGroup: string; name: string }): Promise<Record<string, unknown>>;
    deallocate(opts: { resourceGroup: string; name: string }): Promise<Record<string, unknown>>;
    delete(opts: { resourceGroup: string; name: string }): Promise<Record<string, unknown>>;
  };
  resources(): {
    listByResourceGroup(opts: { resourceGroup: string }): Promise<{ value?: Array<Record<string, unknown>> }>;
    getById(opts: { resourceId: string; apiVersion: string }): Promise<Record<string, unknown>>;
  };
  call(opts: { path: string; apiVersion: string; method?: string; params?: Record<string, string>; body?: unknown }): Promise<unknown>;
  blob(accountUrl: string): {
    listContainers(opts?: Record<string, never>): Promise<{ value?: Array<Record<string, unknown>> }>;
    listBlobs(opts: { container: string }): Promise<{ value?: Array<Record<string, unknown>> }>;
    download(opts: { container: string; blob: string }): Promise<{ bytes: number[] }>;
    upload(opts: { container: string; blob: string; body: string | Uint8Array | ArrayBuffer }): Promise<Record<string, unknown>>;
    deleteBlob(opts: { container: string; blob: string }): Promise<Record<string, unknown>>;
  };
  keyvaultSecrets(vaultUrl: string): {
    listSecrets(opts?: Record<string, never>): Promise<{ value?: Array<Record<string, unknown>> }>;
    getSecret(opts: { name: string }): Promise<{ value: string }>;
    setSecret(opts: { name: string; value: string }): Promise<Record<string, unknown>>;
    deleteSecret(opts: { name: string }): Promise<Record<string, unknown>>;
  };
}`
)

// cloudDocs documents the `cloud` global — the google/aws/azure provider
// handles and every one of their runtime-built services and methods. Keys are
// relative to "cloud" (no "cloud." prefix — SetMemberDocsStructured prepends
// it), matching the convention in docs_fs.go/docs_net.go/docs_db.go.
//
// The provider handles' services (storage()/compute()/s3()/resourceGroups()/…)
// and their methods are built at script-run time from opaque Go funcs, so the
// .d.ts emitter — which recurses only into literal map[string]any namespace
// members — cannot introspect their shape. The deep typing that reaches the
// emitted .d.ts therefore comes from each provider entry's ReturnType
// (googleHandleType/awsHandleType/azureHandleType above), spliced in verbatim.
//
// The MANUAL §17 markdown reference, by contrast, DOES render the flat
// per-service and per-method entries below: its generator emits documented
// members nested under a runtime-handle leaf (writeOrphanChildren in
// pkg/scriptengine/reference.go), grouping them provider → service → method.
// So every provider carries a service-group container entry (summary only)
// plus one entry per method — the composite ReturnType stays as the
// at-a-glance provider overview, and these break the same surface out method
// by method for navigation. Param/return types in the flat entries are copied
// verbatim from the composite so the two views never disagree.
func cloudDocs() map[string]scriptengine.MemberDoc {
	return map[string]scriptengine.MemberDoc{
		"google": {
			Summary: "Google Cloud provider handle. Pure-Go, CGO-free (google.golang.org/api); reuses Application Default Credentials unless credentials is given. Returns an object exposing storage(), compute(), iam(), secrets(), and the generic call() REST escape hatch.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ project?: string; credentials?: string | Record<string, unknown>; scopes?: string[]; quotaProject?: string }", Optional: true, Desc: "project: the default GCP project id used by any service call that omits its own `project`. credentials: a file path (string) to a service-account JSON key, or an inline service-account JSON object; omitted ⇒ Application Default Credentials (gcloud auth application-default login, GOOGLE_APPLICATION_CREDENTIALS, or attached-metadata identity). scopes: OAuth scopes to request; omitted ⇒ each service's default scope. quotaProject: billing/quota project override (X-Goog-User-Project)."},
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

		"google.storage": {
			Summary: "Cloud Storage — buckets and objects (google.golang.org/api/storage/v1). Reached via `cloud.google({...}).storage()`; each method below is called on that service handle.",
		},
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

		"google.compute": {
			Summary: "Compute Engine — VM instances, zones, and disks (google.golang.org/api/compute/v1). Reached via `cloud.google({...}).compute()`; each method below is called on that service handle.",
		},
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

		"google.iam": {
			Summary: "Cloud IAM — service accounts, their keys, and resource IAM policies (google.golang.org/api/iam/v1). Reached via `cloud.google({...}).iam()`; each method below is called on that service handle.",
		},
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

		"google.secrets": {
			Summary: "Secret Manager — secret containers and their versioned values (google.golang.org/api/secretmanager/v1). Reached via `cloud.google({...}).secrets()`; each method below is called on that service handle.",
		},
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

		// --- aws — Amazon Web Services provider (Tasks 3-11) ---
		//
		// The "aws" entry's ReturnType (awsHandleType above) supplies the deep
		// typing for the emitted .d.ts (the emitter can't introspect the
		// runtime-built s3()/ec2()/… handles). The flat per-service and
		// per-method entries that follow drive the MANUAL §17 reference instead
		// (rendered via writeOrphanChildren — see cloudDocs's doc comment); both
		// derive from the same cloud_aws_*.go implementations, so the composite
		// overview and the broken-out entries always agree.
		"aws": {
			Summary:    "Amazon Web Services provider. cloud.aws(opts?) returns a handle with typed services (s3, ec2, iam, secretsmanager, sts, lambda, sqs, cloudwatch, cloudwatchlogs). Pure-Go, CGO-free; reuses the standard AWS credential chain.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ region?: string; profile?: string; credentials?: { accessKeyId: string; secretAccessKey: string; sessionToken?: string } }", Optional: true, Desc: "region: AWS region (default: from the credential chain / AWS_REGION). profile: named profile. credentials: static creds; omitted ⇒ default chain (env, ~/.aws, SSO, IMDS)."}},
			ReturnType: awsHandleType,
			Returns:    "The AWS provider handle exposing the nine typed services. Most service methods take a small typed options object. The three CloudWatch metric methods — cloudwatch().getMetricData/getMetricStatistics/putMetricData — are pass-through: their argument is an AWS-SDK-shaped object with PascalCase keys (e.g. { Namespace, MetricData: [{ MetricName, Value, Unit, Timestamp }] }), forwarded to the SDK input as-is.",
			Errors:     "cloud.aws(opts) itself throws synchronously (not a rejected promise) if opts is provided but is not an object, or credentials is present but is not an object carrying accessKeyId and secretAccessKey (sessionToken optional). Each service method returns a promise that rejects with a structured Error { code, status, message, details } on API/transport failure.",
			Example:    "const who = await cloud.aws({ region: \"eu-north-1\" }).sts().getCallerIdentity({});",
		},

		// --- s3() — S3 buckets and objects (github.com/aws/aws-sdk-go-v2/service/s3) ---

		"aws.s3": {
			Summary: "S3 — buckets and objects (github.com/aws/aws-sdk-go-v2/service/s3). Reached via `cloud.aws({...}).s3()`; each method below is called on that service handle.",
		},
		"aws.s3.listBuckets": {
			Summary:    "List the S3 buckets owned by the caller.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listBuckets takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the S3 ListBuckets response — Buckets (array of { Name, CreationDate }) and Owner.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).s3().listBuckets();`,
		},
		"aws.s3.createBucket": {
			Summary:    "Create an S3 bucket.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ bucket: string }", Desc: "bucket: the new bucket's globally-unique name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the S3 CreateBucket response — Location.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).s3().createBucket({ bucket: "my-new-bucket" });`,
		},
		"aws.s3.deleteBucket": {
			Summary:    "Delete an S3 bucket. The bucket must be empty.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ bucket: string }", Desc: "bucket: the bucket name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).s3().deleteBucket({ bucket: "my-bucket" });`,
		},
		"aws.s3.listObjects": {
			Summary: "List objects in a bucket, optionally filtered by key prefix.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; prefix?: string }", Desc: "bucket: the bucket name. prefix: only list objects whose key starts with this string."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the S3 ListObjectsV2 response — Contents (array of { Key, Size, LastModified, ETag, ... }), IsTruncated, KeyCount, NextContinuationToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).s3().listObjects({ bucket: "my-bucket", prefix: "logs/" });`,
		},
		"aws.s3.headObject": {
			Summary: "Get an object's metadata without downloading its content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key (path within the bucket)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the S3 HeadObject response — ContentLength, ContentType, ETag, LastModified, Metadata, etc. (metadata only, no body).",
			Errors:     awsErrors,
			Example:    `const meta = await cloud.aws({ region: "eu-north-1" }).s3().headObject({ bucket: "my-bucket", key: "logs/a.txt" });`,
		},
		"aws.s3.getObject": {
			Summary: "Download an object's content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key."},
			},
			ReturnType: "Promise<{ bytes: number[] }>",
			Returns:    "A promise resolving to { bytes } where bytes is a plain JS number[] (byte-value array read from the object body), NOT a real Uint8Array — wrap it with new Uint8Array(res.bytes) before treating it as binary data (e.g. before fs.writeBytes or further decoding).",
			Errors:     awsErrors,
			Example: `const res = await cloud.aws({ region: "eu-north-1" }).s3().getObject({ bucket: "my-bucket", key: "logs/a.txt" });
const bytes = new Uint8Array(res.bytes);
runtime.log(bytes.length);`,
		},
		"aws.s3.putObject": {
			Summary: "Upload/overwrite an object's content.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string; body: string | Uint8Array | ArrayBuffer }", Desc: "bucket: the bucket name. key: the object's key. body: a string (encoded as UTF-8) or raw bytes (Uint8Array/ArrayBuffer) to upload as the object's content."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the S3 PutObject response — ETag, VersionId (when bucket versioning is enabled), etc.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).s3().putObject({ bucket: "my-bucket", key: "logs/a.txt", body: "hello" });`,
		},
		"aws.s3.deleteObject": {
			Summary: "Delete an object.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ bucket: string; key: string }", Desc: "bucket: the bucket name. key: the object's key."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).s3().deleteObject({ bucket: "my-bucket", key: "logs/a.txt" });`,
		},

		// --- ec2() — EC2 instances, volumes, and availability zones (github.com/aws/aws-sdk-go-v2/service/ec2) ---

		"aws.ec2": {
			Summary: "EC2 — instances, volumes, and availability zones (github.com/aws/aws-sdk-go-v2/service/ec2). Reached via `cloud.aws({...}).ec2()`; each method below is called on that service handle.",
		},
		"aws.ec2.describeInstances": {
			Summary:    "Describe EC2 instances, optionally filtered to specific instance ids.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ instanceIds?: string[] }", Optional: true, Desc: "instanceIds: filter to specific instance ids; omitted or empty ⇒ describe every instance visible to the caller."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 DescribeInstances response — Reservations (array of { Instances: [...], ReservationId, OwnerId, ... }); instances are nested inside each reservation, not returned as a flat list.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).ec2().describeInstances();`,
		},
		"aws.ec2.runInstances": {
			Summary: "Launch one or more EC2 instances from an AMI.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ imageId: string; instanceType: string; minCount?: number; maxCount?: number }", Desc: "imageId: the AMI id to launch. instanceType: e.g. \"t3.micro\". minCount/maxCount: the minimum/maximum number of instances to launch; both default to 1 when omitted (a sercon convenience — the EC2 RunInstances API itself marks both required)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 RunInstances response — Instances (array of the newly launched instances' descriptions), ReservationId, OwnerId.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).ec2().runInstances({ imageId: "ami-0123456789abcdef0", instanceType: "t3.micro" });`,
		},
		"aws.ec2.terminateInstances": {
			Summary:    "Terminate EC2 instances.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ instanceIds: string[] }", Desc: "instanceIds: the instance ids to terminate."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 TerminateInstances response — TerminatingInstances (array of { InstanceId, CurrentState, PreviousState }).",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).ec2().terminateInstances({ instanceIds: ["i-0123456789abcdef0"] });`,
		},
		"aws.ec2.startInstances": {
			Summary:    "Start stopped EC2 instances.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ instanceIds: string[] }", Desc: "instanceIds: the instance ids to start."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 StartInstances response — StartingInstances (array of { InstanceId, CurrentState, PreviousState }).",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).ec2().startInstances({ instanceIds: ["i-0123456789abcdef0"] });`,
		},
		"aws.ec2.stopInstances": {
			Summary:    "Stop running EC2 instances.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ instanceIds: string[] }", Desc: "instanceIds: the instance ids to stop."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 StopInstances response — StoppingInstances (array of { InstanceId, CurrentState, PreviousState }).",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).ec2().stopInstances({ instanceIds: ["i-0123456789abcdef0"] });`,
		},
		"aws.ec2.describeVolumes": {
			Summary:    "Describe EBS volumes, optionally filtered to specific volume ids.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ volumeIds?: string[] }", Optional: true, Desc: "volumeIds: filter to specific volume ids; omitted or empty ⇒ describe every volume visible to the caller."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 DescribeVolumes response — Volumes (array of EBS volume descriptions).",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).ec2().describeVolumes();`,
		},
		"aws.ec2.describeAvailabilityZones": {
			Summary:    "List the availability zones available in the configured region.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — describeAvailabilityZones takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the EC2 DescribeAvailabilityZones response — AvailabilityZones (array of { ZoneName, State, RegionName, ... }).",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).ec2().describeAvailabilityZones();`,
		},

		// --- iam() — IAM users, roles, and policies (github.com/aws/aws-sdk-go-v2/service/iam) ---

		"aws.iam": {
			Summary: "IAM — users, roles, and policies (github.com/aws/aws-sdk-go-v2/service/iam). Reached via `cloud.aws({...}).iam()`; each method below is called on that service handle.",
		},
		"aws.iam.listUsers": {
			Summary:    "List the IAM users in the account.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listUsers takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM ListUsers response — Users (array of { UserName, Arn, UserId, CreateDate, ... }), IsTruncated, Marker.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).iam().listUsers();`,
		},
		"aws.iam.getUser": {
			Summary:    "Get an IAM user's metadata.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ userName?: string }", Optional: true, Desc: "userName: the IAM user to look up; omitted ⇒ the user making the request (the credentials' own identity)."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM GetUser response — User: { UserName, Arn, UserId, CreateDate, ... }.",
			Errors:     awsErrors,
			Example:    `const me = await cloud.aws({ region: "eu-north-1" }).iam().getUser();`,
		},
		"aws.iam.listRoles": {
			Summary:    "List the IAM roles in the account.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listRoles takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM ListRoles response — Roles (array of { RoleName, Arn, RoleId, AssumeRolePolicyDocument, ... }).",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).iam().listRoles();`,
		},
		"aws.iam.getRole": {
			Summary:    "Get an IAM role's metadata.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ roleName: string }", Desc: "roleName: the role's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM GetRole response — Role: { RoleName, Arn, RoleId, AssumeRolePolicyDocument, ... }.",
			Errors:     awsErrors,
			Example:    `const role = await cloud.aws({ region: "eu-north-1" }).iam().getRole({ roleName: "my-role" });`,
		},
		"aws.iam.listPolicies": {
			Summary:    "List the customer-managed and AWS-managed IAM policies visible to the account.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listPolicies takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM ListPolicies response — Policies (array of { PolicyName, Arn, PolicyId, ... }).",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).iam().listPolicies();`,
		},
		"aws.iam.createUser": {
			Summary:    "Create an IAM user.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ userName: string }", Desc: "userName: the new user's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the IAM CreateUser response — User: the newly created user's description.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).iam().createUser({ userName: "new-user" });`,
		},
		"aws.iam.deleteUser": {
			Summary:    "Delete an IAM user.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ userName: string }", Desc: "userName: the user to delete."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).iam().deleteUser({ userName: "new-user" });`,
		},
		"aws.iam.attachUserPolicy": {
			Summary: "Attach a managed IAM policy to a user.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ userName: string; policyArn: string }", Desc: "userName: the user to attach the policy to. policyArn: the managed policy's ARN."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).iam().attachUserPolicy({ userName: "new-user", policyArn: "arn:aws:iam::aws:policy/ReadOnlyAccess" });`,
		},

		// --- secretsmanager() — Secrets Manager secret containers and values (github.com/aws/aws-sdk-go-v2/service/secretsmanager) ---

		"aws.secretsmanager": {
			Summary: "Secrets Manager — secret containers and their values (github.com/aws/aws-sdk-go-v2/service/secretsmanager). Reached via `cloud.aws({...}).secretsmanager()`; each method below is called on that service handle.",
		},
		"aws.secretsmanager.listSecrets": {
			Summary:    "List the secrets in the account (metadata only — not their values).",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listSecrets takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secrets Manager ListSecrets response — SecretList (array of { Name, ARN, Description, ... }), NextToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).secretsmanager().listSecrets();`,
		},
		"aws.secretsmanager.describeSecret": {
			Summary:    "Get a secret's metadata (not its value — use getSecretValue for that).",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ secretId: string }", Desc: "secretId: the secret's name or ARN."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secrets Manager DescribeSecret response — ARN, Name, Description, VersionIdsToStages, ... (metadata only, not the value).",
			Errors:     awsErrors,
			Example:    `const secret = await cloud.aws({ region: "eu-north-1" }).secretsmanager().describeSecret({ secretId: "db-password" });`,
		},
		"aws.secretsmanager.createSecret": {
			Summary: "Create a secret, optionally with an initial value.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ name: string; secretString?: string }", Desc: "name: the new secret's name. secretString: an optional initial plaintext value; omitted ⇒ the secret is created with no value."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secrets Manager CreateSecret response — ARN, Name, VersionId.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).secretsmanager().createSecret({ name: "db-password", secretString: "s3cr3t" });`,
		},
		"aws.secretsmanager.getSecretValue": {
			Summary:    "Get a secret's current plaintext value.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ secretId: string }", Desc: "secretId: the secret's name or ARN."}},
			ReturnType: "Promise<{ value: string }>",
			Returns:    "A promise resolving to { value } — the decoded secret value. SecretString is returned verbatim when present; otherwise SecretBinary's raw bytes are converted to a string as-is (no further base64 decoding — the SDK already decodes the wire-format blob for both fields).",
			Errors:     awsErrors,
			Example: `const { value } = await cloud.aws({ region: "eu-north-1" }).secretsmanager().getSecretValue({ secretId: "db-password" });
runtime.log(value);`,
		},
		"aws.secretsmanager.putSecretValue": {
			Summary: "Add a new value to an existing secret (creates a new version).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ secretId: string; secretString: string }", Desc: "secretId: the secret's name or ARN. secretString: the new plaintext value."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Secrets Manager PutSecretValue response — ARN, Name, VersionId, VersionStages.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).secretsmanager().putSecretValue({ secretId: "db-password", secretString: "n3w-s3cr3t" });`,
		},
		"aws.secretsmanager.deleteSecret": {
			Summary:    "Delete a secret.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ secretId: string }", Desc: "secretId: the secret's name or ARN."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).secretsmanager().deleteSecret({ secretId: "db-password" });`,
		},

		// --- sts() — STS caller identity and temporary credentials (github.com/aws/aws-sdk-go-v2/service/sts) ---

		"aws.sts": {
			Summary: "STS — caller identity and temporary credentials (github.com/aws/aws-sdk-go-v2/service/sts). Reached via `cloud.aws({...}).sts()`; each method below is called on that service handle.",
		},
		"aws.sts.getCallerIdentity": {
			Summary:    "Get the identity behind the credentials currently in use.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — getCallerIdentity takes no parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the STS GetCallerIdentity response — Account, Arn, UserId. Contains no secrets.",
			Errors:     awsErrors,
			Example:    `const who = await cloud.aws({ region: "eu-north-1" }).sts().getCallerIdentity({});`,
		},
		"aws.sts.assumeRole": {
			Summary: "Assume an IAM role, returning temporary credentials.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ roleArn: string; roleSessionName: string; durationSeconds?: number }", Desc: "roleArn: the ARN of the role to assume. roleSessionName: an identifier for the assumed-role session (shows up in CloudTrail). durationSeconds: session duration in seconds (AWS minimum 900); omitted ⇒ the API's own default (3600)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the STS AssumeRole response — Credentials: { AccessKeyId, SecretAccessKey, SessionToken, Expiration } plus AssumedRoleUser. These are temporary but sensitive credentials — handle/store them with the same care as long-lived ones; never log them.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sts().assumeRole({ roleArn: "arn:aws:iam::123456789012:role/my-role", roleSessionName: "my-session" });`,
		},
		"aws.sts.getSessionToken": {
			Summary:    "Get temporary credentials for the current principal.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ durationSeconds?: number }", Optional: true, Desc: "durationSeconds: session duration in seconds; omitted ⇒ the API's own default (43200 / 12h for IAM-user credentials; 3600 / 1h for root-account credentials)."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the STS GetSessionToken response — Credentials: { AccessKeyId, SecretAccessKey, SessionToken, Expiration }. Same sensitivity note as assumeRole applies.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sts().getSessionToken();`,
		},

		// --- lambda() — Lambda functions and invocations (github.com/aws/aws-sdk-go-v2/service/lambda) ---

		"aws.lambda": {
			Summary: "Lambda — functions and invocations (github.com/aws/aws-sdk-go-v2/service/lambda). Reached via `cloud.aws({...}).lambda()`; each method below is called on that service handle.",
		},
		"aws.lambda.listFunctions": {
			Summary:    "List the Lambda functions in the account/region.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "opts: unused — listFunctions takes no filtering parameters; accepts only an empty object or omission."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Lambda ListFunctions response — Functions (array of { FunctionName, FunctionArn, Runtime, Handler, ... }), NextMarker.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).lambda().listFunctions();`,
		},
		"aws.lambda.getFunction": {
			Summary:    "Get a Lambda function's configuration and code location.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ functionName: string }", Desc: "functionName: the function's name or ARN."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Lambda GetFunction response — Configuration ({ FunctionName, Runtime, Handler, ... }), Code ({ Location, RepositoryType }), Tags, Concurrency.",
			Errors:     awsErrors,
			Example:    `const fn = await cloud.aws({ region: "eu-north-1" }).lambda().getFunction({ functionName: "my-fn" });`,
		},
		"aws.lambda.invoke": {
			Summary: "Synchronously invoke a Lambda function.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ functionName: string; payload?: string | Record<string, unknown> }", Desc: "functionName: the function's name or ARN. payload: the invocation payload — a string sent as-is, or a plain object JSON-serialised before sending; omitted ⇒ no payload."},
			},
			ReturnType: "Promise<{ statusCode: number; payload: string; functionError?: string; executedVersion?: string }>",
			Returns:    "A promise resolving to a hand-built shape (not the raw SDK output, since Payload is raw bytes there): statusCode is the Lambda invocation's HTTP status; payload is the invoked function's raw JSON response body, always returned as a string (never JSON-parsed for you — call JSON.parse yourself); functionError and executedVersion are present only when the SDK response set them.",
			Errors:     awsErrors,
			Example: `const r = await cloud.aws({ region: "eu-north-1" }).lambda().invoke({ functionName: "my-fn", payload: { hello: "world" } });
runtime.log(JSON.parse(r.payload));`,
		},
		"aws.lambda.createFunction": {
			Summary: "Create a Lambda function, from a zip file uploaded inline or a reference to one already in S3.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ functionName: string; role: string; runtime: string; handler: string; zipFile?: string | Uint8Array | ArrayBuffer; s3Bucket?: string; s3Key?: string }", Desc: "functionName: the new function's name. role: the execution role's ARN. runtime: e.g. \"nodejs20.x\", \"provided.al2023\". handler: the entry point, e.g. \"index.handler\". zipFile: the deployment package's bytes (string encoded as UTF-8, or Uint8Array/ArrayBuffer) uploaded inline. s3Bucket/s3Key: alternative to zipFile — reference an existing deployment package already uploaded to S3. Only the code-source field(s) actually supplied are attached to the request."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the Lambda CreateFunction response, which carries the same fields as a function configuration (FunctionName, FunctionArn, Runtime, Role, Handler, CodeSha256, Version, ...).",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).lambda().createFunction({ functionName: "my-fn", role: "arn:aws:iam::123456789012:role/lambda-role", runtime: "nodejs20.x", handler: "index.handler", s3Bucket: "my-bucket", s3Key: "my-fn.zip" });`,
		},
		"aws.lambda.deleteFunction": {
			Summary:    "Delete a Lambda function.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ functionName: string }", Desc: "functionName: the function's name or ARN."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).lambda().deleteFunction({ functionName: "my-fn" });`,
		},

		// --- sqs() — SQS queues and messages (github.com/aws/aws-sdk-go-v2/service/sqs) ---

		"aws.sqs": {
			Summary: "SQS — queues and messages (github.com/aws/aws-sdk-go-v2/service/sqs). Reached via `cloud.aws({...}).sqs()`; each method below is called on that service handle.",
		},
		"aws.sqs.listQueues": {
			Summary:    "List queue URLs, optionally filtered by name prefix.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ prefix?: string }", Optional: true, Desc: "prefix: only list queues whose name starts with this string (sent as QueueNamePrefix); omitted ⇒ list all queues."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SQS ListQueues response — QueueUrls (array of queue URL strings), NextToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sqs().listQueues({ prefix: "orders-" });`,
		},
		"aws.sqs.createQueue": {
			Summary:    "Create a standard or FIFO SQS queue.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ queueName: string }", Desc: "queueName: the new queue's name (append \".fifo\" for a FIFO queue)."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SQS CreateQueue response — QueueUrl: the new queue's URL.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sqs().createQueue({ queueName: "my-queue" });`,
		},
		"aws.sqs.deleteQueue": {
			Summary:    "Delete a queue.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ queueUrl: string }", Desc: "queueUrl: the queue's URL (as returned by createQueue/listQueues)."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).sqs().deleteQueue({ queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue" });`,
		},
		"aws.sqs.sendMessage": {
			Summary: "Send a message to a queue.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ queueUrl: string; messageBody: string }", Desc: "queueUrl: the queue's URL. messageBody: the message text."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SQS SendMessage response — MessageId, MD5OfMessageBody, and (for FIFO queues) SequenceNumber.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).sqs().sendMessage({ queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", messageBody: "hello" });`,
		},
		"aws.sqs.receiveMessage": {
			Summary: "Receive (poll for) messages from a queue.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ queueUrl: string; maxMessages?: number }", Desc: "queueUrl: the queue's URL. maxMessages: the maximum number of messages to return (1-10); omitted ⇒ the API's own default (1)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SQS ReceiveMessage response — Messages (array of { MessageId, ReceiptHandle, Body, MD5OfBody, ... }); absent/empty when the queue currently has no visible messages.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sqs().receiveMessage({ queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", maxMessages: 5 });`,
		},
		"aws.sqs.deleteMessage": {
			Summary: "Delete a message from a queue after successful processing.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ queueUrl: string; receiptHandle: string }", Desc: "queueUrl: the queue's URL. receiptHandle: the receipt handle returned by receiveMessage for this message (not the MessageId)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     awsErrors,
			Example:    `await cloud.aws({ region: "eu-north-1" }).sqs().deleteMessage({ queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", receiptHandle: r.Messages[0].ReceiptHandle });`,
		},
		"aws.sqs.getQueueAttributes": {
			Summary: "Get a queue's attributes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ queueUrl: string; attributeNames?: string[] }", Desc: "queueUrl: the queue's URL. attributeNames: which attributes to fetch (e.g. \"All\", \"ApproximateNumberOfMessages\"); omitted ⇒ none are requested."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SQS GetQueueAttributes response — Attributes: a flat map of attribute name to string value.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).sqs().getQueueAttributes({ queueUrl: "https://sqs.eu-north-1.amazonaws.com/123456789012/my-queue", attributeNames: ["All"] });`,
		},

		// --- cloudwatch() — CloudWatch metrics and alarms (github.com/aws/aws-sdk-go-v2/service/cloudwatch) ---

		"aws.cloudwatch": {
			Summary: "CloudWatch — metrics and alarms (github.com/aws/aws-sdk-go-v2/service/cloudwatch). Reached via `cloud.aws({...}).cloudwatch()`; each method below is called on that service handle.",
		},
		"aws.cloudwatch.listMetrics": {
			Summary: "List published CloudWatch metrics, optionally filtered by namespace/metric name.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ namespace?: string; metricName?: string }", Optional: true, Desc: "namespace: e.g. \"AWS/EC2\"; omitted ⇒ all namespaces. metricName: filter to one metric name within the namespace; omitted ⇒ all metrics."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch ListMetrics response — Metrics (array of { Namespace, MetricName, Dimensions }), NextToken, OwningAccounts.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatch().listMetrics({ namespace: "AWS/EC2" });`,
		},
		"aws.cloudwatch.getMetricData": {
			Summary: "Fetch datapoints for one or more metrics via CloudWatch's metric-math query interface. Pass-through method: the argument is forwarded to the SDK's GetMetricDataInput as-is.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "Record<string, unknown>", Desc: "opts: an AWS-SDK-shaped object with PascalCase keys matching GetMetricDataInput (e.g. { MetricDataQueries: [{ Id, MetricStat: { Metric: { Namespace, MetricName, Dimensions }, Period, Stat } }], StartTime, EndTime }) — JSON round-tripped straight into the Go SDK input struct: RFC3339 timestamp strings unmarshal into the struct's *time.Time fields."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch GetMetricData response — MetricDataResults (array of { Id, Label, Timestamps, Values, StatusCode }), Messages, NextToken.",
			Errors:     awsErrors,
			Example: `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatch().getMetricData({
  MetricDataQueries: [{ Id: "m1", MetricStat: { Metric: { Namespace: "AWS/EC2", MetricName: "CPUUtilization" }, Period: 300, Stat: "Average" } }],
  StartTime: "2026-07-13T00:00:00Z", EndTime: "2026-07-13T01:00:00Z",
});`,
		},
		"aws.cloudwatch.getMetricStatistics": {
			Summary: "Fetch aggregated statistics for a single metric. Pass-through method: the argument is forwarded to the SDK's GetMetricStatisticsInput as-is.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "Record<string, unknown>", Desc: "opts: an AWS-SDK-shaped object with PascalCase keys matching GetMetricStatisticsInput (e.g. { Namespace, MetricName, StartTime, EndTime, Period, Statistics: [\"Average\"], Dimensions? }) — JSON round-tripped straight into the Go SDK input struct."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch GetMetricStatistics response — Datapoints (array of { Timestamp, Average, Sum, Minimum, Maximum, SampleCount, Unit }), Label.",
			Errors:     awsErrors,
			Example: `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatch().getMetricStatistics({
  Namespace: "AWS/EC2", MetricName: "CPUUtilization", StartTime: "2026-07-13T00:00:00Z", EndTime: "2026-07-13T01:00:00Z", Period: 300, Statistics: ["Average"],
});`,
		},
		"aws.cloudwatch.describeAlarms": {
			Summary:    "Describe CloudWatch alarms, optionally filtered to specific alarm names.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ alarmNames?: string[] }", Optional: true, Desc: "alarmNames: filter to specific alarm names; omitted or empty ⇒ describe every alarm."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch DescribeAlarms response — MetricAlarms (array of alarm descriptions), CompositeAlarms.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatch().describeAlarms();`,
		},
		"aws.cloudwatch.putMetricData": {
			Summary: "Publish custom metric datapoints. Pass-through method: the argument is forwarded to the SDK's PutMetricDataInput as-is.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "Record<string, unknown>", Desc: "opts: an AWS-SDK-shaped object with PascalCase keys matching PutMetricDataInput (e.g. { Namespace, MetricData: [{ MetricName, Value, Unit, Timestamp, Dimensions? }] }) — JSON round-tripped straight into the Go SDK input struct."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success — PutMetricData's response carries no payload beyond request metadata.",
			Errors:     awsErrors,
			Example: `await cloud.aws({ region: "eu-north-1" }).cloudwatch().putMetricData({
  Namespace: "MyApp", MetricData: [{ MetricName: "QueueDepth", Value: 42, Unit: "Count" }],
});`,
		},

		// --- cloudwatchlogs() — CloudWatch Logs groups, streams, and Insights queries (github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs) ---

		"aws.cloudwatchlogs": {
			Summary: "CloudWatch Logs — log groups, streams, and Logs Insights queries (github.com/aws/aws-sdk-go-v2/service/cloudwatchlogs). Reached via `cloud.aws({...}).cloudwatchlogs()`; each method below is called on that service handle.",
		},
		"aws.cloudwatchlogs.describeLogGroups": {
			Summary:    "List log groups, optionally filtered by name prefix.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ prefix?: string }", Optional: true, Desc: "prefix: only list log groups whose name starts with this string (sent as LogGroupNamePrefix); omitted ⇒ list all log groups."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs DescribeLogGroups response — LogGroups (array of { LogGroupName, Arn, CreationTime, RetentionInDays, ... }), NextToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().describeLogGroups({ prefix: "/aws/lambda/" });`,
		},
		"aws.cloudwatchlogs.describeLogStreams": {
			Summary:    "List the log streams within a log group.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ logGroupName: string }", Desc: "logGroupName: the log group's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs DescribeLogStreams response — LogStreams (array of { LogStreamName, Arn, CreationTime, FirstEventTimestamp, LastEventTimestamp, ... }), NextToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().describeLogStreams({ logGroupName: "/aws/lambda/my-fn" });`,
		},
		"aws.cloudwatchlogs.getLogEvents": {
			Summary: "Fetch log events from a specific log stream, in chronological order.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ logGroupName: string; logStreamName: string; limit?: number }", Desc: "logGroupName: the log group's name. logStreamName: the log stream's name. limit: max number of log events to return; omitted ⇒ the API's own default."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs GetLogEvents response — Events (array of { Timestamp, Message, IngestionTime }), NextForwardToken, NextBackwardToken. Timestamps here are epoch milliseconds.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().getLogEvents({ logGroupName: "/aws/lambda/my-fn", logStreamName: "2026/07/13/[$LATEST]abc123" });`,
		},
		"aws.cloudwatchlogs.filterLogEvents": {
			Summary: "Search log events across all (or filtered) streams in a log group.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ logGroupName: string; filterPattern?: string }", Desc: "logGroupName: the log group's name. filterPattern: a CloudWatch Logs filter pattern to restrict matching events; omitted ⇒ all events."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs FilterLogEvents response — Events (array of { LogStreamName, Timestamp, Message, IngestionTime, EventId }), SearchedLogStreams, NextToken.",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().filterLogEvents({ logGroupName: "/aws/lambda/my-fn", filterPattern: "ERROR" });`,
		},
		"aws.cloudwatchlogs.startQuery": {
			Summary: "Start a CloudWatch Logs Insights query. Returns immediately with a query id — poll getQueryResults for the results.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ logGroupName: string; queryString: string; startTime: number; endTime: number }", Desc: "logGroupName: the log group to query. queryString: the Logs Insights query. startTime/endTime: the query's time range, in epoch seconds (unlike getLogEvents/filterLogEvents, whose time fields are epoch milliseconds)."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs StartQuery response — QueryId: pass it to getQueryResults to poll for the query's results.",
			Errors:     awsErrors,
			Example: `const { QueryId } = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().startQuery({
  logGroupName: "/aws/lambda/my-fn", queryString: "fields @timestamp, @message | filter @message like /ERROR/",
  startTime: Math.floor(Date.now() / 1000) - 3600, endTime: Math.floor(Date.now() / 1000),
});`,
		},
		"aws.cloudwatchlogs.getQueryResults": {
			Summary:    "Poll for a Logs Insights query's results and status.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ queryId: string }", Desc: "queryId: the id returned by startQuery."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the CloudWatch Logs GetQueryResults response — Results (array of arrays of { Field, Value } — one inner array per matched log record), Statistics, Status (e.g. \"Scheduled\", \"Running\", \"Complete\").",
			Errors:     awsErrors,
			Example:    `const r = await cloud.aws({ region: "eu-north-1" }).cloudwatchlogs().getQueryResults({ queryId: QueryId });`,
		},

		// --- azure — Microsoft Azure provider (Tasks 1-8) — PROVISIONAL ---
		//
		// PROVISIONAL: built against the azure-sdk-for-go modules pinned in
		// go.mod (azcore/azidentity + armresources/armcompute/azblob/
		// azsecrets) and exercised only by httptest-mocked Go unit tests
		// (cloud_azure*_test.go) — there is no live Azure account available in
		// this environment, so none of the wire behaviour documented below has
		// been verified against a real Azure account. Treat any claim about
		// actual Azure REST/SDK response shapes as unverified until a
		// maintainer runs examples/scripts/cloud-azure-smoke.ts (see Task 9)
		// against a real subscription.
		//
		// The "azure" entry's ReturnType (azureHandleType above) supplies the
		// deep typing for the emitted .d.ts (the emitter can't introspect the
		// runtime-built resourceGroups()/compute()/…/blob()/keyvaultSecrets()
		// handles). The flat per-service and per-method entries that follow
		// drive the MANUAL §17 reference instead (rendered via
		// writeOrphanChildren — see cloudDocs's doc comment); both derive from
		// the same cloud_azure_*.go implementations, so the composite overview
		// and the broken-out entries always agree. All are PROVISIONAL — see the
		// PROVISIONAL note above; every azure Example is illustrative only and
		// has not been run against a live account.
		"azure": {
			Summary:    "PROVISIONAL — built against the Azure SDK but not yet verified against a live Azure account. Microsoft Azure provider. cloud.azure(opts?) returns a handle with three ARM (Resource Manager) services — resourceGroups, compute, resources — plus a generic ARM call() REST escape hatch, and two data-plane services — blob, keyvaultSecrets — that take an endpoint URL directly. Pure-Go, CGO-free (azure-sdk-for-go); reuses the standard Azure credential chain.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ subscriptionId?: string; tenantId?: string; clientId?: string; clientSecret?: string }", Optional: true, Desc: "subscriptionId: the ARM subscription id used by the ARM services resourceGroups()/compute()/resources(); omitted ⇒ falls back to the AZURE_SUBSCRIPTION_ID env var (required only when an ARM service is actually invoked — call() targets management.azure.com with the subscription embedded in its path, and blob()/keyvaultSecrets() operate on a caller-supplied endpoint URL, so none of those need a configured subscription). tenantId/clientId/clientSecret: together select a client-secret (service-principal) credential; when any is omitted, falls back to DefaultAzureCredential (environment variables, managed identity, az login, and the other links in the default chain)."}},
			ReturnType: azureHandleType,
			Returns:    "The Azure provider handle: { resourceGroups(), compute(), resources(), call(opts), blob(accountUrl), keyvaultSecrets(vaultUrl) }. resourceGroups()/compute()/resources() return fresh ARM service handles bound to this call's subscription/credential; call() is the generic ARM REST escape hatch for APIs without a typed service above; blob(accountUrl)/keyvaultSecrets(vaultUrl) return data-plane handles bound directly to the given endpoint URL, independent of any subscription.",
			Errors:     "cloud.azure(opts) itself throws synchronously (not a rejected promise) only if opts is provided but is not a plain object — there is no further synchronous validation of the credential fields (a bad/incomplete tenantId/clientId/clientSecret combination fails later, asynchronously, on first credential use). Every service method (ARM and data-plane alike) returns a promise that rejects with a structured Error { code, status, message, details } on API/transport failure — code/status are \"\"/0 for non-API errors (DNS, TLS, timeout, connection refused, credential/token acquisition failure). resourceGroups()/compute()/resources() additionally reject if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID); call(), blob() and keyvaultSecrets() have no such requirement — call() embeds the subscription in its path and targets management.azure.com, while blob()/keyvaultSecrets() operate directly against the caller-supplied accountUrl/vaultUrl.",
			Example: `// PROVISIONAL example — illustrative only, not run against a live account.
const az = cloud.azure({ subscriptionId: "00000000-0000-0000-0000-000000000000" });
const groups = await az.resourceGroups().list();
runtime.log(groups.value?.length ?? 0);

const kv = az.keyvaultSecrets("https://my-vault.vault.azure.net");
const { value } = await kv.getSecret({ name: "db-password" });`,
		},

		// --- resourceGroups() — Resource Groups (armresources.ResourceGroupsClient) — PROVISIONAL ---

		"azure.resourceGroups": {
			Summary: "Resource Groups — ARM (subscription-scoped) container objects for organizing resources (azure-sdk-for-go armresources.ResourceGroupsClient). Reached via `cloud.azure({...}).resourceGroups()`; each method below is called on that service handle.",
		},
		"azure.resourceGroups.list": {
			Summary:    "List every resource group in the subscription, paging through every page the ARM API returns.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "Unused — list ignores any options object entirely; the subscription is that of the parent cloud.azure(...) handle."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — the ARM list envelope wrapping every resource group, each in the ARM SDK's own JSON shape (lowercase-camelCase keys: id, name, location, properties, tags, ...).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example: `const az = cloud.azure({ subscriptionId: "00000000-0000-0000-0000-000000000000" });
const r = await az.resourceGroups().list();
runtime.log(r.value?.length ?? 0);`,
		},
		"azure.resourceGroups.get": {
			Summary:    "Fetch a single resource group by name.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ name: string }", Desc: "name: the resource group's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the resource group's ARM JSON representation (lowercase-camelCase keys: id, name, location, properties, tags, ...).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `const rg = await cloud.azure({ subscriptionId: "..." }).resourceGroups().get({ name: "my-rg" });`,
		},
		"azure.resourceGroups.create": {
			Summary: "Create a resource group, or update it if one with this name already exists (ARM's CreateOrUpdate semantics).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ name: string; location: string }", Desc: "name: the resource group's name. location: the Azure region, e.g. \"westeurope\"."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the created/updated resource group's ARM JSON representation.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).resourceGroups().create({ name: "my-rg", location: "westeurope" });`,
		},
		"azure.resourceGroups.delete": {
			Summary:    "Delete a resource group and everything in it. This is a long-running ARM operation; the call blocks (polls) until it completes.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ name: string }", Desc: "name: the resource group's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} once the delete operation completes.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).resourceGroups().delete({ name: "my-rg" });`,
		},

		// --- compute() — Virtual Machines (armcompute.VirtualMachinesClient) — PROVISIONAL ---

		"azure.compute": {
			Summary: "Virtual Machines — ARM (subscription-scoped) compute instances (azure-sdk-for-go armcompute.VirtualMachinesClient). Reached via `cloud.azure({...}).compute()`; each method below is called on that service handle.",
		},
		"azure.compute.listVirtualMachines": {
			Summary:    "List virtual machines — scoped to a single resource group when one is given, else every VM in the subscription.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ resourceGroup?: string }", Optional: true, Desc: "resourceGroup: restrict the listing to this resource group; omitted ⇒ subscription-wide (the SDK's NewListAllPager)."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — the ARM list envelope wrapping every matching VM, each in the ARM SDK's own JSON shape (lowercase-camelCase keys: id, name, location, properties, ...).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `const r = await cloud.azure({ subscriptionId: "..." }).compute().listVirtualMachines({ resourceGroup: "my-rg" });`,
		},
		"azure.compute.getVirtualMachine": {
			Summary: "Fetch a single virtual machine's metadata.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceGroup: string; name: string }", Desc: "resourceGroup: the VM's resource group. name: the VM's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the VM's ARM JSON representation (lowercase-camelCase keys: id, name, location, properties, ...).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `const vm = await cloud.azure({ subscriptionId: "..." }).compute().getVirtualMachine({ resourceGroup: "my-rg", name: "web-1" });`,
		},
		"azure.compute.start": {
			Summary: "Start a stopped virtual machine. Long-running ARM operation; the call blocks (polls) until it completes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceGroup: string; name: string }", Desc: "resourceGroup: the VM's resource group. name: the VM's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} once the start operation completes.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).compute().start({ resourceGroup: "my-rg", name: "web-1" });`,
		},
		"azure.compute.powerOff": {
			Summary: "Power off a running virtual machine. The VM stays allocated and keeps incurring compute charges (use deallocate() to stop compute billing); disks remain attached. Long-running ARM operation; the call blocks (polls) until it completes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceGroup: string; name: string }", Desc: "resourceGroup: the VM's resource group. name: the VM's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} once the power-off operation completes.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).compute().powerOff({ resourceGroup: "my-rg", name: "web-1" });`,
		},
		"azure.compute.deallocate": {
			Summary: "Deallocate a virtual machine — releases the compute resources (and their billing) while retaining disks and configuration. Long-running ARM operation; the call blocks (polls) until it completes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceGroup: string; name: string }", Desc: "resourceGroup: the VM's resource group. name: the VM's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} once the deallocate operation completes.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).compute().deallocate({ resourceGroup: "my-rg", name: "web-1" });`,
		},
		"azure.compute.delete": {
			Summary: "Delete a virtual machine. Long-running ARM operation; the call blocks (polls) until it completes.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceGroup: string; name: string }", Desc: "resourceGroup: the VM's resource group. name: the VM's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} once the delete operation completes.",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `await cloud.azure({ subscriptionId: "..." }).compute().delete({ resourceGroup: "my-rg", name: "web-1" });`,
		},

		// --- resources() — Generic Resources (armresources.Client) — PROVISIONAL ---

		"azure.resources": {
			Summary: "Generic Resources — ARM (subscription-scoped) cross-resource-type listing and lookup (azure-sdk-for-go armresources.Client — a distinct client from the resourceGroups() service above). Reached via `cloud.azure({...}).resources()`; each method below is called on that service handle.",
		},
		"azure.resources.listByResourceGroup": {
			Summary:    "List every resource (of any type) in a resource group, paging through every page the ARM API returns.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ resourceGroup: string }", Desc: "resourceGroup: the resource group's name."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — the ARM list envelope wrapping every resource in the group, each in the ARM SDK's own generic-resource JSON shape (lowercase-camelCase keys: id, name, type, location, ...).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example:    `const r = await cloud.azure({ subscriptionId: "..." }).resources().listByResourceGroup({ resourceGroup: "my-rg" });`,
		},
		"azure.resources.getById": {
			Summary: "Fetch a single resource by its fully qualified ARM resource ID. Unlike the typed services above, the generic resources API has no per-resource-type default api-version, so the caller must supply one explicitly.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ resourceId: string; apiVersion: string }", Desc: "resourceId: the resource's fully qualified ARM ID (e.g. \"/subscriptions/.../resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/web-1\"). apiVersion: the ARM api-version to request for this resource type, e.g. \"2023-09-01\"."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the resource's ARM JSON representation (lowercase-camelCase keys).",
			Errors:     azureErrors + " Additionally rejects if no subscription is configured (neither opts.subscriptionId nor AZURE_SUBSCRIPTION_ID).",
			Example: `const vm = await cloud.azure({ subscriptionId: "..." }).resources().getById({
  resourceId: "/subscriptions/.../resourceGroups/my-rg/providers/Microsoft.Compute/virtualMachines/web-1",
  apiVersion: "2023-09-01",
});`,
		},

		// --- call() — generic ARM REST escape hatch — PROVISIONAL ---

		"azure.call": {
			Summary: "Generic path-based REST escape hatch onto the ARM (management.azure.com) API — for ARM APIs without a typed service above (resourceGroups/compute/resources). Authenticates the same way as the parent cloud.azure(...) handle, acquiring a management.azure.com/.default bearer token.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ path: string; apiVersion: string; method?: string; params?: Record<string, string>; body?: unknown }", Desc: "path: request path appended to https://management.azure.com as-is, e.g. \"/subscriptions/{id}/providers/Microsoft.Compute/virtualMachines\" — the caller supplies the subscription segment directly. apiVersion: the ARM api-version query parameter (required — ARM has no default). method: HTTP verb, defaults to \"GET\". params: additional query-string parameters (merged with api-version). body: JSON-serialisable request body; sent with Content-Type: application/json when present."},
			},
			ReturnType: "Promise<unknown>",
			Returns:    "A promise resolving to the decoded JSON response body ({} when the response body is empty).",
			Errors:     azureErrors + " Also rejects if `path` or `apiVersion` is missing/empty, or if body is not JSON-serialisable. Note: call() does NOT require a configured subscription — it targets https://management.azure.com and the caller embeds the subscription segment in `path` directly.",
			Example: `// PROVISIONAL example — illustrative only, not run against a live account.
const az = cloud.azure({ subscriptionId: "00000000-0000-0000-0000-000000000000" });
const vms = await az.call({
  path: "/subscriptions/00000000-0000-0000-0000-000000000000/providers/Microsoft.Compute/virtualMachines",
  apiVersion: "2023-09-01",
});`,
		},

		// --- blob(accountUrl) — Blob Storage (azblob.Client) — data-plane — PROVISIONAL ---

		"azure.blob": {
			Summary: "Blob Storage — containers and blobs on a storage account (azure-sdk-for-go azblob.Client). Data-plane: reached via `cloud.azure({...}).blob(accountUrl)`, where accountUrl is the target storage account's blob endpoint (e.g. https://myaccount.blob.core.windows.net) supplied directly by the caller rather than resolved from the subscription — no subscription is required for these methods. Each method below is called on that service handle.",
		},
		"azure.blob.listContainers": {
			Summary:    "List every container in the storage account, paging through every page the SDK's pager returns.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "Unused — listContainers ignores any options object entirely."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — a list envelope wrapping every container. The Storage Blob SDK's structs carry no JSON tags (the wire protocol is XML, not JSON), so toPlain's JSON round-trip falls back to the Go struct field names verbatim: PascalCase keys (Name, Properties, Deleted, ...) — unlike the ARM services and keyvaultSecrets, which come back lowercase-camelCase.",
			Errors:     azureErrors,
			Example: `const blob = cloud.azure({}).blob("https://myaccount.blob.core.windows.net");
const r = await blob.listContainers();
runtime.log(r.value?.length ?? 0);`,
		},
		"azure.blob.listBlobs": {
			Summary:    "List every blob in a container (flat listing — no hierarchy/delimiter), paging through every page the SDK's pager returns.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ container: string }", Desc: "container: the container's name."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — a list envelope wrapping every blob, with PascalCase keys (Name, Properties, Deleted, ...) — see listContainers for why.",
			Errors:     azureErrors,
			Example:    `const r = await cloud.azure({}).blob("https://myaccount.blob.core.windows.net").listBlobs({ container: "logs" });`,
		},
		"azure.blob.download": {
			Summary: "Download a blob's entire content into memory.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ container: string; blob: string }", Desc: "container: the container's name. blob: the blob's name (path within the container)."},
			},
			ReturnType: "Promise<{ bytes: number[] }>",
			Returns:    "A promise resolving to { bytes } where bytes is a plain JS number[] (byte-value array), NOT a real Uint8Array — wrap it with new Uint8Array(res.bytes) before treating it as binary data (e.g. before fs.writeBytes or further decoding).",
			Errors:     azureErrors,
			Example: `const blob = cloud.azure({}).blob("https://myaccount.blob.core.windows.net");
const res = await blob.download({ container: "logs", blob: "a.txt" });
const bytes = new Uint8Array(res.bytes);
runtime.log(bytes.length);`,
		},
		"azure.blob.upload": {
			Summary: "Upload a blob's content, creating or overwriting it. Bodies up to 256 MiB are sent in a single Put Blob request; larger bodies are split into staged blocks and committed (the SDK's UploadBuffer).",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ container: string; blob: string; body: string | Uint8Array | ArrayBuffer }", Desc: "container: the container's name. blob: the blob's name. body: a string (encoded as UTF-8) or raw bytes (Uint8Array/ArrayBuffer) to upload as the blob's content."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SDK's UploadBuffer response, round-tripped via toPlain (PascalCase keys — see listContainers for why).",
			Errors:     azureErrors,
			Example:    `await cloud.azure({}).blob("https://myaccount.blob.core.windows.net").upload({ container: "logs", blob: "a.txt", body: "hello" });`,
		},
		"azure.blob.deleteBlob": {
			Summary: "Delete a blob.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ container: string; blob: string }", Desc: "container: the container's name. blob: the blob's name."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     azureErrors,
			Example:    `await cloud.azure({}).blob("https://myaccount.blob.core.windows.net").deleteBlob({ container: "logs", blob: "a.txt" });`,
		},

		// --- keyvaultSecrets(vaultUrl) — Key Vault Secrets (azsecrets.Client) — data-plane — PROVISIONAL ---

		"azure.keyvaultSecrets": {
			Summary: "Key Vault Secrets — secret values and their versioned metadata in a vault (azure-sdk-for-go azsecrets.Client). Data-plane: reached via `cloud.azure({...}).keyvaultSecrets(vaultUrl)`, where vaultUrl is the target vault's endpoint (e.g. https://myvault.vault.azure.net) supplied directly by the caller rather than resolved from the subscription — no subscription is required for these methods. Each method below is called on that service handle.",
		},
		"azure.keyvaultSecrets.listSecrets": {
			Summary:    "List every secret's properties in the vault (metadata only — never includes values, matching the SDK's own List operation), paging through every page the SDK's pager returns.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "Record<string, never>", Optional: true, Desc: "Unused — listSecrets ignores any options object entirely."}},
			ReturnType: "Promise<{ value?: Array<Record<string, unknown>> }>",
			Returns:    "A promise resolving to { value } — a list envelope wrapping every secret's properties. Uses the SDK's own MarshalJSON, so keys come back lowercase-camelCase (id, attributes, contentType, managed, tags, ...) — the same convention as the ARM services, unlike blob's PascalCase.",
			Errors:     azureErrors,
			Example: `const kv = cloud.azure({}).keyvaultSecrets("https://my-vault.vault.azure.net");
const r = await kv.listSecrets();
runtime.log(r.value?.length ?? 0);`,
		},
		"azure.keyvaultSecrets.getSecret": {
			Summary:    "Fetch the latest version of a secret and its decoded plaintext value.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ name: string }", Desc: "name: the secret's name."}},
			ReturnType: "Promise<{ value: string }>",
			Returns:    "A promise resolving to { value } — the secret's current plaintext value (already decoded; never the raw wire form). The value is never logged anywhere in this path.",
			Errors:     azureErrors,
			Example: `const kv = cloud.azure({}).keyvaultSecrets("https://my-vault.vault.azure.net");
const { value } = await kv.getSecret({ name: "db-password" });
runtime.log(value);`,
		},
		"azure.keyvaultSecrets.setSecret": {
			Summary: "Create a new version of a secret with the given value.",
			Params: []scriptengine.Param{
				{Name: "opts", Type: "{ name: string; value: string }", Desc: "name: the secret's name. value: the plaintext value to store; never logged."},
			},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to the SDK's SetSecret response, round-tripped via toPlain (lowercase-camelCase keys: id, attributes, ... — same convention as the ARM services).",
			Errors:     azureErrors,
			Example:    `await cloud.azure({}).keyvaultSecrets("https://my-vault.vault.azure.net").setSecret({ name: "db-password", value: "s3cr3t" });`,
		},
		"azure.keyvaultSecrets.deleteSecret": {
			Summary:    "Delete a secret and all of its versions.",
			Params:     []scriptengine.Param{{Name: "opts", Type: "{ name: string }", Desc: "name: the secret's name."}},
			ReturnType: "Promise<Record<string, unknown>>",
			Returns:    "A promise resolving to {} on success.",
			Errors:     azureErrors,
			Example:    `await cloud.azure({}).keyvaultSecrets("https://my-vault.vault.azure.net").deleteSecret({ name: "db-password" });`,
		},
	}
}
