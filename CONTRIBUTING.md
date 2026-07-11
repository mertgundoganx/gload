# Contributing to gload

Thank you for your interest in contributing to gload. This document provides guidelines for contributing.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/your-username/gload.git`
3. Create a branch: `git checkout -b feature/your-feature`
4. Make your changes
5. Run tests: `make test`
6. Commit: `git commit -m "Add your feature"`
7. Push: `git push origin feature/your-feature`
8. Open a Pull Request

## Development Setup

### Prerequisites

- Go 1.26 or later
- Git

### Build and Test

```bash
make build      # Build the binary
make test       # Run all tests
make test-v     # Run tests with verbose output
make cover      # Generate coverage report
make vet        # Run go vet
make lint       # Run linter (requires golangci-lint)
```

### Running Locally

```bash
make run        # Build and start the web server
```

## Code Guidelines

- Follow standard Go conventions (`go fmt`, `go vet`)
- Write tests for new features
- Keep functions focused and small
- Use meaningful variable and function names
- Add comments only where the logic is not self-evident
- Do not add dependencies unless absolutely necessary

## Project Structure

```
internal/
  server/       # HTTP handlers (split by domain)
  runner/       # Load test engine
  metrics/      # Metrics collection and histograms
  storage/      # SQLite database layer
  notifier/     # Webhook, Slack, Teams, Discord, Email
  scheduler/    # Cron-based test scheduling
  report/       # HTML/PDF report generation
  plugin/       # Protocol plugins (WebSocket, GraphQL, gRPC, TCP)
  faker/        # Random data generation
  logger/       # Structured logging
  prom/         # Prometheus metrics
  github/       # GitHub PR comment integration
  junit/        # JUnit XML report generation
  worker/       # Distributed worker node
pkg/
  config/       # CLI flags and configuration
web/
  templates/    # HTML templates
  static/       # CSS, JavaScript
```

## Pull Request Process

1. Ensure your code compiles: `make build`
2. Ensure all tests pass: `make test`
3. Update README.md if you added a new feature
4. Describe your changes clearly in the PR description
5. Link any related issues

## Reporting Issues

- Use GitHub Issues
- Include steps to reproduce
- Include expected vs actual behavior
- Include Go version (`go version`) and OS

## License

By contributing, you agree that your contributions will be licensed under the MIT License, as described in the [LICENSE](LICENSE) file.
