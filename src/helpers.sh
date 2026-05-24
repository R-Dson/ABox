# ==============================================================================
# Helper Functions
# ==============================================================================

log_error() {
    echo "ERROR: $*" >&2
}

# Structured logging -- active when ABX_VERBOSE=true or --verbose flag
ABX_LOG_FILE=""

log_setup() {
    if [[ "$ABOX_VERBOSE" == "true" || "$CLI_VERBOSE" == "true" ]]; then
        local log_dir="${ABX_LOG_DIR:-$HOME/.local/state/abx}"
        mkdir -p "$log_dir"
        ABX_LOG_FILE="$log_dir/abx.log"
        echo "[$(date +%T)] INFO: abx session started" >> "$ABX_LOG_FILE"
    fi
}

log_debug() {
    [[ -n "$ABX_LOG_FILE" ]] && echo "[$(date +%T)] DEBUG: $*" >> "$ABX_LOG_FILE"
}

log_info() {
    [[ -n "$ABX_LOG_FILE" ]] && echo "[$(date +%T)] INFO: $*" >> "$ABX_LOG_FILE"
}

detect_runtime() {
    if [[ -n "$ABOX_RUNTIME" ]]; then
        if command -v "$ABOX_RUNTIME" >/dev/null 2>&1 && "$ABOX_RUNTIME" info >/dev/null 2>&1; then
            echo "$ABOX_RUNTIME"
            return 0
        fi
        log_error "Specified runtime '$ABOX_RUNTIME' is not available or healthy."
        return 1
    fi

    if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
        echo "docker"
        return 0
    fi

    if command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
        echo "podman"
        return 0
    fi

    log_error "No healthy container runtime found (Docker or Podman)."
    return 1
}

# Resolve the editors.json config file path.
# Checks: env var override, dev source tree, installed location, alongside binary
_resolve_editors_json() {
    local candidates=(
        "${ABX_EDITORS_JSON:-}"
        "${SCRIPT_DIR:-$(dirname "$(readlink -f "${BASH_SOURCE[1]:-$0}")")}/../config/editors.json"
        "/usr/local/share/abx/editors.json"
        "$(dirname "$(readlink -f "${BASH_SOURCE[1]:-$0}")")/editors.json"
    )
    for path in "${candidates[@]}"; do
        if [[ -n "$path" && -f "$path" ]]; then
            echo "$path"
            return 0
        fi
    done
    return 1
}

get_editor_info() {
    local editor="$1"
    local json_path
    json_path=$(_resolve_editors_json) || {
        log_error "editors.json not found. Set ABX_EDITORS_JSON or reinstall abx."
        return 1
    }

    local entry
    entry=$(jq -r ".editors.\"$editor\" // empty" "$json_path")

    # Fall back to opencode for unknown editors
    if [[ -z "$entry" ]]; then
        entry=$(jq -r '.editors.opencode // empty' "$json_path")
    fi

    if [[ -z "$entry" ]]; then
        log_error "No editor entry found for '$editor' and no opencode fallback."
        return 1
    fi

    local image_tag cmd_name config_path env_vars legacy_path
    image_tag=$(echo "$entry" | jq -r '.image_tag')
    cmd_name=$(echo "$entry" | jq -r '.cmd_name')
    config_path=$(echo "$entry" | jq -r '.config_path')
    env_vars=$(echo "$entry" | jq -r '[.env_vars[]] | join(",")')
    legacy_path=$(echo "$entry" | jq -r '.legacy_path // ""')

    echo "${image_tag}|${cmd_name}|${config_path}|${env_vars}|${legacy_path}"
}

ensure_host_path_exists() {
    local path="$1"
    local rel_path="$2"

    if [[ ! -e "$path" ]]; then
        local filename=$(basename "$rel_path")
        if [[ "$filename" == *.* ]] && { [[ "${filename:0:1}" != "." ]] || [[ "$filename" == *.*.* ]]; }; then
            mkdir -p "$(dirname "$path")"
            touch "$path"
        else
            mkdir -p "$path"
        fi
    fi
}
