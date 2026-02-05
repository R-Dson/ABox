# ==============================================================================
# Volume Synchronization Logic
# ==============================================================================

sync_to_vols() {
    local mounts=""
    local script="set -e;"
    
    # Config
    if [[ -e "$HOST_CONFIG_PATH" ]]; then
        if [[ -f "$HOST_CONFIG_PATH" ]]; then
            mounts="$mounts -v $HOST_CONFIG_PATH:/host/config_file:ro"
            script="$script cp /host/config_file /vol/config/$(basename "$HOST_CONFIG_PATH") &"
        else
            mounts="$mounts -v $HOST_CONFIG_PATH:/host/config:ro"
            script="$script cp -r /host/config/. /vol/config/ &"
        fi
        mounts="$mounts -v $CONFIG_VOL:/vol/config"
    fi
    
    # Cache/State/Share (Always directories)
    for type in CACHE STATE SHARE; do
        local host_var="HOST_$type"
        local vol_var="${type}_VOL"
        if [[ -e "${!host_var}" ]]; then
            mounts="$mounts -v ${!host_var}:/host/${type,,}:ro -v ${!vol_var}:/vol/${type,,}"
            script="$script cp -r /host/${type,,}/. /vol/${type,,}/ &"
        fi
    done
    
    script="$script wait; chown -R $HOST_UID:$HOST_GID /vol/config /vol/cache /vol/state /vol/share 2>/dev/null || true"
    
    if [[ -n "$mounts" ]]; then
        $CONTAINER_RUNTIME run --rm --user $HOST_UID:$HOST_GID $mounts alpine sh -c "$script"
    fi
}

sync_from_vols() {
    local mounts=""
    local script="set -e;"
    
    # Config
    if [[ -f "$HOST_CONFIG_PATH" ]]; then
        mounts="$mounts -v $CONFIG_VOL:/vol/config:ro -v $(dirname "$HOST_CONFIG_PATH"):/host/config_dir"
        script="$script cp /vol/config/$(basename "$HOST_CONFIG_PATH") /host/config_dir/ &"
    else
        mounts="$mounts -v $CONFIG_VOL:/vol/config:ro -v $HOST_CONFIG_PATH:/host/config"
        script="$script cp -r /vol/config/. /host/config/ &"
    fi
    
    # Others
    for type in CACHE STATE SHARE; do
        local host_var="HOST_$type"
        local vol_var="${type}_VOL"
        mounts="$mounts -v ${!vol_var}:/vol/${type,,}:ro -v ${!host_var}:/host/${type,,}"
        script="$script cp -r /vol/${type,,}/. /host/${type,,}/ &"
    done
    
    script="$script wait"
    $CONTAINER_RUNTIME run --rm --user $HOST_UID:$HOST_GID $mounts alpine sh -c "$script"
}

sync_workspace() {
    local exclude_file="$1"
    local workspace_vol="$2"
    local -a patterns=()
    local include_list="/tmp/abx_include_list_$$"
    local temp_tar="/tmp/abx_sync_$$.tar"
    
    # Read exclusion patterns into array
    if [[ -f "$exclude_file" ]]; then
        while IFS= read -r pattern; do
            [[ -n "$pattern" ]] && patterns+=("$pattern")
        done < <(read_exclusions "$exclude_file")
    fi
    
    if [[ ${#patterns[@]} -eq 0 ]]; then
        # No exclusions - simple tar copy
        (
            cd "$TARGET_DIR" || exit 1
            tar -cf - . | $CONTAINER_RUNTIME run --rm -i --user $HOST_UID:$HOST_GID -v "$workspace_vol:/dst" alpine tar -xf - -C /dst
        )
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
    
    # Create tar with only included files and stream to container
    (
        cd "$TARGET_DIR" || exit 1
        tar -cf "$temp_tar" -T "$include_list" 2>/dev/null
    )
    
    # Extract to workspace volume
    $CONTAINER_RUNTIME run --rm -i --user $HOST_UID:$HOST_GID -v "$workspace_vol:/dst" alpine tar -xf "$temp_tar" -C /dst
    local exit_code=$?
    
    # Cleanup
    rm -f "$include_list" "$temp_tar"
    
    return $exit_code
}

sync_workspace_back() {
    local target_dir="$1"
    local workspace_vol="$2"
    
    # Stream files back from volume to host using tar
    $CONTAINER_RUNTIME run --rm --user $HOST_UID:$HOST_GID -v "$workspace_vol:/src" alpine tar -cf - -C /src . | \
    ( cd "$target_dir" && tar -xf - )
}
