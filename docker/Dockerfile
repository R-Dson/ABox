FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive

# 1. Install Runtime Dependencies & Developer Tools
RUN apt-get update && apt-get install -y \
    curl git unzip build-essential \
    python3 python3-pip python3-venv \
    nodejs npm \
    jq ripgrep \
    gosu procps \
    nano vim \
    && apt-get clean && rm -rf /var/lib/apt/lists/*

# 2. Create the Unprivileged User 'agent'
RUN groupadd -g 1000 agent && \
    useradd -u 1000 -g agent -m -s /bin/bash agent

# 3. Install OpenCode and make it system-wide
RUN curl -fsSL https://opencode.ai/install | bash \
    && mv /root/.opencode/bin/opencode /usr/local/bin/opencode \
    && rm -rf /root/.opencode

# 4. Pre-create XDG directories
RUN mkdir -p /home/agent/.config/opencode \
    && mkdir -p /home/agent/.local/share/opencode \
    && mkdir -p /home/agent/.local/state/opencode \
    && chown -R agent:agent /home/agent

# 5. Setup Entrypoint
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

WORKDIR /workspace
ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
CMD ["opencode"]
