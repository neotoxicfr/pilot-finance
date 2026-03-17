# <img src="go/static/logo.svg" alt="Pilot Logo" width="35" style="vertical-align: middle;"> Pilot Finance

![GitHub Release](https://img.shields.io/github/v/release/neotoxicfr/pilot-finance?logo=github&label=Version&color=grey)
![GitHub repo size](https://img.shields.io/github/repo-size/neotoxicfr/pilot-finance?logo=github&label=Repo%20size&color=grey)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?logo=docker&logoColor=white)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/neotoxicfr/pilot-finance?filename=go%2Fgo.mod&logo=go&logoColor=white&labelColor=deepskyblue&color=deepskyblue)
![HTMX](https://img.shields.io/badge/HTMX-3D72D7?logo=htmx&logoColor=white)
![Docker Build](https://github.com/neotoxicfr/pilot-finance/actions/workflows/docker-publish.yml/badge.svg)
![CI](https://github.com/neotoxicfr/pilot-finance/actions/workflows/ci.yml/badge.svg)
![E2E](https://github.com/neotoxicfr/pilot-finance/actions/workflows/e2e.yml/badge.svg)
![Lighthouse](https://github.com/neotoxicfr/pilot-finance/actions/workflows/lighthouse.yml/badge.svg)
![CodeQL](https://github.com/neotoxicfr/pilot-finance/actions/workflows/codeql.yml/badge.svg)
![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen?logo=go)
![Dependabot](https://img.shields.io/badge/dependabot-active-brightgreen?logo=dependabot&label=Dependabot)
![GitHub License](https://img.shields.io/github/license/neotoxicfr/pilot-finance?color=brightgreen)

:fr: [Version francaise](README.fr.md)

**Pilot Finance** is a self-hosted personal finance cockpit. Track your net worth, simulate yields with compound interest, and manage recurring operations — with complete privacy. All data is encrypted at rest, no external service is ever contacted at runtime.

---

## Features

### Core

- **Net worth tracking** — Visualize the overall evolution of your assets over time
- **Yield simulation** — Compound interest projection over multiple years, with automatic payment of non-reinvested interest to a target account
- **Recurring operations** — Monthly income & expense tracking with automatic projection
- **Multi-language** — French and English interface
- **Multi-currency** — EUR, USD, GBP, CHF, JPY, CAD, AUD (configurable per user)

### Security

- **AES-256-GCM** encryption of all sensitive data (emails, balances, amounts, rates, IPs, user agents)
- **bcrypt** password hashing (cost 12, 12-char minimum, 5-criteria complexity)
- **Passkeys** (WebAuthn) and **2FA** (TOTP) support
- **Strict CSP** — per-request nonces, `@alpinejs/csp` build (no `unsafe-eval`, no `unsafe-inline`)
- **CSRF protection** — Origin/Referer validation on all mutating requests
- **Rate limiting** — 120 req/min global, 10 req/min on auth routes
- **Session versioning** — automatic logout on all devices after password change
- **Audit log** — full traceability of authentication and account events (admin view)
- **Docker Secrets** support for all sensitive environment variables

### Quality

- **100% test coverage** enforced in CI
- **E2E tests** — Playwright across Chromium, Firefox, WebKit, Mobile Chrome
- **Accessibility** — axe-core audit integrated in E2E, WCAG AA contrast, `prefers-reduced-motion` support
- **Lighthouse CI** — performance threshold at 80%
- **CodeQL** + **Trivy** container scanning + **Dependabot**
- **Health check API** — database and memory monitoring endpoint
- **Prometheus `/metrics`** — request latency, error rates, DB stats

### Design

- **Responsive** — smooth experience on all devices, PWA-ready
- **Dark mode** — automatic (system) or manual toggle
- **Drag & drop** reorder on desktop, tap-to-move arrows on mobile
- **Lightweight** — ~8 MB Docker image, ~30 MB RAM, <1s start. Zero CDN requests

---

## Quick Start

### Prerequisites

- A domain name (required for Passkeys and HTTPS)
- A reverse proxy (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.)

### 1. Generate encryption keys

```bash
openssl rand -hex 32  # ENCRYPTION_KEY
openssl rand -hex 32  # BLIND_INDEX_KEY
openssl rand -hex 32  # AUTH_SECRET
```

### 2. Create `docker-compose.yml`

```yaml
services:
  pilot:
    image: ghcr.io/neotoxicfr/pilot-finance:latest
    container_name: pilot
    restart: unless-stopped
    security_opt:
      - no-new-privileges:true
    cap_drop:
      - ALL
    environment:
      - TZ=Europe/Paris
      - HOST=pilot.your-domain.tld
      - ALLOW_REGISTER=true
      - SMTP_HOST=
      - SMTP_PORT=587
      - SMTP_USER=
      - SMTP_PASS=
      - SMTP_FROM=
      - DATABASE_URL=file:/data/pilot.db
      - ENCRYPTION_KEY=
      - BLIND_INDEX_KEY=
      - AUTH_SECRET=
    volumes:
      - ./data:/data
    healthcheck:
      test: ["/app/server", "healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

### 3. Start

```bash
mkdir -p data && docker compose up -d
```

### 4. Register & lock

Open your domain, create your account, then set `ALLOW_REGISTER=false` and restart:

```bash
docker compose down && docker compose up -d
```

The application listens on port **3000** inside the container. Point your reverse proxy there.

---

## Updating

```bash
docker compose pull && docker compose up -d
```

---

## Backup

Your data lives in `./data/pilot.db`. Back it up regularly. The application also creates automatic rotating backups at startup.

---

## Environment Variables

| Variable | Required | Description |
| :--- | :---: | :--- |
| `HOST` | Yes | FQDN without protocol (e.g. `pilot.example.com`). Used for Passkeys and email links. |
| `ENCRYPTION_KEY` | Yes | 32-byte hex key for AES data encryption. **If lost, encrypted data is unrecoverable.** |
| `BLIND_INDEX_KEY` | Yes | 32-byte hex key for secure email search indexes. |
| `AUTH_SECRET` | Yes | 32+ byte hex key for JWT session signing. |
| `DATABASE_URL` | Yes | SQLite path (e.g. `file:/data/pilot.db`). |
| `ALLOW_REGISTER` | No | `true` / `false`. Set to `false` after initial registration. |
| `TZ` | No | Container timezone (e.g. `Europe/Paris`). |
| `SMTP_HOST` | No | SMTP server. Enables email verification and password recovery. |
| `SMTP_PORT` | No | SMTP port (default: 587). |
| `SMTP_USER` | No | SMTP username. |
| `SMTP_PASS` | No | SMTP password. |
| `SMTP_FROM` | No | Sender email address. |

> **Docker Secrets**: Sensitive variables support the `_FILE` suffix (e.g. `AUTH_SECRET_FILE=/run/secrets/auth_secret`). The app reads the file content at startup. Supported: `AUTH_SECRET`, `ENCRYPTION_KEY`, `BLIND_INDEX_KEY`, `SMTP_PASS`, `DATABASE_URL`.

---

## Stack

| | |
|---|---|
| Backend | Go 1.26 + chi router |
| Frontend | HTMX 2.0 + Alpine.js 3.15 (CSP build) + Tailwind CSS v4 |
| Database | SQLite (WAL mode) + automatic rotating backups |
| Charts | Chart.js 4.5 |
| Auth | bcrypt + TOTP (pquerna/otp) + WebAuthn (go-webauthn) |
| CI/CD | GitHub Actions (unit tests, E2E, CodeQL, Trivy, Lighthouse, GHCR, auto-release) |
| E2E | Playwright (Chromium, Firefox, WebKit, Mobile Chrome) |
| Docker | ~8 MB image (scratch base, UPX compressed) |

---

## Security & Privacy

- **Zero plaintext storage** — all sensitive data encrypted with AES-256-GCM
- **Zero external dependency** — no CDN, no analytics, no telemetry. All assets served locally
- **Startup verification** — server refuses to start with missing or weak encryption keys
- **Structured error codes** — every error response includes an `X-Error-Code` header

Full security policy: [SECURITY.md](SECURITY.md)

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, commit conventions, and code guidelines.

---

## Credits

Built with AI assistance for code structure and optimization. The final application is purely deterministic — no AI algorithms or third-party data processing at runtime. Your data stays 100% local and private.

---

## License

[MIT](LICENSE)
