#!/bin/bash
# ==============================================================================
# ABox - Agnostic Sandbox Runtime
# ==============================================================================

set -o pipefail

# --- Configuration & Defaults ---
ABX_CONF="$HOME/.config/abx.conf"
HOST_UID=${HOST_UID:-$(id -u)}
HOST_GID=${HOST_GID:-$(id -g)}
TIMESTAMP=$(date +%s)

# Source helper modules
SCRIPT_DIR="$(dirname "$(readlink -f "$0")")"
source "$SCRIPT_DIR/helpers.sh"
source "$SCRIPT_DIR/exclusion.sh"
source "$SCRIPT_DIR/container.sh"
source "$SCRIPT_DIR/sync.sh"

# --- Main Execution ---

CLI_EDITOR=""
CLI_SHELL=false
CLI_IT=false
CLI_OFFLINE=false
SET_DEFAULT=""
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
        *)                  POSITIONAL_ARGS+=("$1"); shift ;;
    esac
done

if [[ -n "$SET_DEFAULT" ]]; then
    mkdir -p "$(dirname "$ABX_CONF")"
    echo "EDITOR=$SET_DEFAULT" > "$ABX_CONF"
    echo "Default editor set to: $SET_DEFAULT"
    if [[ ${#POSITIONAL_ARGS[@]} -eq 0 && -z "$CLI_EDITOR" && "$CLI_SHELL" == "false" && "$CLI_IT" == "false" ]]; then
        exit 0
    fi
fi

set -- "${POSITIONAL_ARGS[@]}"

EDITOR_NAME="$CLI_EDITOR"
[[ -z "$EDITOR_NAME" ]] && EDITOR_NAME="$ABOX_EDITOR"
if [[ -z "$EDITOR_NAME" && -f "$ABX_CONF" ]]; then
    EDITOR_NAME=$(grep "^EDITOR=" "$ABX_CONF" | cut -d= -f2)
fi
[[ -z "$EDITOR_NAME" ]] && EDITOR_NAME="opencode"

CONTAINER_RUNTIME=$(detect_runtime) || exit 1
IFS='|' read -r IMAGE_NAME COMMAND_NAME CONFIG_REL_PATH ENV_VAR_NAMES LEGACY_PATH <<< "$(get_editor_info "$EDITOR_NAME")"

TARGET_DIR="."
if [[ $# -gt 0 ]] && [[ -d "$1" ]]; then
    TARGET_DIR="$1"
    shift
fi
TARGET_DIR=$(cd "$TARGET_DIR" && pwd)

EXCLUDE_FILE="$TARGET_DIR/.abxignore"
USE_EXCLUSIONS=false
[[ -f "$EXCLUDE_FILE" ]] && USE_EXCLUSIONS=true

HOST_CONFIG_PATH="$HOME/$CONFIG_REL_PATH"
HOST_CACHE="$HOME/.cache/$EDITOR_NAME"
HOST_STATE="$HOME/.local/state/$EDITOR_NAME"
HOST_SHARE="$HOME/.local/share/$EDITOR_NAME"

ensure_host_path_exists "$HOST_CONFIG_PATH" "$CONFIG_REL_PATH"
mkdir -p "$HOST_CACHE" "$HOST_STATE" "$HOST_SHARE"

# Initialize and create volumes
init_volumes "$EDITOR_NAME" "$TIMESTAMP"
create_volumes "$USE_EXCLUSIONS"
setup_volume_cleanup "$USE_EXCLUSIONS"
init_volume_ownership "$USE_EXCLUSIONS"

echo "Syncing data to sandbox..."
sync_to_vols
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Applying exclusions from $EXCLUDE_FILE..."
    sync_workspace "$EXCLUDE_FILE" "$WORKSPACE_VOL"
fi

# Prepare container execution
INTERACTIVE_FLAGS=$(get_interactive_flags "$CLI_SHELL" "$CLI_IT")
EXEC_CMD=$(get_exec_cmd "$COMMAND_NAME" "$CLI_SHELL")
ENV_FLAGS=$(build_env_flags "$ENV_VAR_NAMES")
CONFIG_MOUNTS=$(build_config_mounts "$EDITOR_NAME" "$CONFIG_REL_PATH" "$LEGACY_PATH")
WORKSPACE_MOUNT=$(build_workspace_mount "$TARGET_DIR" "$USE_EXCLUSIONS")
PULL_POLICY=$(get_pull_policy "$CLI_OFFLINE")

# Run container
run_container "$IMAGE_NAME" "$EXEC_CMD" "$@"
EXIT_CODE=$?

echo "Syncing changes back to host..."
sync_from_vols
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Syncing workspace changes back to $TARGET_DIR..."
    sync_workspace_back "$TARGET_DIR" "$WORKSPACE_VOL"
fi

exit $EXIT_CODE
