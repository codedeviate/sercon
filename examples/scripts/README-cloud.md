# `cloud-google-smoke.ts` — live GCP smoke test

`cloud-google-smoke.ts` exercises the real `cloud.google(...)` surface
(`storage()`, `compute()`, `iam()`, `secrets()`, and the generic `call()`
REST escape hatch) against an actual Google Cloud project. It is
**maintainer-run, not part of CI**:

- It is **not** listed in `Makefile`'s `DEMO_SCRIPTS`, so `make demo` never
  runs it.
- Everything else under `examples/scripts/` that talks to Google Cloud is
  covered by `httptest`-mocked unit tests (see `cmd/sercon/cloud_google_*.go`
  and their `_test.go` siblings) — this script is the one piece that needs a
  real project and real credentials, and is meant to be run by hand before a
  release that touches the `cloud` namespace.

## Required credentials

The script authenticates via Google's Application Default Credentials
(ADC), the same mechanism `cloud.google(...)` itself uses when no
`credentials` option is passed. Set up ADC with **one** of:

- **User credentials (local dev):**
  ```
  gcloud auth application-default login
  ```
- **Service account key file:**
  ```
  export GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account.json
  ```
- **Attached identity** (GCE/GKE/Cloud Run metadata server) — nothing to
  set; ADC picks it up automatically when running on GCP infrastructure.

The identity needs read access to the target project — `roles/viewer` is
the simplest grant, or the narrower `storage.buckets.list`,
`compute.zones.list`, `iam.serviceAccounts.list`, and
`secretmanager.secrets.list` permissions if you'd rather scope it down.

## Running it

```
PROJECT=my-gcp-project ./examples/scripts/cloud-google-smoke.ts
```

or, without relying on the shebang / executable bit:

```
PROJECT=my-gcp-project sercon examples/scripts/cloud-google-smoke.ts
```

Without `PROJECT` set, the script logs a message and exits 0 (self-skip —
safe to leave in any script glob). With `PROJECT` set it calls, in order:
`storage().listBuckets()` (+ `listObjects()` on the first bucket, if any),
`iam().listServiceAccounts()`, `compute().listZones()`,
`secrets().listSecrets()`, and finally `call()` against the same
`compute.zones.list` endpoint via the raw path form, to prove the generic
escape hatch works too. Every call is read-only — nothing is created,
modified, or deleted.

On failure it logs the structured error's `message`/`code`/`status` and
re-throws, so the process exits non-zero (sercon maps an uncaught script
exception to exit code 4).

# `cloud-aws-smoke.ts` — live AWS smoke test

`cloud-aws-smoke.ts` exercises the real `cloud.aws(...)` surface
(`sts()`, `s3()`, `cloudwatchlogs()`) against an actual AWS account. It is
**maintainer-run, not part of CI**:

- It is **not** listed in `Makefile`'s `DEMO_SCRIPTS`, so `make demo` never
  runs it.
- Everything else under `examples/scripts/` that talks to AWS is covered by
  `httptest`-mocked unit tests (see `cmd/sercon/cloud_aws_*.go` and their
  `_test.go` siblings) — this script is the one piece that needs a real
  account and real credentials, and is meant to be run by hand before a
  release that touches the `cloud` namespace.

## Required credentials

The script authenticates via the standard AWS credential chain, the same
mechanism `cloud.aws(...)` itself uses when no `credentials` option is
passed. Set up credentials with **one** of:

- **Environment variables:**
  ```
  export AWS_ACCESS_KEY_ID=...
  export AWS_SECRET_ACCESS_KEY=...
  export AWS_SESSION_TOKEN=...   # only if using temporary credentials
  ```
- **Shared config/credentials files** (`~/.aws/config`, `~/.aws/credentials`),
  optionally via a named profile passed as `cloud.aws({ profile: "..." })`.
- **SSO login:**
  ```
  aws sso login --profile my-profile
  ```
- **Attached identity** (EC2 instance role, ECS task role, Lambda execution
  role via IMDS) — nothing to set; the chain picks it up automatically when
  running on AWS infrastructure.

A region is also required — set `AWS_REGION` (or `AWS_DEFAULT_REGION`).

The identity needs read access to the target account — the AWS managed
`ReadOnlyAccess` policy is the simplest grant, or the narrower
`sts:GetCallerIdentity`, `s3:ListAllMyBuckets`, and `logs:DescribeLogGroups`
permissions if you'd rather scope it down.

## Running it

```
AWS_REGION=eu-north-1 ./examples/scripts/cloud-aws-smoke.ts
```

or, without relying on the shebang / executable bit:

```
AWS_REGION=eu-north-1 sercon examples/scripts/cloud-aws-smoke.ts
```

