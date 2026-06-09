# ==============================================================================
# Volume Synchronization Logic
# ==============================================================================

# Lightweight image for sync operations (cp, tar, chown) — avoids pulling the
# full editor image (~1GB) for what amounts to simple file copies (~5MB Alpine).
readonly SYNC_IMAGE="${ABOX_SYNC_IMAGE:-ghcr.io/r-dson/abox:sync}"

# Snapshot host file mtimes before session starts, used for conflict detection.
# Usage: snapshot_mtimes <paths...>
# Stores to /tmp/abx_mtimes_$VOL_ID
snapshot_mtimes() {
    local snapshot_file="/tmp/abx_mtimes_${VOL_ID}"
    : > "$snapshot_file"
    for path in "$@"; do
        if [[ -e "$path" ]]; then
            find "$path" -type f -printf "%T@ %p\n" 2>/dev/null >> "$snapshot_file" || true
        fi
    done
}

# Check if host files changed since snapshot. Returns list of changed files.
# Usage: check_conflicts
# Reads from /tmp/abx_mtimes_$VOL_ID
check_conflicts() {
    local snapshot_file="/tmp/abx_mtimes_${VOL_ID}"
    [[ ! -f "$snapshot_file" ]] && return 1

    local conflicts=""
    while IFS=" " read -r orig_mtime filepath; do
        if [[ -f "$filepath" ]]; then
            local current_mtime
            current_mtime=$(stat -c "%Y" "$filepath" 2>/dev/null || echo "0")
            if [[ "$current_mtime" != "$orig_mtime" ]]; then
                conflicts="$conflicts\n  $filepath"
            fi
        fi
    done < "$snapshot_file"

    if [[ -n "$conflicts" ]]; then
        echo -e "WARNING: Host files changed during session:$conflicts"
        echo "Use --force-sync to overwrite anyway."
        return 0  # conflicts found
    fi
    return 1  # no conflicts
}

sync_to_vols() {
    local mounts=""
    local script="set -e;"
    
    # Config
    if [[ -e "$HOST_CONFIG_PATH" ]]; then
        if [[ -f "$HOST_CONFIG_PATH" ]]; then
            mounts="$mounts -v $HOST_CONFIG_PATH:/host/config_file:ro"
            script="$script mkdir -p /vol/config.tmp && cp /host/config_file /vol/config.tmp/ && mv /vol/config.tmp /vol/config &"
        else
            mounts="$mounts -v $HOST_CONFIG_PATH:/host/config:ro"
            script="$script mkdir -p /vol/config.tmp && cp -r /host/config/. /vol/config.tmp/ && mv /vol/config.tmp /vol/config &"
        fi
        mounts="$mounts -v $CONFIG_VOL:/vol/config"
    fi
    
    # Cache/State/Share (Always directories)
    for type in CACHE STATE SHARE; do
        local host_var="HOST_$type"
        local vol_var="${type}_VOL"
        if [[ -e "${!host_var}" ]]; then
            mounts="$mounts -v ${!host_var}:/host/${type,,}:ro -v ${!vol_var}:/vol/${type,,}"
            script="$script mkdir -p /vol/${type,,}.tmp && cp -r /host/${type,,}/. /vol/${type,,}.tmp/ && mv /vol/${type,,}.tmp /vol/${type,,} &"
        fi
    done
    
    script="$script wait; chown -R $HOST_UID:$HOST_GID /vol/config /vol/cache /vol/state /vol/share 2>/dev/null || true"
    
    if [[ -n "$mounts" ]]; then
        $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh $mounts $SYNC_IMAGE -c "$script"
    fi
}

