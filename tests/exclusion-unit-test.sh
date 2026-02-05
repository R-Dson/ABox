#!/bin/bash
# ABox Content Exclusion Unit Test
# Tests the actual exclusion logic used in production
set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

# Get the directory where this script is located
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

# Source the actual exclusion module used in production
source "$PROJECT_ROOT/src/exclusion.sh"

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

# 2. Test read_exclusions
echo -n "Testing read_exclusions() ... "
EXCLUSIONS=$(read_exclusions "$TEST_ROOT/src/.abxignore")
if echo "$EXCLUSIONS" | grep -q "secret.key" && \
   echo "$EXCLUSIONS" | grep -q "node_modules" && \
   echo "$EXCLUSIONS" | grep -q "dist" && \
   echo "$EXCLUSIONS" | grep -q "aboba" && \
   ! echo "$EXCLUSIONS" | grep -q "^#"; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}: read_exclusions() did not parse correctly"
    exit 1
fi

# 3. Test has_exclusions
echo -n "Testing has_exclusions() ... "
if has_exclusions "$TEST_ROOT/src/.abxignore"; then
    echo -e "${GREEN}PASS${NC}"
else
    echo -e "${RED}FAIL${NC}: has_exclusions() should return true"
    exit 1
fi

# Test with empty file
touch "$TEST_ROOT/src/empty.abxignore"
if ! has_exclusions "$TEST_ROOT/src/empty.abxignore"; then
    echo -e "${GREEN}PASS${NC} (empty file correctly returns false)"
else
    echo -e "${RED}FAIL${NC}: has_exclusions() should return false for empty file"
    exit 1
fi

# 4. Test prepare_exclusion_file
echo -n "Testing prepare_exclusion_file() ... "
CLEAN_FILE=$(prepare_exclusion_file "$TEST_ROOT/src/.abxignore")
if [[ -n "$CLEAN_FILE" ]] && [[ -f "$CLEAN_FILE" ]]; then
    if grep -q "secret.key" "$CLEAN_FILE" && ! grep -q "^#" "$CLEAN_FILE"; then
        echo -e "${GREEN}PASS${NC}"
        rm -f "$CLEAN_FILE"
    else
        echo -e "${RED}FAIL${NC}: Clean file not prepared correctly"
        rm -f "$CLEAN_FILE"
        exit 1
    fi
else
    echo -e "${RED}FAIL${NC}: prepare_exclusion_file() did not return valid path"
    exit 1
fi

# 5. Test tar exclusion logic
echo -n "Testing tar exclusion logic ... "
CLEAN_FILE=$(prepare_exclusion_file "$TEST_ROOT/src/.abxignore")
TAR_OPTS=$(get_tar_exclusion_opts "$CLEAN_FILE")

# Run tar with exclusions
(
    cd "$TEST_ROOT/src" || exit 1
    tar $TAR_OPTS . | tar -xf - -C "$TEST_ROOT/dst"
)

rm -f "$CLEAN_FILE"

# Verify results
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

check_exists "include.txt"
check_exists ".abxignore"
check_missing "secret.key"
check_missing "node_modules"
check_missing "dist"
check_missing "aboba"

if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}PASS${NC}"
else
    exit 1
fi

# 6. Test cleanup_exclusion_file
echo -n "Testing cleanup_exclusion_file() ... "
CLEAN_FILE=$(prepare_exclusion_file "$TEST_ROOT/src/.abxignore")
[[ -f "$CLEAN_FILE" ]] || { echo -e "${RED}FAIL${NC}: File not created"; exit 1; }
cleanup_exclusion_file
[[ ! -f "$CLEAN_FILE" ]] || { echo -e "${RED}FAIL${NC}: File not cleaned up"; exit 1; }
echo -e "${GREEN}PASS${NC}"

# Cleanup
rm -rf "$TEST_ROOT"

echo "=========================================="
echo -e "${GREEN}All Exclusion Logic Tests Passed!${NC}"
echo "=========================================="
