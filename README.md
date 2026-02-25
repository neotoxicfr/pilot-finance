# <img src="go/static/logo.svg" alt="Pilot Logo" width="35" style="vertical-align: middle;"> Pilot Finance

![GitHub Release](https://img.shields.io/github/v/release/neotoxicfr/pilot-finance?logo=github&label=Version&color=grey)
![GitHub repo size](https://img.shields.io/github/repo-size/neotoxicfr/pilot-finance?logo=github&label=Repo%20size&color=grey)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?logo=docker&logoColor=white)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/neotoxicfr/pilot-finance?filename=go%2Fgo.mod&logo=go&logoColor=white&labelColor=deepskyblue&color=deepskyblue)
![HTMX](https://img.shields.io/badge/HTMX-3D72D7?logo=htmx&logoColor=white)
![Docker Build](https://github.com/neotoxicfr/pilot-finance/actions/workflows/docker-publish.yml/badge.svg)
![CodeQL](https://github.com/neotoxicfr/pilot-finance/actions/workflows/codeql.yml/badge.svg)
![Dependabot](https://img.shields.io/badge/dependabot-active-limegreen?logo=dependabot&label=Dependabot)
![GitHub License](https://img.shields.io/github/license/neotoxicfr/pilot-finance?color=limegreen)

🇫🇷 [Version française](README.fr.md)

**Pilot Finance** is a personal financial cockpit designed for self-hosting. A simple and secure application to track your net worth, yields and recurring operations — with complete privacy.

---

## ✨ Features

* 💰 **Net worth tracking** : Visualize the overall evolution of your assets.
* 📈 **Yield simulation** : Manage compound interests and project your gains over multiple years, with automatic payment of non-reinvested interest to a target account.
* 🔄 **Recurring operations** : Automate tracking of your monthly income and expenses.
* 🌍 **Multi-language & Multi-currency** : Interface available in French and English. Currency display configurable per user (EUR, USD, GBP, CHF, JPY, CAD, AUD).
* 🔐 **Enhanced security** :
    * **Security middleware** : Strict CSP, security headers (HSTS, X-Frame-Options), dynamic nonces.
    * **bcrypt** : Secure password hashing with complexity validation (5 criteria).
    * **Advanced rate limiting** : Multi-level protection (login, register, 2FA, reset).
    * AES-256-GCM encryption of sensitive data (email, account names, transactions).
    * **Session versioning** : Automatic logout from all devices on password change.
    * Native support for **Passkeys** (WebAuthn) and 2FA (TOTP).
    * **Health Check API** : Database and memory monitoring.
* 📧 **Email management** (Optional) : Account verification on registration and password recovery.
* 📱 **Responsive interface** : Smooth experience across all devices (mobile, tablet and desktop).
* ⚡ **Optimal performance** : Ultra-lightweight Go backend (~15MB binary), HTMX + Alpine.js frontend (~30KB JS), zero external CDN requests.

---

## 🚀 What's new in v2.1

* 🌍 **Multi-language support** (FR / EN) — interface language configurable per user from Settings
* 💱 **Multi-currency support** — display currency configurable per user (EUR, USD, GBP, CHF, JPY, CAD, AUD)
* ⚙️ **Preferences panel** added to Settings

---

## 🚀 What's new in v2.0

Version 2.0 is a **complete technical rewrite**:

| Metric | v1.x (Next.js) | v2.0 (Go) |
|--------|----------------|-----------|
| Docker image | ~300 MB | ~20 MB |
| RAM usage | ~200 MB | ~30 MB |
| Start time | ~5s | <1s |
| JS Frontend | ~500 KB | ~30 KB |
| CDN dependencies | Multiple | **0** (self-hosted) |

### Tech stack
- **Backend**: Go 1.26 + chi router
- **Frontend**: HTMX 2.0 + Alpine.js 3.15 + Tailwind CSS (compiled at build)
- **Database**: SQLite (WAL mode)
- **Charts**: Chart.js 4.5
- **Zero external dependency**: all JS/CSS assets are self-hosted

---

## 🚀 Installation with Docker

The recommended method is to use **Docker Compose**.

### 1. Prerequisites
* A domain name (required for Passkeys and SSL validation).
* A reverse proxy already configured (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.).

### 2. Configuration (`docker-compose.yml`)

Create a `docker-compose.yml` file in your working directory:

```yaml
services:
  pilot:
    image: ghcr.io/neotoxicfr/pilot-finance:latest
    container_name: pilot
    restart: unless-stopped
    security_opt:
      - no-new-privileges=true
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

### 3. Start

Launch the container with:
```bash
docker compose up -d
```
The application listens on port **3000** inside the container.

---

## 🛠️ Environment variables

| Variable | Description |
| :--- | :--- |
| **HOST** | Your fully qualified domain name without the protocol (e.g. `pilot.example.com`). Required for Passkeys and email links. |
| **ENCRYPTION_KEY** | **Critical**. 32-byte (hex) key for AES data encryption. If lost, encrypted data is unrecoverable. |
| **BLIND_INDEX_KEY** | **Critical**. 32-byte (hex) key for secure search indexes (emails). |
| **AUTH_SECRET** | **Critical**. Key of at least 32 bytes for JWT session cookie signing. |
| **ALLOW_REGISTER** | Allows or blocks new account creation. It is recommended to set this to `false` after your initial registration. |
| **DATABASE_URL** | Path to your SQLite database (e.g. `file:/data/pilot.db`). |
| **TZ** | Container timezone (e.g. `Europe/Paris`) for accurate operation dates. |

---

## 🛡️ Security & Privacy

Pilot Finance was built with security by default:

* **Zero plaintext storage**: Account names and transaction labels are encrypted. Only your server with its unique key can read them.
* **Zero external dependency**: No requests to CDNs or third-party services. All assets are compiled and served locally.
* **Startup verification**: The system refuses to start if encryption keys are missing or too weak.
* **Passkeys protection**: Using Passkeys provides robust protection against phishing and eliminates the need to memorize complex passwords.
* **Password validation**: 5 mandatory criteria (length, uppercase, lowercase, number, special character).

---

## 🤖 Credits & Design

This project was designed with the assistance of Artificial Intelligence for code structure and optimization. However, **the final code is purely applicative** and uses no AI algorithms or third-party data processing services at runtime. Your cockpit remains 100% local and private.

---

## 📝 License

This project is distributed under the **MIT** license.
