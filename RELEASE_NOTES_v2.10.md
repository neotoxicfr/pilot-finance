# Release Notes -- v2.10.0

**Date**: 2026-03-06
**Branch**: `develop`
**Scope**: 50 files changed, 1843 insertions, 721 deletions (8 commits)

---

## Highlights

v2.10 is a comprehensive quality pass addressing 34 findings from an 8-agent analysis (backend architecture, frontend, DevOps, performance, QA, UX, UI, and legal compliance).

Key improvements:
- **Monetary precision**: all financial values migrated from `float64` to `int64` centimes
- **Security hardening**: CSRF always active, MFA password re-verification, session caching
- **Accessibility**: WCAG AA contrast, reduced-motion support, consent checkbox
- **Live updates**: summary card refreshes on every account and recurring operation change (OOB swap)
- **Legal alignment**: privacy/legal pages updated for simulation tool context

---

## Security

| Change | Detail |
|--------|--------|
| CSRF always active | No longer skipped when `HOST` env is unset; falls back to request `Host` header |
| `/metrics` protected | Endpoint now requires `RequireAuth` + `RequireAdmin` middleware |
| MFA disable re-auth | Disabling 2FA requires entering current password |
| Session cache | `sync.Map`-based cache (30s TTL) avoids DB query on every authenticated request |
| Client IP hardening | `getClientIP` prefers `RemoteAddr` over spoofable `X-Forwarded-For` / `X-Real-IP` |
| Password Unicode | `ValidatePassword` uses `utf8.RuneCountInString` (not `len`) for correct character count |
| Audit anonymization | User deletion anonymizes audit logs (`ip` / `user_agent` set to `"deleted-user"`) instead of deleting them |
| Decrypt warning logs | `Decrypt` now logs `slog.Warn` when falling back to plaintext (detects unencrypted legacy data) |

## Performance

| Change | Impact |
|--------|--------|
| `crypto.Init` sync.Once | Pre-computes AES cipher block once; eliminates redundant `aes.NewCipher` per call |
| `db.Init` sync.Once | Prevents double-init and goroutine leaks on concurrent calls |
| DB connection pool | `MaxOpenConns=10`, `MaxIdleConns=10`, `ConnMaxLifetime=1h` |
| Parallel audit decrypt | `errgroup` + semaphore (8 goroutines) for bulk audit log decryption |

## Data Model

### int64 centimes migration

All monetary values (`Account.Balance`, `RecurringOperation.Amount`) changed from `float64` to `int64` (centimes).

- **Storage**: values stored as centimes (e.g., `5000.50` EUR becomes `500050`)
- **Encryption**: new `EncryptCents` / `DecryptCents` functions
- **Backward compatibility**: `DecryptCents` detects legacy float-format encrypted strings (containing `.`) and auto-converts
- **HTML forms**: `parseCents` helper converts user input (float string) to centimes; edit forms convert centimes back to euros (`balance / 100`) before display
- **Templates**: all money functions (`formatMoney`, `formatBalance`, `formatFloat`, `mult`, `add`, `sub`, `ge`, `gt`, `abs`) accept `interface{}` with type-switching:
  - `int64` = centimes (divided by 100)
  - `float64` = direct value
  - `int` = direct value
- **Projection engine**: converts cents to float64 at calculation boundaries only
- **Yields** (`YieldMin`, `YieldMax`): remain `float64` (percentages, not monetary)

### Migration path

No database migration needed. The encrypted values in SQLite are strings. `DecryptCents` auto-detects the format:
- New format: `"500050"` (integer string) -> parsed as `int64`
- Legacy format: `"5000.50"` (float string) -> parsed as float, multiplied by 100, rounded

## Frontend / UX

| Change | Detail |
|--------|--------|
| Summary card OOB | Summary card (income/expenses/net) updates live on every account and recurring operation change via `hx-swap-oob` |
| Edit form centimes fix | Account and recurring edit forms correctly convert centimes to euros before display (`/ 100`) |
| Button contrast | Primary buttons: `#3b82f6` -> `#2563eb` (passes WCAG AA on white) |
| Reduced motion | `prefers-reduced-motion: reduce` disables CSS transitions |
| Password mixin | `pwdMixin()` uses `Object.defineProperties` to safely transfer getter descriptors without invoking them |
| Passkey toasts | `alert()` calls replaced with non-blocking `showPasskeyToast()` notifications |
| Consent checkbox | Registration form includes a consent checkbox (links to privacy policy) |
| Language detection | `Accept-Language` header used to set user language at registration |

## Legal / Privacy

| Change | Detail |
|--------|--------|
| Privacy policy | Rewritten for simulation/projection tool context (not banking) |
| Cookies section | New section describing session cookie usage |
| Contact section | Added contact information for data inquiries |
| Legal notices | Updated date and simulation tool framing |

## CI/CD

