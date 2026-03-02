# Pilot Finance

## Projet
Application web self-hosted de gestion financiere personnelle. Suivi patrimoine, yields, operations recurrentes.

## Stack
- **Go 1.26** — chi v5, jwt/v5, modernc/sqlite, x/crypto, x/sync
- **Frontend** : HTMX 2.0.8, Alpine.js CSP 3.15.3, Chart.js 4.5.1, Tailwind CSS v4
- **Zero CDN** — tout local, JS charge par page
- **Docker** : multi-stage (node:25-alpine > Go builder > alpine:3.23), user non-root

## Architecture
```
go/cmd/server/main.go            entrypoint, CSP headers, routes, HSTS, graceful shutdown
go/internal/handlers/             pages, accounts, recurring, settings, auth, passkey, mfa
go/internal/handlers/hooks.go     pattern DI (variables de fonction package-level)
go/internal/handlers/helpers.go   serverError(), cookies, decryptAccountNames
go/internal/handlers/errcodes.go  12 codes d'erreur structures (clientError, jsonError, jsonSuccess)
go/internal/db/                   sqlite WAL, accounts, users, audit, authenticators
go/internal/middleware/            auth JWT, nonce, CSRF, request ID
go/internal/crypto/               AES-256-GCM, HMAC blind index, bcrypt cost 12
go/internal/projection/           3 scenarios, granularite mensuelle si years<=2
go/internal/i18n/                 fr.json + en.json (245 cles)
go/templates/                     layouts/base.html, pages/, components/
```

## Migrations (000-010)
| # | Nom | Description |
|---|-----|-------------|
| 000 | base_schema | Tables users, accounts, recurring_operations, authenticators |
| 001 | backup_eligible | Colonne backup_eligible sur authenticators |
| 002 | user_language | Colonne language sur users |
| 003 | user_currency | Colonne currency sur users |
| 004 | yield_frequency | Colonne yield_frequency sur accounts |
| 005 | payout_frequency | Colonne payout_frequency sur accounts |
| 006 | indexes | Index sur accounts, recurring, authenticators, users |
| 007 | audit_log | Table audit_log |
| 008 | encrypt_account_fields | Chiffrement in-place balance/yields/reinvestment (avec backup) |
| 009 | encrypt_recurring_amount | Chiffrement in-place montants recurrents |
| 010 | audit_log_indexes | Index sur audit_log(user_id, created_at) |

## Securite
- CSP strict (nonces dynamiques, pas de unsafe-eval ni unsafe-inline pour scripts)
- `style-src 'unsafe-inline'` requis par Tailwind CSS v4 (documente dans le code)
- AES-256-GCM sur tous les champs sensibles (emails, soldes, montants, taux)
- bcrypt cost 12, Passkeys WebAuthn, 2FA TOTP
- CSRF Origin+Referer, rate limiting multi-niveau, session versioning
- HSTS (aussi present dans le reverse proxy)

## Tests
- **100% couverture** sur les 10 packages mesures (cmd/server et db exclus)
- Seuil 100% enforce dans CI (le build echoue en dessous)
- CI : `go test -race -timeout 600s -coverprofile=coverage.out $(go list ./... | grep -v -E '/cmd/|/db$')`
- Pattern DI hooks pour testabilite sans mock lib
- Benchmarks : `go test -bench=. ./internal/projection/`
- Linting : golangci-lint (errcheck, govet, staticcheck, unused, ineffassign, gosimple)
- govulncheck pour vulnerabilites

### Pattern de test handlers
```go
cleanup := setupHandlerTest(t) // init crypto, jwt, db temp, i18n, templates
defer cleanup()
uid := newUser(t, "email@test.com", "ValidP@ss1!", "USER")
req := injectUser(post("/path", url.Values{...}), &middleware.User{ID: uid, ...})
rr := httptest.NewRecorder()
HandlerFunc(rr, req)
```

## CI/CD
- **ci.yml** : build + test race + couverture 100% + golangci-lint + govulncheck
- **release.yml** : tag v* > tests > GitHub Release avec notes auto
- **docker-publish.yml** : build et push image Docker
- **codeql.yml** : analyse securite statique
- **dependabot.yml** : Go modules + Docker + GitHub Actions (hebdomadaire)
- **lefthook.yml** : pre-commit (lint, vet, build)

## Commandes courantes
```bash
# Tests (tous les packages)
cd go
go test -timeout 600s ./...

# Tests avec couverture CI (exclusions)
make coverage

# Benchmarks projection
go test -bench=. -benchmem ./internal/projection/

# Lint
golangci-lint run

# Build local
go build -o server ./cmd/server

# Docker
docker build -t pilot-finance go/
```

## Regles ABSOLUES
1. **Ne JAMAIS downgrader node** — toujours latest (node:25-alpine ou plus recent)
2. **Ne JAMAIS commiter sur main** — declenche le build Docker GitHub
3. **Commandes Bash separees** — jamais chainees avec &&
4. **git push AVANT le deploy** — le serveur fait git pull

## Deploy
- Branche de dev : develop
- SSH : `root@neotoxic.net` dossier `/mnt/docker/pilot`
- Sequence : build > git add > commit > push > ssh deploy
- Co-Authored-By dans les commits
- **Worktree** : toujours merger dans develop + supprimer la branche remote temporaire avant de finir

## Conventions
- HTMX partials : toujours passer "Currency" et "T" explicitement
- Alpine CSP : Alpine.data('name', fn), x-data="name" sans parentheses
- Modals : <dialog> natif + showModal(), fermeture via methode Alpine
- Tailwind v4 : pas de bg-background/90 (pas de canaux RGB)
- i18n : {{index .T "key"}}, dans range : {{index $.T "key"}}
- Erreurs 500 : utiliser `serverError(w, "contexte", err)` pour logger + repondre
- account-row : passer dict "Account" . "Currency" $.Currency "T" $.T

## Preferences utilisateur
- Reponses concises
- Commit + deploy apres chaque changement sans demander
- L'utilisateur n'est pas developpeur — Claude ecrit tout le code
