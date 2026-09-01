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
- **Rate limiting** — 120 req/min global, 10 req/min sur les routes d'authentification (voir `DISABLE_RATE_LIMIT` et `TRUSTED_PROXIES` plus bas)
- **Session versioning** — deconnexion automatique sur tous les appareils apres changement de mot de passe
- **Journal d'audit** — tracabilite complete des evenements d'authentification et de compte (vue admin)
- **Conteneur non privilegie** — execution en uid/gid 1000 sur une base `scratch` (ni shell, ni gestionnaire de paquets)
- Support **Docker Secrets** pour toutes les variables sensibles

### Qualite

- **100% de couverture de tests** appliquee en CI (hors `cmd/server` et `internal/db`, dont les tests tournent sans seuil de couverture)
- **Tests E2E** — Playwright sur Chromium, Firefox et Mobile Chrome en CI (WebKit en local)
- **Accessibilite** — audit axe-core en E2E sur chaque page, dans les **deux themes (clair et sombre)**, contraste des couleurs inclus ; support `prefers-reduced-motion`
- **Lighthouse CI** — seuil de performance a 80%
- **CodeQL** + **Trivy** (scan des dependances et scan d'image, tous deux bloquants) + **Dependabot**
- **Chaine de build verrouillee** — actions GitHub epinglees par SHA, outillage d'assets (Tailwind, esbuild) fige en versions exactes et installe avec `--ignore-scripts`
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
      # Logs JSON, et X-Forwarded-For n'est cru que venant de TRUSTED_PROXIES
      - ENV=production
      # IPs/CIDR autorises a poser X-Forwarded-For. Obligatoire si ENV=production.
      # A restreindre a votre reverse-proxy ; 172.16.0.0/12 couvre les bridges Docker.
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
      # Le premier element DOIT etre CMD / CMD-SHELL / NONE : sinon Compose
      # n'installe aucune sonde ET ecrase celle (fonctionnelle) de l'image.
      test: ["CMD", "/app/server", "healthcheck"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

L'image embarque deja un healthcheck : vous pouvez supprimer entierement le bloc `healthcheck:` si vous preferez.

### 3. Demarrer

Le conteneur tourne en uid/gid **1000** : le dossier de donnees doit lui appartenir.

```bash
mkdir -p data && sudo chown -R 1000:1000 data
docker compose up -d
```

### 4. S'inscrire et verrouiller

Ouvrez votre domaine, creez votre compte, puis passez `ALLOW_REGISTER=false` et redemarrez :

```bash
docker compose down && docker compose up -d
```

L'application ecoute sur le port **3000** a l'interieur du conteneur. Configurez votre reverse-proxy pour y rediriger le trafic.

---

## Mise a jour

Les migrations de schema s'executent automatiquement au demarrage et ne sont **pas reversibles**. Sauvegardez d'abord :

```bash
docker compose down
cp -a data data-backup-$(date +%F)
docker compose pull && docker compose up -d
docker compose logs -f pilot   # le serveur sort en code 1 plutot que servir un schema incertain
```

### Migration depuis la v2.23.0 ou anterieure

Les images precedentes tournaient en root : les fichiers de `./data` appartiennent donc a root. A partir de la prochaine version, le serveur tourne en uid/gid **1000** et ne pourra plus ouvrir la base tant que le dossier ne lui appartient pas — operation a faire une seule fois :

```bash
docker compose down
sudo chown -R 1000:1000 data
docker compose pull && docker compose up -d
```

---

## Sauvegarde et restauration

Tout se trouve dans `./data` :

| Fichier | Contenu |
| :--- | :--- |
| `pilot.db` | La base de donnees |
| `pilot.db-wal`, `pilot.db-shm` | Journal WAL — **fait partie de la base**. Ne jamais copier `pilot.db` seul depuis une instance en marche. |
| `pilot.db.backup.1` / `.2` / `.3` | Instantanes rotatifs automatiques (`.1` = le plus recent), ecrits au demarrage puis toutes les 24 h |
| `pilot.db.bak` | Instantane unique pris avant la migration de chiffrement. Sur une instance migree depuis la version Node, **il contient les montants en clair** — a supprimer une fois vos donnees verifiees. |

> **Les cles font partie de la sauvegarde.** Une base sans son `ENCRYPTION_KEY` et son `BLIND_INDEX_KEY` est un bloc illisible. Ces deux cles sont **definitives** : il n'existe aucune rotation de cle. Conservez-les a cote (et non a l'interieur) de vos sauvegardes.

### Sauvegarder

Sauvegarde a froid — la seule qui garantisse un etat coherent :

```bash
docker compose down
cp -a data data-backup-$(date +%F)
docker compose up -d
```

Sauvegarde a chaud — copiez l'instantane rotatif le plus recent. Produit par `VACUUM INTO`, c'est un fichier autonome et coherent (pas de `-wal` a joindre) :

```bash
cp data/pilot.db.backup.1 /quelque/part/pilot-$(date +%F).db
```

Vous pouvez aussi exporter vos comptes et operations en CSV depuis **Parametres → Export**.

### Restaurer

Copier un instantane par-dessus `pilot.db` puis redemarrer **est silencieusement annule** : le `-wal` residuel est rejoue au demarrage suivant, les anciennes donnees reviennent, et `integrity_check` repond quand meme `ok`. Les fichiers `-wal`/`-shm` doivent etre supprimes dans le meme geste :

```bash
docker compose down
rm -f data/pilot.db data/pilot.db-wal data/pilot.db-shm   # la suppression des -wal/-shm est obligatoire
cp /chemin/vers/pilot.db.backup.1 data/pilot.db
sudo chown 1000:1000 data/pilot.db
docker compose up -d
```

Restaurez avec les **memes** `ENCRYPTION_KEY` / `BLIND_INDEX_KEY` que ceux utilises a l'ecriture de la sauvegarde, sinon aucun montant ni email ne pourra etre dechiffre.

---

## Variables d'environnement

| Variable | Requis | Description |
| :--- | :---: | :--- |
| `HOST` | Oui | FQDN sans protocole (ex : `pilot.exemple.com`). Utilise pour les Passkeys et les liens mail. |
| `ENCRYPTION_KEY` | Oui | Cle hex de 32 octets pour le chiffrement AES. **Si perdue, les donnees chiffrees sont irrecuperables.** |
| `BLIND_INDEX_KEY` | Oui | Cle hex de 32 octets pour les index de recherche securises. |
| `AUTH_SECRET` | Oui | Cle hex de 32+ octets pour la signature des sessions JWT. |
| `DATABASE_URL` | Oui | Chemin SQLite (ex : `file:/data/pilot.db`). |
| `TRUSTED_PROXIES` | En production | IPs et/ou plages CIDR separees par des virgules, autorisees a poser `X-Forwarded-For` / `X-Real-IP` (ex : `172.16.0.0/12`). **Obligatoire quand `ENV=production` — le serveur refuse de demarrer sans.** Laissee vide hors production, `X-Forwarded-For` est accepte de n'importe quelle source (usage dev uniquement), ce qui permet a un client de falsifier son IP et de contourner les limites de debit. |
| `ENV` | Non | `production` bascule les logs en JSON **et rend `TRUSTED_PROXIES` obligatoire**. Toute autre valeur, ou l'absence de valeur, signifie developpement. |
| `ALLOW_REGISTER` | Non | `true` / `false` (defaut `false`). Passer a `false` apres l'inscription initiale. |
| `PORT` | Non | Port d'ecoute du serveur dans le conteneur (defaut : `3000`). |
| `TZ` | Non | Fuseau horaire du conteneur (ex : `Europe/Paris`). |
| `DISABLE_RATE_LIMIT` | Non | `true` **desactive tout le rate limiting** (les limites 120 req/min et 10 req/min sur l'auth). Prevu pour la suite E2E uniquement — a ne jamais poser sur une instance exposee. |
| `SMTP_HOST` | Non | Serveur SMTP. Active la verification email et la recuperation de mot de passe. |
| `SMTP_PORT` | Non | Port SMTP (defaut : 587). |
| `SMTP_USER` | Non | Identifiant SMTP. |
| `SMTP_PASS` | Non | Mot de passe SMTP. |
| `SMTP_FROM` | Non | Adresse email d'expedition. |
| `SMTP_SECURE` | Non | `true` = TLS implicite (typiquement port 465). Sinon STARTTLS (typiquement port 587). |

> **Attention a la polarite** : `ALLOW_REGISTER` et `DISABLE_RATE_LIMIT` se lisent en sens inverse. `ALLOW_REGISTER=true` **ouvre** l'inscription ; `DISABLE_RATE_LIMIT=true` **retire** une protection. Dans les deux cas, seule la chaine exacte `true` est prise en compte.

> **Cles definitives** : `ENCRYPTION_KEY` et `BLIND_INDEX_KEY` ne peuvent pas etre changees — il n'existe aucun mecanisme de version de cle. En changer une rend les donnees existantes illisibles. La seule voie de sortie est Parametres → Export, puis une base vierge avec les nouvelles cles et un reimport. `AUTH_SECRET` peut etre change librement : cela invalide seulement les sessions actives.

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
| CI/CD | GitHub Actions (tests unitaires, E2E, CodeQL, Trivy, Lighthouse, GHCR, auto-release) — la publication d'image est conditionnee a la suite de tests |
| E2E | Playwright — Chromium, Firefox, Mobile Chrome en CI ; WebKit en local |
| Docker | Image ~8 Mo (base scratch, compression UPX), execution en uid 1000 |

---

## Securite et confidentialite

- **Zero stockage en clair** — toutes les donnees sensibles chiffrees avec AES-256-GCM
- **Zero dependance externe** — pas de CDN, pas d'analytics, pas de telemetrie. Tous les assets servis localement
- **Verification au demarrage** — le serveur refuse de demarrer si les cles sont absentes ou trop courtes
- **Codes d'erreur structures** — chaque reponse d'erreur inclut un header `X-Error-Code`
- **Suppression de compte** — comptes, operations recurrentes et authentificateurs sont supprimes immediatement ; les lignes du journal d'audit sont **anonymisees** (conservees sans identite jusqu'a la purge automatique a 90 jours), et les sauvegardes rotatives en gardent une copie chiffree jusqu'a leur rotation

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
