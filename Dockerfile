# Build Stage
FROM golang:1.23 AS builder

WORKDIR /src
COPY . .

# Build all Go binaries
# We assume setup/build.sh handles everything correctly
RUN chmod +x setup/build.sh && ./setup/build.sh

# Runtime Stage
FROM python:3.12-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    git \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Install Poetry
ENV POETRY_HOME="/opt/poetry" \
    POETRY_VIRTUALENVS_IN_PROJECT=false \
    POETRY_NO_INTERACTION=1 \
    PATH="$POETRY_HOME/bin:$PATH"

RUN curl -sSL https://install.python-poetry.org | python3 -

WORKDIR /app

# Copy Project Root Marker (Required for server to locate resources/tools)
COPY go.work /app/go.work

# Copy Binaries
COPY --from=builder /src/MCP-NG/bin /app/MCP-NG/bin

# Copy Tools (Configuration & Python Source)
COPY MCP-NG/tools /app/MCP-NG/tools

# Copy Shared Python Lib
COPY mcp-shared-proto-py /app/mcp-shared-proto-py

# Copy Admin UI
COPY MCP-NG/admin_ui /app/MCP-NG/admin_ui

# Create Resources Directory
RUN mkdir -p /app/resources

# Install Python Dependencies
# 1. Shared Proto (Dependency for others)
RUN cd /app/mcp-shared-proto-py && poetry install

# 2. Tools
RUN cd /app/MCP-NG/tools/python/code_interpreter && poetry install
RUN cd /app/MCP-NG/tools/python/fastapi_calculator && poetry install

# 3. Admin UI
RUN cd /app/MCP-NG/admin_ui && poetry install

# Expose Ports
# 8080: Meta-Server
# 7860: Admin UI
EXPOSE 8080 7860

# Default Command: Run Meta-Server
CMD ["/app/MCP-NG/bin/server", "--transport=http"]
