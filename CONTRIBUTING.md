# Contributing to Pilot Finance

Thank you for considering contributing to Pilot Finance! This guide will help you get started.

## Development Setup

### Prerequisites

- **Go 1.26+**
- **Node.js 25+** (for Tailwind CSS compilation and E2E tests)
- **SQLite** (bundled via `modernc.org/sqlite`, no system install needed)

### Clone and Build

```bash
git clone https://github.com/neotoxicfr/pilot-finance.git
cd pilot-finance/go

# Generate encryption keys for local dev
export AUTH_SECRET=$(openssl rand -hex 32)
export ENCRYPTION_KEY=$(openssl rand -hex 32)
export BLIND_INDEX_KEY=$(openssl rand -hex 32)
export ALLOW_REGISTER=true

# Build and run
go build -o server ./cmd/server
./server
```

The server starts on `http://localhost:3000`.

### Running Tests

```bash
# All tests
cd go
go test -timeout 600s ./...

# With race detector (CI mode)
go test -race -timeout 600s ./...

# Coverage (100% enforced)
go test -race -timeout 600s -coverprofile=coverage.out $(go list ./... | grep -v -E '/cmd/|/db$')

# Benchmarks
go test -bench=. -benchmem ./internal/projection/

# Lint
golangci-lint run
```

### E2E Tests

```bash
cd e2e
npm ci
npx playwright install --with-deps

# Run (server must be running on :3000)
npx playwright test

# With UI
npx playwright test --ui
```

### Load Tests

```bash
# Install k6: https://k6.io/docs/get-started/installation/
k6 run loadtest/smoke.js
k6 run loadtest/stress.js
```

## Commit Convention

This project uses [Conventional Commits](https://www.conventionalcommits.org/) for clear commit history and release notes.

### Format

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

### Types

| Type | Description |
|------|-------------|
| `feat` | New feature |
| `fix` | Bug fix |
| `docs` | Documentation only |
| `style` | Formatting, missing semicolons, etc. |
| `refactor` | Code change that neither fixes a bug nor adds a feature |
| `perf` | Performance improvement |
| `test` | Adding or updating tests |
| `build` | Build system or external dependencies |
| `ci` | CI configuration |
| `chore` | Other changes that don't modify src or test files |

### Examples

```
feat(accounts): add drag-and-drop reordering
fix(auth): prevent session fixation on password change
perf(projection): optimize monthly granularity calculation
test(e2e): add accessibility audit with axe-core
docs: update installation guide with quick start
ci: add Lighthouse performance audit workflow
```

### Breaking Changes

Prefix the body with `BREAKING CHANGE:` or append `!` after the type:

```
feat(api)!: change health endpoint response format

BREAKING CHANGE: /api/health now returns JSON instead of plain text
```

## Code Guidelines

### Go

- **100% test coverage** is enforced in CI (excluding `cmd/server` and `db`)
- Use the **DI hooks pattern** (`hooks.go`) for testability — no mock libraries
- Errors: use `serverError(w, "context", err)` for 500 responses
- Encryption: all sensitive fields must use `crypto.Encrypt()` / `crypto.Decrypt()`
- Linting: code must pass `golangci-lint` (errcheck, govet, staticcheck, unused, ineffassign, gosimple)

### Frontend

- **HTMX** for server-driven interactions — partials must pass `Currency` and `T` explicitly
- **Alpine.js CSP mode** — register with `Alpine.data('name', fn)`, use `x-data="name"` (no parentheses)
- **Tailwind CSS v4** — no `bg-background/90` (no RGB channel syntax)
- **Modals** — use native `<dialog>` + `showModal()`, close via Alpine method
- **i18n** — `{{index .T "key"}}`, inside range: `{{index $.T "key"}}`
- **No CDN** — all JS loaded locally, per-page (not globally bundled)

### Security

- Never store plaintext sensitive data
- All forms require CSRF validation (Origin + Referer)
- CSP nonces are required on all `<script>` tags
- Rate limiting applies to all auth endpoints

## Pull Request Process

1. **Branch from `develop`** — never commit directly to `main`
2. **Write tests** — maintain 100% coverage
3. **Follow commit convention** — keeps history clean
4. **CI must pass** — Go tests, lint, E2E, Lighthouse
5. **Keep PRs focused** — one feature or fix per PR

## Architecture Overview

```
go/cmd/server/main.go            → Entrypoint, routes, middleware chain
go/internal/handlers/             → HTTP handlers (pages, API, auth)
go/internal/handlers/hooks.go     → DI function variables for testing
go/internal/db/                   → SQLite operations (WAL mode)
go/internal/middleware/            → Auth JWT, nonce, CSRF, request ID
go/internal/crypto/               → AES-256-GCM, HMAC blind index, bcrypt
go/internal/projection/           → Financial projections (3 scenarios)
go/internal/i18n/                 → fr.json + en.json translations
go/internal/mail/                 → SMTP TLS/STARTTLS, i18n email templates
go/internal/metrics/              → Prometheus counters and histograms
go/internal/auth/                 → JWT generation, session versioning
go/internal/config/               → Environment configuration loading
go/internal/ratelimit/            → Multi-level rate limiting (IP, user, global)
go/internal/templates/            → Template loading and caching
go/templates/                     → Go HTML templates (layouts, pages, components)
e2e/                              → Playwright E2E tests
loadtest/                         → k6 load test scripts
```

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

---

# Contribuer à Pilot Finance

Merci de votre intérêt ! Voici la marche à suivre :

1. **Ouvrir une Issue d'abord** — Discutez du changement souhaité avant de coder.
2. **Pull Request** — Soumettez vos modifications sur une branche séparée depuis `develop`.
3. **Convention de commit** — Utilisez les [Conventional Commits](https://www.conventionalcommits.org/fr/) (`feat:`, `fix:`, `docs:`, etc.)
4. **Tests** — Maintenez la couverture à 100%. La CI échouera sinon.
5. **Standards de code** — Respectez les patterns existants (handlers chi, partials HTMX, Alpine.js CSP).
6. **Sécurité** — Ne commitez jamais de fichiers `.env`, de bases de données ou de clés de chiffrement.
