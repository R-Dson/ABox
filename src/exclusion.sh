# ==============================================================================
# Content Exclusion Logic
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

# Creates a clean exclusion file for tar command
# Usage: prepare_exclusion_file <exclude_file>
# Returns: Path to temp file via stdout, or empty if no exclusions
prepare_exclusion_file() {
    local exclude_file="$1"
    local temp_file="/tmp/abx_exclude_$$"
    
    if [[ -f "$exclude_file" ]]; then
        read_exclusions "$exclude_file" > "$temp_file"
        if [[ -s "$temp_file" ]]; then
            echo "$temp_file"
            return 0
        fi
    fi
    
    rm -f "$temp_file"
    return 1
}

# Cleans up the temporary exclusion file
# Usage: cleanup_exclusion_file
cleanup_exclusion_file() {
    local temp_file="/tmp/abx_exclude_$$"
    rm -f "$temp_file"
}

# Generates tar options for exclusion
# Usage: get_tar_exclusion_opts <exclude_file>
# Returns: tar options string via stdout
get_tar_exclusion_opts() {
    local exclude_file="$1"
    local opts="-cf -"
    
    if [[ -f "$exclude_file" ]] && [[ -s "$exclude_file" ]]; then
        opts="$opts -X $exclude_file"
    fi
    
    echo "$opts"
}

# Checks if exclusions are active (file exists and has content)
# Usage: has_exclusions <exclude_file>
# Returns: 0 if exclusions exist, 1 otherwise
has_exclusions() {
    local exclude_file="$1"
    
    [[ -f "$exclude_file" ]] && [[ -s "$exclude_file" ]]
}
