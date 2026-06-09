#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

[[ -f .github/dependabot.yml ]] || fail "missing dependabot config"
[[ -f .github/CODEOWNERS ]] || fail "missing CODEOWNERS"
[[ -f .github/workflows/security.yml ]] || fail "missing security workflow"

for ecosystem in gomod github-actions docker pip; do
    grep -q "package-ecosystem: \"$ecosystem\"" .github/dependabot.yml || fail "dependabot missing $ecosystem"
done

for pattern in '/.github/workflows/' '/docker/' '/go.mod' '/go.sum' '/.goreleaser.yml' '/install' '/internal/sync/' '/internal/container/' '/config/seccomp/'; do
    grep -q "$pattern" .github/CODEOWNERS || fail "CODEOWNERS missing $pattern"
done

for token in 'go tool govulncheck ./...' 'actionlint' 'gitleaks' 'hadolint' 'github/codeql-action/init'; do
    grep -q "$token" .github/workflows/security.yml || fail "security workflow missing $token"
done

echo "PASS: security governance files present"
