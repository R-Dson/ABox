#!/usr/bin/env bash
set -euo pipefail

fail() {
	echo "FAIL: $*" >&2
	exit 1
}

script=config/sync_versions.py

# 1. Script must be valid Python
python3 -c "import py_compile; py_compile.compile('$script', doraise=True)" || fail "sync_versions.py has syntax errors"

# 2. Request timeouts must be configured
grep -q 'timeout' "$script" || fail "sync_versions.py must configure request timeouts"

# 3. Must handle 'pi' editor (explicitly included or skipped with assertion)
grep -q '"pi"' "$script" || fail "sync_versions.py must handle 'pi' editor"

# 4. Must fail non-zero on fetch/schema errors
grep -qE '(raise|sys\.exit|exit\()' "$script" || fail "sync_versions.py must exit non-zero on errors"

# 5. Must use UTF-8 encoding for reads and writes
if grep -q 'open(' "$script"; then
	grep -q 'encoding=' "$script" || fail "sync_versions.py must use explicit encoding= for file I/O"
fi

# 6. Must use atomic writes (temp file + os.replace)
grep -q 'os.replace' "$script" || fail "sync_versions.py must use atomic writes (os.replace)"
grep -q 'NamedTemporaryFile\|mkstemp' "$script" || fail "sync_versions.py must write to temp file before os.replace"

echo "PASS: sync version script hardening checks"
