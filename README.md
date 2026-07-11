<h1 align="center">⚡ gload</h1>

<p align="center">
  <strong>A high-performance HTTP load tester with a full web UI — from a one-line CLI test to answering "can my system survive launch day?"</strong>
</p>

<p align="center">
  <a href="https://github.com/mertgundoganx/gload/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/mertgundoganx/gload/actions/workflows/ci.yml/badge.svg"></a>
  <a href="https://codecov.io/gh/mertgundoganx/gload"><img alt="Coverage" src="https://codecov.io/gh/mertgundoganx/gload/graph/badge.svg"></a>
  <img alt="Go" src="https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white">
  <img alt="License" src="https://img.shields.io/badge/License-MIT-green">
  <img alt="Protocols" src="https://img.shields.io/badge/protocols-HTTP%20%C2%B7%20WS%20%C2%B7%20GraphQL%20%C2%B7%20gRPC%20%C2%B7%20TCP-7c3aed">
  <img alt="PRs Welcome" src="https://img.shields.io/badge/PRs-welcome-brightgreen">
</p>

---

gload is a load-testing tool that grows with you. Fire a quick test from the terminal, or open the web dashboard to manage services, watch live charts, keep result history, and run realistic multi-stage scenarios. The engine is built for real throughput — sharded, lock-light metrics that stay out of the way so your CPU goes into actual requests, not bookkeeping.

**What makes gload stand out:**

- 🚀 **Fast, honest engine** — a sharded metrics core (per-request recording ~56 ns, zero allocations) that spends CPU on I/O, not lock contention. Everything shown is measured, never faked.
- 🖥️ **Real web UI** — dashboard, live SSE/WebSocket streaming, result history, comparisons, and print-ready reports. No framework, loads instantly.
- 🌊 **Staged ramping** — smooth linear ramps between stages (closed-model concurrency *or* open-model arrival rate), plus six ready-made patterns (Smoke → Steady → Ramp → Spike → Stress → Soak).
- 🎯 **Find Capacity** — one click auto-ramps to your system's saturation knee and tells you, in plain language, the max sustainable RPS and how many instances you need.
- 📺 **Simulate Launch** — rehearse a TV-ad / viral traffic surge by entering one number (peak req/sec); get a **READY / AT RISK / NOT READY** verdict.
- 🔌 **Multi-protocol** — HTTP/1.1 & HTTP/2, WebSocket, GraphQL, gRPC, and TCP out of the box.
- 🔗 **Batteries included** — scenarios & request chaining, scheduling, notifications (Slack/Teams/Discord/email/webhook), distributed testing, Prometheus + Grafana, GitHub PR comments, and JUnit for CI.

---

## Table of Contents

