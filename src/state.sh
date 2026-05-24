# ==============================================================================
# Global Variable Contract
# ==============================================================================
# This file documents every global variable shared across modules.
# Source this file first in the bundle order so it serves as a living reference.
#
# Modules: main.sh → helpers.sh → exclusion.sh → audit.sh → container.sh → sync.sh
# ==============================================================================

# --- Set by main.sh, read everywhere ---
# EDITOR_NAME           The resolved editor name (from CLI flag, env var, or config)
# TIMESTAMP             Unix timestamp for this session
# CONTAINER_RUNTIME     "docker" or "podman" (set by detect_runtime)
# HOST_UID              Host user ID (default: $(id -u))
# HOST_GID              Host group ID (default: $(id -g))
# TARGET_DIR            Absolute path to the workspace directory
# EXCLUDE_FILE          Path to the active .abxignore (or temp file)
# USE_EXCLUSIONS        "true" if exclusions are active
# EXIT_CODE             Container exit code

# --- Set by main.sh CLI parser ---
# CLI_EDITOR            --editor value
# CLI_SHELL             --shell flag
# CLI_IT                --force-it flag
# CLI_OFFLINE           --offline flag
# CLI_NO_INTERNET       --no-internet flag
# CLI_STRICT_NETWORK    --strict-network flag
# CLI_VERBOSE           --verbose flag
# CLI_FORCE_SYNC        --force-sync flag
# CLI_ENV               Array of --env KEY values
# CLI_EXCLUDE_URL       --exclude-url value
# SET_DEFAULT           --default-editor value
# SET_EXCLUDE_URL       --default-exclude-url value

# --- Set by get_editor_info(), read by container.sh and main.sh ---
# IMAGE_NAME            Docker image tag (e.g. ghcr.io/r-dson/abox:claude)
# COMMAND_NAME          Binary name inside container (e.g. claude)
# CONFIG_REL_PATH       Relative config path (e.g. .claude)
# ENV_VAR_NAMES         Comma-separated env var names (e.g. ANTHROPIC_API_KEY)
# LEGACY_PATH           Legacy config path for backward compat (e.g. .opencode)

# --- Set by init_volumes() in container.sh, read by sync.sh and container.sh ---
# VOL_ID                Volume identifier suffix (e.g. "claude-1234567890")
# CONFIG_VOL            Docker volume name for config data
# CACHE_VOL             Docker volume name for cache data
# STATE_VOL             Docker volume name for state data
# SHARE_VOL             Docker volume name for share data
# WORKSPACE_VOL         Docker volume name for workspace data (only when USE_EXCLUSIONS=true)

# --- Set by container.sh run_container(), read by cleanup trap ---
# STRICT_NET_NAME       Name of the isolated Docker network (--strict-network)

# --- Set by container.sh build functions, read by run_container() ---
# INTERACTIVE_FLAGS     "-it" or "-i" based on TTY/flags
# EXEC_CMD              Command to run inside container
# ENV_FLAGS             -e flags for API keys and custom env vars
# CONFIG_MOUNTS         -v flags for config/cache/state/share volumes
# WORKSPACE_MOUNT       -v flag for workspace (volume or bind mount)
# PULL_POLICY           "always" or "missing" based on --offline

# --- Host paths set by main.sh, read by sync.sh ---
# HOST_CONFIG_PATH      Absolute path to editor config (e.g. ~/.claude)
# HOST_CACHE            Absolute path to editor cache (e.g. ~/.cache/claude)
# HOST_STATE            Absolute path to editor state (e.g. ~/.local/state/claude)
# HOST_SHARE            Absolute path to editor share (e.g. ~/.local/share/claude)

# --- Constants defined in sync.sh ---
# SYNC_IMAGE            Lightweight Alpine image for sync operations
# ABX_SECURITY_EXCLUSIONS  Array of hardcoded security exclusion patterns
