#!/bin/bash
# ABox Content Exclusion Unit Test (Logic only, no Docker required)
set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

TEST_ROOT="/tmp/abx-unit-test-$$"
mkdir -p "$TEST_ROOT/src"
mkdir -p "$TEST_ROOT/dst"

# 1. Setup Mock Workspace
echo "Setting up mock files..."
touch "$TEST_ROOT/src/include.txt"
touch "$TEST_ROOT/src/secret.key"
mkdir -p "$TEST_ROOT/src/node_modules/dep"
touch "$TEST_ROOT/src/node_modules/dep/index.js"
mkdir -p "$TEST_ROOT/src/dist"
touch "$TEST_ROOT/src/dist/bundle.js"
mkdir -p "$TEST_ROOT/src/aboba"
touch "$TEST_ROOT/src/aboba/data.json"
touch "$TEST_ROOT/src/.abxignore"

# Define exclusions
cat > "$TEST_ROOT/src/.abxignore" <<EOF
# Comments should be ignored
secret.key
node_modules
dist
aboba
EOF

echo "=========================================="
echo "ABox Exclusion Logic Unit Test"
echo "=========================================="

# 2. Extract Logic from bin/abx
# This mimics the sync_workspace() logic without the container part
test_sync_logic() {
    local src="$1"
    local dst="$2"
    local ignore_file="$src/.abxignore"
    
    local clean_exclude="/tmp/abx_unit_exclude_$$"
    grep -vE '^\s*#' "$ignore_file" | grep -vE '^\s*$' > "$clean_exclude"
    
    local tar_opts="-cf -"
    if [[ -s "$clean_exclude" ]]; then
        tar_opts="$tar_opts -X $clean_exclude"
    fi

    echo "Running tar exclusion logic..."
    (
        cd "$src" || exit 1
        tar $tar_opts . | tar -xf - -C "$dst"
    )
    
    rm -f "$clean_exclude"
}

# 3. Run the logic
test_sync_logic "$TEST_ROOT/src" "$TEST_ROOT/dst"

# 4. Verify results
FAILED=0

check_exists() {
    if [[ ! -e "$TEST_ROOT/dst/$1" ]]; then
        echo -e "${RED}FAIL${NC}: Expected '$1' to exist, but it was missing."
        FAILED=1
    fi
}

check_missing() {
    if [[ -e "$TEST_ROOT/dst/$1" ]]; then
        echo -e "${RED}FAIL${NC}: Expected '$1' to be excluded, but it was found."
        FAILED=1
    fi
}

echo -n "Verifying inclusions ... "
check_exists "include.txt"
check_exists ".abxignore"
[[ $FAILED -eq 0 ]] && echo -e "${GREEN}OK${NC}" || exit 1

echo -n "Verifying exclusions ... "
check_missing "secret.key"
check_missing "node_modules"
check_missing "dist"
check_missing "aboba"
[[ $FAILED -eq 0 ]] && echo -e "${GREEN}OK${NC}" || exit 1

# Cleanup
rm -rf "$TEST_ROOT"

echo "=========================================="
echo -e "${GREEN}Logical Exclusion Test Passed!${NC}"
echo "=========================================="
