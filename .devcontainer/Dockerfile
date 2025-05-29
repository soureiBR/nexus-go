# Development container for Go WhatsApp API
FROM mcr.microsoft.com/devcontainers/go:1.24-bullseye

# Install additional tools and dependencies
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends \
    # Common development tools
    git \
    curl \
    wget \
    unzip \
    vim \
    nano \
    jq \
    # Build tools
    gcc \
    musl-dev \
    # Database tools
    sqlite3 \
    # Network tools
    netcat \
    telnet \
    # Process tools
    htop \
    tree \
    && apt-get autoremove -y && apt-get clean -y && rm -rf /var/lib/apt/lists/*

# Install Go development tools
RUN go install github.com/cosmtrek/air@latest \
    && go install github.com/swaggo/swag/cmd/swag@latest \
    && go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest \
    && go install honnef.co/go/tools/cmd/staticcheck@latest \
    && go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Set up working directory
WORKDIR /workspace

# Configure Git (will be overridden by user config)
RUN git config --global --add safe.directory /workspace

# Create necessary directories
RUN mkdir -p /workspace/sessions && \
    mkdir -p /workspace/data && \
    chown -R vscode:vscode /workspace

# Switch to vscode user
USER vscode

# Set environment variables
ENV GO111MODULE=on
ENV GOPROXY=https://proxy.golang.org,direct
ENV GOSUMDB=sum.golang.org
ENV CGO_ENABLED=1

# Expose ports
EXPOSE 8080 5672 15672
