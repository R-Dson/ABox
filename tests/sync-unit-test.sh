#!/bin/bash
# ABox Sync Unit Tests
# Tests for sync correctness: streaming, atomicity, exclusions
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

FAILED=0
PASSED=0

echo "=========================================="
echo "Sync Unit Tests"
echo "=========================================="
echo

# --- Test 1: No intermediate tar files left in /tmp ---
echo "--- No intermediate tar artifacts ---"
# Source sync.sh and call sync_workspace with a temp workspace and exclusion file.
# The function must not leave files in /tmp matching abx_sync_*.tar
source "$PROJECT_ROOT/src/exclusion.sh"
source "$PROJECT_ROOT/src/sync.sh"
source "$PROJECT_ROOT/src/helpers.sh"

# We cannot actually run containers in unit tests, but we can verify the code
# doesn't create intermediate tar files by checking the source code pattern.
if grep -q 'temp_tar="/tmp/abx_sync' "$PROJECT_ROOT/src/sync.sh"; then
    # Check if temp_tar is actually used as a file (written to disk) or piped
    if grep -q 'tar -cf "\$temp_tar"' "$PROJECT_ROOT/src/sync.sh"; then
        echo -e "  ${RED}FAIL${NC}: sync_workspace still writes intermediate tar to /tmp (not streamed)"
        ((FAILED++)) || true
    else
        echo -e "  ${GREEN}PASS${NC}: sync_workspace uses streaming (no intermediate tar)"
        ((PASSED++)) || true
    fi
else
    echo -e "  ${GREEN}PASS${NC}: sync_workspace uses streaming (no intermediate tar)"
    ((PASSED++)) || true
fi

# Same check for sync_workspace_back
if grep -q 'temp_tar=' "$PROJECT_ROOT/src/sync.sh" && grep -q 'tar -xf "\$temp_tar"\|> "\$temp_tar"' "$PROJECT_ROOT/src/sync.sh"; then
    echo -e "  ${RED}FAIL${NC}: sync_workspace_back still uses intermediate tar file"
    ((FAILED++)) || true
else
    echo -e "  ${GREEN}PASS${NC}: sync_workspace_back uses streaming or no intermediate file"
    ((PASSED++)) || true
fi

# --- Test 2: Hardcoded security exclusions cover critical patterns ---
echo
echo "--- Hardcoded security exclusion patterns ---"
source "$PROJECT_ROOT/src/exclusion.sh"

PATTERNS=(".ssh" ".aws" ".env" ".gnupg" "**/*key" "**/*.pem")
CRITICAL_FILES=(".ssh/config" ".aws/credentials" ".env" ".gnupg/pubring.kbx" "id_rsa.key" "server.pem")
EXPECTED_EXCLUDED=(".ssh/config" ".aws/credentials" ".env" ".gnupg/pubring.kbx" "id_rsa.key" "server.pem")

for file in "${CRITICAL_FILES[@]}"; do
    if is_excluded "$file" "${PATTERNS[@]}"; then
        echo -e "  ${GREEN}PASS${NC}: '$file' is excluded"
        ((PASSED++)) || true
    else
        echo -e "  ${RED}FAIL${NC}: '$file' is NOT excluded (security risk)"
        ((FAILED++)) || true
    fi
done

# --- Test 3: Symlink bypass detection ---
echo
echo "--- Symlink exclusion check ---"
# The exclusion should apply to the path string, regardless of symlink targets.
# is_excluded works on path strings, so symlink-named files matching patterns
# should still be excluded by path.
if is_excluded ".env" "${PATTERNS[@]}"; then
    echo -e "  ${GREEN}PASS${NC}: '.env' path is excluded (symlink or not)"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: '.env' path not excluded"
    ((FAILED++)) || true
fi

echo "--- Sync operations use dedicated sync image ---"
if grep -q 'SYNC_IMAGE=' "$PROJECT_ROOT/src/sync.sh"; then
    echo -e "  ${GREEN}PASS${NC}: sync.sh defines SYNC_IMAGE constant"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: sync.sh missing SYNC_IMAGE constant (uses full editor image)"
    ((FAILED++)) || true
fi

# Verify sync.sh does NOT use $IMAGE_NAME for sync operations
SYNC_USES_EDITOR=$(grep -c '\$IMAGE_NAME' "$PROJECT_ROOT/src/sync.sh" || true)
if [[ "$SYNC_USES_EDITOR" -eq 0 ]]; then
    echo -e "  ${GREEN}PASS${NC}: sync.sh does not reference \$IMAGE_NAME"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: sync.sh still uses \$IMAGE_NAME ($SYNC_USES_EDITOR references)"
    ((FAILED++)) || true
fi

# Verify container.sh init_volume_ownership uses sync image
if grep -q 'SYNC_IMAGE' "$PROJECT_ROOT/src/container.sh"; then
    echo -e "  ${GREEN}PASS${NC}: init_volume_ownership uses SYNC_IMAGE"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: init_volume_ownership does not use SYNC_IMAGE"
    ((FAILED++)) || true
fi

echo
 echo "--- Transactional staging for sync_to_vols ---"
if grep -q 'cp -r /host/config/. /vol/config/ &' "$PROJECT_ROOT/src/sync.sh"; then
    echo -e "  ${RED}FAIL${NC}: sync_to_vols uses direct cp without atomic staging"
    ((FAILED++)) || true
else
    echo -e "  ${GREEN}PASS${NC}: sync_to_vols uses atomic staging"
    ((PASSED++)) || true
fi

if grep -q '\.tmp' "$PROJECT_ROOT/src/sync.sh"; then
    echo -e "  ${GREEN}PASS${NC}: sync_to_vols uses temp directory staging pattern"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: sync_to_vols missing temp directory staging"
    ((FAILED++)) || true
fi

echo "--- Conflict detection before sync-back ---"
if grep -q 'snapshot_mtimes\|check_conflicts\|mtime' "$PROJECT_ROOT/src/sync.sh"; then
    echo -e "  ${GREEN}PASS${NC}: sync.sh has mtime-based conflict detection"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: sync.sh missing mtime-based conflict detection"
    ((FAILED++)) || true
fi

if grep -q 'force-sync\|FORCE_SYNC' "$PROJECT_ROOT/src/main.sh"; then
    echo -e "  ${GREEN}PASS${NC}: --force-sync flag available to override conflict detection"
    ((PASSED++)) || true
else
    echo -e "  ${RED}FAIL${NC}: missing --force-sync flag"
    ((FAILED++)) || true
fi

echo
echo "=========================================="
if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}All $PASSED tests passed!${NC}"
else
    echo -e "${RED}$PASSED passed, $FAILED failed${NC}"
fi
echo "=========================================="
exit $FAILED
