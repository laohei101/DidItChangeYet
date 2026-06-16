# HTTP Change Watcher

A single-binary Go daemon that monitors URLs and API endpoints for changes and
fires alerts via **Telegram**, **email (SMTP)**, or **ntfy.sh** when a
user-defined condition is met.

---

## Features

| Feature | Details |
|---|---|
| **Watcher types** | `json` · `html` · `http_status` · `text` (full-body hash) |
| **Conditions** | `changed` · `above` · `below` · `equals` · `contains` · `regex` · `absence` |
| **Alert channels** | Telegram Bot · SMTP email · ntfy.sh (any server) |
| **Scheduling** | Duration strings (`5m`, `1h`) or full cron expressions (`*/15 * * * *`) |
| **State persistence** | JSON file — survives restarts without re-triggering |
| **Env-var substitution** | `{{MY_SECRET}}` in config values |
| **Dashboard** | Optional web UI at `http://localhost:8080` |
| **Run-once mode** | `-once` flag for scripted / one-shot checks |
| **Zero CGO** | Single static binary |

---

## Quick start

### 1. Build

```bash
git clone https://github.com/laohei101/diditchangeyet.git
cd diditchangeyet
go build -o http-watcher .
```

### 2. Configure

```bash
cp config.example.yaml config.yaml
# Edit config.yaml — set your URLs and alert credentials
```

### 3. Run

```bash
./http-watcher --config config.yaml
```

To verify your config without waiting for a scheduled check:

```bash
./http-watcher --config config.yaml --once
```

---

## Configuration reference

```yaml
global_interval: "5m"      # default poll interval (duration or cron)
global_timeout: "30s"      # default HTTP request timeout
state_file: "state.json"   # where last-known values are persisted
log_format: "text"         # "text" or "json"
log_level: "info"          # "debug" | "info" | "warn" | "error"

dashboard:
  enabled: true
  port: 8080

watches:
  - id: <unique-string>         # required — used in alerts and state
    url: <string>               # required — full URL to fetch
    method: GET                 # HTTP method (default: GET)
    headers:
      Header-Name: value
    body: ""                    # optional request body
    type: json                  # json | html | http_status | text
    json_path: "data.price"     # gjson path — type=json only
    css_selector: "h1"          # CSS selector — type=html only
    attribute: ""               # HTML attribute to read (empty = text content)
    condition: changed          # see Conditions table below
    threshold: "50"             # value for above/below/equals/contains
    pattern: "In Stock"         # regex for condition=regex
    interval: "10m"             # overrides global_interval
    timeout: "15s"              # overrides global_timeout
    alerts:
      - type: telegram | email | ntfy
        # see Alert config below
```

### Watcher types

| `type` | What is extracted |
|---|---|
| `json` | A single value at `json_path` (supports nested paths like `data.items.0.price`) |
| `html` | Text content (or an attribute) of the first element matching `css_selector` |
| `http_status` | The HTTP response status code as a string (e.g. `"200"`) |
| `text` | SHA-256 hash of the entire response body |

### Conditions

| `condition` | Trigger when… |
|---|---|
| `changed` | Current value ≠ previous value |
| `above` | Current value (numeric) > `threshold` |
| `below` | Current value (numeric) < `threshold` |
| `equals` | Current value == `threshold` (string comparison) |
| `contains` | Current value contains `threshold` as a substring |
| `regex` | Current value matches the regular expression in `pattern` |
| `absence` | Field not found in response, or request fails entirely |

### Alert channels

#### Telegram

```yaml
alerts:
  - type: telegram
    token: "{{TELEGRAM_TOKEN}}"   # Bot API token from @BotFather
    chat_id: "123456789"          # Your chat / group ID
```

#### Email (SMTP)

