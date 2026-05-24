#!/bin/bash
# ==============================================================================
# ABox - Agnostic Sandbox Runtime
# ==============================================================================

set -o pipefail

# --- Configuration & Defaults ---
ABX_CONF="$HOME/.config/abx.conf"
ABX_JSON_CONF="$HOME/.config/abx/config.json"
HOST_UID=${HOST_UID:-$(id -u)}
HOST_GID=${HOST_GID:-$(id -g)}
# Source helper modules
SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
source "$SCRIPT_DIR/helpers.sh"
source "$SCRIPT_DIR/exclusion.sh"
source "$SCRIPT_DIR/audit.sh"
source "$SCRIPT_DIR/container.sh"
source "$SCRIPT_DIR/sync.sh"

TIMESTAMP=$(date +%s)

# --- Version flag handling ---
if [[ "$1" == "--version" ]] || [[ "$1" == "-v" ]]; then
    if [[ -n "$ABX_VERSION" ]]; then
        echo "$ABX_VERSION"
    else
        echo "(dev build)"
    fi
    exit 0
fi

# --- Main Execution ---

if [[ "$1" == "audit" ]]; then
    shift
    run_audit "$@"
    exit 0
fi

CLI_EDITOR=""
CLI_SHELL=false
CLI_IT=false
CLI_OFFLINE=false
CLI_STRICT_NETWORK=false
CLI_NO_INTERNET=false
CLI_VERBOSE=false
CLI_ENV=()
CLI_FORCE_SYNC=false
SET_DEFAULT=""
CLI_EXCLUDE_URL=""
SET_EXCLUDE_URL=""
POSITIONAL_ARGS=()

while [[ $# -gt 0 ]]; do
    case $1 in
        --editor)           CLI_EDITOR="$2"; shift 2 ;;
        --editor=*)         CLI_EDITOR="${1#*=}"; shift ;;
        --default-editor)   SET_DEFAULT="$2"; shift 2 ;;
        --default-editor=*) SET_DEFAULT="${1#*=}"; shift ;;
        --shell)            CLI_SHELL=true; shift ;;
        --force-it)         CLI_IT=true; shift ;;
        --offline)          CLI_OFFLINE=true; shift ;;
        --strict-network)   CLI_STRICT_NETWORK=true; shift ;;
        --no-internet)      CLI_NO_INTERNET=true; shift ;;
        --verbose)         CLI_VERBOSE=true; shift ;;
        --force-sync)      CLI_FORCE_SYNC=true; shift ;;
        --exclude-url)      CLI_EXCLUDE_URL="$2"; shift 2 ;;
        --exclude-url=*)    CLI_EXCLUDE_URL="${1#*=}"; shift ;;
        --env)              CLI_ENV+=("$2"); shift 2 ;;
        --env=*)            CLI_ENV+=("${1#*=}"); shift ;;
        --default-exclude-url) SET_EXCLUDE_URL="$2"; shift 2 ;;
        --default-exclude-url=*) SET_EXCLUDE_URL="${1#*=}"; shift ;;
        *)                  POSITIONAL_ARGS+=("$1"); shift ;;
    esac
done

