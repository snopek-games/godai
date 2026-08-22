ARG GO_VERSION=1.25.12

FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -tags selfupdate -o /out/godai ./cmd/godai \
 && go build -o /out/godai-eval ./cmd/godai-eval

FROM debian:bookworm-slim
ARG GODOT_VERSION=4.6.3
ARG CLAUDE_VERSION=2.1.229

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates curl git unzip wget libfontconfig1 \
 && rm -rf /var/lib/apt/lists/*

COPY scripts/install-godot.sh /tmp/install-godot.sh
RUN GODOT_VERSION=${GODOT_VERSION} /tmp/install-godot.sh && rm /tmp/install-godot.sh

# Installed under /opt/claude and opened up, so the image works under any
# --user, not just root.
RUN mkdir -p /opt/claude \
 && HOME=/opt/claude bash -c 'curl -fsSL https://claude.ai/install.sh | bash -s "$CLAUDE_VERSION"' \
 && ln -s /opt/claude/.local/bin/claude /usr/local/bin/claude \
 && chmod -R a+rX /opt/claude \
 && claude --version

COPY --from=build /out/godai /out/godai-eval /usr/local/bin/

# HOME=/tmp keeps Claude Code and Godot writing state somewhere any --user can.
ENV GODAI=/usr/local/bin/godai \
    GODOT=/usr/local/bin/godot \
    DISABLE_AUTOUPDATER=1 \
    HOME=/tmp

WORKDIR /work
ENTRYPOINT ["godai-eval"]
