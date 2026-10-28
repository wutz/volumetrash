# volumetrash — Next Steps to Release

The operator is scaffolded, tested, and ready to publish. This file documents the steps to complete from the GitHub web UI.

## Step 1: Create the volumetrash PR

The PR could not be created from the sandbox due to GitHub API network errors. Create it manually:

1. Go to https://github.com/wutz/volumetrash
2. Click "Compare & pull request" (or create a new PR from `feat/volumetrash-operator` to `main`)
3. Use this title and body:

**Title:**
```
feat: purge expired trashed volumes
```

**Body:**
```
## Summary

- Moves the forgeai volume-trash reconciler into this operator. A trashed PersistentVolumeClaim is purged once its `forgeai.wutz.dev/expire-at` annotation has passed.
- Retention is the owning project's `trashRetention`, else the platform default read live from the `forgeai-platform` ConfigMap (default 7d).
- A volume mounted by an unfinished pod is kept; a finished pod wakes the purge.
- Adds a Helm chart. Pushing a `v*` tag publishes `ghcr.io/wutz/volumetrash` and `oci://ghcr.io/wutz/charts/volumetrash`.

## Verification

- `make test`: envtest suite (4 specs) and unit tests for `internal/trash` and `internal/config`.
- `make lint`: 0 issues.
- `make helm-lint` and `helm template` (default and `leaderElection=true`).

## Not verified here

- The chart's RBAC was checked by lint and render only. It has not been installed on a live cluster. The scaffolded kind e2e in CI covers the operator deployment through `config/`, not the Helm chart.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

4. Once created, enable auto-merge (GitHub will offer it once checks pass)

## Step 2: Release volumetrash v0.1.0

Once the PR is merged:

```bash
cd /Users/wutz/dev/volumetrash
git checkout main
git pull origin main
git tag v0.1.0
git push origin v0.1.0
```

This triggers `.github/workflows/release.yml`, which:
- Publishes `ghcr.io/wutz/volumetrash:0.1.0` (multi-platform: amd64, arm64)
- Publishes `oci://ghcr.io/wutz/charts/volumetrash:0.1.0`

Verify in ~5–10 minutes by running:
```bash
helm pull oci://ghcr.io/wutz/charts/volumetrash --version 0.1.0
```

## Step 3: Verify forgeai PR #120

The forgeai PR (#120) is now waiting for checks to pass. Once CI passes (which it should, since:
- `make test` and `make manifests generate` ran locally and passed
- All reconciler references are removed
- RBAC is correctly narrowed

GitHub will auto-merge it (auto-merge was enabled).

Verify at: https://github.com/wutz/forgeai/pull/120

---

## Status Summary

| Component | Status | Notes |
| --- | --- | --- |
| **volumetrash operator** | Ready | Code complete, tested, branch pushed |
| **volumetrash PR** | Blocked | Network error on sandbox; create manually |
| **volumetrash release** | Pending | Tag v0.1.0 after PR merges |
| **forgeai refactor** | Auto-merging | PR #120, awaiting CI checks |

Once the volumetrash chart is published, forgeai can install it from the Dependencies view.
