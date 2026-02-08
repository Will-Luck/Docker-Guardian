# syntax = docker/dockerfile:latest

ARG ALPINE_VERSION=3.20

# ── Builder ───────────────────────────────────────────────────────────
FROM golang:1.24-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /guardian ./cmd/guardian

# ── Runtime ───────────────────────────────────────────────────────────
FROM alpine:${ALPINE_VERSION}

ENV AUTOHEAL_CONTAINER_LABEL=autoheal \
    AUTOHEAL_START_PERIOD=0 \
    AUTOHEAL_INTERVAL=5 \
    AUTOHEAL_DEFAULT_STOP_TIMEOUT=10 \
    AUTOHEAL_ONLY_MONITOR_RUNNING=false \
    AUTOHEAL_MONITOR_DEPENDENCIES=true \
    AUTOHEAL_DEPENDENCY_START_DELAY=5 \
    AUTOHEAL_BACKUP_LABEL="docker-volume-backup.stop-during-backup" \
    AUTOHEAL_BACKUP_CONTAINER="" \
    AUTOHEAL_GRACE_PERIOD=300 \
    AUTOHEAL_WATCHTOWER_COOLDOWN=300 \
    AUTOHEAL_WATCHTOWER_SCOPE=all \
    AUTOHEAL_WATCHTOWER_EVENTS=orchestration \
    AUTOHEAL_UNHEALTHY_THRESHOLD=1 \
    AUTOHEAL_BACKOFF_MULTIPLIER=2 \
    AUTOHEAL_BACKOFF_MAX=300 \
    AUTOHEAL_BACKOFF_RESET_AFTER=600 \
    AUTOHEAL_RESTART_BUDGET=5 \
    AUTOHEAL_RESTART_WINDOW=300 \
    METRICS_PORT=0 \
    DOCKER_SOCK=/var/run/docker.sock \
    CURL_TIMEOUT=30 \
    WEBHOOK_URL="" \
    WEBHOOK_JSON_KEY="content" \
    APPRISE_URL="" \
    POST_RESTART_SCRIPT="" \
    NOTIFY_EVENTS="actions" \
    NOTIFY_RATE_LIMIT=60 \
    NOTIFY_HOSTNAME="" \
    NOTIFY_GOTIFY_URL="" \
    NOTIFY_GOTIFY_TOKEN="" \
    NOTIFY_DISCORD_WEBHOOK="" \
    NOTIFY_SLACK_WEBHOOK="" \
    NOTIFY_TELEGRAM_TOKEN="" \
    NOTIFY_TELEGRAM_CHAT_ID="" \
    NOTIFY_PUSHOVER_TOKEN="" \
    NOTIFY_PUSHOVER_USER="" \
    NOTIFY_PUSHBULLET_TOKEN="" \
    NOTIFY_LUNASEA_WEBHOOK="" \
    NOTIFY_EMAIL_SMTP="" \
    NOTIFY_EMAIL_FROM="" \
    NOTIFY_EMAIL_TO="" \
    NOTIFY_EMAIL_USER="" \
    NOTIFY_EMAIL_PASS=""

RUN apk add --no-cache tzdata

COPY --from=builder /guardian /guardian

HEALTHCHECK --interval=5s CMD pgrep guardian || exit 1

ENTRYPOINT ["/guardian"]

CMD ["autoheal"]
