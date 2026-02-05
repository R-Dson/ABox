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

VOL_ID="$EDITOR_NAME-$TIMESTAMP"
CONFIG_VOL="abox-config-$VOL_ID"
CACHE_VOL="abox-cache-$VOL_ID"
STATE_VOL="abox-state-$VOL_ID"
SHARE_VOL="abox-share-$VOL_ID"
WORKSPACE_VOL="abox-workspace-$VOL_ID"

for vol in "$CONFIG_VOL" "$CACHE_VOL" "$STATE_VOL" "$SHARE_VOL"; do
    $CONTAINER_RUNTIME volume create "$vol" > /dev/null 2>&1
done

if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    $CONTAINER_RUNTIME volume create "$WORKSPACE_VOL" > /dev/null 2>&1
fi

cleanup() {
    if [[ "$USE_EXCLUSIONS" == "true" ]]; then
        $CONTAINER_RUNTIME volume rm "$WORKSPACE_VOL" > /dev/null 2>&1
    fi
    $CONTAINER_RUNTIME volume rm "$CONFIG_VOL" "$CACHE_VOL" "$STATE_VOL" "$SHARE_VOL" > /dev/null 2>&1
}
trap cleanup EXIT

# Initialize Volume Ownership
vol_mounts="-v $CONFIG_VOL:/config -v $CACHE_VOL:/cache -v $STATE_VOL:/state -v $SHARE_VOL:/share"
chown_targets="/config /cache /state /share"
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    vol_mounts="$vol_mounts -v $WORKSPACE_VOL:/workspace"
    chown_targets="$chown_targets /workspace"
fi
$CONTAINER_RUNTIME run --rm \
    $vol_mounts \
    alpine sh -c "chown -R $HOST_UID:$HOST_GID $chown_targets"

echo "Syncing data to sandbox..."
sync_to_vols
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Applying exclusions from $EXCLUDE_FILE..."
    sync_workspace "$EXCLUDE_FILE" "$WORKSPACE_VOL"
fi

INTERACTIVE_FLAGS="-i"
[[ -t 0 || "$CLI_SHELL" == "true" || "$CLI_IT" == "true" ]] && INTERACTIVE_FLAGS="-it"

EXEC_CMD="$COMMAND_NAME"
[[ "$CLI_SHELL" == "true" ]] && EXEC_CMD="bash"

ENV_FLAGS=""
IFS=',' read -ra ADDR <<< "$ENV_VAR_NAMES"
for env_var in "${ADDR[@]}"; do
    [[ -n "${!env_var}" ]] && ENV_FLAGS="$ENV_FLAGS -e $env_var"
done

GUEST_CONFIG_PATH="/home/agent/$CONFIG_REL_PATH"
CONFIG_MOUNTS="-v $CONFIG_VOL:$GUEST_CONFIG_PATH"
[[ -n "$LEGACY_PATH" ]] && CONFIG_MOUNTS="$CONFIG_MOUNTS -v $CONFIG_VOL:/home/agent/$LEGACY_PATH"
CONFIG_MOUNTS="$CONFIG_MOUNTS -v $CACHE_VOL:/home/agent/.cache/$EDITOR_NAME"
CONFIG_MOUNTS="$CONFIG_MOUNTS -v $STATE_VOL:/home/agent/.local/state/$EDITOR_NAME"
CONFIG_MOUNTS="$CONFIG_MOUNTS -v $SHARE_VOL:/home/agent/.local/share/$EDITOR_NAME"

[[ -d "$HOME/.claude" ]] && CONFIG_MOUNTS="$CONFIG_MOUNTS -v $HOME/.claude:/home/agent/.claude:ro,z"
CONFIG_MOUNTS="$CONFIG_MOUNTS -v abox-brew:/home/linuxbrew/.linuxbrew"

PULL_POLICY="always"
[[ "$CLI_OFFLINE" == "true" ]] && PULL_POLICY="missing"

WORKSPACE_MOUNT="-v $TARGET_DIR:/workspace"
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    WORKSPACE_MOUNT="-v $WORKSPACE_VOL:/workspace"
fi

$CONTAINER_RUNTIME run --rm $INTERACTIVE_FLAGS \
    --pull "$PULL_POLICY" \
    --name "abox-$EDITOR_NAME-$(basename "$TARGET_DIR")-$TIMESTAMP" \
    --hostname abx \
    -e HOST_UID="$HOST_UID" \
    -e HOST_GID="$HOST_GID" \
    $ENV_FLAGS \
    --cap-drop=ALL \
    --cap-add=CHOWN \
    --cap-add=SETUID \
    --cap-add=SETGID \
    --cap-add=DAC_OVERRIDE \
    $WORKSPACE_MOUNT \
    $CONFIG_MOUNTS \
    "$IMAGE_NAME" "$EXEC_CMD" "$@"
EXIT_CODE=$?

echo "Syncing changes back to host..."
sync_from_vols
if [[ "$USE_EXCLUSIONS" == "true" ]]; then
    echo "Syncing workspace changes back to $TARGET_DIR..."
    sync_workspace_back "$TARGET_DIR" "$WORKSPACE_VOL"
fi

exit $EXIT_CODE