Without `AWS_REGION`/`AWS_DEFAULT_REGION` set, the script logs a message and
exits 0 (self-skip — safe to leave in any script glob; note there is no
`runtime.exit()` in this sercon build, so the skip path is a plain if/else
rather than an early exit call). With a region set it calls, in order:
`sts().getCallerIdentity({})`, `s3().listBuckets()`, and
`cloudwatchlogs().describeLogGroups({})`. Every call is read-only — nothing
is created, modified, or deleted.

On failure it logs the structured error's `message`/`code`/`status` but does
**not** rethrow, so the process still exits 0 — this script follows the
network-tolerant-by-default convention (log-and-skip over hard failure) for
recon/smoke scripts hitting live endpoints.

# `cloud-azure-smoke.ts` — live Azure smoke test — PROVISIONAL / UNVERIFIED

**PROVISIONAL.** `cloud-azure-smoke.ts` exercises the real `cloud.azure(...)`
surface (`resourceGroups()`, `compute()`, the ARM `call()` escape hatch, and
the `blob()`/`keyvaultSecrets()` data-plane services) against an actual Azure
subscription. Unlike its Google/AWS counterparts above, **this script has
never been run against a live Azure account** — there are no Azure
credentials available in the environment it was authored in, so its behavior
against real Azure is unverified. It is **maintainer-run, not part of CI**:

- It is **not** listed in `Makefile`'s `DEMO_SCRIPTS`, so `make demo` never
  runs it.
- Everything else under `examples/scripts/` that talks to Azure is covered
  by `httptest`-mocked unit tests (see `cmd/sercon/cloud_azure*.go` and their
  `_test.go` siblings) — this script is the one piece that would need a real
  subscription and real credentials to validate, and is meant to be run by
  hand — by a maintainer with actual Azure access — before relying on the
  `cloud.azure` surface in production.

## Required credentials

The script authenticates via `cloud.azure(...)`'s own credential
resolution: an explicit client-secret (service-principal) credential when
`tenantId`/`clientId`/`clientSecret` are all supplied, else
`DefaultAzureCredential`'s chain (environment variables, managed identity,
`az login`, and the other links in the default chain). Set up **one** of:

- **Service-principal (client-secret) credential:**
  ```
  export AZURE_SUBSCRIPTION_ID=...
  export AZURE_TENANT_ID=...
  export AZURE_CLIENT_ID=...
  export AZURE_CLIENT_SECRET=...
  ```
- **Azure CLI login (DefaultAzureCredential falls back to it):**
  ```
  az login
  export AZURE_SUBSCRIPTION_ID=...
  ```
- **Attached identity** (VM/App Service/AKS managed identity) — set only
  `AZURE_SUBSCRIPTION_ID`; `DefaultAzureCredential` picks up the attached
  identity automatically.

`blob()`/`keyvaultSecrets()` are data-plane services with no subscription
requirement of their own, but the script needs real endpoint URLs to
exercise them:

```
export AZURE_STORAGE_ACCOUNT_URL=https://myaccount.blob.core.windows.net
export AZURE_KEYVAULT_URL=https://myvault.vault.azure.net
```

The identity needs (at least) read access — Azure's built-in **Reader**
role covers the ARM services (`resourceGroups`, `compute`, `resources`,
`call()`); **Storage Blob Data Reader** and **Key Vault Secrets User** (or
equivalent access-policy grants) cover `blob()`/`keyvaultSecrets()`
respectively.

## Running it

```
AZURE_SUBSCRIPTION_ID=... ./examples/scripts/cloud-azure-smoke.ts
```

or, without relying on the shebang / executable bit:

```
AZURE_SUBSCRIPTION_ID=... sercon examples/scripts/cloud-azure-smoke.ts
```

Without `AZURE_SUBSCRIPTION_ID` set, the script logs a message and exits 0
(self-skip — safe to leave in any script glob; note there is no
`runtime.exit()` in this sercon build, so the skip path is a plain if/else
rather than an early exit call). With a subscription id set it calls, in
order: `resourceGroups().list()`, `compute().listVirtualMachines()`, and
`call()` against the same resource-groups-list endpoint via the raw path
form, to prove the ARM escape hatch works too. `blob()`/`keyvaultSecrets()`
each self-skip independently if their respective endpoint URL env var isn't
set. Every call is read-only (list/get) — nothing is created, modified, or
deleted.

On failure it logs the structured error's `message`/`code`/`status` but does
**not** rethrow, so the process still exits 0 — this script follows the
network-tolerant-by-default convention (log-and-skip over hard failure) for
recon/smoke scripts hitting live endpoints. **Because this script is
untested against real Azure, treat its first real run as the actual
validation of the `cloud.azure` surface — not this documentation.**