| Change | Detail |
|--------|--------|
| Trivy scanning | Container image vulnerability scanning added to CI pipeline |
| Dependabot | Configured for Go modules (`go/`) and GitHub Actions (`.github/workflows/`) |
| Lighthouse | Disabled simulated throttling (`throttlingMethod: "provided"`), increased FCP/load timeout to 60s, added `--disable-software-rasterizer` |
| E2E | Fixed `pwdMixin` spread operator bug -- replaced with `Object.defineProperties` for proper getter transfer |

## Tests

- All **13 packages pass** with 0 failures
- `ResetForTest()` added to `db` package (mirrors `crypto.ResetForTest()`)
- All test setup functions call `ResetForTest()` before `Init` to handle `sync.Once`
- `mustInit(t)` helper in crypto tests for clean state
- Nil `cipherBlock` guard prevents panic in `Decrypt`
- New `templates_test.go` with `toFloat64` and format function coverage
- New `auth_internal_test.go` for session cache middleware
- Updated tests for: CSRF always-active, MFA password re-auth, RemoteAddr-first IP, int64 centimes

## Documentation

- README (EN + FR) rewritten: removed completed roadmap, reorganized features into Core/Security/Quality/Design sections, added Required column to env vars table
- SECURITY.md updated for v2.10.x
- CONTRIBUTING.md already up to date

## Commits

```
8ecca41 fix: update summary card OOB when recurring operations change
f7e700e fix: convert centimes to euros in account/recurring edit forms
9f7cb96 docs: rewrite README — remove completed roadmap, reorganize features
4832cdb docs: prepare v2.10.0 release notes and update security policy
e3132c8 fix(lighthouse): disable simulated throttling and increase FCP timeout
d72f6f4 fix(e2e): use Object.defineProperties for pwdMixin instead of spread
1fe941f fix(ci): restore 100% test coverage and fix E2E consent checkbox
0b76141 feat: v2.10 comprehensive quality pass — security, performance, UX, legal
```

## Files changed (50)

```
.github/dependabot.yml                       +8
.github/workflows/ci.yml                     +16
.github/workflows/lighthouse.yml             +8/-3
.lighthouserc.json                           +11/-1
README.fr.md                                 +110/-111
README.md                                    +108/-109
RELEASE_NOTES_v2.10.md                       +157
SECURITY.md                                  +6/-6
e2e/tests/helpers.ts                         +2
go/cmd/server/main.go                        +10/-5
go/internal/crypto/crypto.go                 +101/-40
go/internal/crypto/crypto_test.go            +260/-90
go/internal/db/accounts.go                   +20/-15
go/internal/db/accounts_test.go              +60/-50
go/internal/db/audit.go                      +53/-10
go/internal/db/models.go                     +4/-4
go/internal/db/sqlite.go                     +29/-10
go/internal/db/testhelper_test.go            +2
go/internal/db/users.go                      +16/-6
go/internal/handlers/accounts.go             +31/-10
go/internal/handlers/auth.go                 +49/-10
go/internal/handlers/coverage2_test.go       +6/-6
go/internal/handlers/coverage3_test.go       +6/-6
go/internal/handlers/coverage_test.go        +183/-4
go/internal/handlers/extra_test.go           +20/-10
go/internal/handlers/handlers_test.go        +2
go/internal/handlers/mfa.go                  +26/-5
go/internal/handlers/pages.go                +9/-5
go/internal/handlers/recurring.go            +41/-4
go/internal/middleware/auth.go               +57/-10
go/internal/middleware/auth_internal_test.go  +60
go/internal/middleware/auth_test.go          +2
go/internal/middleware/csrf.go               +37/-10
go/internal/middleware/nonce_csrf_test.go     +85/-10
go/internal/projection/projection.go         +24/-15
go/internal/projection/projection_test.go    +70/-50
go/internal/templates/templates.go           +97/-50
go/internal/templates/templates_test.go      +80
go/locales/en.json                           +39/-15
go/locales/fr.json                           +239/-130
go/static/css/app.css                        +19/-5
go/static/js/charts.js                       +15
go/static/js/passkey.js                      +44/-10
go/templates/layouts/base.html               +25
go/templates/pages/accounts.html             +4/-4
go/templates/pages/legal.html                +3/-2
go/templates/pages/login.html                +29/-20
go/templates/pages/privacy.html              +13/-8
go/templates/pages/reset-password.html       +21/-15
go/templates/pages/settings.html             +21/-15
```

## Breaking changes

- **API**: `CreateAccountWithYield`, `UpdateAccountWithYield`, `UpdateAccountBalance`, `CreateRecurring`, `UpdateRecurring` -- `balance`/`amount` parameter type changed from `float64` to `int64` (centimes)
- **Templates**: money-related template functions now accept `interface{}` instead of `float64`
- **CSRF**: POST requests without `Origin` or `Referer` headers are now **always rejected** (even when `HOST` env is not set)
- **MFA**: `POST /api/mfa/disable` now requires `current_password` form field
