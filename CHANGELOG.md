# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Changed

- Migrated the WebSocket library from the deprecated `nhooyr.io/websocket` to
  its maintained successor `github.com/coder/websocket` (drop-in, no behavior
  change).

### Internal

- Replaced numeric HTTP status literals with `http.StatusMethodNotAllowed` and
  removed an unused interface (staticcheck cleanups).
- Added a `.golangci.yml` config and made the codebase pass `golangci-lint`
  cleanly: intentionally-ignored errors are now explicit (`_ =`) and
  stylistic-only checks are quieted.
- Wired `golangci-lint` into GitHub Actions CI as a dedicated `lint` job.

## [1.0.0] - 2026-07-12

First public release. gload is a high-performance HTTP load tester with a full
web UI — from a one-line CLI test to answering "can my system survive launch day?"

### Engine

- Sharded metrics core: per-request recording in ~56 ns with zero allocations,
  so CPU goes into requests instead of lock contention; snapshots never contend
  with recording.
- Memory-efficient latency histogram (constant ~500 KB regardless of duration).
- Lock-free circuit breaker hot path (mutex taken only on state transitions).
- Configurable concurrency, duration, and timeout.
- Rate limiting (RPS cap).
- **Staged ramping** — smooth linear interpolation between stages, in
  closed-model concurrency **or** open-model arrival rate (req/sec), with an
  in-flight cap to bound a thundering herd.
- Think time / pacing (fixed and random delays).
- Request chaining with response value extraction (JSONPath, headers, cookies).
- Request weights for weighted random endpoint selection.
- Response body validation (contains, regex, json_path, status_code).
- Dynamic data sources with round-robin templating.
- Faker data generation (25+ types: UUID, email, name, phone, etc.).
- Environment-variable support in URLs, headers, and body.
- Cookie/session persistence per virtual user.
- HTTP/2 support with multiplexing.
- Connection-pool controls (keep-alive, max idle, DNS cache) and connection warm-up.
- Adaptive concurrency (auto-scale based on P95 latency).
- Request batching (multiple requests per worker iteration).
- Warm-up period (exclude initial seconds from results).
- Coordinated-omission correction.
- Per-request timing breakdown (DNS, TCP, TLS, TTFB, Transfer).
- Multipart/form-data upload support.

### Capacity & Launch

- **Find Capacity** — an adaptive probe that ramps load level by level, detects
  the saturation knee, and reports max sustainable RPS with scaling guidance and
  an interactive "target RPS → instances" calculator. Results persist, are
  shareable as a print-ready report, and show as a badge on dashboard cards.
- **Simulate Launch** — a one-input (peak req/sec) open-model traffic spike for
  rehearsing a launch surge, followed by a **Launch Readiness** verdict
  (READY / AT RISK / NOT READY).
- **Custom ramp builder** — define your own multi-stage ramp (concurrency or
  arrival rate) directly in the UI.

### Protocols

- HTTP/1.1 & HTTP/2 (default)
- WebSocket (message send/receive)
- GraphQL (query/mutation with error detection)
- gRPC (H2+TLS connectivity)
- TCP (raw connectivity)
- Plugin system for custom protocols

### Web UI

- Service management (create, edit, delete, clone) with a tabbed form
  (Request, Load, Checks, Advanced, Steps).
- cURL import from browser DevTools.
- Test templates (8 predefined: REST CRUD, Auth Flow, Health Check, etc.).
- Real-time metrics streaming (SSE + WebSocket) with live RPS and latency charts.
- Test history with sortable columns, pagination, and per-result deletion.
- Test comparison (side-by-side with color-coded deltas).
- Trend analysis and anomaly detection (2-sigma deviation alerts).
- AI-powered insights (pattern checks) and capacity-planning projections.
- Rate-limiting analysis (429 detection and charting).
- SSL/TLS details (protocol, cipher, certificate info) and timing breakdown.
- Drag-and-drop service reordering; bulk operations (multi-select delete/queue/export).
- Dark/light theme, keyboard shortcuts, unsaved-changes warning, search/filter.
- Mobile responsive with hamburger menu; onboarding guide for new users.
- Real-time collaboration (WebSocket broadcast).

### Load Patterns

- Six lifecycle-ordered built-ins: **Smoke → Steady → Ramp Up → Spike → Stress → Soak**.
- Test profiles (Light, Medium, Heavy presets).

### Assertions & Reporting

- Pass/fail assertions (RPS, latency percentiles, error rate).
- Print-ready HTML reports with SVG charts, including per-second timeline charts.
- Comparative reports (2–5 results side-by-side).
- PDF export, JUnit XML for CI/CD, CSV/JSON result export, and shareable result URLs.

### Notifications

- Webhook, Slack, Microsoft Teams, Discord, and Email (SMTP).
- Configurable triggers (all tests, fail only, disabled).

### Scheduling & Queue

- Cron-based scheduling (5-field expressions).
- Test queue (sequential execution) with bulk queue operations.

### CLI & Infrastructure

- CLI mode with a terminal UI (Bubbletea).
- SQLite persistent storage for services, results, and settings.
- Docker multi-stage build and Docker Compose with a Prometheus + Grafana stack.
- Prometheus metrics endpoint (`/metrics`), health/readiness endpoints
  (`/health`, `/ready`), and opt-in pprof profiling (`/debug/pprof`).
- Graceful shutdown on SIGINT/SIGTERM with partial-summary output.
- Structured logging with JSON mode and configurable levels.
- GitHub Actions CI (race detector + coverage) and GitHub PR-comment integration.
- Distributed testing (coordinator + worker nodes).
- Multi-tenancy with workspaces; data-retention policy (auto-purge old results).
- Import/export services as JSON.

[1.0.0]: https://github.com/mertgundoganx/gload/releases/tag/v1.0.0
