#!/usr/bin/env bash
set -euo pipefail

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

entrypoint=docker/entrypoint.sh

# 1. GID collision: must not fail just because numeric GID already exists
# macOS commonly uses GID 20 ("staff"). The entrypoint must handle this gracefully.
if grep -q 'ERROR.*GID.*already taken' "$entrypoint"; then
	# Check if there's an alternative path that reuses the existing group
	if ! grep -q 'groupmod.*-g.*GROUP_ID' "$entrypoint" && ! grep -q 'addgroup.*gid' "$entrypoint"; then
		fail "entrypoint must handle GID collision gracefully (e.g., reuse existing group)"
	fi
fi

# 2. Default command must be quoted to prevent word splitting
if grep -q 'exec gosu agent \$DEFAULT_COMMAND' "$entrypoint"; then
	fail "default command must be quoted: use \"\$DEFAULT_COMMAND\" not \$DEFAULT_COMMAND"
fi
grep -q '"\$DEFAULT_COMMAND"' "$entrypoint" || fail "default command must use quoted \"\$DEFAULT_COMMAND\""

# 3. chown failures must be visible (not silently swallowed)
# The script should log when chown fails rather than silently ignoring
if grep -q 'chown.*|| true' "$entrypoint"; then
	fail "chown failures must be visible, not silently ignored with || true"
fi
grep -q 'chown' "$entrypoint" || fail "entrypoint must handle ownership"

# 4. Script must be valid bash
bash -n "$entrypoint" || fail "entrypoint has syntax errors"

echo "PASS: entrypoint hardening checks"
