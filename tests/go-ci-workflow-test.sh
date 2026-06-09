#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

workflow=.github/workflows/go-ci.yml

grep -q 'go tool govulncheck ./...' "$workflow" || fail "Go CI should run govulncheck"
grep -q 'actionlint' "$workflow" || fail "Go CI should run actionlint"
if grep -q 'bc -l' "$workflow"; then
    fail "Go CI coverage gate should not require bc"
fi
if grep -q "grep -v -E '(cmd/abx\$|internal/runtime\$)'" "$workflow"; then
    fail "Go CI coverage should include pure internal/runtime tests"
fi
grep -q "grep -v -E 'cmd/abx\$'" "$workflow" || fail "Go CI should only exclude cmd/abx from coverage"

echo "PASS: Go CI workflow includes security checks and portable coverage gate"
