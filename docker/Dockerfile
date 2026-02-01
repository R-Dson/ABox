FROM debian:bookworm-slim

ARG DEBIAN_FRONTEND=noninteractive
ARG INSTALL_CMD=""
ARG VERSION=""
ARG COMMAND_NAME="opencode"
ENV DEFAULT_COMMAND=$COMMAND_NAME

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

# Install common dependencies
RUN apt-get update && apt-get install -y \
    curl wget ca-certificates git unzip gosu procps jq ripgrep \
    build-essential locales openssl file \
    python3 python3-pip python3-venv nodejs npm \
    nano vim && \
    apt-get clean && rm -rf /var/lib/apt/lists/*

# Set locale
RUN sed -i -e 's/# en_US.UTF-8 UTF-8/en_US.UTF-8 UTF-8/' /etc/locale.gen && \
    locale-gen
ENV LANG=en_US.UTF-8
ENV LANGUAGE=en_US:en
ENV LC_ALL=en_US.UTF-8

# Install uv (modern python manager)
RUN curl -LsSf https://astral.sh/uv/install.sh | sh && \
    mv /root/.local/bin/uv /usr/local/bin/uv && \
    mv /root/.local/bin/uvx /usr/local/bin/uvx

# Install Bun (fast JS/TS manager)
RUN curl -fsSL https://bun.sh/install | bash && \
    mv /root/.bun/bin/bun /usr/local/bin/bun

# Install GitHub CLI (gh)
RUN curl -fsSL https://cli.github.com/packages/githubcli-archive-keyring.gpg | dd of=/usr/share/keyrings/githubcli-archive-keyring.gpg \
    && chmod go+r /usr/share/keyrings/githubcli-archive-keyring.gpg \
    && echo "deb [arch=$(dpkg --print-architecture) signed-by=/usr/share/keyrings/githubcli-archive-keyring.gpg] https://cli.github.com/packages stable main" | tee /etc/apt/sources.list.d/github-cli.list > /dev/null \
    && apt-get update && apt-get install -y gh && apt-get clean && rm -rf /var/lib/apt/lists/*

# Create non-root user
RUN groupadd -g 1000 agent && \
    useradd -u 1000 -g agent -m -s /bin/bash agent

# Install Homebrew (Linuxbrew)
ENV HOMEBREW_PREFIX=/home/linuxbrew/.linuxbrew
ENV HOMEBREW_CELLAR=/home/linuxbrew/.linuxbrew/Cellar
ENV HOMEBREW_REPOSITORY=/home/linuxbrew/.linuxbrew/Homebrew
RUN mkdir -p /home/linuxbrew/.linuxbrew && \
    chown -R agent:agent /home/linuxbrew && \
    gosu agent bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Add Homebrew to PATH
ENV PATH="${HOMEBREW_PREFIX}/bin:${HOMEBREW_PREFIX}/sbin:${PATH}"

# Run editor-specific installation logic
RUN VERSION="$VERSION" eval "$INSTALL_CMD"


# Copy generic entrypoint
COPY docker/entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

# Setup workspace
RUN mkdir -p /workspace && chown agent:agent /workspace
WORKDIR /workspace

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
