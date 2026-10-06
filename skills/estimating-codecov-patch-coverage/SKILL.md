---
name: estimating-codecov-patch-coverage
description: Use when preparing or reviewing an integration-service pull request that changes Go code and needs a local preflight check against the 85% Codecov patch-coverage policy.
---

# Estimate Codecov Patch Coverage

Use this skill from the repository root before opening or updating a pull request
that changes Go code. The repository's `hack/check_patch_coverage.py` estimates
coverage of the added lines in the patch, not coverage of the whole repository.

1. Run:

   ```bash
   make codecov-estimate
   ```

   This runs the normal test suite to produce a fresh `cover.out`, then estimates
   coverage against the 85% target. Override `CODECOV_BASE` for a pull request
   targeting another branch, or `CODECOV_TARGET` if the policy changes.

2. Review the changed lines listed as uncovered and add tests for those paths.
3. Re-run `make codecov-estimate` after making changes.

Do not proceed with PR preparation when `make test` fails, the checker cannot run,
the checker warns that the local Go version differs from CI, or the estimate is
below 85%. The checker exits 1 below 85% and exits 2 when it cannot produce a
reliable estimate.

If the checker warns about the Go version, install the version specified in
`.github/workflows/pr.yaml` and re-run with it, for example:

```bash
GOTOOLCHAIN=go1.26.0 make codecov-estimate
```

The checker compares the working tree with the merge base of `origin/main` by
default. Use `make codecov-estimate CODECOV_BASE=origin/<branch>` when the pull
request targets a different branch, and ensure the selected base ref is current.
Run it again after the final Go edit; it rejects a coverage profile older than
any changed Go source file.

Without an explicit threshold, the checker reads the target from `codecov.yml`.
The Make target passes 85% explicitly because the repository may temporarily
have a lower informational target while it prepares for enforcement.

Changes with no coverable Go lines are reported as not applicable. Generated,
vendored and other paths listed in `codecov.yml` are excluded from the estimate.
The checker mirrors Codecov's Go coverage-region merging and Go source-line fixes;
it was calibrated against the published result for integration-service PR 1706.
It remains a preflight estimate because Codecov uses the uploaded CI report and
the provider's server-side pull-request diff. The Codecov PR check is authoritative.

## Reading the Codecov result

Codecov's pull-request result is about the changed lines, not the whole
repository. A result such as `70.77% <100.00%> (+0.25%)` means project coverage,
patch coverage, and the project change respectively; the patch value is the one
that will be enforced. A local result at or above 85% increases confidence but is
not a guarantee that Codecov will pass.
