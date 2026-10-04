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

# Mount a modern application project at /app before running tusk start.
# The project supplies bootstrap/app.php; the Engine creates
# .tusk/runtime/worker.php there and RoadRunner runs it through PHP.
# This Engine image does not install a repository-owned project worker.

# Create a default tusk.json
RUN echo '{"port": 8080, "worker_count": 4, "address": "0.0.0.0", "project_root": "/app"}' > /etc/tusk.json

# Expose Port
EXPOSE 8080

# Entrypoint
CMD ["tusk", "start"]
