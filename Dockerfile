# syntax = docker/dockerfile:latest

ARG ALPINE_VERSION=3.20

FROM alpine:${ALPINE_VERSION}

RUN apk add --no-cache curl jq

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
    DOCKER_SOCK=/var/run/docker.sock \
    CURL_TIMEOUT=30 \
    WEBHOOK_URL="" \
    WEBHOOK_JSON_KEY="content" \
    APPRISE_URL="" \
    POST_RESTART_SCRIPT=""

COPY docker-entrypoint /

HEALTHCHECK --interval=5s CMD pgrep -f autoheal || exit 1

ENTRYPOINT ["/docker-entrypoint"]

CMD ["autoheal"]
