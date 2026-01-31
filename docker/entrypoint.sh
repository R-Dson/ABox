#!/bin/bash
set -e

USER_ID=${HOST_UID:-1000}
GROUP_ID=${HOST_GID:-1000}

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