```yaml
alerts:
  - type: email
    smtp_host: smtp.gmail.com
    smtp_port: 587           # 587 = STARTTLS, 465 = implicit TLS
    tls: false               # set true for port 465
    username: "you@gmail.com"
    password: "{{GMAIL_APP_PASSWORD}}"
    from: "watcher@example.com"
    to: "alerts@example.com"
```

#### ntfy.sh

```yaml
alerts:
  - type: ntfy
    server: https://ntfy.sh   # or your self-hosted instance
    topic: my-alerts
    priority: high            # min | low | default | high | urgent
```

### Environment variable substitution

Any config value can reference an environment variable with `{{VAR_NAME}}`:

```yaml
token: "{{TELEGRAM_TOKEN}}"
password: "{{SMTP_PASSWORD}}"
```

Values that have no matching env var are left as-is, so missing variables are
visible in logs without silently breaking the config.

---

## CLI flags

```
-config string    Path to config file (default "config.yaml")
-once             Run all watches once and exit
-log-json         Emit structured JSON logs
-version          Print version and exit
```

---

## Docker

### Build

```bash
docker build -t http-watcher .
```

### Run

```bash
docker run -d \
  --name http-watcher \
  --restart unless-stopped \
  -v $(pwd)/config.yaml:/data/config.yaml \
  -v $(pwd)/state.json:/data/state.json \
  -p 8080:8080 \
  -e TELEGRAM_TOKEN=your-bot-token \
  -e TELEGRAM_CHAT_ID=your-chat-id \
  http-watcher
```

### Docker Compose

```yaml
services:
  http-watcher:
    build: .
    restart: unless-stopped
    volumes:
      - ./config.yaml:/data/config.yaml
      - ./state.json:/data/state.json
    ports:
      - "8080:8080"
    environment:
      - TELEGRAM_TOKEN=${TELEGRAM_TOKEN}
      - TELEGRAM_CHAT_ID=${TELEGRAM_CHAT_ID}
```

---

## Dashboard

Enable the dashboard in `config.yaml`:

```yaml
dashboard:
  enabled: true
  port: 8080
```

Then open `http://localhost:8080` to see all watch statuses, last-known values,
last check time, error state, and a **Run now** button to trigger an ad-hoc
check immediately.

The dashboard auto-refreshes every 10 seconds and exposes a JSON API at
`GET /api/status`.

---

## Project structure

```
.
├── main.go                 # entry point, flag parsing, signal handling
├── signal_unix.go          # SIGTERM/SIGINT handler (Linux/macOS)
├── signal_windows.go       # os.Interrupt handler (Windows)
├── config/
│   └── config.go           # YAML loading, env-var expansion, validation
├── watcher/
│   ├── watcher.go          # fetch → evaluate → alert → persist loop
│   ├── checkers.go         # HTTP client + JSON/HTML/text/status extraction
│   └── state.go            # thread-safe JSON state persistence
├── alert/
│   ├── alert.go            # Alerter interface + factory
│   ├── telegram.go         # Telegram Bot API
│   ├── email.go            # SMTP (STARTTLS + implicit TLS)
│   └── ntfy.go             # ntfy.sh HTTP API
├── scheduler/
│   └── scheduler.go        # robfig/cron wrapper, duration↔cron conversion
├── dashboard/
│   └── dashboard.go        # minimal web UI + /api/status JSON endpoint
├── Dockerfile
├── config.example.yaml
└── go.mod
```

---

## Dependencies

| Package | Purpose |
|---|---|
| `github.com/robfig/cron/v3` | Cron / `@every` scheduling |
| `github.com/PuerkitoBio/goquery` | CSS selector HTML parsing |
| `github.com/tidwall/gjson` | JSON path extraction |
| `github.com/goccy/go-yaml` | YAML config parsing |

All other features (HTTP client, SMTP, crypto, signal handling) use the Go
standard library.

---

## Graceful shutdown

The daemon catches `SIGTERM` and `SIGINT` (Ctrl-C), waits for any in-progress
checks to complete, then exits cleanly.

---

## License

MIT
