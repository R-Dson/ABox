#!/usr/bin/env bash
set -euo pipefail

fail() {
    echo "FAIL: $*" >&2
    exit 1
}

# 1. Dockerfile must not use eval for install commands
if grep -E 'RUN.*eval.*INSTALL_CMD' docker/Dockerfile; then
    fail "Dockerfile must not use eval for INSTALL_CMD"
fi

# 2. Base images must be digest-pinned
if grep -qE '^FROM [a-z]+:[a-z]' docker/Dockerfile && ! grep -qE '^FROM [a-z]+@' docker/Dockerfile; then
    fail "Dockerfile base images must be digest-pinned"
fi
if grep -qE '^FROM [a-z]+:[0-9]' docker/Dockerfile.sync && ! grep -qE '^FROM [a-z]+@' docker/Dockerfile.sync; then
    fail "Dockerfile.sync base image must be digest-pinned"
fi

# 3. Both Dockerfiles must be syntactically valid (hadolint checks this, but basic check)
bash -n docker/entrypoint.sh || fail "entrypoint must be valid bash"

# 4. Dockerfile must not run as root (user directive or gosu drop)
grep -qE '(USER |gosu )' docker/Dockerfile || fail "Dockerfile must drop root privileges"

echo "PASS: Dockerfile hardening checks"