sync_from_vols() {
    local mounts=""
    local script="set -e;"
    
    # Config
    if [[ -f "$HOST_CONFIG_PATH" ]]; then
        mounts="$mounts -v $CONFIG_VOL:/vol/config:ro -v $(dirname "$HOST_CONFIG_PATH"):/host/config_dir"
        script="$script tar -cf - -C /vol/config $(basename "$HOST_CONFIG_PATH") | tar -xf - -C /host/config_dir &"
    else
        mounts="$mounts -v $CONFIG_VOL:/vol/config:ro -v $HOST_CONFIG_PATH:/host/config"
        script="$script tar -cf - -C /vol/config . | tar -xf - -C /host/config &"
    fi
    
    # Others
    for type in CACHE STATE SHARE; do
        local host_var="HOST_$type"
        local vol_var="${type}_VOL"
        mounts="$mounts -v ${!vol_var}:/vol/${type,,}:ro -v ${!host_var}:/host/${type,,}"
        script="$script tar -cf - -C /vol/${type,,} . | tar -xf - -C /host/${type,,} &"
    done
    
    script="$script wait"
    $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh $mounts $SYNC_IMAGE -c "$script"
}

# Hardcoded security patterns — always excluded from workspace sync
readonly ABX_SECURITY_EXCLUSIONS=(".ssh" ".aws" ".env" ".gnupg" "**/*key" "**/*.pem")

sync_workspace() {
    local exclude_file="$1"
    local workspace_vol="$2"
    local -a patterns=()
    local include_list
    include_list=$(mktemp)
    
    # Read exclusion patterns into array
    if [[ -f "$exclude_file" ]]; then
        while IFS= read -r pattern; do
            [[ -n "$pattern" ]] && patterns+=("$pattern")
        done < <(read_exclusions "$exclude_file")
    fi
    
    # Add hardcoded security exclusions
    patterns+=("${ABX_SECURITY_EXCLUSIONS[@]}")
    
    if [[ ${#patterns[@]} -eq 0 ]]; then
        # No exclusions - simple tar copy via pipe
        (
            cd "$TARGET_DIR" || exit 1
            tar -cf - . | $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/dst" $SYNC_IMAGE -c "tar -xf - -C /dst"
        )
        rm -f "$include_list"
        return $?
    fi
    
    # Build list of files to include (not excluded)
    (
        cd "$TARGET_DIR" || exit 1
        find . -type f -print0 2>/dev/null | while IFS= read -r -d '' path; do
            # Remove leading ./
            path="${path#./}"
            if ! is_excluded "$path" "${patterns[@]}"; then
                printf '%s\n' "$path"
            fi
        done
    ) > "$include_list"
    
    # Stream tar directly via pipe — no intermediate file on host disk
    local exit_code=0
    (
        cd "$TARGET_DIR" || exit 1
        tar -cf - -T "$include_list" 2>/dev/null | \
            $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/dst" $SYNC_IMAGE -c "tar -xf - -C /dst"
    ) || exit_code=$?
    
    rm -f "$include_list"
    return $exit_code
}

sync_workspace_back() {
    local target_dir="$1"
    local workspace_vol="$2"
    local exclude_file="$3"
    local -a patterns=()
    local include_list
    include_list=$(mktemp)
    local raw_list
    raw_list=$(mktemp)
    
    # Read exclusion patterns into array
    if [[ -f "$exclude_file" ]]; then
        while IFS= read -r pattern; do
            [[ -n "$pattern" ]] && patterns+=("$pattern")
        done < <(read_exclusions "$exclude_file")
    fi
    
    # Add hardcoded security exclusions
    patterns+=("${ABX_SECURITY_EXCLUSIONS[@]}")
    
    # Build list of files to include (not excluded)
    # 1. Get list of files from volume (run find as root to ensure visibility)
    $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh -v "$workspace_vol:/src" -w /src $SYNC_IMAGE -c "find . -type f -print0" 2>/dev/null > "$raw_list"
    
    # 2. Filter on host
    while IFS= read -r -d '' path; do
        # Remove leading ./
        path="${path#./}"
        if ! is_excluded "$path" "${patterns[@]}"; then
            printf '%s\n' "$path"
        fi
    done < "$raw_list" > "$include_list"
    
    # Stream files back from volume to host — pipe directly, no intermediate file
    if [[ -s "$include_list" ]]; then
        $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/src" $SYNC_IMAGE -c "tar -cf - -C /src -T -" < "$include_list" | \
            ( cd "$target_dir" && tar -xf - )
    fi
    
    # Cleanup
    rm -f "$include_list" "$raw_list"
}
