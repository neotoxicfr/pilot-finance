# <img src="go/static/logo.svg" alt="Pilot Logo" width="35" style="vertical-align: middle;"> Pilot Finance

![GitHub Release](https://img.shields.io/github/v/release/neotoxicfr/pilot-finance?logo=github&label=Version&color=grey)
![GitHub repo size](https://img.shields.io/github/repo-size/neotoxicfr/pilot-finance?logo=github&label=Repo%20size&color=grey)
![Docker](https://img.shields.io/badge/docker-%230db7ed.svg?logo=docker&logoColor=white)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/neotoxicfr/pilot-finance?filename=go%2Fgo.mod&logo=go&logoColor=white&labelColor=deepskyblue&color=deepskyblue)
![HTMX](https://img.shields.io/badge/HTMX-3D72D7?logo=htmx&logoColor=white)
![Docker Build](https://github.com/neotoxicfr/pilot-finance/actions/workflows/docker-publish.yml/badge.svg)
![CI](https://github.com/neotoxicfr/pilot-finance/actions/workflows/ci.yml/badge.svg)
![E2E](https://github.com/neotoxicfr/pilot-finance/actions/workflows/e2e.yml/badge.svg)
![CodeQL](https://github.com/neotoxicfr/pilot-finance/actions/workflows/codeql.yml/badge.svg)
![Coverage](https://img.shields.io/badge/coverage-100%25-brightgreen?logo=go)
![Dependabot](https://img.shields.io/badge/dependabot-active-brightgreen?logo=dependabot&label=Dependabot)
![GitHub License](https://img.shields.io/github/license/neotoxicfr/pilot-finance?color=brightgreen)

:gb: [English version](README.md)

**Pilot Finance** est un cockpit financier personnel conçu pour l'auto-hébergement. Une application simple et sécurisée pour suivre votre patrimoine net, vos rendements et vos opérations récurrentes — en toute confidentialité.

---

## Fonctionnalités

* **Suivi de patrimoine** — Visualisez l'évolution globale de vos actifs dans le temps.
* **Simulation de rendements** — Gérez vos intérêts composés et projetez vos gains sur plusieurs années, avec versement automatique des intérêts non réinvestis vers un compte cible.
* **Opérations récurrentes** — Suivez vos revenus et dépenses mensuels avec projection automatique.
* **Multi-langue & Multi-devise** — Interface disponible en français et en anglais. Devise d'affichage configurable par utilisateur (EUR, USD, GBP, CHF, JPY, CAD, AUD).
* **Sécurité par défaut** :
    * Build **`@alpinejs/csp`** — pas d'`unsafe-eval` dans la CSP ; tous les composants Alpine enregistrés côté serveur
    * **Content Security Policy** stricte avec nonces dynamiques par requête — pas d'`unsafe-inline` pour les scripts
    * **`X-Frame-Options: DENY`** + **`Permissions-Policy`** — protection clickjacking et restriction des API navigateur
    * **Protection CSRF** — validation des headers Origin/Referer sur toutes les requêtes mutantes
    * Chiffrement **AES-256-GCM** de toutes les données sensibles (emails, noms de comptes, libellés de transactions)
    * Hashing des mots de passe **bcrypt** — coût 12, minimum 12 caractères, complexité 5 critères, upgrade automatique du coût au login
    * **Session versioning** — déconnexion automatique de tous les appareils en cas de changement de mot de passe
    * **Rate limiting** multi-niveaux — 120 req/min global, 10 req/min sur les routes d'authentification
    * Support natif des **Passkeys** (WebAuthn) et **2FA** (TOTP)
    * **Journal d'audit** — traçabilité complète des événements d'authentification et de compte (admin)
    * **API Health Check** — endpoint de monitoring de la base de données et de la mémoire
* **Email** (optionnel) — Vérification du compte à l'inscription et récupération de mot de passe.
* **Responsive** — Expérience fluide sur tous les supports. Glisser-déposer sur desktop, boutons de déplacement sur mobile. Compatible PWA.
* **Léger** — Image Docker ~40 Mo, ~30 Mo de RAM, démarrage <1s. JS chargé par page (pas de bundle global). Zéro requête CDN.

---

## Installation avec Docker

La méthode recommandée est **Docker Compose**.

### Prérequis

* Un nom de domaine (indispensable pour les Passkeys et HTTPS).
* Un reverse-proxy déjà configuré (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.).

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
      - HOST=pilot.votre-domaine.tld  # Votre domaine sans https (ex: pilot.exemple.com)
      - ALLOW_REGISTER=true           # Mettre à false après votre inscription initiale
      - SMTP_HOST=                    # Optionnel : activer les emails
      - SMTP_PORT=587
      - SMTP_USER=
      - SMTP_PASS=
      - SMTP_FROM=
      - DATABASE_URL=file:/data/pilot.db
      - ENCRYPTION_KEY=               # Obligatoire : openssl rand -hex 32
      - BLIND_INDEX_KEY=              # Obligatoire : openssl rand -hex 32
      - AUTH_SECRET=                  # Obligatoire : openssl rand -hex 32
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

L'application écoute sur le port **3000** à l'intérieur du conteneur.

### Démarrage rapide (étape par étape)

1. **Générer les clés de chiffrement** (exécuter chaque commande une fois, sauvegarder le résultat) :
   ```bash
   openssl rand -hex 32  # → ENCRYPTION_KEY
   openssl rand -hex 32  # → BLIND_INDEX_KEY
   openssl rand -hex 32  # → AUTH_SECRET
   ```

2. **Créer le dossier du projet** :
   ```bash
   mkdir -p ~/pilot-finance/data
   cd ~/pilot-finance
   ```

3. **Créer `docker-compose.yml`** avec le modèle ci-dessus, en remplaçant les clés et le domaine.

4. **Démarrer l'application** :
   ```bash
   docker compose up -d
   ```

5. **Configurer votre reverse proxy** pour rediriger le trafic de votre domaine vers `http://localhost:3000`.

6. **Ouvrir votre domaine** dans un navigateur, créer votre compte, puis passer `ALLOW_REGISTER=false` et redémarrer :
   ```bash
   docker compose down
   docker compose up -d
   ```

### Mise à jour

```bash
docker compose pull
docker compose up -d
```

### Sauvegarde

Vos données sont stockées dans `./data/pilot.db`. Sauvegardez ce fichier régulièrement. L'application crée des sauvegardes rotatives automatiques au démarrage.

---

## Variables d'environnement

| Variable | Description |
| :--- | :--- |
| **HOST** | Votre nom de domaine complet sans le protocole (ex: `pilot.exemple.com`). Indispensable pour les Passkeys et les liens mail. |
| **ENCRYPTION_KEY** | **Critique.** Clé hex de 32 octets pour le chiffrement AES. Si perdue, les données chiffrées sont irrécupérables. Générer avec `openssl rand -hex 32`. |
| **BLIND_INDEX_KEY** | **Critique.** Clé hex de 32 octets pour les index de recherche sécurisés (emails). Générer avec `openssl rand -hex 32`. |
| **AUTH_SECRET** | **Critique.** Clé d'au moins 32 octets pour la signature des cookies de session JWT. Générer avec `openssl rand -hex 32`. |
| **ALLOW_REGISTER** | Permet ou bloque la création de nouveaux comptes. Passer à `false` après votre inscription. |
| **DATABASE_URL** | Chemin vers votre base SQLite (ex: `file:/data/pilot.db`). |
| **TZ** | Fuseau horaire du conteneur (ex: `Europe/Paris`) pour la précision des dates. |
| **SMTP_HOST / PORT / USER / PASS / FROM** | Optionnel. Active les fonctionnalités email (vérification, réinitialisation). |

---

## Sécurité et Confidentialité

* **Zéro stockage en clair** — Les noms de comptes et libellés de transactions sont chiffrés avec AES-256-GCM. Seul votre serveur détient la clé.
* **Zéro dépendance externe** — Aucune requête CDN à l'exécution. Tous les assets JS et CSS sont compilés et servis localement.
* **CSP stricte** — Nonces dynamiques par requête + build `@alpinejs/csp` (pas d'`unsafe-eval`). Pas d'`unsafe-inline` dans `script-src`.
* **Vérification au démarrage** — Le serveur refuse de démarrer si les clés de chiffrement sont absentes ou trop courtes. L'intégrité du schéma est vérifiée à chaque démarrage.
* **Passkeys** — WebAuthn offre une authentification résistante au phishing sans mémorisation de mot de passe.
* **Codes d'erreur structurés** — Chaque réponse d'erreur inclut un header `X-Error-Code` pour un traitement programmatique.

---

## Stack technique

| | |
|---|---|
| Backend | Go 1.26 + chi router |
| Frontend | HTMX 2.0 + Alpine.js 3.15 (build CSP) + Tailwind CSS v4 |
| Base de données | SQLite (mode WAL) + backups rotatifs automatiques |
| Graphiques | Chart.js 4.5 |
| Auth | bcrypt + TOTP (pquerna/otp) + WebAuthn (go-webauthn) |
| CI/CD | GitHub Actions (tests, E2E, CodeQL, Trivy, image GHCR, auto-release) |
| E2E | Playwright (Chromium, Firefox, WebKit, Mobile Chrome) |
| Image Docker | ~40 Mo (base alpine:3.23) |

---

## Feuille de route

| Catégorie | Élément | Statut |
|---|---|---|
| **UI/UX** | Refonte visuelle — glassmorphism, dark mode, micro-interactions | Fait |
| **Sécurité** | Rate limiting par compte (protection brute-force distribué) | Fait |
| **Sécurité** | Verrouillage temporaire du compte après N échecs de connexion | Fait |
| **Sécurité** | Directive CSP `report-to` vers `/api/csp-report` en production | Fait |
| **Sécurité** | Forcer TLS sur les connexions SMTP sortantes | Fait |
| **Sécurité** | Scan de vulnérabilités Docker (Trivy) en CI | Fait |
| **Monitoring** | Endpoint Prometheus `/metrics` (latence, erreurs, stats DB) | Fait |
| **Testing** | Tests E2E navigateur — Chromium, Firefox, WebKit, Mobile (Playwright) | Fait |
| **Testing** | Audit d'accessibilité (axe-core) intégré aux tests E2E | Fait |
| **CI/CD** | Tests E2E dans GitHub Actions avec matrice multi-navigateurs | Fait |
| **CI/CD** | Webhook de déploiement auto sur push vers develop | Fait |

---

## Crédits

Ce projet a été conçu avec l'assistance d'une Intelligence Artificielle pour la structure et l'optimisation du code. Le code final est purement applicatif et n'utilise aucun algorithme d'IA ou service tiers lors de son exécution. Votre cockpit reste 100% local et privé.

---

## Licence

Distribué sous licence **MIT**.
