# Publishing volumetrash v0.1.0

## Prerequisites

- GitHub access restored (API and git transport both working)
- volumetrash PR merged to `main`
- You are in the volumetrash repo directory

## One Command to Publish Everything

```bash
cd /Users/wutz/dev/volumetrash && \
git checkout main && \
git pull origin main && \
git tag v0.1.0 && \
git push origin v0.1.0
```

This single command:
1. Checks out the main branch
2. Pulls the latest code (your merged PR)
3. Creates the v0.1.0 tag locally
4. Pushes the tag to GitHub

## What Happens Automatically

GitHub Actions will immediately trigger `.github/workflows/release.yml`, which:

### Image Build & Push
- Runs `ko build --bare` with `KO_DOCKER_REPO=ghcr.io/wutz/volumetrash`
- Builds multi-platform images: `linux/amd64` and `linux/arm64`
- Tags as: `ghcr.io/wutz/volumetrash:0.1.0`
- Pushes to GitHub Container Registry

### Helm Chart Package & Push
- Runs `make helm-package CHART_VERSION=0.1.0`
  - Packages: `charts/volumetrash/` → `dist/volumetrash-0.1.0.tgz`
  - Sets Chart.yaml version and appVersion to 0.1.0
- Runs `make helm-push CHART_VERSION=0.1.0`
  - Pushes to: `oci://ghcr.io/wutz/charts/volumetrash:0.1.0`

## Verification (5–10 minutes after pushing the tag)

```bash
# Verify the image exists
docker pull ghcr.io/wutz/volumetrash:0.1.0

# Verify the chart exists
helm pull oci://ghcr.io/wutz/charts/volumetrash --version 0.1.0

# Check release page
open https://github.com/wutz/volumetrash/releases/tag/v0.1.0
```

## Why This Works

- The release workflow is triggered **only** by `v*` tags (`on: push: tags: - "v*"`)
- The workflow extracts the version by stripping the `v` prefix: `v0.1.0` → `0.1.0`
- It validates the version matches `MAJOR.MINOR.PATCH` format (0.1.0 ✓)
- Both `ko` and `helm` use the same version, so image and chart versions match

## Next: forgeai Installs from the Published Chart

Once the chart is published to `oci://ghcr.io/wutz/charts/volumetrash:0.1.0`, forgeai can:
- Install via the Dependencies view (when PR #120 is merged)
- Reference the exact chart version in `gui/app/chart.go`: `volumetrashChartVersion = "0.1.0"`
