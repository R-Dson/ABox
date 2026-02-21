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
        $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh $mounts $IMAGE_NAME -c "$script"
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
    $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh $mounts $IMAGE_NAME -c "$script"
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
    
    # Add hardcoded security exclusions
    patterns+=(".ssh" ".aws" ".env" ".gnupg" "**/*key" "**/*.pem")
    
    if [[ ${#patterns[@]} -eq 0 ]]; then
        # No exclusions - simple tar copy
        (
            cd "$TARGET_DIR" || exit 1
            tar -cf - . | $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/dst" $IMAGE_NAME -c "tar -xf - -C /dst"
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
    $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/dst" $IMAGE_NAME -c "tar -xf - -C /dst" < "$temp_tar"
    local exit_code=$?
    
    # Cleanup
    rm -f "$include_list" "$temp_tar"
    
    return $exit_code
}

sync_workspace_back() {
    local target_dir="$1"
    local workspace_vol="$2"
    local exclude_file="$3"
    local -a patterns=()
    local include_list=$(mktemp)
    local raw_list=$(mktemp)
    local temp_tar=$(mktemp)
    
    # Read exclusion patterns into array
    if [[ -f "$exclude_file" ]]; then
        while IFS= read -r pattern; do
            [[ -n "$pattern" ]] && patterns+=("$pattern")
        done < <(read_exclusions "$exclude_file")
    fi
    
    # Add hardcoded security exclusions
    patterns+=(".ssh" ".aws" ".env" ".gnupg" "**/*key" "**/*.pem")
    
    # Build list of files to include (not excluded)
    # 1. Get list of files from volume (run find as root to ensure visibility)
    $CONTAINER_RUNTIME run --rm --user 0:0 --entrypoint sh -v "$workspace_vol:/src" -w /src $IMAGE_NAME -c "find . -type f -print0" 2>/dev/null > "$raw_list"
    
    # 2. Filter on host
    while IFS= read -r -d '' path; do
        # Remove leading ./
        path="${path#./}"
        if ! is_excluded "$path" "${patterns[@]}"; then
            printf '%s\n' "$path"
        fi
    done < "$raw_list" > "$include_list"
    
    # Stream files back from volume to host using tar, only including filtered files
    if [[ -s "$include_list" ]]; then
        $CONTAINER_RUNTIME run --rm -i --user 0:0 --entrypoint sh -v "$workspace_vol:/src" $IMAGE_NAME -c "tar -cf - -C /src -T -" < "$include_list" > "$temp_tar"
        if [[ -s "$temp_tar" ]]; then
            ( cd "$target_dir" && tar -xf "$temp_tar" )
        fi
    fi
    
    # Cleanup
    rm -f "$include_list" "$raw_list" "$temp_tar"
}

