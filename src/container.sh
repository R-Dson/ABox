# ==============================================================================
# Container Execution
# ==============================================================================

# Build environment variable flags for container
# Usage: build_env_flags <env_var_names>
# Returns: String of -e flags
build_env_flags() {
    local env_var_names="$1"
    local env_flags=""
    local IFS=',' 
    local vars
    read -ra vars <<< "$env_var_names"
    
    for env_var in "${vars[@]}"; do
        [[ -n "${!env_var}" ]] && env_flags="$env_flags -e $env_var"
    done
    
    echo "$env_flags"
}

# Build config volume mounts for the container
# Usage: build_config_mounts <editor_name> <config_rel_path> <legacy_path>
build_config_mounts() {
    local editor_name="$1"
    local config_rel_path="$2"
    local legacy_path="$3"
    local guest_config_path="/home/agent/$config_rel_path"
    
    local mounts="-v $CONFIG_VOL:$guest_config_path"
    
    [[ -n "$legacy_path" ]] && mounts="$mounts -v $CONFIG_VOL:/home/agent/$legacy_path"
    mounts="$mounts -v $CACHE_VOL:/home/agent/.cache/$editor_name"
    mounts="$mounts -v $STATE_VOL:/home/agent/.local/state/$editor_name"
    mounts="$mounts -v $SHARE_VOL:/home/agent/.local/share/$editor_name"
    
    # Mount Claude skills if they exist
    if [[ -d "$HOME/.claude" ]]; then
        mounts="$mounts -v $HOME/.claude:/home/agent/.claude:ro,z"
    elif [[ -d "$HOME/.claude/skills" ]]; then
        mounts="$mounts -v $HOME/.claude/skills:/home/agent/.claude/skills:ro,z"
    fi
    
    # Mount gitconfig if it exists
    if [[ -f "$HOME/.gitconfig" ]]; then
        mounts="$mounts -v $HOME/.gitconfig:/home/agent/.gitconfig:ro,z"
    fi

    # Mount homebrew
    mounts="$mounts -v abox-brew:/home/linuxbrew/.linuxbrew"

    echo "$mounts"
}

# Build workspace mount based on exclusion settings
# Usage: build_workspace_mount <target_dir> <use_exclusions>
build_workspace_mount() {
    local target_dir="$1"
    local use_exclusions="$2"
    
    if [[ "$use_exclusions" == "true" ]]; then
        echo "-v $WORKSPACE_VOL:/workspace"
    else
        echo "-v $target_dir:/workspace"
    fi
}

# Determine pull policy based on offline flag
# Usage: get_pull_policy <offline_flag>
get_pull_policy() {
    local offline="$1"
    [[ "$offline" == "true" ]] && echo "missing" || echo "always"
}

# Determine interactive flags based on TTY and CLI options
# Usage: get_interactive_flags <shell_flag> <force_it_flag>
get_interactive_flags() {
    local shell_flag="$1"
    local force_it="$2"
    
    if [[ -t 0 || "$shell_flag" == "true" || "$force_it" == "true" ]]; then
        echo "-it"
    else
        echo "-i"
    fi
}

# Run the container with all configured options
# Usage: run_container <image_name> <exec_cmd> [additional_args...]
run_container() {
    local image_name="$1"
    local exec_cmd="$2"
    shift 2
    
    local network_flags=""
    if [[ "$CLI_NO_INTERNET" == "true" ]]; then
        network_flags="--network none"
    elif [[ "$CLI_STRICT_NETWORK" == "true" ]]; then
        # Block common cloud metadata hostnames to prevent SSRF
        network_flags="--add-host metadata:127.0.0.1 --add-host metadata.google.internal:127.0.0.1 --add-host 169.254.169.254:127.0.0.1"
    fi
    
    $CONTAINER_RUNTIME run --rm $INTERACTIVE_FLAGS \
        $network_flags \
        --pull "$PULL_POLICY" \
        --name "abox-$EDITOR_NAME-$(basename "$TARGET_DIR")-$TIMESTAMP" \
        --hostname abx \
        --memory="${ABOX_MEMORY:-4g}" \
        --cpus="${ABOX_CPUS:-2}" \
        -e HOST_UID="$HOST_UID" \
        -e HOST_GID="$HOST_GID" \
        $ENV_FLAGS \
        --cap-drop=ALL \
        --cap-add=CHOWN \
        --cap-add=SETUID \
        --cap-add=SETGID \
        --cap-add=DAC_OVERRIDE \
        --security-opt=no-new-privileges \
        $WORKSPACE_MOUNT \
        $CONFIG_MOUNTS \
        "$image_name" "$exec_cmd" "$@"
}