if [[ -n "$SET_DEFAULT" ]]; then
    ABX_JSON_DIR="$(dirname "$ABX_JSON_CONF")"
    mkdir -p "$ABX_JSON_DIR"
    local_exclude_url="${SET_EXCLUDE_URL:-}"
    if [[ -n "$local_exclude_url" ]]; then
        jq -n --arg ed "$SET_DEFAULT" --arg url "$local_exclude_url" '{editor: $ed, exclude_url: $url}' > "$ABX_JSON_CONF"
        echo "Default editor set to: $SET_DEFAULT"
        echo "Default exclude URL set to: $local_exclude_url"
    else
        jq -n --arg ed "$SET_DEFAULT" '{editor: $ed, exclude_url: ""}' > "$ABX_JSON_CONF"
        echo "Default editor set to: $SET_DEFAULT"
    fi
    # Also update legacy config for backward compat
    echo "EDITOR=$SET_DEFAULT" > "$ABX_CONF"
    if [[ -n "$local_exclude_url" ]]; then
        echo "EXCLUDE_URL=$local_exclude_url" >> "$ABX_CONF"
    fi
    if [[ ${#POSITIONAL_ARGS[@]} -eq 0 && -z "$CLI_EDITOR" && "$CLI_SHELL" == "false" && "$CLI_IT" == "false" ]]; then
        exit 0
    fi
fi

set -- "${POSITIONAL_ARGS[@]}"

log_setup

EDITOR_NAME="$CLI_EDITOR"
[[ -z "$EDITOR_NAME" ]] && EDITOR_NAME="$ABOX_EDITOR"

# Read editor from config: prefer JSON, fall back to legacy abx.conf
if [[ -z "$EDITOR_NAME" ]]; then
    if [[ -f "$ABX_JSON_CONF" ]]; then
        EDITOR_NAME=$(jq -r '.editor // empty' "$ABX_JSON_CONF")
    fi
    if [[ -z "$EDITOR_NAME" && -f "$ABX_CONF" ]]; then
        EDITOR_NAME=$(grep "^EDITOR=" "$ABX_CONF" | cut -d= -f2)
    fi
fi
[[ -z "$EDITOR_NAME" ]] && EDITOR_NAME="opencode"

EXCLUDE_URL="$CLI_EXCLUDE_URL"
if [[ -z "$EXCLUDE_URL" ]]; then
    if [[ -f "$ABX_JSON_CONF" ]]; then
        EXCLUDE_URL=$(jq -r '.exclude_url // empty' "$ABX_JSON_CONF")
    fi
    if [[ -z "$EXCLUDE_URL" && -f "$ABX_CONF" ]]; then
        EXCLUDE_URL=$(grep "^EXCLUDE_URL=" "$ABX_CONF" | cut -d= -f2)
    fi
fi

CONTAINER_RUNTIME=$(detect_runtime) || exit 1
IFS='|' read -r IMAGE_NAME COMMAND_NAME CONFIG_REL_PATH ENV_VAR_NAMES LEGACY_PATH <<< "$(get_editor_info "$EDITOR_NAME")"

if [[ -z "$IMAGE_NAME" || -z "$COMMAND_NAME" ]]; then
    log_error "Unknown editor '$EDITOR_NAME'. Check editors.json or run 'abx --help'."
    exit 1
fi

TARGET_DIR="."
if [[ $# -gt 0 ]] && [[ -d "$1" ]]; then
    TARGET_DIR="$1"
    shift
fi
TARGET_DIR=$(cd "$TARGET_DIR" && pwd)

EXCLUDE_FILE="$TARGET_DIR/.abxignore"
USE_EXCLUSIONS=false
TEMP_EXCLUDE_FILE=""

if [[ -f "$EXCLUDE_FILE" ]] || [[ -n "$EXCLUDE_URL" ]]; then
    USE_EXCLUSIONS=true
fi

if [[ -z "$EXCLUDE_FILE" ]] || [[ ! -f "$EXCLUDE_FILE" ]]; then
    for secret in .ssh .aws .env .gnupg; do
        if [[ -e "$TARGET_DIR/$secret" ]]; then
            USE_EXCLUSIONS=true
            break
        fi
    done
fi

if [[ -n "$EXCLUDE_URL" ]]; then
    local_patterns=""
    if [[ -f "$EXCLUDE_FILE" ]]; then
local_patterns=$(grep -vE '^\s*#' "$EXCLUDE_FILE" | grep -vE '^\s*$')
    fi
    
    url_patterns=$(fetch_exclusions "$EXCLUDE_URL") || exit 1
    
    unified_patterns=$(unify_patterns "$local_patterns" "$url_patterns")
    
    TEMP_EXCLUDE_FILE=$(mktemp)
    echo "$unified_patterns" > "$TEMP_EXCLUDE_FILE"
    EXCLUDE_FILE="$TEMP_EXCLUDE_FILE"
    
    trap 'rm -f "$TEMP_EXCLUDE_FILE"' EXIT
    
    if [[ -n "$local_patterns" ]]; then
        echo "Applying unified exclusions (local + $EXCLUDE_URL)..."
    else
        echo "Applying exclusions from $EXCLUDE_URL..."
    fi
fi

HOST_CONFIG_PATH="$HOME/$CONFIG_REL_PATH"
HOST_CACHE="$HOME/.cache/$EDITOR_NAME"
HOST_STATE="$HOME/.local/state/$EDITOR_NAME"
HOST_SHARE="$HOME/.local/share/$EDITOR_NAME"

ensure_host_path_exists "$HOST_CONFIG_PATH" "$CONFIG_REL_PATH"
mkdir -p "$HOST_CACHE" "$HOST_STATE" "$HOST_SHARE"

# Initialize and create volumes
init_volumes "$EDITOR_NAME" "$TIMESTAMP"
create_volumes "$USE_EXCLUSIONS"
# Setup cleanup trap to remove volumes on exit
setup_volume_cleanup "$USE_EXCLUSIONS"
init_volume_ownership "$USE_EXCLUSIONS"

echo "Syncing data to sandbox..."
snapshot_mtimes "$HOST_CONFIG_PATH" "$HOST_CACHE" "$HOST_STATE" "$HOST_SHARE"
sync_to_vols
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Applying exclusions from $EXCLUDE_FILE..."
    sync_workspace "$EXCLUDE_FILE" "$WORKSPACE_VOL"
fi

# Prepare container execution
INTERACTIVE_FLAGS=$(get_interactive_flags "$CLI_SHELL" "$CLI_IT")
EXEC_CMD=$(get_exec_cmd "$COMMAND_NAME" "$CLI_SHELL")
ENV_FLAGS=$(build_env_flags "$ENV_VAR_NAMES")

# Add custom env vars from --env flags and .abxenv file
for env_var_name in "${CLI_ENV[@]}"; do
    ENV_FLAGS="$ENV_FLAGS -e $env_var_name"
done
if [[ -f "$TARGET_DIR/.abxenv" ]]; then
    while IFS='=' read -r key _value; do
        [[ -n "$key" && "$key" != \#* ]] && ENV_FLAGS="$ENV_FLAGS -e $key"
    done < "$TARGET_DIR/.abxenv"
fi
CONFIG_MOUNTS=$(build_config_mounts "$EDITOR_NAME" "$CONFIG_REL_PATH" "$LEGACY_PATH")
WORKSPACE_MOUNT=$(build_workspace_mount "$TARGET_DIR" "$USE_EXCLUSIONS")
PULL_POLICY=$(get_pull_policy "$CLI_OFFLINE")

# Run container
run_container "$IMAGE_NAME" "$EXEC_CMD" "$@"
EXIT_CODE=$?

echo "Syncing changes back to host..."
if [[ "$CLI_FORCE_SYNC" != "true" ]] && check_conflicts; then
    echo "Skipping sync-back due to host file conflicts. Use --force-sync to override."
else
    sync_from_vols
fi
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Syncing workspace changes back to $TARGET_DIR..."
    sync_workspace_back "$TARGET_DIR" "$WORKSPACE_VOL" "$EXCLUDE_FILE"
fi


exit $EXIT_CODE
