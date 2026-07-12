#!/usr/bin/env sercon
// PROVISIONAL / UNVERIFIED — live smoke test for the cloud.azure(...) surface
// (resourceGroups()/compute()/resources() ARM services, the ARM call()
// escape hatch, and the blob()/keyvaultSecrets() data-plane services) — run
// against a REAL Azure subscription you can read. This script has NEVER been
// run against a live Azure account: there are no Azure credentials available
// in the environment that authored it, so every claim below about what it
// will actually print is unverified. NOT part of `make demo`: it needs live
// credentials and touches a real subscription, so it is intentionally
// excluded from Makefile's DEMO_SCRIPTS (which is entirely offline/
// mock-driven — see cmd/sercon/cloud_azure*_test.go for the httptest-mocked
// coverage that *is* verified). This script is maintainer-run; see
// README-cloud.md for the credential setup.
//
// Self-skips (no runtime.exit() — it does not exist in this sercon build)
// without AZURE_SUBSCRIPTION_ID set, mirroring cloud-aws-smoke.ts's/
// cloud-google-smoke.ts's "self-skips when unavailable" convention.
//
// Needs a subscription id plus a credential. Either:
//   - AZURE_SUBSCRIPTION_ID + AZURE_TENANT_ID + AZURE_CLIENT_ID +
//     AZURE_CLIENT_SECRET (a service-principal / client-secret credential,
//     passed explicitly to cloud.azure({...})), or
//   - AZURE_SUBSCRIPTION_ID alone, relying on DefaultAzureCredential's chain
//     (environment variables in the azidentity-recognized shape, managed
//     identity, az login, etc. — see cloud_azure.go's credential()).
// blob()/keyvaultSecrets() additionally need real endpoint URLs — set
// AZURE_STORAGE_ACCOUNT_URL (e.g. https://myaccount.blob.core.windows.net)
// and AZURE_KEYVAULT_URL (e.g. https://myvault.vault.azure.net) to exercise
// those; both self-skip individually when unset (unlike the subscription
// check, missing endpoint URLs don't skip the whole script since the ARM
// services can still run without them).
//
// Usage:
//   AZURE_SUBSCRIPTION_ID=... AZURE_TENANT_ID=... AZURE_CLIENT_ID=... \
//   AZURE_CLIENT_SECRET=... ./examples/scripts/cloud-azure-smoke.ts
// or
//   AZURE_SUBSCRIPTION_ID=... sercon examples/scripts/cloud-azure-smoke.ts
//
// Every call below is read-only (list/get) — nothing is created, modified,
// or deleted. On failure the error is logged (code/status/message) but NOT
// rethrown, so the process still exits 0 — see the network-tolerant-by-
// default convention (log-and-skip over hard failure) for recon/smoke
// scripts that hit live endpoints.

const subscriptionId = runtime.env.get("AZURE_SUBSCRIPTION_ID");

if (!subscriptionId) {
  runtime.log("cloud-azure-smoke: set AZURE_SUBSCRIPTION_ID=<subscription-id> (plus AZURE_TENANT_ID/AZURE_CLIENT_ID/AZURE_CLIENT_SECRET, or rely on DefaultAzureCredential) to run this. Skipping. [PROVISIONAL: unverified against a live Azure account]");
} else {
  const tenantId = runtime.env.get("AZURE_TENANT_ID");
  const clientId = runtime.env.get("AZURE_CLIENT_ID");
  const clientSecret = runtime.env.get("AZURE_CLIENT_SECRET");

  const az = cloud.azure({
    subscriptionId,
    ...(tenantId ? { tenantId } : {}),
    ...(clientId ? { clientId } : {}),
    ...(clientSecret ? { clientSecret } : {}),
  });

  try {
    // resourceGroups() — list resource groups in the subscription (read-only).
    const groups = await az.resourceGroups().list();
    runtime.log(`resourceGroups.list ok: ${(groups.value ?? []).length} resource group(s)`);

    // compute() — list virtual machines subscription-wide (read-only).
    const vms = await az.compute().listVirtualMachines();
    runtime.log(`compute.listVirtualMachines ok: ${(vms.value ?? []).length} VM(s)`);

    // call() — generic ARM REST escape hatch, hitting the same
    // resourceGroups.list endpoint via the raw path form to prove the
    // fallback works too.
    const rawGroups: any = await az.call({
      path: `/subscriptions/${subscriptionId}/resourcegroups`,
      apiVersion: "2021-04-01",
    });
    runtime.log(`call() ok: ${(rawGroups.value ?? []).length} resource group(s) via generic call()`);

    runtime.log("cloud-azure-smoke: ARM services OK");
  } catch (e: any) {
    runtime.log(`cloud-azure-smoke: ARM services FAILED: ${e.message} (code=${e.code} status=${e.status})`);
  }

  // blob() — data-plane, needs a real storage account URL. Self-skips
  // independently since it has nothing to do with the subscription above.
  const storageAccountUrl = runtime.env.get("AZURE_STORAGE_ACCOUNT_URL");
  if (!storageAccountUrl) {
    runtime.log("cloud-azure-smoke: set AZURE_STORAGE_ACCOUNT_URL=<https://account.blob.core.windows.net> to exercise blob(). Skipping blob().");
  } else {
    try {
      const blob = az.blob(storageAccountUrl);
      const containers = await blob.listContainers();
      runtime.log(`blob.listContainers ok: ${(containers.value ?? []).length} container(s)`);
    } catch (e: any) {
      runtime.log(`cloud-azure-smoke: blob() FAILED: ${e.message} (code=${e.code} status=${e.status})`);
    }
  }

  // keyvaultSecrets() — data-plane, needs a real vault URL. Self-skips
  // independently, same reasoning as blob() above.
  const keyvaultUrl = runtime.env.get("AZURE_KEYVAULT_URL");
  if (!keyvaultUrl) {
    runtime.log("cloud-azure-smoke: set AZURE_KEYVAULT_URL=<https://vault.vault.azure.net> to exercise keyvaultSecrets(). Skipping keyvaultSecrets().");
  } else {
    try {
      const kv = az.keyvaultSecrets(keyvaultUrl);
      const secrets = await kv.listSecrets();
      runtime.log(`keyvaultSecrets.listSecrets ok: ${(secrets.value ?? []).length} secret(s) (metadata only, no values)`);
    } catch (e: any) {
      runtime.log(`cloud-azure-smoke: keyvaultSecrets() FAILED: ${e.message} (code=${e.code} status=${e.status})`);
    }
  }

  runtime.log("cloud-azure-smoke: DONE [PROVISIONAL: this run is the first real signal on whether any of the above actually works against live Azure]");
}
