# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| latest  | Yes       |

We recommend always running the latest release.

## Reporting a Vulnerability

If you discover a security vulnerability in Strait, please report it responsibly.

**Do not open a public GitHub issue.**

Instead, email **security@strait.dev** with:

- A description of the vulnerability
- Steps to reproduce (or a proof of concept)
- The affected version(s)
- Any potential impact assessment

Do not include live credentials, full database dumps, or customer data unless we explicitly ask for a secure transfer path. Redacted logs, minimal proofs of concept, and synthetic test data are preferred.

We will acknowledge your report within **48 hours** and aim to provide a fix or mitigation within **7 days** for critical issues.

For non-security bugs, setup questions, and feature requests, use GitHub Issues or GitHub Discussions instead of the security mailbox.

## Security Practices

- Go dependencies are checked with `govulncheck`
- JavaScript dependencies are checked with Bun audit, with critical advisories treated as release blockers
- Secrets are scanned with gitleaks in local hooks and CI
- GitHub Actions workflows use pinned SHA hashes for supply chain security
- Docker images are published to `ghcr.io` with signed metadata
- The Go binary is compiled with race detection in CI
- Secrets are never logged or exposed in error messages
- RBAC and scoped API keys enforce least-privilege access
- Webhook endpoints validate TLS and reject private IP ranges (configurable)

## Disclosure Policy

We follow coordinated disclosure. After a fix is released, we will:

1. Credit the reporter (unless they prefer anonymity)
2. Publish a security advisory on GitHub
3. Document the fix in the changelog
