#!/bin/bash
# ABox Content Exclusion Comprehensive Unit Test
# Tests all fnmatch patterns as specified in GitHub Copilot documentation
# https://docs.github.com/en/copilot/how-tos/configure-content-exclusion/exclude-content-from-copilot
set -e

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Get the directory where this script is located
TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

# Source the actual exclusion module used in production
source "$PROJECT_ROOT/src/exclusion.sh"

TEST_ROOT="/tmp/abx-fnmatch-test-$$"
FAILED=0
PASSED=0

# Test result tracking
test_start() {
    echo -n "Testing $1 ... "
}

test_pass() {
    echo -e "${GREEN}PASS${NC}"
    ((PASSED++)) || true
}

test_fail() {
    echo -e "${RED}FAIL${NC}: $1"
    ((FAILED++)) || true
}

# Setup test directory
setup_test_dir() {
    rm -rf "$TEST_ROOT"
    mkdir -p "$TEST_ROOT/src"
    mkdir -p "$TEST_ROOT/dst"
}

# Check if file exists in destination (returns 0 if exists, 1 if missing)
check_exists() {
    if [[ ! -e "$TEST_ROOT/dst/$1" ]]; then
        echo "  ERROR: Expected '$1' to exist"
        return 1
    fi
    return 0
}

# Check if file is excluded/missing from destination (returns 0 if missing, 1 if exists)
check_missing() {
    if [[ -e "$TEST_ROOT/dst/$1" ]]; then
        echo "  ERROR: Expected '$1' to be excluded but was found"
        return 1
    fi
    return 0
}

