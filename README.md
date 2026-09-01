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
- **Rate limiting** — 120 req/min global, 10 req/min on auth routes (see `DISABLE_RATE_LIMIT` and `TRUSTED_PROXIES` below)
- **Session versioning** — automatic logout on all devices after password change
- **Audit log** — full traceability of authentication and account events (admin view)
- **Non-root container** — runs as uid/gid 1000 on a `scratch` base (no shell, no package manager)
- **Docker Secrets** support for all sensitive environment variables

### Quality

- **100% test coverage** enforced in CI (excluding `cmd/server` and `internal/db`, whose tests run without a coverage threshold)
- **E2E tests** — Playwright on Chromium, Firefox and Mobile Chrome in CI (WebKit is run locally)
- **Accessibility** — axe-core audit in E2E on every page, in **both light and dark themes**, colour contrast included; `prefers-reduced-motion` support
- **Lighthouse CI** — performance threshold at 80%
- **CodeQL** + **Trivy** (dependency scan and image scan, both blocking) + **Dependabot**
- **Pinned build chain** — GitHub Actions pinned by commit SHA, asset toolchain (Tailwind, esbuild) pinned to exact versions and installed with `--ignore-scripts`
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
      # JSON logs, and X-Forwarded-For is only trusted from TRUSTED_PROXIES
      - ENV=production
      # IPs/CIDRs allowed to set X-Forwarded-For. Required when ENV=production.
      # Narrow this to your reverse proxy; 172.16.0.0/12 covers Docker bridges.
      - TRUSTED_PROXIES=172.16.0.0/12
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
      # The first element must be CMD / CMD-SHELL / NONE, otherwise Compose
      # installs no probe *and* overrides the (working) one from the image.
      test: ["CMD", "/app/server", "healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

The image already ships a healthcheck, so you can drop the `healthcheck:` block entirely if you prefer.

### 3. Start

The container runs as uid/gid **1000**, so the data directory must belong to it:

```bash
mkdir -p data && sudo chown -R 1000:1000 data
docker compose up -d
```

### 4. Register & lock

Open your domain, create your account, then set `ALLOW_REGISTER=false` and restart:

```bash
docker compose down && docker compose up -d
```

The application listens on port **3000** inside the container. Point your reverse proxy there.

---

## Updating

Schema migrations run automatically at startup and are **not reversible**. Take a backup first:

```bash
docker compose down
cp -a data data-backup-$(date +%F)
docker compose pull && docker compose up -d
docker compose logs -f pilot   # the server exits 1 rather than serve an uncertain schema
```

### Upgrading from v2.23.0 or earlier

Older images ran as root, so the files in `./data` belong to root. From the next
release the server runs as uid/gid **1000** and will not be able to open the
database until you hand the directory over — a one-time operation:

```bash
docker compose down
sudo chown -R 1000:1000 data
docker compose pull && docker compose up -d
```

---

## Backup & Restore

Everything lives in `./data`:

| File | Contents |
| :--- | :--- |
| `pilot.db` | The database |
| `pilot.db-wal`, `pilot.db-shm` | Write-ahead log — **part of the database**. Never copy `pilot.db` on its own from a running instance. |
| `pilot.db.backup.1` / `.2` / `.3` | Automatic rotating snapshots (`.1` = most recent), written at startup and every 24 h |
| `pilot.db.bak` | One-off snapshot taken before the encryption migration. On an instance migrated from the Node version **it contains plaintext amounts** — delete it once you have checked your data. |

> **The keys are part of the backup.** A database without its `ENCRYPTION_KEY` and `BLIND_INDEX_KEY` is an unreadable block. Both keys are **permanent** — there is no key rotation. Store them alongside (not inside) your backups.

### Backing up

Cold backup — the only one that captures a guaranteed-consistent state:

```bash
docker compose down
cp -a data data-backup-$(date +%F)
docker compose up -d
```

Hot backup — copy the most recent rotating snapshot. It is produced by `VACUUM INTO`, so it is a self-contained, consistent file (no `-wal` needed):

```bash
cp data/pilot.db.backup.1 /somewhere/pilot-$(date +%F).db
```

You can also export your accounts and operations as CSV from **Settings → Export**.

### Restoring

Copying a snapshot over `pilot.db` and restarting **is silently undone**: the leftover `-wal` file is replayed on the next start, your old data comes back, and `integrity_check` still reports `ok`. The `-wal`/`-shm` files must be removed in the same step:

```bash
docker compose down
rm -f data/pilot.db data/pilot.db-wal data/pilot.db-shm   # removing -wal/-shm is mandatory
cp /path/to/pilot.db.backup.1 data/pilot.db
sudo chown 1000:1000 data/pilot.db
docker compose up -d
```

Restore with the **same** `ENCRYPTION_KEY` / `BLIND_INDEX_KEY` the backup was written with, otherwise every amount and email fails to decrypt.

---

## Environment Variables

| Variable | Required | Description |
| :--- | :---: | :--- |
| `HOST` | Yes | FQDN without protocol (e.g. `pilot.example.com`). Used for Passkeys and email links. |
| `ENCRYPTION_KEY` | Yes | 32-byte hex key for AES data encryption. **If lost, encrypted data is unrecoverable.** |
| `BLIND_INDEX_KEY` | Yes | 32-byte hex key for secure email search indexes. |
| `AUTH_SECRET` | Yes | 32+ byte hex key for JWT session signing. |
| `DATABASE_URL` | Yes | SQLite path (e.g. `file:/data/pilot.db`). |
| `TRUSTED_PROXIES` | In production | Comma-separated IPs and/or CIDR ranges allowed to set `X-Forwarded-For` / `X-Real-IP` (e.g. `172.16.0.0/12`). **Mandatory when `ENV=production` — the server refuses to start without it.** Left empty outside production, `X-Forwarded-For` is accepted from any source (development only), which lets a client forge its IP and bypass the rate limits. |
| `ENV` | No | `production` switches logs to JSON **and makes `TRUSTED_PROXIES` mandatory**. Any other value, or unset, means development. |
| `ALLOW_REGISTER` | No | `true` / `false` (default `false`). Set to `false` after initial registration. |
| `PORT` | No | Port the server listens on inside the container (default: `3000`). |
| `TZ` | No | Container timezone (e.g. `Europe/Paris`). |
| `DISABLE_RATE_LIMIT` | No | `true` **turns off all rate limiting** (the 120 req/min and 10 req/min auth limits). Intended for the E2E suite only — never set it on an exposed instance. |
| `SMTP_HOST` | No | SMTP server. Enables email verification and password recovery. |
| `SMTP_PORT` | No | SMTP port (default: 587). |
| `SMTP_USER` | No | SMTP username. |
| `SMTP_PASS` | No | SMTP password. |
| `SMTP_FROM` | No | Sender email address. |
| `SMTP_SECURE` | No | `true` = implicit TLS (typically port 465). Otherwise STARTTLS (typically port 587). |

> **Note on polarity**: `ALLOW_REGISTER` and `DISABLE_RATE_LIMIT` read in opposite directions. `ALLOW_REGISTER=true` **opens** registration; `DISABLE_RATE_LIMIT=true` **removes** a protection. Only the literal string `true` is honoured in both cases.

> **Keys are permanent**: `ENCRYPTION_KEY` and `BLIND_INDEX_KEY` cannot be rotated — there is no key-version mechanism. Changing one makes existing data unreadable. The only way out is Settings → Export, then a fresh database with the new keys and a re-import. `AUTH_SECRET` can be changed freely; it only invalidates active sessions.

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
| CI/CD | GitHub Actions (unit tests, E2E, CodeQL, Trivy, Lighthouse, GHCR, auto-release) — image publication is gated on the test suite |
| E2E | Playwright — Chromium, Firefox, Mobile Chrome in CI; WebKit locally |
| Docker | ~8 MB image (scratch base, UPX compressed), runs as uid 1000 |

---

## Security & Privacy

- **Zero plaintext storage** — all sensitive data encrypted with AES-256-GCM
- **Zero external dependency** — no CDN, no analytics, no telemetry. All assets served locally
- **Startup verification** — server refuses to start with missing or weak encryption keys
- **Structured error codes** — every error response includes an `X-Error-Code` header
- **Account deletion** — accounts, recurring operations and authenticators are removed immediately; audit-log rows are **anonymised** (kept without identity until the automatic 90-day purge), and the rotating backups still hold an encrypted copy until they rotate out

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
