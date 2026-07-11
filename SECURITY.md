# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in gload, please report it responsibly.

**Do not open a public GitHub issue for security vulnerabilities.**

Instead, please report it privately through GitHub: go to the repository's
**Security** tab → **Advisories** → **Report a vulnerability**. This keeps the
details private until a fix is available.

### What to include:

- Description of the vulnerability
- Steps to reproduce
- Potential impact
- Suggested fix (if any)

### Response Timeline

- Acknowledgment within 48 hours
- Assessment within 7 days
- Fix or mitigation within 30 days for critical issues

## Security Considerations

gload is a load testing tool. By design, it sends high volumes of HTTP requests to target endpoints. Please ensure:

- You have authorization to test the target service
- You are not violating any terms of service
- You are testing in appropriate environments (staging, not production)
- Secrets (webhook URLs, SMTP credentials) are masked in API responses but are **not encrypted at rest** in the SQLite database — protect the `~/.gload/gload.db` file accordingly
- The `/debug/pprof` profiling endpoints are disabled by default and only enabled with `GLOAD_PPROF=1`
- WebSocket upgrades enforce a same-origin check by default (override with `GLOAD_WS_ORIGINS`)

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |
| < 1.0   | No        |
