#!/bin/bash
set -e
cd /Users/wutz/dev/volumetrash

echo "=== Step 1: Create volumetrash PR ==="
gh pr create \
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

## Not verified

- The chart's RBAC was checked by lint and render only. It has not been installed on a live cluster. The scaffolded kind e2e in CI covers the operator deployment through \`config/\`, not the Helm chart.

🤖 Generated with [Claude Code](https://claude.com/claude-code)"

echo ""
echo "=== Step 2: Get PR number and enable auto-merge ==="
PR_NUM=$(gh pr list --head feat/volumetrash-operator --base main --json number -q '.[0].number')
echo "PR number: $PR_NUM"

gh pr merge $PR_NUM --auto --squash

echo ""
echo "=== Step 3: Wait for PR to merge (polling for up to 3 minutes) ==="
for i in {1..18}; do
  STATE=$(gh pr view $PR_NUM --json state -q '.state' 2>/dev/null || echo "")
  if [ "$STATE" = "MERGED" ]; then
    echo "PR merged successfully!"
    break
  fi
  echo "Attempt $i/18: State is $STATE, waiting..."
  sleep 10
done

echo ""
echo "=== Step 4: Checkout main, pull latest, and create release tag ==="
git checkout main
git pull origin main
git tag v0.1.0
git push origin v0.1.0

echo ""
echo "=== Release workflow triggered ==="
echo "Monitor at: https://github.com/wutz/volumetrash/actions"
echo "Image will be published to: ghcr.io/wutz/volumetrash:0.1.0"
echo "Chart will be published to: oci://ghcr.io/wutz/charts/volumetrash:0.1.0"
