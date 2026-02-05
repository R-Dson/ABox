# ==============================================================================
# Helper Functions
# ==============================================================================

log_error() {
    echo "ERROR: $*" >&2
}

read_exclusions() {
    local exclude_file="$1"
    local exclusions=()
    
    if [[ -f "$exclude_file" ]]; then
        while IFS= read -r line || [[ -n "$line" ]]; do
            line="${line#"${line%%[![:space:]]*}"}"
            [[ -z "$line" || "$line" == \#* ]] && continue
            exclusions+=("$line")
        done < "$exclude_file"
    fi
    
    printf '%s\n' "${exclusions[@]}"
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

get_editor_info() {
    local editor=$1
    case "$editor" in
        aider)   echo "ghcr.io/r-dson/abox:aider|aider|.aider.conf.yml|OPENAI_API_KEY,ANTHROPIC_API_KEY|" ;;
        claude)   echo "ghcr.io/r-dson/abox:claude|claude|.claude|ANTHROPIC_API_KEY|" ;;
        codex)   echo "ghcr.io/r-dson/abox:codex|codex|.codex||" ;;
        copilot)   echo "ghcr.io/r-dson/abox:copilot|copilot|.copilot|GITHUB_TOKEN|" ;;
        cursor)   echo "ghcr.io/r-dson/abox:cursor|cursor|.cursor||" ;;
        gemini)   echo "ghcr.io/r-dson/abox:gemini|gemini|.gemini|GOOGLE_API_KEY|" ;;
        goose)   echo "ghcr.io/r-dson/abox:goose|goose|.config/goose||" ;;
        vibe)   echo "ghcr.io/r-dson/abox:vibe|vibe|.vibe||" ;;
        opencode|*) echo "ghcr.io/r-dson/abox:opencode|opencode|.config/opencode||.opencode" ;;
    esac
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
