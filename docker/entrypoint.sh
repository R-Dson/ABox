#!/bin/bash
set -e

USER_ID=${HOST_UID:-1000}
GROUP_ID=${HOST_GID:-1000}

# Validate inputs are integers
if ! [[ "$USER_ID" =~ ^[0-9]+$ ]] || ! [[ "$GROUP_ID" =~ ^[0-9]+$ ]]; then
    echo "ERROR: HOST_UID and HOST_GID must be integers."
    exit 1
fi

# Collision Detection: Check if UID is already taken by a non-agent user
if id -u "$USER_ID" >/dev/null 2>&1; then
    EXISTING_USER=$(id -nu "$USER_ID")
    if [ "$EXISTING_USER" != "agent" ]; then
        echo "ERROR: UID $USER_ID is already taken by user '$EXISTING_USER'."
        exit 1
    fi
fi

# Handle GID collision gracefully: if GID is taken by a non-agent group,
# reuse that group for the agent user. macOS commonly assigns GID 20 ("staff").
if getent group "$GROUP_ID" >/dev/null 2>&1; then
    EXISTING_GROUP=$(getent group "$GROUP_ID" | cut -d: -f1)
    if [ "$EXISTING_GROUP" != "agent" ]; then
        # Reuse the existing group — change agent's primary group
        usermod -g "$EXISTING_GROUP" agent
    fi
else
    groupmod -g "$GROUP_ID" agent
fi

# Update UID if needed
if [ "$(id -u agent)" != "$USER_ID" ]; then
    usermod -u "$USER_ID" agent
fi

# Fix ownership of persistent volumes if they changed
if [ "$(stat -c '%u' /home/agent 2>/dev/null)" != "$USER_ID" ]; then
    if ! chown -R agent:agent /home/agent 2>/dev/null; then
        echo "WARNING: could not chown /home/agent — some files may have incorrect ownership"
    fi
fi

# Drop root privileges and execute the command
if [ $# -eq 0 ]; then
    exec gosu agent "$DEFAULT_COMMAND"
else
    exec gosu agent "$@"
fi
