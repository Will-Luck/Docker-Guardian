# Notifications

Docker-Guardian supports 9 notification services natively. Multiple services can be active simultaneously. Action notifications retry up to 3 times with exponential backoff. Rate limiting prevents notification floods (default: 1 per container per 60 seconds).

## Services

| Service | Env Vars | Notes |
|---|---|---|
| **Gotify** | `NOTIFY_GOTIFY_URL`, `NOTIFY_GOTIFY_TOKEN` | POST to `{url}/message?token={token}` |
| **Discord** | `NOTIFY_DISCORD_WEBHOOK` | Full webhook URL from Discord settings |
| **Slack** | `NOTIFY_SLACK_WEBHOOK` | Full webhook URL from Slack app config |
| **Telegram** | `NOTIFY_TELEGRAM_TOKEN`, `NOTIFY_TELEGRAM_CHAT_ID` | Bot token from @BotFather |
| **Pushover** | `NOTIFY_PUSHOVER_TOKEN`, `NOTIFY_PUSHOVER_USER` | App token + user key |
| **Pushbullet** | `NOTIFY_PUSHBULLET_TOKEN` | Access token from account settings |
| **LunaSea** | `NOTIFY_LUNASEA_WEBHOOK` | Custom webhook URL |
| **Email** | `NOTIFY_EMAIL_SMTP`, `NOTIFY_EMAIL_FROM`, `NOTIFY_EMAIL_TO`, `NOTIFY_EMAIL_USER`, `NOTIFY_EMAIL_PASS` | SMTP. Format: `host:port` |
| **Webhook** | `WEBHOOK_URL`, `WEBHOOK_JSON_KEY` | Generic webhook (legacy) |

`APPRISE_URL` also still works for Apprise users.

## Event Filtering (`NOTIFY_EVENTS`)

Controls which events trigger notifications. Accepts keywords or numbers, comma-separated. Default: `actions`.

| # | Keyword | Events | Default |
|---|---|---|---|
| 1 | `startup` | Guardian boot confirmation (test notification) | No |
| 2 | `actions` | Restart success/failure + orphan start success/failure + circuit breaker | **Yes** |
| 3 | `failures` | Only failure events (restart failed, start failed) | No |
| 4 | `skips` | Orchestration skip, backup skip, grace period skip | No |
| 5 | `debug` | All of the above + logs every notification dispatch to console | No |

`failures` (3) is a subset of `actions` (2). If both are set, `actions` takes precedence.

**Examples:**

```bash
-e NOTIFY_EVENTS=actions            # default — success + failure
-e NOTIFY_EVENTS=actions,startup    # actions + boot test
-e NOTIFY_EVENTS=2,1               # same as above, numbered
-e NOTIFY_EVENTS=failures           # only failures
-e NOTIFY_EVENTS=all                # everything except debug (1-4)
-e NOTIFY_EVENTS=debug              # everything + console logging (5)
```

## Per-Container Filtering

Add `autoheal.notify=false` as a label to suppress notifications for a specific container. The container will still be restarted/stopped as configured, but no notification is sent.

```bash
docker run --label autoheal.notify=false ...
```

## Hostname Prefix

Set `NOTIFY_HOSTNAME` to prepend `[hostname]` to all notification messages. Useful when running Guardian on multiple hosts.

```bash
-e NOTIFY_HOSTNAME=prod-server-1
# Notifications become: "[prod-server-1] Container xyz found to be unhealthy..."
```

## Healthcheck Output

Restart notifications automatically include the last healthcheck output (truncated to 200 characters) for immediate context on what failed.
