# Build Stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Dependencies
COPY go.mod ./
# COPY go.sum ./
RUN go mod download

# Source
COPY . .

# Build
RUN go build -o tusk ./cmd/tusk

# Runtime Stage
FROM alpine:3.19

WORKDIR /app

# Install PHP for the worker (and extensions as needed)
RUN apk add --no-cache \
    php82 \
    php82-ctype \
    php82-curl \
    php82-dom \
    php82-fileinfo \
    php82-mbstring \
    php82-openssl \
    php82-pdo \
    php82-phar \
    php82-session \
    php82-xml \
    php82-tokenizer

# Link php82 to php
RUN ln -sf /usr/bin/php82 /usr/bin/php

# Copy Engine Binary
COPY --from=builder /app/tusk /usr/local/bin/tusk

# Copy Worker Script (if part of the release, or it might be mounted)
# For the engine image itself, we might just provide the engine.
# But for a usable image, we probably want the worker.php too.
# Let's assume the user mounts their code to /app.
# But the engine needs its internal worker.php? No, the engine spawns the USER's worker.php?
# Wait, the engine spawns `worker.php` which is the bridge.
# The bridge `worker.php` is currently in the root of the repo.
# We should probably compile/embed it or copy it.
COPY worker.php /usr/local/bin/worker.php

# Create a default tusk.json
RUN echo '{"port": 8080, "worker_count": 4, "address": "0.0.0.0", "project_root": "/app"}' > /etc/tusk.json

# Expose Port
EXPOSE 8080

# Entrypoint
CMD ["tusk", "start"]
