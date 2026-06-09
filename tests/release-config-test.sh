#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

if grep -q '{{- .Version }}_' .goreleaser.yml; then
    fail "GoReleaser archive name includes version; installer expects abx_<os>_<arch>.tar.gz"
fi
grep -q 'abx_' .goreleaser.yml || fail "GoReleaser archive template should use abx_ prefix"
grep -q 'replace_existing_artifacts: false' .goreleaser.yml || fail "GoReleaser must not replace existing artifacts"

grep -q 'shasum -a 256' install || fail "installer should support shasum -a 256"
grep -q 'openssl dgst -sha256' install || fail "installer should support openssl SHA-256 fallback"
if grep -q 'Skipping integrity verification' install; then
    fail "installer must fail closed when checksums are unavailable"
fi

grep -q "tags: \['v\*'\]" .github/workflows/release.yml || fail "release workflow should run on tags"
if grep -q 'branches:' .github/workflows/release.yml; then
    fail "release workflow should not run production release on branch pushes"
fi

echo "PASS: release config matches installer expectations"
