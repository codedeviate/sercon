#!/usr/bin/env sercon
// Live smoke test for the cloud.aws(...) surface — run against a REAL AWS
// account you can read. NOT part of `make demo`: it needs live credentials
// and touches a real account, so it is intentionally excluded from
// Makefile's DEMO_SCRIPTS (which is entirely offline/mock-driven). This
// script is maintainer-run; see README-cloud.md for the credential setup.
// Self-skips (no runtime.exit() — it does not exist in this sercon build)
// without AWS_REGION/AWS_DEFAULT_REGION set, mirroring cloud-google-smoke.ts's
// "self-skips when unavailable" convention.
//
// Needs the standard AWS credential chain — env vars
// (AWS_ACCESS_KEY_ID/AWS_SECRET_ACCESS_KEY/AWS_SESSION_TOKEN), ~/.aws/config
// + ~/.aws/credentials (profiles, SSO), or an attached identity (EC2/ECS/Lambda
// instance role via IMDS) — plus a region. The identity needs (at least)
// read access: sts:GetCallerIdentity, s3:ListAllMyBuckets, and
// logs:DescribeLogGroups.
//
// Usage:
//   AWS_REGION=eu-north-1 ./examples/scripts/cloud-aws-smoke.ts
// or
//   AWS_REGION=eu-north-1 sercon examples/scripts/cloud-aws-smoke.ts
//
// Every call below is read-only — nothing is created, modified, or deleted.
// On failure the error is logged (code/status/message) but NOT rethrown, so
// the process still exits 0 — see the network-tolerant-by-default convention
// (log-and-skip over hard failure) for recon/smoke scripts that hit live
// endpoints.

const region = runtime.env.get("AWS_REGION") ?? runtime.env.get("AWS_DEFAULT_REGION");

if (!region) {
  runtime.log("cloud-aws-smoke: set AWS_REGION=<region> (or AWS_DEFAULT_REGION), plus AWS credentials, to run this. Skipping.");
} else {
  const aws = cloud.aws({ region });

  try {
    // sts() — who are we running as (read-only, no secrets in the response).
    const id = await aws.sts().getCallerIdentity({});
    runtime.log(`sts.getCallerIdentity ok: account ${id.Account}`);

    // s3() — list buckets (read-only).
    const buckets = await aws.s3().listBuckets();
    runtime.log(`s3.listBuckets ok: ${(buckets.Buckets ?? []).length} bucket(s)`);

    // cloudwatchlogs() — list log groups (read-only).
    const groups = await aws.cloudwatchlogs().describeLogGroups({});
    runtime.log(`cloudwatchlogs.describeLogGroups ok: ${(groups.LogGroups ?? []).length} group(s)`);

    runtime.log("cloud-aws-smoke: ALL OK");
  } catch (e: any) {
    runtime.log(`cloud-aws-smoke: FAILED: ${e.message} (code=${e.code} status=${e.status})`);
  }
}
