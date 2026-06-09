#!/usr/bin/env bash
set -euo pipefail

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

workflow=.github/workflows/publish.yml

# 1. setup-matrix must validate editors_json against known editor allowlist
grep -q 'ALLOWED_EDITORS' "$workflow" || fail "setup-matrix must define an editor allowlist"
grep -q 'jq -e' "$workflow" || fail "setup-matrix must validate JSON with jq -e"

# 2. PR builds must not execute changed INSTALL_CMD — only build (no push), use baseline install
if grep -A5 'pull_request' "$workflow" | grep -q 'INSTALL_CMD'; then
	fail "PR builds must not use repo-changed INSTALL_CMD"
fi
grep -q 'github.event_name.*pull_request' "$workflow" || fail "publish must gate push/scan/sign on PR"

# 3. Trivy must be version-pinned (not @master)
if grep -q 'trivy-action@master' "$workflow"; then
	fail "trivy-action must be version-pinned, not @master"
fi

# 4. Third-party actions must be version-pinned (no @master or @main for actions)
for bad in '@master' '@main'; do
	if grep -q "uses:.*$bad" "$workflow"; then
		fail "third-party action uses $bad — must be version-pinned"
	fi
done

# 5. Signing jobs must grant id-token: write
grep -q 'id-token: write' "$workflow" || fail "publish-images signing job must grant id-token: write"

# 6. Sync image must be scanned, SBOMed, and signed
sync_job=$(sed -n '/publish-sync-image:/,/^[a-z]/p' "$workflow")
echo "$sync_job" | grep -q 'trivy' || fail "sync image must be scanned with trivy"
echo "$sync_job" | grep -q 'sbom' || fail "sync image must generate SBOM"
echo "$sync_job" | grep -q 'cosign' || fail "sync image must be signed with cosign"

# 7. SBOM files must be uploaded as artifacts
grep -q 'upload-artifact' "$workflow" || fail "SBOM files must be uploaded as artifacts"

# 8. Smoke test before push/sign
grep -qi 'smoke' "$workflow" || grep -q 'docker run' "$workflow" || fail "publish must smoke-test images before push/sign"

# 9. Scan/SBOM/sign uses canonical single tag
if grep -q 'image-ref.*steps.meta.outputs.tags' "$workflow"; then
	fail "scan must use canonical ref (steps.ref.outputs.canonical), not multi-tag string"
fi
grep -q 'steps.ref.outputs.canonical' "$workflow" || fail "scan/SBOM/sign must use canonical image reference"

echo "PASS: publish workflow hardening checks"
