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
    
    # Enable globstar, extglob and nocasematch for pattern matching
    shopt -s globstar 2>/dev/null || true
    shopt -s extglob 2>/dev/null || true
    shopt -s nocasematch 2>/dev/null || true
    
    # Handle brace expansion
    local expanded_patterns
    if [[ "$pattern" == *{*\}* ]]; then
        expanded_patterns=$(eval echo "$pattern")
    else
        expanded_patterns="$pattern"
    fi
    
    for p in $expanded_patterns; do
        # Convert fnmatch globstar (**/) to bash optional globstar (@(**/|))
        # This allows **/ to match zero directories
        local bash_p="${p//\*\*\//@(**/|)}"
        
        if [[ "$bash_p" == /* ]]; then
            # Absolute pattern
            local p_no_slash="${bash_p#/}"
            if [[ "$path" == $p_no_slash ]] || [[ "$path" == $p_no_slash/** ]]; then
                # Restore case match setting before returning
                shopt -u nocasematch 2>/dev/null || true
                return 0
            fi
        else
            # Relative pattern
            if [[ "$path" == $bash_p ]] || [[ "$path" == $bash_p/** ]] || \
               [[ "$path" == **/$bash_p ]] || [[ "$path" == **/$bash_p/** ]]; then
                # Restore case match setting before returning
                shopt -u nocasematch 2>/dev/null || true
                return 0
            fi
        fi
    done
    
    shopt -u nocasematch 2>/dev/null || true
    return 1
}

# Check if a path should be excluded based on patterns
# Usage: is_excluded <path> <patterns_array>
is_excluded() {
    local path="$1"
    shift
    local patterns=("$@")
    
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
