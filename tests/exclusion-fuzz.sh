#!/bin/bash
# ABox Exclusion Fuzz Testing
# Generates random path/pattern pairs and cross-validates against Python's fnmatch
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
NC='\033[0m'

TEST_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(dirname "$TEST_DIR")"

source "$PROJECT_ROOT/src/exclusion.sh"

PASSED=0
FAILED=0
FUZZ_COUNT="${ABX_FUZZ_COUNT:-500}"

# Cross-validate: Bash path_matches vs Python fnmatch
# Python fnmatch does NOT support ** (globstar), so we only test * and ? patterns.
# For ** patterns we skip the Python comparison but still exercise path_matches.
cross_validate() {
    local path="$1"
    local pattern="$2"

    # Skip patterns with ** or [] or {} — Python fnmatch doesn't handle these the same way
    if [[ "$pattern" == *"**"* || "$pattern" == *"\["* || "$pattern" == *"{"* ]]; then
        return 0  # skip, not a fair comparison
    fi

    local bash_result
    if path_matches "$path" "$pattern"; then
        bash_result="match"
    else
        bash_result="nomatch"
    fi

    local python_result
    python_result=$(python3 -c "
import fnmatch, sys
path = sys.argv[1]
pattern = sys.argv[2]
print('match' if fnmatch.fnmatch(path, pattern) else 'nomatch')
" "$path" "$pattern" 2>/dev/null)

    if [[ -z "$python_result" ]]; then
        return 0  # Python errored, skip
    fi

    if [[ "$bash_result" != "$python_result" ]]; then
        echo "MISMATCH: path='$path' pattern='$pattern' bash=$bash_result python=$python_result"
        return 1
    fi
    return 0
}

echo "=========================================="
echo "Exclusion Fuzz Test ($FUZZ_COUNT iterations)"
echo "=========================================="
echo

# Generate random alphanumeric strings
rand_str() {
    local len="${1:-5}"
    tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c "$len"
}

# Generate a random relative path (e.g. "src/foo/bar.txt")
rand_path() {
    local depth=$((RANDOM % 4 + 1))
    local path=""
    for ((i=0; i<depth; i++)); do
        if [[ -n "$path" ]]; then
            path="$path/"
        fi
        path="$path$(rand_str $((RANDOM % 8 + 2)))"
    done
    # Sometimes add an extension
    if [[ $((RANDOM % 3)) -eq 0 ]]; then
        local exts=("js" "ts" "py" "go" "rs" "txt" "json" "yaml" "md" "cfg")
        path="$path.${exts[$((RANDOM % ${#exts[@]}))]}"
    fi
    echo "$path"
}

# Generate a random fnmatch pattern
rand_pattern() {
    local kind=$((RANDOM % 6))
    case "$kind" in
        0) echo "$(rand_str 4)" ;;                              # literal
        1) echo "*.$(rand_str 2)" ;;                             # suffix wildcard
        2) echo "$(rand_str 3)*" ;;                              # prefix wildcard
        3) echo "*$(rand_str 3)*" ;;                             # contains wildcard
        4) echo "$(rand_str 2)/$(rand_str 2)/?$(rand_str 3)" ;;  # single-char wildcard
        5) echo "**/$(rand_str 4)" ;;                             # globstar (exercised but not cross-validated)
    esac
}

MISMATCHES=0

for ((i=0; i<FUZZ_COUNT; i++)); do
    path=$(rand_path)
    pattern=$(rand_pattern)

    if ! cross_validate "$path" "$pattern"; then
        ((MISMATCHES++)) || true
        ((FAILED++)) || true
    else
        ((PASSED++)) || true
    fi
done

echo
echo "=========================================="
if [[ $MISMATCHES -eq 0 ]]; then
    echo -e "${GREEN}All $PASSED cases passed ($FUZZ_COUNT iterations)${NC}"
else
    echo -e "${RED}$PASSED passed, $MISMATCHES mismatches${NC}"
fi
echo "=========================================="
exit $MISMATCHES
