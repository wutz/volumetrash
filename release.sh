#!/bin/bash
set -e

cd /Users/wutz/dev/volumetrash

# Step 1: Create volumetrash PR
echo "Creating volumetrash PR..."
PR_URL=$(gh pr create \
  --base main \
  --head feat/volumetrash-operator \
  --title "feat: purge expired trashed volumes" \
  --body "## Summary

- Moves the forgeai volume-trash reconciler into this operator. A trashed PersistentVolumeClaim is purged once its \`forgeai.wutz.dev/expire-at\` annotation has passed.
- Retention is the owning project's \`trashRetention\`, else the platform default read live from the \`forgeai-platform\` ConfigMap (default 7d).
- A volume mounted by an unfinished pod is kept; a finished pod wakes the purge.
- Adds a Helm chart. Pushing a \`v*\` tag publishes \`ghcr.io/wutz/volumetrash\` and \`oci://ghcr.io/wutz/charts/volumetrash\`.

## Verification

- \`make test\`: envtest suite (4 specs) and unit tests for \`internal/trash\` and \`internal/config\`.
- \`make lint\`: 0 issues.
- \`make helm-lint\` and \`helm template\` (default and \`leaderElection=true\`).

## Not verified here

- The chart's RBAC was checked by lint and render only. It has not been installed on a live cluster. The scaffolded kind e2e in CI covers the operator deployment through \`config/\`, not the Helm chart.

🤖 Generated with [Claude Code](https://claude.com/claude-code)" \
  2>&1)

echo "PR created: $PR_URL"

# Step 2: Enable auto-merge on the PR
echo "Enabling auto-merge..."
PR_NUMBER=$(echo "$PR_URL" | grep -oE '[0-9]+$')
gh pr merge "$PR_NUMBER" --auto --squash

echo "Auto-merge enabled on PR #$PR_NUMBER"

# Wait for PR to merge (poll for up to 5 minutes)
echo "Waiting for PR to merge (max 5 minutes)..."
for i in {1..30}; do
  if gh pr view "$PR_NUMBER" --json state -q '.state' | grep -q "MERGED"; then
    echo "PR merged!"
    break
  fi
  echo "Checking... ($i/30)"
  sleep 10
done

# Step 3: Update main and tag release
echo "Checking out main and pulling latest..."
git checkout main
git pull origin main

echo "Creating and pushing tag v0.1.0..."
git tag v0.1.0
git push origin v0.1.0

echo "Release workflow triggered. Check https://github.com/wutz/volumetrash/actions"
