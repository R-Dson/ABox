#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

workflow=.github/workflows/sync-editors.yml

if grep -q 'delete-workflow-runs' "$workflow"; then
    fail "sync-editors workflow must not delete workflow run history"
fi
grep -q 'cp config/editors.json internal/config/editors.json' "$workflow" || fail "workflow should update embedded Go registry copy"
grep -q 'cp config/editors.json bin/editors.json' "$workflow" || fail "workflow should update bundle registry copy"
grep -q 'git add config/editors.json internal/config/editors.json bin/editors.json' "$workflow" || fail "workflow should commit all registry copies"

echo "PASS: sync-editors workflow preserves history and updates registry copies"
