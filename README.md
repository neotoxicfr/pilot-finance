# <img src="go/static/logo.svg" alt="Pilot Logo" width="35" style="vertical-align: middle;"> Pilot Finance

![GitHub Release](https://img.shields.io/github/v/release/neotoxicfr/pilot-finance?logo=github&label=Version&color=grey)
![GitHub repo size](https://img.shields.io/github/repo-size/neotoxicfr/pilot-finance?logo=github&label=Repo%20size&color=grey)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?logo=docker&logoColor=white)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/neotoxicfr/pilot-finance?filename=go%2Fgo.mod&logo=go&logoColor=white&labelColor=deepskyblue&color=deepskyblue)
![HTMX](https://img.shields.io/badge/HTMX-3D72D7?logo=htmx&logoColor=white)
![Docker Build](https://github.com/neotoxicfr/pilot-finance/actions/workflows/docker-publish.yml/badge.svg)
![CI](https://github.com/neotoxicfr/pilot-finance/actions/workflows/ci.yml/badge.svg)
![CodeQL](https://github.com/neotoxicfr/pilot-finance/actions/workflows/codeql.yml/badge.svg)
![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen?logo=go)
![Dependabot](https://img.shields.io/badge/dependabot-active-brightgreen?logo=dependabot&label=Dependabot)
![GitHub License](https://img.shields.io/github/license/neotoxicfr/pilot-finance?color=brightgreen)

:fr: [Version française](README.fr.md)

**Pilot Finance** is a personal financial cockpit designed for self-hosting. A simple and secure application to track your net worth, yields and recurring operations — with complete privacy.

---

## Features

* **Net worth tracking** — Visualize the overall evolution of your assets over time.
* **Yield simulation** — Manage compound interest and project gains over multiple years, with automatic payment of non-reinvested interest to a target account.
* **Recurring operations** — Track monthly income and expenses with automatic projection.
* **Multi-language & Multi-currency** — Interface available in French and English. Currency display configurable per user (EUR, USD, GBP, CHF, JPY, CAD, AUD).
* **Security by default** :
    * **`@alpinejs/csp`** build — no `unsafe-eval` in CSP; all Alpine components registered server-side
    * Strict **Content Security Policy** with per-request dynamic nonces — no `unsafe-inline` for scripts
    * **`X-Frame-Options: DENY`** + **`Permissions-Policy`** — clickjacking protection and browser API restrictions
    * **CSRF protection** — Origin/Referer header validation on all mutating requests
    * **AES-256-GCM** encryption of all sensitive data (emails, account names, transaction labels)
    * **bcrypt** password hashing — cost 12, 12-character minimum, 5-criteria complexity validation, automatic cost upgrade on login
    * **Session versioning** — automatic logout from all devices on password change
    * Multi-level **rate limiting** — global 120 req/min, auth routes 10 req/min
    * Native **Passkeys** (WebAuthn) and **2FA** (TOTP) support
    * **Audit log** — full traceability of authentication and account events (admin)
    * **Health Check API** — database and memory monitoring endpoint
* **Email** (optional) — Account verification on registration and password recovery.
* **Responsive** — Smooth experience on all devices. Drag & drop reorder on desktop, tap-to-move arrows on mobile. PWA-ready.
* **Lightweight** — ~40 MB Docker image, ~30 MB RAM, <1s start time. JS loaded per-page (no global bundle). Zero CDN requests.

---

## Installation with Docker

The recommended method is **Docker Compose**.

### Prerequisites

* A domain name (required for Passkeys and HTTPS).
* A reverse proxy already configured (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.).

### `docker-compose.yml`

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
      - HOST=pilot.your-domain.tld  # Your domain without https (e.g. pilot.example.com)
      - ALLOW_REGISTER=true         # Set to false after your initial registration
      - SMTP_HOST=                  # Optional: enable emails
      - SMTP_PORT=587
      - SMTP_USER=
      - SMTP_PASS=
      - SMTP_FROM=
      - DATABASE_URL=file:/data/pilot.db
      - ENCRYPTION_KEY=             # Required: openssl rand -hex 32
      - BLIND_INDEX_KEY=            # Required: openssl rand -hex 32
      - AUTH_SECRET=                # Required: openssl rand -hex 32
    volumes:
      - ./data:/data
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "http://127.0.0.1:3000/api/health"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

```bash
docker compose up -d
```

The application listens on port **3000** inside the container.

---

## Environment variables

| Variable | Description |
| :--- | :--- |
| **HOST** | Your fully qualified domain name without the protocol (e.g. `pilot.example.com`). Required for Passkeys and email links. |
| **ENCRYPTION_KEY** | **Critical.** 32-byte hex key for AES data encryption. If lost, encrypted data is unrecoverable. Generate with `openssl rand -hex 32`. |
| **BLIND_INDEX_KEY** | **Critical.** 32-byte hex key for secure email search indexes. Generate with `openssl rand -hex 32`. |
| **AUTH_SECRET** | **Critical.** Key of at least 32 bytes for JWT session signing. Generate with `openssl rand -hex 32`. |
| **ALLOW_REGISTER** | Allows or blocks new account creation. Set to `false` after your initial registration. |
| **DATABASE_URL** | Path to your SQLite database (e.g. `file:/data/pilot.db`). |
| **TZ** | Container timezone (e.g. `Europe/Paris`) for accurate operation dates. |
| **SMTP_HOST / PORT / USER / PASS / FROM** | Optional. Enable email features (verification, password reset). |

---

## Security & Privacy

* **Zero plaintext storage** — Account names and transaction labels are encrypted with AES-256-GCM. Only your server holds the key.
* **Zero external dependency** — No CDN requests at runtime. All JS and CSS assets are compiled and served locally.
* **Strict CSP** — Per-request nonces + `@alpinejs/csp` build (no `unsafe-eval`). No `unsafe-inline` in `script-src`.
* **Startup verification** — The server refuses to start if encryption keys are missing or too short. Schema integrity is verified at every startup.
* **Passkeys** — WebAuthn provides phishing-resistant authentication without passwords.
* **Structured error codes** — Every error response includes an `X-Error-Code` header for programmatic error handling.

---

## Stack

| | |
|---|---|
| Backend | Go 1.26 + chi router |
| Frontend | HTMX 2.0 + Alpine.js 3.15 (CSP build) + Tailwind CSS v4 |
| Database | SQLite (WAL mode) + automatic rotating backups |
| Charts | Chart.js 4.5 |
| Auth | bcrypt + TOTP (pquerna/otp) + WebAuthn (go-webauthn) |
| CI/CD | GitHub Actions (tests, CodeQL, GHCR image, auto-release) |
| Docker image | ~40 MB (alpine:3.23 base) |

---

## Credits

This project was designed with AI assistance for code structure and optimization. The final code is purely applicative and uses no AI algorithms or third-party data processing at runtime. Your cockpit remains 100% local and private.

---

## License

Distributed under the **MIT** license.
