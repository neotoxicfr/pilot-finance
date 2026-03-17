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

:gb: [English version](README.md)

**Pilot Finance** est un cockpit financier personnel concu pour l'auto-hebergement. Suivez votre patrimoine net, simulez vos rendements avec interets composes et gerez vos operations recurrentes — en toute confidentialite. Toutes les donnees sont chiffrees au repos, aucun service externe n'est jamais contacte.

---

## Fonctionnalites

### Coeur

- **Suivi de patrimoine** — Visualisez l'evolution globale de vos actifs dans le temps
- **Simulation de rendements** — Projection des interets composes sur plusieurs annees, avec versement automatique des interets non reinvestis vers un compte cible
- **Operations recurrentes** — Suivi des revenus et depenses mensuels avec projection automatique
- **Multi-langue** — Interface en francais et en anglais
- **Multi-devise** — EUR, USD, GBP, CHF, JPY, CAD, AUD (configurable par utilisateur)

### Securite

- Chiffrement **AES-256-GCM** de toutes les donnees sensibles (emails, soldes, montants, taux, IPs, user agents)
- Hashing **bcrypt** des mots de passe (cout 12, minimum 12 caracteres, complexite 5 criteres)
- Support **Passkeys** (WebAuthn) et **2FA** (TOTP)
- **CSP stricte** — nonces dynamiques par requete, build `@alpinejs/csp` (pas d'`unsafe-eval`, pas d'`unsafe-inline`)
- **Protection CSRF** — validation Origin/Referer sur toutes les requetes mutantes
- **Rate limiting** — 120 req/min global, 10 req/min sur les routes d'authentification
- **Session versioning** — deconnexion automatique sur tous les appareils apres changement de mot de passe
- **Journal d'audit** — tracabilite complete des evenements d'authentification et de compte (vue admin)
- Support **Docker Secrets** pour toutes les variables sensibles

### Qualite

- **100% de couverture de tests** appliquee en CI
- **Tests E2E** — Playwright sur Chromium, Firefox, WebKit, Mobile Chrome
- **Accessibilite** — audit axe-core integre aux E2E, contraste WCAG AA, support `prefers-reduced-motion`
- **Lighthouse CI** — seuil de performance a 80%
- **CodeQL** + scan de conteneur **Trivy** + **Dependabot**
- **API Health Check** — monitoring base de donnees et memoire
- **Prometheus `/metrics`** — latence, taux d'erreurs, stats DB

### Design

- **Responsive** — experience fluide sur tous les supports, compatible PWA
- **Dark mode** — automatique (systeme) ou basculement manuel
- **Glisser-deposer** sur desktop, boutons de deplacement sur mobile
- **Leger** — image Docker ~8 Mo, ~30 Mo de RAM, demarrage <1s. Zero requete CDN

---

## Demarrage rapide

### Prerequis

- Un nom de domaine (indispensable pour les Passkeys et HTTPS)
- Un reverse-proxy (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.)

### 1. Generer les cles de chiffrement

```bash
openssl rand -hex 32  # ENCRYPTION_KEY
openssl rand -hex 32  # BLIND_INDEX_KEY
openssl rand -hex 32  # AUTH_SECRET
```

### 2. Creer `docker-compose.yml`

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
      - HOST=pilot.votre-domaine.tld
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

### 3. Demarrer

```bash
mkdir -p data && docker compose up -d
```

### 4. S'inscrire et verrouiller

Ouvrez votre domaine, creez votre compte, puis passez `ALLOW_REGISTER=false` et redemarrez :

```bash
docker compose down && docker compose up -d
```

L'application ecoute sur le port **3000** a l'interieur du conteneur. Configurez votre reverse-proxy pour y rediriger le trafic.

---

## Mise a jour

```bash
docker compose pull && docker compose up -d
```

---

## Sauvegarde

Vos donnees sont dans `./data/pilot.db`. Sauvegardez ce fichier regulierement. L'application cree aussi des sauvegardes rotatives automatiques au demarrage.

---

## Variables d'environnement

| Variable | Requis | Description |
| :--- | :---: | :--- |
| `HOST` | Oui | FQDN sans protocole (ex : `pilot.exemple.com`). Utilise pour les Passkeys et les liens mail. |
| `ENCRYPTION_KEY` | Oui | Cle hex de 32 octets pour le chiffrement AES. **Si perdue, les donnees chiffrees sont irrecuperables.** |
| `BLIND_INDEX_KEY` | Oui | Cle hex de 32 octets pour les index de recherche securises. |
| `AUTH_SECRET` | Oui | Cle hex de 32+ octets pour la signature des sessions JWT. |
| `DATABASE_URL` | Oui | Chemin SQLite (ex : `file:/data/pilot.db`). |
| `ALLOW_REGISTER` | Non | `true` / `false`. Passer a `false` apres l'inscription initiale. |
| `TZ` | Non | Fuseau horaire du conteneur (ex : `Europe/Paris`). |
| `SMTP_HOST` | Non | Serveur SMTP. Active la verification email et la recuperation de mot de passe. |
| `SMTP_PORT` | Non | Port SMTP (defaut : 587). |
| `SMTP_USER` | Non | Identifiant SMTP. |
| `SMTP_PASS` | Non | Mot de passe SMTP. |
| `SMTP_FROM` | Non | Adresse email d'expedition. |

> **Docker Secrets** : Les variables sensibles supportent le suffixe `_FILE` (ex : `AUTH_SECRET_FILE=/run/secrets/auth_secret`). L'app lit le contenu du fichier au demarrage. Supportees : `AUTH_SECRET`, `ENCRYPTION_KEY`, `BLIND_INDEX_KEY`, `SMTP_PASS`, `DATABASE_URL`.

---

## Stack technique

| | |
|---|---|
| Backend | Go 1.26 + chi router |
| Frontend | HTMX 2.0 + Alpine.js 3.15 (build CSP) + Tailwind CSS v4 |
| Base de donnees | SQLite (mode WAL) + backups rotatifs automatiques |
| Graphiques | Chart.js 4.5 |
| Auth | bcrypt + TOTP (pquerna/otp) + WebAuthn (go-webauthn) |
| CI/CD | GitHub Actions (tests unitaires, E2E, CodeQL, Trivy, Lighthouse, GHCR, auto-release) |
| E2E | Playwright (Chromium, Firefox, WebKit, Mobile Chrome) |
| Docker | Image ~8 Mo (base alpine:3.23) |

---

## Securite et confidentialite

- **Zero stockage en clair** — toutes les donnees sensibles chiffrees avec AES-256-GCM
- **Zero dependance externe** — pas de CDN, pas d'analytics, pas de telemetrie. Tous les assets servis localement
- **Verification au demarrage** — le serveur refuse de demarrer si les cles sont absentes ou trop courtes
- **Codes d'erreur structures** — chaque reponse d'erreur inclut un header `X-Error-Code`

Politique de securite complete : [SECURITY.md](SECURITY.md)

---

## Contribuer

Voir [CONTRIBUTING.md](CONTRIBUTING.md) pour la configuration de developpement, les conventions de commit et les regles de code.

---

## Credits

Concu avec l'assistance d'une Intelligence Artificielle pour la structure et l'optimisation du code. L'application finale est purement deterministe — aucun algorithme d'IA ou service tiers a l'execution. Vos donnees restent 100% locales et privees.

---

## Licence

[MIT](LICENSE)