# Determine the command to execute inside container
# Usage: get_exec_cmd <command_name> <shell_flag>
get_exec_cmd() {
    local command_name="$1"
    local shell_flag="$2"
    
    [[ "$shell_flag" == "true" ]] && echo "bash" || echo "$command_name"
}

# ==============================================================================
# Volume Management (Container-related)
# ==============================================================================

# Initialize volume names based on editor and timestamp
# Usage: init_volumes <editor_name> <timestamp>
# Sets: CONFIG_VOL, CACHE_VOL, STATE_VOL, SHARE_VOL, WORKSPACE_VOL, VOL_ID
init_volumes() {
    local editor_name="$1"
    local timestamp="$2"
    
    VOL_ID="${editor_name}-${timestamp}"
    CONFIG_VOL="abox-config-$VOL_ID"
    CACHE_VOL="abox-cache-$VOL_ID"
    STATE_VOL="abox-state-$VOL_ID"
    SHARE_VOL="abox-share-$VOL_ID"
    WORKSPACE_VOL="abox-workspace-$VOL_ID"
    
    # Export so they're available globally
    export VOL_ID CONFIG_VOL CACHE_VOL STATE_VOL SHARE_VOL WORKSPACE_VOL
}

# Create all required volumes
# Usage: create_volumes <use_exclusions>
create_volumes() {
    local use_exclusions="$1"
    
    for vol in "$CONFIG_VOL" "$CACHE_VOL" "$STATE_VOL" "$SHARE_VOL"; do
        $CONTAINER_RUNTIME volume create "$vol" > /dev/null 2>&1
    done
    
    if [[ "$use_exclusions" == "true" ]]; then
        $CONTAINER_RUNTIME volume create "$WORKSPACE_VOL" > /dev/null 2>&1
    fi
}

# Setup cleanup trap to remove volumes on exit
# Usage: setup_volume_cleanup <use_exclusions>
setup_volume_cleanup() {
    local use_exclusions="$1"
    
    cleanup() {
        if [[ "$use_exclusions" == "true" ]]; then
            $CONTAINER_RUNTIME volume rm "$WORKSPACE_VOL" > /dev/null 2>&1
        fi
        $CONTAINER_RUNTIME volume rm "$CONFIG_VOL" "$CACHE_VOL" "$STATE_VOL" "$SHARE_VOL" > /dev/null 2>&1
    }
    trap cleanup EXIT
}

# Initialize volume ownership with proper UID/GID
# Usage: init_volume_ownership <use_exclusions>
init_volume_ownership() {
    local use_exclusions="$1"
    
    local vol_mounts="-v $CONFIG_VOL:/config -v $CACHE_VOL:/cache -v $STATE_VOL:/state -v $SHARE_VOL:/share"
    local chown_targets="/config /cache /state /share"
    
    if [[ "$use_exclusions" == "true" ]]; then
        vol_mounts="$vol_mounts -v $WORKSPACE_VOL:/workspace"
        chown_targets="$chown_targets /workspace"
    fi
    
    $CONTAINER_RUNTIME run --rm \
        $vol_mounts \
        alpine sh -c "chown -R $HOST_UID:$HOST_GID $chown_targets"
}
