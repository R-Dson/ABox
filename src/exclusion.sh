# ==============================================================================
# Content Exclusion Logic - GitHub Copilot Fnmatch Compatible
# ==============================================================================
# Implements fnmatch pattern matching as specified in GitHub Copilot documentation
# https://docs.github.com/en/copilot/how-tos/configure-content-exclusion/exclude-content-from-copilot
#
# Supported patterns:
#   *          - matches anything except /
#   ?          - matches single character
#   **         - recursive directory matching (globstar)
#   [abc]      - character class
#   {a,b}      - brace expansion (alternation)
#   Leading /  - anchors to root
# ==============================================================================

# Reads exclusion patterns from a file, removing comments and empty lines
# Usage: read_exclusions <exclude_file>
# Returns: List of patterns (one per line) via stdout
read_exclusions() {
    local exclude_file="$1"
    
    if [[ -f "$exclude_file" ]]; then
        grep -vE '^\s*#' "$exclude_file" | grep -vE '^\s*$'
    fi
}

# Check if a path matches a fnmatch pattern
# Usage: path_matches <path> <pattern>
# Returns: 0 if matches, 1 otherwise
path_matches() {
    local path="$1"
    local pattern="$2"
    local p
    
    # Enable globstar, extglob and nocasematch for pattern matching
    shopt -s globstar 2>/dev/null || true
    shopt -s extglob 2>/dev/null || true
    shopt -s nocasematch 2>/dev/null || true
    
    # Handle brace expansion
    local expanded_patterns
    if [[ "$pattern" == *{*\}* ]]; then
        # We use a subshell for eval to avoid messing with current shell state
        # and set -f to avoid expanding globs after brace expansion
        expanded_patterns=$(set -f; eval echo "$pattern")
    else
        expanded_patterns="$pattern"
    fi
    
    # Disable globbing for the loop to avoid expanding patterns against the current directory
    set -f
    for p in $expanded_patterns; do
        # Convert fnmatch globstar (**/) to bash optional globstar (@(**/|))
        # This allows **/ to match zero directories
        local bash_p="${p//\*\*\//@(**/|)}"
        
        if [[ "$bash_p" == /* ]]; then
            # Absolute pattern
            local p_no_slash="${bash_p#/}"
            if [[ "$path" == $p_no_slash ]] || [[ "$path" == $p_no_slash/** ]]; then
                set +f
                shopt -u nocasematch 2>/dev/null || true
                return 0
            fi
        else
            # Relative pattern
            if [[ "$path" == $bash_p ]] || [[ "$path" == $bash_p/** ]] || \
               [[ "$path" == **/$bash_p ]] || [[ "$path" == **/$bash_p/** ]]; then
                set +f
                shopt -u nocasematch 2>/dev/null || true
                return 0
            fi
        fi
    done
    set +f
    
    shopt -u nocasematch 2>/dev/null || true
    return 1
}

# Check if a path should be excluded based on patterns
# Usage: is_excluded <path> <patterns_array>
is_excluded() {
    local path="$1"
    shift
    local patterns=("$@")
    local pattern
    
    for pattern in "${patterns[@]}"; do
        if path_matches "$path" "$pattern"; then
            return 0
        fi
    done
    return 1
}

# Checks if exclusions are active (file exists and has content)
# Usage: has_exclusions <exclude_file>
# Returns: 0 if exclusions exist, 1 otherwise
has_exclusions() {
    local exclude_file="$1"
    
    [[ -f "$exclude_file" ]] && [[ -s "$exclude_file" ]]
}

# Fetch exclusion patterns from a URL
# Usage: fetch_exclusions <URL>
# Returns: Patterns via stdout, exits with error on failure
fetch_exclusions() {
    local url="$1"
    local response
    local http_code

    if [[ ! "$url" =~ ^https?:// ]]; then
        echo "ERROR: Invalid exclude URL format: $url" >&2
        return 1
    fi

    response=$(curl -s --max-time 10 -w "\n%{http_code}" "$url" 2>&1)
    http_code=$(echo "$response" | tail -n1)

    case "$http_code" in
        200)
            echo "$response" | sed '$d'
            ;;
        000)
            if echo "$response" | grep -qi "could not resolve\|no route\|connection refused"; then
                echo "ERROR: Network unavailable, cannot fetch exclude URL: $url" >&2
            else
                echo "ERROR: Timeout fetching exclude URL after 10s: $url" >&2
            fi
            return 1
            ;;
        404)
            echo "ERROR: HTTP 404 Not Found: $url" >&2
            return 1
            ;;
        403)
            echo "ERROR: HTTP 403 Forbidden: $url" >&2
            return 1
            ;;
        [45]*)
            echo "ERROR: HTTP $http_code error fetching: $url" >&2
            return 1
            ;;
        *)
            echo "ERROR: HTTP $http_code error fetching: $url" >&2
            return 1
            ;;
    esac
}

# Calculate pattern strictness (more wildcards = more restrictive)
# Usage: pattern_strictness <pattern>
# Returns: Integer - higher = more restrictive
_pattern_strictness() {
    local pattern="$1"
    local score=0

    score=$((score + $(echo "$pattern" | grep -o '\*\*' | wc -l) * 2))
    score=$((score + $(echo "$pattern" | grep -o '\*' | wc -l)))
    score=$((score + $(echo "$pattern" | grep -o '?' | wc -l)))

    echo "$score"
}

# Extract parent directory from pattern for grouping
_pattern_base() {
    local pattern="$1"
    local base
    
    # Remove trailing wildcards for directory matching
    base="${pattern%%\**}"      # Remove first ** onwards
    base="${base%\*}"           # Remove trailing *
    base="${base%/*}"           # Remove trailing /component
    base="${base%/}"            # Clean trailing slash
    
    [[ -z "$base" && "$pattern" == /* ]] && base="/"
    echo "$base"
}

# Check if pattern A is less restrictive than pattern B (B should win)
_is_less_restrictive() {
    local pattern_a="$1"
    local pattern_b="$2"
    
    [[ "$pattern_a" == "$pattern_b" ]] && return 1
    
    local a_base b_base
    a_base=$(_pattern_base "$pattern_a")
    b_base=$(_pattern_base "$pattern_b")
    
    [[ "$a_base" != "$b_base" ]] && return 1
    
    local a_strict b_strict
    a_strict=$(_pattern_strictness "$pattern_a")
    b_strict=$(_pattern_strictness "$pattern_b")
    
    [[ "$a_strict" -lt "$b_strict" ]] && return 0
    return 1
}

# Unify two sets of patterns, strictest wins for conflicts
# Usage: unify_patterns <local_patterns> <url_patterns>
# Returns: Unified patterns via stdout
unify_patterns() {
    local local_patterns="$1"
    local url_patterns="$2"
    local all_patterns result
    
    all_patterns=$(printf '%s\n%s\n' "$local_patterns" "$url_patterns" | grep -v '^$')
    
    result=""
    while IFS= read -r candidate; do
        [[ -z "$candidate" ]] && continue
        
        # Check if candidate is dominated by any existing (existing is more restrictive)
        local skip=0
        while IFS= read -r existing; do
            [[ -z "$existing" ]] && continue
            if _is_less_restrictive "$candidate" "$existing"; then
                skip=1
                break
            fi
        done <<< "$result"
        
        if [[ "$skip" -eq 1 ]]; then
            continue
        fi
        
        # Remove any existing patterns that candidate dominates (candidate is more restrictive)
        local new_result=""
        while IFS= read -r existing; do
            [[ -z "$existing" ]] && continue
            if _is_less_restrictive "$existing" "$candidate"; then
                continue
            fi
            new_result="${new_result}${new_result:+
}${existing}"
        done <<< "$result"
        
        new_result="${new_result}${new_result:+
}${candidate}"
        result="$new_result"
    done <<< "$all_patterns"
    
    echo "$result" | sort -u
}