- [Features](#features)
- [Quick Start](#quick-start)
- [Usage](#usage)
- [Web UI Guide](#web-ui-guide)
- [Configuration](#configuration)
- [Capacity & Launch Simulation](#capacity--launch-simulation)
- [Protocol Support](#protocol-support)
- [Notifications](#notifications)
- [Scheduling](#scheduling)
- [CI/CD Integration](#cicd-integration)
- [Monitoring](#monitoring)
- [API Reference](#api-reference)
- [Architecture](#architecture)
- [Data Management](#data-management)
- [Multi-Tenancy](#multi-tenancy)
- [Distributed Testing](#distributed-testing)
- [Keyboard Shortcuts](#keyboard-shortcuts)
- [Development](#development)
- [Contributing](#contributing)
- [License](#license)

---

## Features

### Load Testing

| Feature | Description |
|---|---|
| Configurable concurrency | Set the number of concurrent virtual users |
| Rate limiting | Cap requests per second (0 = unlimited) |
| Arrival rate mode | Open workload model — new arrivals per second (a "thundering herd") |
| Staged linear ramping | Smooth linear ramps between stages (concurrency or arrival rate) |
| Load patterns | Six built-in patterns: Smoke, Steady Load, Ramp Up, Spike, Stress, Soak |
| Find Capacity | Auto-ramp to the saturation knee; reports max sustainable RPS + sizing |
| Simulate Launch | One-input open-model spike with a READY/AT RISK/NOT READY verdict |
| Test profiles | Save Light / Medium / Heavy presets per service |
| Think time | Fixed or random delay between requests per worker |
| Warm-up duration | Exclude initial seconds from final metrics |
| Adaptive concurrency | Auto-scale workers to maintain a target P95 latency |
| Request batching | Send N requests per worker iteration |
| Cookie jar | Per-worker session persistence across requests |

### Scenarios and Protocols

| Feature | Description |
|---|---|
| Request chaining | Multi-step scenarios with extractors (body, header, cookie) |
| Weighted steps | Distribute traffic across endpoints by weight |
| Dynamic data | `{{gen.*}}` placeholders, `{{env.*}}` variables, JSON data sources |
| HTTP/1.1 and HTTP/2 | Automatic or forced protocol selection |
| WebSocket | Connection + message echo testing |
| GraphQL | Query and variable support with error detection |
| gRPC | Connection and health check testing (TLS + plaintext) |
| TCP | Raw TCP connectivity checks |
| Multipart uploads | Form-data with file fields |

### Observability

| Feature | Description |
|---|---|
| Real-time streaming | SSE and WebSocket streams during test execution |
| Latency percentiles | Real measured Min / Avg / P50 / P95 / P99 / Max |
| TLS analysis | Protocol version, cipher suite, certificate details |
| Rate limit detection | 429 tracking with Retry-After header parsing |
| Timeline data | Per-interval RPS and latency snapshots (true interval values, not cumulative) |
| Request timing | DNS, TCP, TLS, TTFB, transfer breakdown (sampled for low overhead) |

### Analytics

| Feature | Description |
|---|---|
| Capacity probe (measured) | Auto-ramp to the real saturation knee and report max sustainable RPS |
| Launch readiness | READY / AT RISK / NOT READY verdict after a launch-spike simulation |
| Performance insights | Automated trend analysis across test runs |
| Capacity estimation (projected) | Quick max-RPS projection from one result using Little's Law |
| Trend & anomaly detection | Per-service trend charts and 2-sigma deviation alerts |
| Comparison reports | Side-by-side metric comparison of 2–5 test runs |

### Integrations

| Feature | Description |
|---|---|
| Prometheus metrics | `/metrics` endpoint with all key gauges and counters |
| Grafana | Pre-built dashboard via docker-compose |
| Webhook notifications | Generic webhook, Slack, Teams, Discord |
| Email notifications | SMTP-based alerts on test completion |
| GitHub PR comments | Post test results directly to pull requests |
| JUnit XML | Standard test report format for CI systems |
| cURL import | Create services from browser DevTools exports |
| Scheduled tests | Cron-based recurring test execution |

---

## Quick Start

### Prerequisites

- Go 1.26 or later
- (Optional) Docker and Docker Compose for the full monitoring stack

### Install from Source

```bash
git clone https://github.com/mertgundoganx/gload.git
cd gload
make build
```

The binary is placed in the current directory as `./gload`.

### Docker

Pull the prebuilt multi-arch image (linux/amd64 + arm64) from GHCR, published on each release:

```bash
docker run -p 8080:8080 -v gload-data:/home/app/.gload ghcr.io/mertgundoganx/gload:latest --web
```

Or build it yourself:

```bash
docker build -t gload:latest .
docker run -p 8080:8080 -v gload-data:/home/app/.gload gload:latest --web
```

### Docker Compose (Full Stack)

Start gload with Prometheus and Grafana:

```bash
make compose
```

This starts three services:

| Service | URL | Credentials |
|---|---|---|
| gload | http://localhost:8080 | -- |
| Prometheus | http://localhost:9090 | -- |
| Grafana | http://localhost:3000 | admin / gload |

Stop the stack:

```bash
make compose-down
```

---

## Usage

### CLI Mode

Run a quick load test from the terminal:

```bash
# Basic GET test
gload -u https://api.example.com/health -c 50 -d 30s

# POST with headers and body
gload -u https://api.example.com/users \
  -m POST \
  -H 'Content-Type: application/json' \
  -H 'Authorization: Bearer TOKEN' \
  -b '{"name": "{{gen.name}}", "email": "{{gen.email}}"}' \
  -c 100 -d 60s -r 500

# Headless mode for CI (no TUI)
gload -u https://api.example.com/health -c 10 -d 10s --no-ui
```

#### CLI Flags

| Flag | Default | Description |
|---|---|---|
| `-u` | (required) | Target URL |
| `-m` | `GET` | HTTP method (GET, POST, PUT, DELETE, PATCH) |
| `-c` | `10` | Number of concurrent workers |
| `-d` | `10s` | Test duration (e.g. `10s`, `1m`, `5m`) |
| `-t` | `30s` | Request timeout |
| `-r` | `0` | Requests per second limit (0 = unlimited) |
| `-H` | -- | HTTP header (repeatable, e.g. `-H 'Key: Value'`) |
| `-b` | -- | Request body |
| `--cookies` | `false` | Enable per-worker cookie jar |
| `--no-ui` | `false` | Disable TUI, print summary only (CI mode) |
| `--web` | `false` | Start web UI server |
| `--port` | `8080` | Web server port |
| `--worker` | `false` | Run as a distributed worker node |
| `--workers` | -- | Comma-separated worker URLs |
| `--log-level` | `info` | Log level (debug, info, warn, error) |
| `--log-json` | `false` | Enable JSON structured log output |
| `--version` | -- | Print version and exit |

### Web UI Mode

Start the web dashboard:

```bash
gload --web                # default port 8080
gload --web --port 8080    # custom port
```

Then open `http://localhost:8080` (or the custom port) in your browser.

### Docker

```bash
# Web UI
docker run -p 8080:8080 -v gload-data:/home/app/.gload gload:latest --web

# CLI mode
docker run --rm gload:latest -u https://api.example.com/health -c 50 -d 30s --no-ui
```

---

## Web UI Guide

### Dashboard

The dashboard shows all configured services with their latest test status, RPS, latency, and error rate. Services currently running display a real-time progress indicator.

### Creating a Service

1. Click "Add Service" on the dashboard.
2. Fill in the URL, method, headers, body, concurrency, and duration.
3. Optionally configure assertions, scenarios, data sources, and advanced settings.
4. Save the service.

You can also create services by importing a cURL command or using a pre-built template (REST CRUD, Auth Flow, Health Check, Stress Test, GraphQL, WebSocket, and more).

### Running a Test

- Click **Run** on any service to start with its saved settings.
- **Run Profile** — execute a predefined Light / Medium / Heavy configuration.
- **Run Pattern** — apply a Smoke, Steady, Ramp, Spike, Stress, or Soak load shape.
- **Capacity** — auto-ramp to the saturation knee and get max sustainable RPS + sizing.
- **Simulate Launch** — one-input open-model spike with a launch-readiness verdict.
- **Run Distributed** — split load across worker nodes.

Real-time metrics stream to the UI via SSE / WebSocket during execution, with live per-interval RPS and latency charts. Compare runs, and export CSV / JSON / HTML / PDF / JUnit.

### Viewing Results

Each completed test shows:
- RPS, total requests, error rate
- Latency percentiles (avg, P50, P95, P99, min, max)
- Status codes breakdown
- TLS certificate details
- Request timing breakdown (DNS, TCP, TLS, TTFB, transfer)
- Rate limit analysis (429 detection)
- Assertion results (pass/fail)
- Response validation results
- Per-second timeline chart

### Test History

Click "History" on a service to browse all previous test runs. Each entry includes timestamped metrics and can be annotated with notes.

### Comparing Results

Select any two test results and generate a side-by-side comparison report. Metrics are color-coded: green for the better value, red for the worse.

---

## Configuration

### Service Settings

| Field | Type | Default | Description |
|---|---|---|---|
| `name` | string | -- | Display name for the service |
| `url` | string | -- | Target URL |
| `method` | string | `GET` | HTTP method |
| `headers` | map | `{}` | Request headers |
| `body` | string | -- | Request body |
| `concurrency` | int | `10` | Number of concurrent workers |
| `duration` | string | `10s` | Test duration |
| `timeout` | string | `30s` | Per-request timeout |
| `tags` | string | -- | Comma-separated tags for filtering |
| `group_name` | string | -- | Group/category label |

### Load Test Parameters

| Field | Type | Default | Description |
|---|---|---|---|
| `arrival_rate` | int | `0` | New virtual users per second (open model, 0 = use concurrency) |
| `think_time_ms` | int | `0` | Fixed delay between requests in ms |
| `think_time_max_ms` | int | `0` | If > 0, random delay between think_time and this value |
| `warmup_seconds` | int | `0` | Exclude first N seconds from results |
| `requests_per_iteration` | int | `1` | Requests sent per worker iteration |

### Advanced Settings

| Field | Type | Default | Description |
|---|---|---|---|
| `http2` | bool | `true` | Enable HTTP/2 |
| `disable_keep_alive` | bool | `false` | Disable HTTP keep-alive |
| `max_idle_conns` | int | `100` | Maximum idle connections |
| `dns_cache` | bool | `false` | Enable DNS response caching |
| `warmup_conns` | int | `0` | Pre-establish N connections before test |
| `adaptive_concurrency` | bool | `false` | Auto-scale workers to hit target latency |
| `adaptive_target_ms` | float | `500` | Target P95 latency for adaptive mode |
| `cookie_jar` | bool | `false` | Enable per-worker cookie persistence |
| `content_type` | string | `json` | Body encoding: `json`, `form`, `multipart` |
| `protocol` | string | `http` | Protocol plugin: `http`, `websocket`, `graphql`, `grpc`, `tcp` |

### Assertions and Thresholds

Assertions define pass/fail criteria evaluated after each test. If any assertion fails, the test status is set to `fail`.

```json
{
  "assertions": [
    {"metric": "p95_latency", "operator": "lt", "value": 500},
    {"metric": "error_rate", "operator": "lt", "value": 5},
    {"metric": "rps", "operator": "gt", "value": 100}
  ]
}
```

**Available metrics:** `rps`, `avg_latency`, `p95_latency`, `p99_latency`, `min_latency`, `max_latency`, `error_rate`

**Operators:** `gt` (>), `lt` (<), `gte` (>=), `lte` (<=), `eq` (==)

Latency values are in milliseconds. Error rate is a percentage (0-100).

### Response Validations

Validations run against every response during the test:

```json
{
  "validations": [
    {"type": "status_code", "value": "200"},
    {"type": "contains", "value": "success"},
    {"type": "not_contains", "value": "error"},
    {"type": "regex", "value": "\"id\":\\s*\\d+"},
    {"type": "json_path", "path": "data.status", "value": "active"}
  ]
}
```

| Type | Description |
|---|---|
| `status_code` | Assert the HTTP status code |
| `contains` | Response body contains the string |
| `not_contains` | Response body does not contain the string |
| `regex` | Response body matches the regex pattern |
| `json_path` | JSON field at dot-notation path equals value |

### Test Profiles

Save multiple configurations per service for quick re-use:

```json
{
  "profiles": [
    {"name": "Light",  "concurrency": 10,  "duration": "10s", "rps": 50},
    {"name": "Medium", "concurrency": 100, "duration": "30s", "rps": 500},
    {"name": "Heavy",  "concurrency": 500, "duration": "120s", "rps": 0}
  ]
}
```

Run a profile via the API by name (case-insensitive) or by zero-based index:

```bash
curl -X POST http://localhost:8080/api/services/1/run-profile \
  -H 'Content-Type: application/json' \
  -d '{"name": "Heavy"}'

# equivalent, by index
curl -X POST http://localhost:8080/api/services/1/run-profile \
  -H 'Content-Type: application/json' \
  -d '{"profile_index": 2}'
```

### Scenarios and Request Chaining

Define multi-step flows where each step can extract values for use in subsequent steps:

```json
{
  "steps": [
    {
      "name": "Login",
      "url": "https://api.example.com/login",
      "method": "POST",
      "body": "{\"email\": \"{{gen.email}}\", \"password\": \"test123\"}",
      "extractors": [
        {"name": "token", "source": "body", "path": "data.access_token"},
        {"name": "user_id", "source": "body", "path": "data.id"}
      ]
    },
    {
      "name": "Get Profile",
      "url": "https://api.example.com/users/{{user_id}}",
      "method": "GET",
      "headers": {"Authorization": "Bearer {{token}}"}
    },
    {
      "name": "Update Profile",
      "url": "https://api.example.com/users/{{user_id}}",
      "method": "PUT",
      "headers": {"Authorization": "Bearer {{token}}"},
      "body": "{\"name\": \"{{gen.name}}\"}",
      "validations": [
        {"type": "status_code", "value": "200"}
      ]
    }
  ]
}
```

**Extractor sources:**

| Source | Path | Description |
|---|---|---|
| `body` | `data.token` or `results[0].id` | JSONPath dot notation on response body |
| `header` | `Authorization` | Response header name |
| `cookie` | `session_id` | Response cookie name |

**Weighted steps:** Set `weight` on each step to distribute requests randomly instead of running sequentially. A step with `weight: 70` receives ~70% of traffic.

### Dynamic Data and Faker

Use `{{gen.*}}` placeholders in URLs, headers, and bodies. Each request generates a fresh random value.

| Placeholder | Example Output |
|---|---|
| `{{gen.uuid}}` | `a1b2c3d4-e5f6-4789-abcd-ef0123456789` |
| `{{gen.timestamp}}` | `2025-01-15T10:30:00Z` |
| `{{gen.unix}}` | `1705312200` |
| `{{gen.unix_ms}}` | `1705312200000` |
| `{{gen.date}}` | `2025-01-15` |
| `{{gen.email}}` | `xkqmwlpz@test.io` |
| `{{gen.name}}` | `James Williams` |
| `{{gen.first_name}}` | `Patricia` |
| `{{gen.last_name}}` | `Rodriguez` |
| `{{gen.phone}}` | `+12145553821` |
| `{{gen.int}}` | `4829` (0-9999) |
| `{{gen.int100}}` | `42` (0-99) |
| `{{gen.int1000}}` | `738` (0-999) |
| `{{gen.float}}` | `482.37` |
| `{{gen.bool}}` | `true` or `false` |
| `{{gen.hex16}}` | `a3f1b2c4d5e6f789` |
| `{{gen.hex32}}` | `a3f1b2c4d5e6f789a3f1b2c4d5e6f789` |
| `{{gen.alpha8}}` | `xkqmwlpz` |
| `{{gen.alpha16}}` | `xkqmwlpzabcdefgh` |
| `{{gen.alnum12}}` | `a3f1b2c4d5e6` |
| `{{gen.ip}}` | `192.168.42.7` |
| `{{gen.useragent}}` | `Mozilla/5.0 (Windows NT 10.0; ...)` |
| `{{gen.word}}` | `consectetur` |
| `{{gen.paragraph}}` | `lorem ipsum dolor sit amet.` |
| `{{gen.color}}` | `purple` |
| `{{gen.country}}` | `US` |
| `{{gen.city}}` | `Tokyo` |

**Data sources:** Provide a JSON array of objects. Each worker iteration picks the next row (round-robin):

```json
{
  "data_source": [
    {"user_id": "1", "name": "Alice"},
    {"user_id": "2", "name": "Bob"},
    {"user_id": "3", "name": "Charlie"}
  ]
}
```

Reference values in URLs and bodies as `{{user_id}}` and `{{name}}`.

### Load Patterns

A **stage** has a `duration` and a `target`. Within each stage the load ramps
**linearly** from the previous stage's target to this one — so a stage with the
same target holds steady, and a short stage to a higher target is a spike. By
default `target` is the concurrency (virtual users); set `open_model: true` and
it becomes an arrival rate in requests/second.

Six patterns ship built-in, ordered from lightest to heaviest — a full testing
lifecycle:

| Pattern | What it does |
|---|---|
| **Smoke Test** | 5 users for a minute — a pre-flight sanity check before anything heavy. |
| **Steady Load** | Ramps to ~100 and holds 10 min — confirms it handles expected everyday traffic. |
| **Ramp Up** | Smooth linear ramp 0 → 500 then hold — shows how latency/throughput evolve as load grows. |
| **Spike Test** | Baseline, then sudden spikes to 1000 and back — burst handling and recovery. |
| **Stress Test** | Steps up 200 → 500 → 1000 → 2000, holding at each — finds the breaking point. |
| **Soak Test** | Moderate load for 30 min — surfaces memory leaks and gradual degradation. |

Run a pattern by name:

```bash
curl -X POST http://localhost:8080/api/services/1/run-pattern \
  -H 'Content-Type: application/json' \
  -d '{"pattern_name": "Spike Test"}'
```

Or provide custom stages (this example ramps 0→200 over 30s, holds a minute, eases off):

```bash
curl -X POST http://localhost:8080/api/services/1/run-pattern \
  -H 'Content-Type: application/json' \
  -d '{
    "stages": [
      {"duration": "30s", "target": 200},
      {"duration": "60s", "target": 200},
      {"duration": "30s", "target": 0}
    ]
  }'
```

For an **open-model** spike (arrival rate in req/sec instead of concurrency —
the realistic shape for a launch surge), add `"open_model": true`. The
[Simulate Launch](#capacity--launch-simulation) UI builds exactly this for you
from a single number.

### cURL Import

Paste a cURL command from your browser DevTools into the web UI import dialog. gload parses the URL, method, headers, and body to create a service configuration automatically.

---

## Capacity & Launch Simulation

Two features answer the questions you actually care about before going live:
*how much can this take?* and *will it survive launch day?* Both are one click
on the service detail page — the **Capacity** and **Simulate Launch** buttons.

### Find Capacity

Instead of guessing a concurrency number, **Find Capacity** ramps load up one
level at a time, measures steady-state throughput and latency at each, and stops
the moment it detects the **knee** — the point where throughput stops growing and
latency starts climbing. It doesn't blindly overload; it stops as soon as it has
the answer.

You get a plain-language verdict and a per-level table:

> **Saturates at ~8 concurrent · ~380 req/sec.** Beyond ~8 concurrent requests,
> average latency rises from 21 ms to 34 ms while throughput stays flat.

Plus **scaling guidance**: one instance's sustainable throughput (with a 70%
headroom figure) and an interactive *"target RPS → instances needed"* calculator.

```bash
# Start a probe (async, no config — it auto-ramps to the knee and stops)
curl -X POST http://localhost:8080/api/services/1/capacity-probe

# Fetch the stored result (persists across restarts)
curl http://localhost:8080/api/services/1/capacity-probe
```

The measured ceiling also shows up as a **badge** on the dashboard card, so a
service's capacity is visible at a glance after a single probe.

### Simulate Launch

Rehearse a sudden traffic surge — a TV ad in prime time, a viral moment — with
**one number**. Enter your expected **peak requests/second** and gload builds an
**open-model** spike (arrival rate ramps to the peak in ~20s, holds, then eases
off). Because it's open-model, new arrivals keep coming even if the target
slows — a realistic *thundering herd*, not a polite queue.

When it finishes you get a **Launch Readiness** verdict on the service page:

| Verdict | Meaning |
|---|---|
| ✅ **READY** | Held the peak with low errors and stable latency. |
| ⚠️ **AT RISK** | Reached the peak but errors climbed or latency degraded — add headroom. |
| ❌ **NOT READY** | Throughput plateaued below target or errors spiked — it would buckle. |

It's honest about subtle failures: a run can have **0 errors** and still be
flagged NOT READY if throughput topped out at half the target and latency
ballooned (requests queuing instead of failing).

> **Tip:** run Find Capacity first to learn one instance's ceiling and size your
> peak, and always test the real path (through your CDN / load balancer), not
> localhost.

---

## Protocol Support

### HTTP (default)

Standard HTTP/1.1 and HTTP/2 load testing. All features (scenarios, assertions, validations, data sources) are fully supported.

### WebSocket

```json
{
  "protocol": "websocket",
  "protocol_config": {
    "url": "wss://echo.example.com/ws",
    "message": "hello",
    "expect_response": "true",
    "timeout": "10s"
  }
}
```

### GraphQL

```json
{
  "protocol": "graphql",
  "protocol_config": {
    "url": "https://api.example.com/graphql",
    "query": "{ users(limit: 10) { id name email } }",
    "variables": "{\"limit\": 10}",
    "timeout": "30s",
    "header_Authorization": "Bearer TOKEN"
  }
}
```

GraphQL errors in the response are detected and counted as failures.

### gRPC

```json
{
  "protocol": "grpc",
  "protocol_config": {
    "address": "api.example.com:443",
    "tls": "true",
    "plaintext": "false",
    "timeout": "10s"
  }
}
```

Tests HTTP/2 + TLS connection establishment and gRPC health check frames.

### TCP

```json
{
  "protocol": "tcp",
  "protocol_config": {
    "address": "db.example.com:5432",
    "timeout": "5s"
  }
}
```

Measures TCP connection latency without sending application-layer data.

---

## Notifications

Configure notifications in the web UI under Settings. Notifications are sent after each test completes.

### Notification Triggers

Set `notify_on` to control when notifications fire:

| Value | Behavior |
|---|---|
| `all` | Notify on every test completion |
| `fail_only` | Notify only when assertions fail |
| `none` | Disable notifications |

Notifications are disabled until `notify_on` is set (unset behaves like `none`).

### Webhook

Set `webhook_url` in Settings. gload sends a POST request with the test result JSON payload.

### Slack

Set `slack_webhook_url` to a Slack Incoming Webhook URL. Messages are formatted with service name, status, RPS, latency, and error rate.

### Microsoft Teams

Set `teams_webhook_url` to a Teams Incoming Webhook URL.

### Discord

Set `discord_webhook_url` to a Discord Webhook URL.

### Email (SMTP)

Configure the following settings:

| Setting | Description |
|---|---|
| `smtp_host` | SMTP server hostname |
| `smtp_port` | SMTP server port |
| `smtp_username` | Authentication username |
| `smtp_password` | Authentication password |
| `email_from` | Sender email address |
| `email_to` | Comma-separated recipient addresses |

---

## Scheduling

Schedule recurring tests using standard 5-field cron expressions (minute, hour, day-of-month, month, day-of-week).

```bash
# Run every hour
curl -X POST http://localhost:8080/api/schedules \
  -H 'Content-Type: application/json' \
  -d '{"service_id": 1, "cron_expr": "0 * * * *"}'

# Run at 6 AM every weekday
curl -X POST http://localhost:8080/api/schedules \
  -H 'Content-Type: application/json' \
  -d '{"service_id": 1, "cron_expr": "0 6 * * 1-5"}'

# Run every 30 minutes
curl -X POST http://localhost:8080/api/schedules \
  -H 'Content-Type: application/json' \
  -d '{"service_id": 1, "cron_expr": "*/30 * * * *"}'
```

Schedules can be enabled/disabled and managed via the web UI or the `/api/schedules` endpoints.

---

## CI/CD Integration

### GitHub Actions

```yaml
name: Load Test
on:
  pull_request:
    branches: [main]

jobs:
  load-test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v7

      - name: Build gload
        run: make build

      - name: Start target service
        run: docker compose up -d my-service

      - name: Run load test
        run: |
          ./gload -u http://localhost:3000/api/health \
            -c 50 -d 30s -r 500 --no-ui

      - name: Start gload web server
        run: ./gload --web --port 8080 &

      - name: Create service and run test
        run: |
          # Create service
          SERVICE_ID=$(curl -s -X POST http://localhost:8080/api/services \
            -H 'Content-Type: application/json' \
            -d '{
              "name": "CI Test",
              "url": "http://localhost:3000/api/health",
              "method": "GET",
              "concurrency": 50,
              "duration": "30s",
              "assertions": "[{\"metric\":\"p95_latency\",\"operator\":\"lt\",\"value\":500},{\"metric\":\"error_rate\",\"operator\":\"lt\",\"value\":5}]"
            }' | jq -r '.id')

          # Run test
          curl -X POST http://localhost:8080/api/services/$SERVICE_ID/run
          sleep 35

          # Export JUnit results
          curl -s http://localhost:8080/api/services/$SERVICE_ID/junit > results.xml

      - name: Upload JUnit results
        if: always()
        uses: dorny/test-reporter@v1
        with:
          name: Load Test Results
          path: results.xml
          reporter: java-junit
```

### Exit Codes

In CLI mode with `--no-ui`, gload exits with code `1` if the error rate exceeds 10%.

### JUnit XML Reports

Generate JUnit-compatible XML from the latest test result:

```bash
curl -s http://localhost:8080/api/services/1/junit > junit-results.xml
```

The report includes assertion results mapped to JUnit test cases. Failed assertions appear as test failures.

### GitHub PR Comments

Post a formatted test summary directly to a GitHub pull request:

```bash
# Requires GITHUB_TOKEN and GITHUB_REPOSITORY ("owner/repo") environment variables.
# The PR number is read from GLOAD_PR_NUMBER, or derived automatically from
# GITHUB_REF (refs/pull/123/merge) when running inside GitHub Actions.
curl -X POST http://localhost:8080/api/services/1/github-comment
```

---

## Monitoring

### Prometheus Metrics

gload exposes a `/metrics` endpoint in Prometheus text format.

| Metric | Type | Description |
|---|---|---|
| `gload_running_tests` | gauge | Number of currently running tests |
| `gload_total_services` | gauge | Total configured services |
| `gload_tests_completed_total` | counter | Total tests completed |
| `gload_tests_failed_total` | counter | Total tests that failed assertions |
| `gload_requests_total` | counter | Total HTTP requests across all tests |
| `gload_errors_total` | counter | Total HTTP errors across all tests |
| `gload_last_rps` | gauge | RPS of the most recent test |
| `gload_last_avg_latency_ms` | gauge | Average latency of the most recent test |
| `gload_last_p95_latency_ms` | gauge | P95 latency of the most recent test |
| `gload_last_error_rate` | gauge | Error rate of the most recent test |

### Grafana Dashboard

The docker-compose stack includes a pre-provisioned Grafana instance with dashboards:

```bash
make compose
# Open http://localhost:3000 (admin / gload)
```

The provisioning files are in `monitoring/grafana/provisioning/` and `monitoring/grafana/dashboards/`.

### Profiling (pprof)

Standard Go pprof endpoints are available when the web server is running:

| Endpoint | Description |
|---|---|
| `/debug/pprof/` | Index of all profiles |
| `/debug/pprof/profile` | CPU profile (30s default) |
| `/debug/pprof/heap` | Heap memory profile |
| `/debug/pprof/goroutine` | Goroutine stacks |
| `/debug/pprof/trace` | Execution trace |

```bash
go tool pprof http://localhost:8080/debug/pprof/heap
```

---

## API Reference

### Services

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/services` | List all services (supports `?workspace=slug` filter) |
| `POST` | `/api/services` | Create a new service |
| `GET` | `/api/services/:id` | Get a single service |
| `PUT` | `/api/services/:id` | Update a service |
| `DELETE` | `/api/services/:id` | Delete a service |
| `POST` | `/api/services/:id/clone` | Clone a service |
| `POST` | `/api/services/bulk-delete` | Bulk delete services by ID array |
| `GET` | `/api/services/export` | Export all services as JSON |
| `POST` | `/api/services/import` | Import services from JSON |

### Test Execution

| Method | Path | Description |
|---|---|---|
| `POST` | `/api/services/:id/run` | Start a load test |
| `POST` | `/api/services/:id/stop` | Stop a running test |
| `POST` | `/api/services/:id/run-profile` | Run with a named profile |
| `POST` | `/api/services/:id/run-pattern` | Run a load pattern or custom stages (`open_model: true` for arrival-rate spikes) |
| `POST` | `/api/services/:id/capacity-probe` | Start a capacity probe (knee finder) |
| `GET` | `/api/services/:id/capacity-probe` | Latest capacity probe result |
| `POST` | `/api/services/:id/run-distributed` | Run distributed across workers |
| `GET` | `/api/services/:id/stream` | SSE stream of real-time metrics |
| `GET` | `/api/services/:id/ws` | WebSocket stream of real-time metrics |

### Results and Reports

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/services/:id/results` | Latest test result |
| `GET` | `/api/services/:id/history` | Last 50 test results |
| `GET` | `/api/services/:id/results/export?format=csv` | Export results as CSV |
| `GET` | `/api/services/:id/results/export?format=json` | Export results as JSON |
| `GET` | `/api/services/:id/report` | HTML report of latest result |
| `GET` | `/api/services/:id/pdf` | Printable HTML report (auto-opens print dialog) |
| `GET` | `/api/services/:id/junit` | JUnit XML report |
| `GET` | `/api/services/:id/results/:rid/share` | Shareable HTML page for a result |
| `PUT` | `/api/services/:id/results/:rid/note` | Update a test result note |
| `GET` | `/api/compare-report?ids=5,8` | Side-by-side comparison of two results |

### Analytics

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/services/:id/insights` | Automated performance insights |
| `GET` | `/api/services/:id/capacity` | Quick capacity **projection** from the last result (Little's Law) |
| `GET` | `/api/services/:id/capacity-probe` | Capacity-probe run history |
| `POST` | `/api/services/:id/capacity-probe` | Start an adaptive capacity probe |
| `GET` | `/api/services/:id/capacity-report?run=<id>` | Print-ready capacity report |
| `POST` | `/api/services/:id/github-comment` | Post result to GitHub PR |

### Configuration

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/settings` | Get all settings |
| `PUT` | `/api/settings` | Update settings (key-value map) |
| `GET` | `/api/templates` | List pre-built service templates |
| `GET` | `/api/patterns` | List available load patterns |
| `GET` | `/api/plugins` | List registered protocol and collector plugins |

### Scheduling

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/schedules` | List all schedules |
| `POST` | `/api/schedules` | Create a schedule |
| `PUT` | `/api/schedules/:id` | Enable/disable a schedule |
| `DELETE` | `/api/schedules/:id` | Delete a schedule |

### Queue

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/queue` | List queued tests |
| `POST` | `/api/queue` | Add a service to the queue |
| `DELETE` | `/api/queue/:id` | Remove from queue |
| `POST` | `/api/queue/bulk-add` | Bulk add services to queue |

### Workspaces

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/workspaces` | List all workspaces |
| `POST` | `/api/workspaces` | Create a workspace |
| `DELETE` | `/api/workspaces/:id` | Delete a workspace |
| `GET` | `/api/workspaces/:id/services` | List services in a workspace |

### System

| Method | Path | Description |
|---|---|---|
| `GET` | `/api/version` | Version info (version, Go, OS, arch, goroutines) |
| `GET` | `/api/workers` | List distributed worker status |
| `GET` | `/api/ws/events` | WebSocket for real-time event broadcasts |
| `GET` | `/health` | Health check (returns uptime) |
| `GET` | `/ready` | Readiness check (verifies database connectivity) |
| `GET` | `/metrics` | Prometheus metrics |

---

## Architecture

```
gload/
  main.go                     CLI entry point, flag parsing, mode dispatch
  pkg/config/                  Shared configuration types (Config, Stage, Assertion, Step, etc.)
  internal/
    runner/                    Load test execution engine
    metrics/                   Real-time metrics collection and snapshots
    ui/                        Terminal UI (Bubble Tea)
    server/                    Web server, API handlers, WebSocket broadcast
    storage/                   SQLite persistence (services, results, schedules, workspaces, queue, capacity)
    scheduler/                 Cron-based test scheduling
    plugin/                    Protocol plugin registry (TCP, WebSocket, GraphQL, gRPC)
    faker/                     Dynamic data generators ({{gen.*}} placeholders)
    notifier/                  Webhook, Slack, Teams, Discord, email notifications
    prom/                      Prometheus metrics endpoint
    report/                    HTML report generation
    junit/                     JUnit XML report generation
    github/                    GitHub PR comment integration
    logger/                    Structured logging
    worker/                    Distributed worker node server
  web/
    templates/                 HTML templates (embedded)
    static/                    CSS, JS assets (embedded)
  monitoring/
    prometheus.yml             Prometheus scrape configuration
    grafana/                   Grafana provisioning and dashboards
```

**Data flow:**

1. A test is triggered via CLI flags or the web API.
2. The runner drives load — a fixed pool of virtual users (closed model) or a ramping arrival rate (open model), optionally shaped by stages.
3. Each response is recorded into **sharded**, mostly-atomic metrics, so thousands of workers don't serialize on a single lock (~56 ns/record, zero allocations).
4. A reader merges the shards on demand into live snapshots (RPS, latency percentiles, status codes, timeline) — without blocking recording.
5. Snapshots are streamed to the TUI or web UI via SSE/WebSocket.
6. On completion, the final snapshot is persisted to SQLite, assertions are evaluated, and notifications are sent.

---

## Data Management

### Storage (SQLite)

All data is stored in `~/.gload/gload.db`. The database is created automatically on first run.

### Backup and Restore

```bash
# Backup
cp ~/.gload/gload.db ~/.gload/gload.db.bak

# Restore
cp ~/.gload/gload.db.bak ~/.gload/gload.db
```

### Data Retention

Configure `retention_days` in Settings. A background worker runs hourly and purges test results older than the configured number of days.

```bash
curl -X PUT http://localhost:8080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"retention_days": "90"}'
```

### Import and Export

Export all services:

```bash
curl -s http://localhost:8080/api/services/export > gload-export.json
```

Import services into another instance:

```bash
curl -X POST http://localhost:8080/api/services/import \
  -H 'Content-Type: application/json' \
  -d @gload-export.json
```

Export test results for a specific service:

```bash
# JSON
curl -s http://localhost:8080/api/services/1/results/export?format=json > results.json

# CSV
curl -s http://localhost:8080/api/services/1/results/export?format=csv > results.csv
```

---

## Multi-Tenancy

Workspaces isolate services into separate groups for teams or environments.

```bash
# Create a workspace
curl -X POST http://localhost:8080/api/workspaces \
  -H 'Content-Type: application/json' \
  -d '{"name": "Production", "slug": "production", "description": "Production API tests"}'

# List services in a workspace
curl http://localhost:8080/api/services?workspace=production

# Or use the X-Workspace header
curl http://localhost:8080/api/services -H 'X-Workspace: production'
```

Services are assigned to a workspace on creation. If no workspace is specified, the `default` workspace is used.

---

## Distributed Testing

Split load across multiple worker nodes to generate higher throughput.

### Start Worker Nodes

```bash
# On machine 1
gload --worker --port 8081

# On machine 2
gload --worker --port 8082
```

### Configure Workers

Set worker URLs in Settings:

```bash
curl -X PUT http://localhost:8080/api/settings \
  -H 'Content-Type: application/json' \
  -d '{"worker_urls": "http://worker1:8081,http://worker2:8082"}'
```

### Run a Distributed Test

```bash
curl -X POST http://localhost:8080/api/services/1/run-distributed
```

Concurrency is split evenly across workers. Each worker receives the full service configuration and runs its portion independently.

### Check Worker Health

```bash
curl http://localhost:8080/api/workers
```

---

## Keyboard Shortcuts

### CLI TUI

| Key | Action |
|---|---|
| `q` | Quit / stop the test |
| `Ctrl+C` | Force quit |

### Web UI

Keyboard shortcuts are available in the web dashboard. Refer to the in-app help for the full list.

---

## Development

### Prerequisites

- Go 1.26+
- (Optional) `golangci-lint` for linting
- (Optional) the standalone [Tailwind CSS CLI](https://tailwindcss.com/blog/standalone-cli) for regenerating the web UI stylesheet

### Building

```bash
make build          # Build the binary
make run            # Build and start the web server
make docker         # Build Docker image
```

### Web UI Styles

The web UI uses a locally compiled Tailwind CSS v4 stylesheet embedded in the binary (`web/static/css/tailwind.css`). After adding or removing Tailwind classes in `web/templates/` or `web/static/js/app.js`, regenerate it:

```bash
make css
```

### Testing

```bash
make test           # Run all tests
make test-v         # Verbose output
make cover          # Generate coverage report (coverage.html)
```

### Linting

```bash
make lint           # Run golangci-lint
make vet            # Run go vet
```

### Cross-Compilation

```bash
make release
```

Builds binaries for:
- `linux/amd64`, `linux/arm64`
- `darwin/amd64`, `darwin/arm64`
- `windows/amd64`

### Makefile Targets

| Target | Description |
|---|---|
| `make help` | Show all available targets |
| `make build` | Build the gload binary |
| `make test` | Run all tests |
| `make test-v` | Run tests with verbose output |
| `make cover` | Run tests with coverage report |
| `make lint` | Run golangci-lint |
| `make vet` | Run go vet |
| `make clean` | Remove build artifacts |
| `make run` | Build and start the web server |
| `make css` | Regenerate the embedded Tailwind CSS |
| `make docker` | Build Docker image |
| `make compose` | Start full stack (gload + Prometheus + Grafana) |
| `make compose-down` | Stop the full stack |
| `make release` | Cross-compile for all platforms |

### Project Structure

See [Architecture](#architecture) for a detailed breakdown of all packages.

---

## Contributing

1. Fork the repository.
2. Create a feature branch (`git checkout -b feature/my-feature`).
3. Write tests for new functionality.
4. Run `make test && make lint` to validate.
5. Commit your changes and open a pull request **against the `development` branch**.

`main` is the protected release branch; contributions land on `development`
first. See [CONTRIBUTING.md](CONTRIBUTING.md) for details.

---

## License

MIT License — free to use, modify, distribute, and sell. See [LICENSE](LICENSE) for details.

Third-party dependencies bundled into the compiled binary retain their own permissive licenses (MIT, BSD-3-Clause, ISC); their notices are reproduced in [THIRD_PARTY_LICENSES.md](THIRD_PARTY_LICENSES.md).
