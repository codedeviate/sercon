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
