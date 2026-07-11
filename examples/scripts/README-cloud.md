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
