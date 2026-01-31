#!/bin/bash
set -e

USER_ID=${HOST_UID:-1000}
GROUP_ID=${HOST_GID:-1000}

# Validate inputs are integers
if ! [[ "$USER_ID" =~ ^[0-9]+$ ]] || ! [[ "$GROUP_ID" =~ ^[0-9]+$ ]]; then
    echo "ERROR: HOST_UID and HOST_GID must be integers."
    exit 1
fi

# Collision Detection: Check if UID/GID is already taken by a non-agent user
if id -u "$USER_ID" >/dev/null 2>&1; then
    EXISTING_USER=$(id -nu "$USER_ID")
    if [ "$EXISTING_USER" != "agent" ]; then
        echo "ERROR: UID $USER_ID is already taken by user '$EXISTING_USER'."
        exit 1
    fi
fi

if getent group "$GROUP_ID" >/dev/null 2>&1; then
    EXISTING_GROUP=$(getent group "$GROUP_ID" | cut -d: -f1)
    if [ "$EXISTING_GROUP" != "agent" ]; then
         echo "ERROR: GID $GROUP_ID is already taken by group '$EXISTING_GROUP'."
         exit 1
    fi
fi

# Update internal user to match host identity
if [ "$(id -u agent)" != "$USER_ID" ]; then
    groupmod -g "$GROUP_ID" agent
    usermod -u "$USER_ID" -g "$GROUP_ID" agent
fi

# Fix ownership of persistent volumes if they changed
if [ "$(stat -c '%u' /home/agent/.local/share/opencode)" != "$USER_ID" ]; then
    chown -R agent:agent /home/agent
fi

# Drop root privileges and execute opencode
exec gosu agent "$@"
