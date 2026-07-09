#!/usr/bin/env sercon
// Live smoke test for the cloud.google(...) surface (storage/compute/iam/
// secrets + the generic call() escape hatch) — run against a REAL GCP
// project you can read. NOT part of `make demo`: it needs live credentials
// and touches a real project, so it is intentionally excluded from
// Makefile's DEMO_SCRIPTS (which is entirely offline/mock-driven). This
// script is maintainer-run; see README-cloud.md for the credential setup.
// Self-skips (exit 0) without PROJECT set, mirroring the agent-browser-*
// examples' "self-skips when unavailable" convention.
//
// Needs Application Default Credentials — either
//   gcloud auth application-default login
// or GOOGLE_APPLICATION_CREDENTIALS pointed at a service-account JSON key.
// The identity must have (at least) read access on the target project:
//   roles/viewer, or the narrower storage.buckets.list +
//   iam.serviceAccounts.list permissions.
//
// Usage:
//   PROJECT=my-gcp-project ./examples/scripts/cloud-google-smoke.ts
// or
//   PROJECT=my-gcp-project sercon examples/scripts/cloud-google-smoke.ts
//
// A failed API call rethrows after logging code/status/message, so the
// process exits non-zero (sercon maps an uncaught script exception to
// exit code 4 — see exitThrow in cmd/sercon/main.go).

const project = runtime.env.get("PROJECT");

if (!project) {
  runtime.log("cloud-google-smoke: set PROJECT=<gcp-project-id> (and ADC / GOOGLE_APPLICATION_CREDENTIALS) to run this. Skipping.");
} else {
  const g = cloud.google({ project });

  try {
    // storage() — list buckets (read-only).
    const buckets = await g.storage().listBuckets({ project });
    const bucketList = (buckets.items ?? []) as Array<Record<string, unknown>>;
    runtime.log(`storage.listBuckets ok: ${bucketList.length} bucket(s)`);

    // If there's at least one bucket, exercise listObjects too (still
    // read-only) to touch a second storage method.
    if (bucketList.length > 0) {
      const bucketName = String(bucketList[0].name ?? "");
      if (bucketName) {
        const objects = await g.storage().listObjects({ bucket: bucketName });
        runtime.log(`storage.listObjects ok: ${(objects.items ?? []).length} object(s) in "${bucketName}"`);
      }
    }

    // iam() — list service accounts (read-only).
    const sas = await g.iam().listServiceAccounts({ project });
    runtime.log(`iam.listServiceAccounts ok: ${(sas.accounts ?? []).length} service account(s)`);

    // compute() — list zones (read-only; works even with zero VMs).
    const zones = await g.compute().listZones({ project });
    runtime.log(`compute.listZones ok: ${(zones.items ?? []).length} zone(s)`);

    // secrets() — list secrets (read-only).
    const secrets = await g.secrets().listSecrets({ project });
    runtime.log(`secrets.listSecrets ok: ${(secrets.secrets ?? []).length} secret(s)`);

    // call() — generic REST escape hatch, hitting the same zones.list
    // endpoint via the raw path form to prove the fallback works too.
    const rawZones: any = await g.call({ api: "compute", path: `/compute/v1/projects/${project}/zones` });
    runtime.log(`call() ok: ${(rawZones.items ?? []).length} zone(s) via generic call()`);

    runtime.log("cloud-google-smoke: ALL OK");
  } catch (e: any) {
    runtime.log(`cloud-google-smoke: FAILED: ${e.message} (code=${e.code} status=${e.status})`);
    throw e;
  }
}