# Run sync with given exclusion patterns and check results
# Arrays are passed as space-separated strings to avoid nameref issues
run_test() {
    local test_name="$1"
    local exclude_patterns="$2"
    local create_files="$3"
    local should_exist="$4"
    local should_miss="$5"
    local -a patterns=()
    local include_list="/tmp/abx_include_list_$$"
    local temp_tar="/tmp/abx_sync_$$.tar"
    
    setup_test_dir
    
    # Create all test files
    for file in $create_files; do
        mkdir -p "$TEST_ROOT/src/$(dirname "$file")"
        touch "$TEST_ROOT/src/$file"
    done
    
    # Run sync using fnmatch-based exclusion
    echo "$exclude_patterns" > "$TEST_ROOT/src/.abxignore"
    
    # Read patterns
    while IFS= read -r pattern; do
        [[ -n "$pattern" ]] && patterns+=("$pattern")
    done < <(read_exclusions "$TEST_ROOT/src/.abxignore")
    
    if [[ ${#patterns[@]} -eq 0 ]]; then
        # No exclusions
        (
            cd "$TEST_ROOT/src" || exit 1
            tar -cf - . | tar -xf - -C "$TEST_ROOT/dst"
        )
    else
        # Build list of files to include (not excluded)
        (
            cd "$TEST_ROOT/src" || exit 1
            find . -type f -print0 2>/dev/null | while IFS= read -r -d '' path; do
                path="${path#./}"
                if ! is_excluded "$path" "${patterns[@]}"; then
                    printf '%s\n' "$path"
                fi
            done
        ) > "$include_list"
        
        # Create tar with included files and extract
        (
            cd "$TEST_ROOT/src" || exit 1
            tar -cf "$temp_tar" -T "$include_list" 2>/dev/null
        )
        tar -xf "$temp_tar" -C "$TEST_ROOT/dst"
        rm -f "$include_list" "$temp_tar"
    fi
    
    # Check results
    test_start "$test_name"
    local test_failed=0
    
    for file in $should_exist; do
        if ! check_exists "$file"; then
            test_failed=1
        fi
    done
    
    for file in $should_miss; do
        if ! check_missing "$file"; then
            test_failed=1
        fi
    done
    
    if [[ $test_failed -eq 0 ]]; then
        test_pass
    else
        test_fail "$test_name"
    fi
}

echo "=========================================="
echo "ABox Fnmatch Pattern Compliance Test"
echo "Testing against GitHub Copilot exclusion spec"
echo "=========================================="
echo

# ==============================================================================
# BASIC PATTERNS
# ==============================================================================
run_test \
    "basic filename exclusion" \
    "secret.key" \
    "include.txt secret.key data.json" \
    "include.txt data.json" \
    "secret.key"

# ==============================================================================
# WILDCARD PATTERNS (* and ?)
# ==============================================================================
run_test \
    "wildcard * prefix" \
    "secret*" \
    "secret.txt secret.key secret_data.json my_secret.txt notasecret.txt" \
    "my_secret.txt notasecret.txt" \
    "secret.txt secret.key secret_data.json"

run_test \
    "wildcard * suffix" \
    "*.cfg" \
    "config.cfg settings.cfg data.json test.cfg.bak" \
    "data.json test.cfg.bak" \
    "config.cfg settings.cfg"

run_test \
    "wildcard ? single character" \
    "package?" \
    "package1 package2 package12 package" \
    "package12 package" \
    "package1 package2"

# ==============================================================================
# GLOBSTAR ** PATTERNS
# ==============================================================================
run_test \
    "globstar **/.env" \
    "**/.env" \
    ".env a/.env a/b/.env config.env" \
    "config.env" \
    ".env a/.env a/b/.env"

run_test \
    "globstar /scripts/**" \
    "/scripts/**" \
    "scripts/test.sh scripts/deploy/prod.sh lib/scripts/util.sh main.sh" \
    "lib/scripts/util.sh main.sh" \
    "scripts scripts/test.sh scripts/deploy scripts/deploy/prod.sh"

# ==============================================================================
# CHARACTER CLASSES [abc]
# ==============================================================================
run_test \
    "character class [abc]" \
    "*.m[dk]" \
    "file.md file.mk file.txt file.mdd" \
    "file.txt file.mdd" \
    "file.md file.mk"

run_test \
    "character class range [0-9]" \
    "test[0-9].log" \
    "test1.log test2.log testA.log" \
    "testA.log" \
    "test1.log test2.log"

# ==============================================================================
# BRACE EXPANSION {a,b}
# ==============================================================================
run_test \
    "brace expansion {a,b}" \
    "{server,session}*" \
    "server.js session.js service.js client.js" \
    "service.js client.js" \
    "server.js session.js"

run_test \
    "brace expansion {a,b} with suffix" \
    "app.min.{js,css}" \
    "app.min.js app.min.css app.js styles.min.css" \
    "app.js styles.min.css" \
    "app.min.js app.min.css"

# ==============================================================================
# COMPLEX GITHUB EXAMPLES
# ==============================================================================
run_test \
    "GitHub: /src/**/temp.rb" \
    "/src/**/temp.rb" \
    "src/temp.rb src/a/temp.rb src/a/b/temp.rb lib/temp.rb" \
    "lib/temp.rb" \
    "src/temp.rb src/a/temp.rb src/a/b/temp.rb"

run_test \
    "GitHub: /__tests__/**" \
    "/__tests__/**" \
    "__tests__/test.js __tests__/unit/utils.test.js components/__tests__/Button.test.js" \
    "components/__tests__/Button.test.js" \
    "__tests__ __tests__/test.js __tests__/unit __tests__/unit/utils.test.js"

# ==============================================================================
# CASE INSENSITIVITY
# ==============================================================================
run_test \
    "case insensitivity" \
    "SECRET.KEY" \
    "secret.key" \
    "" \
    "secret.key"

# ==============================================================================
# CLEANUP
# ==============================================================================
rm -rf "$TEST_ROOT"

echo
if [[ $FAILED -eq 0 ]]; then
    echo -e "${GREEN}==========================================${NC}"
    echo -e "${GREEN}All $PASSED tests passed!${NC}"
    echo -e "${GREEN}==========================================${NC}"
    exit 0
else
    echo -e "${RED}==========================================${NC}"
    echo -e "${RED}$PASSED passed, $FAILED failed${NC}"
    echo -e "${RED}==========================================${NC}"
    exit 1
fi
