# 🔬 Pilot Finance v2.10.0 — Quality Pass, Centimes Migration & Live Updates

Major release — **50 files changed**, **1 843 additions**, **721 deletions**.

---

Release majeure — **50 fichiers modifies**, **1 843 ajouts**, **721 suppressions**.

## 🔒 Security / Securite

- **CSRF always active** — No longer skipped when `HOST` env is unset; falls back to request `Host` header
- **Metrics endpoint protected** — `/metrics` now requires authentication + admin role
- **MFA disable re-auth** — Disabling 2FA requires entering current password
- **Session cache** — `sync.Map`-based cache (30s TTL) reduces DB queries on authenticated requests
- **Client IP hardening** — Prefers `RemoteAddr` over spoofable `X-Forwarded-For` / `X-Real-IP`
- **Password Unicode** — Character count uses rune length instead of byte length
- **Audit anonymization** — User deletion anonymizes audit logs instead of deleting them

---

- **CSRF toujours actif** — Plus ignore quand `HOST` n'est pas defini ; fallback sur le header `Host` de la requete
- **Endpoint metrics protege** — `/metrics` requiert desormais authentification + role admin
- **Re-auth pour desactiver MFA** — Desactiver la 2FA exige le mot de passe actuel
- **Cache de session** — Cache `sync.Map` (TTL 30s) reduit les requetes DB sur les requetes authentifiees
- **Durcissement IP client** — Prefere `RemoteAddr` aux headers `X-Forwarded-For` / `X-Real-IP` falsifiables
- **Mot de passe Unicode** — Le comptage de caracteres utilise la longueur en runes au lieu des octets
- **Anonymisation audit** — La suppression d'un utilisateur anonymise les logs d'audit au lieu de les supprimer

## 💰 Centimes Migration

- **int64 centimes** — All monetary values (`Balance`, `Amount`) migrated from `float64` to `int64` centimes for perfect precision
- **Zero-downtime migration** — No database migration needed; `DecryptCents` auto-detects legacy float format and converts on read
- **Template type-switching** — All money functions (`formatMoney`, `formatBalance`, etc.) accept both `int64` (centimes) and `float64` (direct)

---

- **Centimes int64** — Toutes les valeurs monetaires (`Balance`, `Amount`) migrees de `float64` vers `int64` centimes pour une precision parfaite
- **Migration sans interruption** — Aucune migration de base de donnees necessaire ; `DecryptCents` detecte automatiquement l'ancien format float et convertit a la lecture
- **Type-switching templates** — Toutes les fonctions monetaires (`formatMoney`, `formatBalance`, etc.) acceptent `int64` (centimes) et `float64` (direct)

## 🎨 UI/UX

- **Live summary card** — Income/expenses/net card updates instantly on every account or recurring operation change (HTMX OOB swap)
- **Edit form fix** — Account and recurring edit forms correctly display euros instead of raw centimes
- **Button contrast** — Primary buttons upgraded to WCAG AA compliance on white backgrounds
- **Reduced motion** — `prefers-reduced-motion` disables all CSS transitions
- **Passkey toasts** — Blocking `alert()` calls replaced with non-blocking toast notifications
- **Consent checkbox** — Registration form includes consent checkbox with privacy policy link
- **Language detection** — `Accept-Language` header sets user language automatically at registration

---

- **Summary card en direct** — La carte revenus/depenses/solde se met a jour instantanement a chaque modification de compte ou d'operation recurrente (HTMX OOB swap)
- **Correction formulaires d'edition** — Les formulaires de modification de compte et d'operation affichent correctement les euros au lieu des centimes bruts
- **Contraste boutons** — Boutons principaux mis en conformite WCAG AA sur fond blanc
- **Mouvement reduit** — `prefers-reduced-motion` desactive toutes les transitions CSS
- **Toasts passkey** — Les `alert()` bloquants remplaces par des notifications toast non-bloquantes
- **Checkbox de consentement** — Le formulaire d'inscription inclut une case de consentement avec lien vers la politique de confidentialite
- **Detection de langue** — Le header `Accept-Language` definit automatiquement la langue a l'inscription

## ⚡ Performance

- **Crypto init once** — AES cipher block pre-computed once at startup
- **DB init once** — Prevents double-init and goroutine leaks on concurrent calls
- **Connection pool** — `MaxOpenConns=10`, `MaxIdleConns=10`, `ConnMaxLifetime=1h`
- **Parallel audit decrypt** — Bulk audit log decryption parallelized (8 goroutines)

---

- **Crypto init unique** — Bloc chiffrement AES pre-calcule une seule fois au demarrage
- **DB init unique** — Empeche la double-initialisation et les fuites de goroutines
- **Pool de connexions** — `MaxOpenConns=10`, `MaxIdleConns=10`, `ConnMaxLifetime=1h`
- **Dechiffrement audit parallele** — Dechiffrement en masse des logs d'audit parallelise (8 goroutines)

## ⚖️ Legal / Juridique

- **Privacy policy rewritten** — Updated for simulation/projection tool context (not banking)
- **Cookie section** — New section describing session cookie usage
- **Contact section** — Added contact information for data inquiries
- **Legal notices** — Updated date and simulation tool framing

---

- **Politique de confidentialite reecrite** — Mise a jour pour le contexte d'outil de simulation/projection (pas bancaire)
- **Section cookies** — Nouvelle section decrivant l'utilisation des cookies de session
- **Section contact** — Ajout des informations de contact pour les demandes de donnees
- **Mentions legales** — Date et cadrage outil de simulation mis a jour

## ⚙️ CI/CD

- **Trivy scanning** — Container image vulnerability scanning added to pipeline
- **Dependabot** — Configured for Go modules and GitHub Actions with grouped updates
- **Lighthouse fix** — Disabled simulated throttling, increased timeouts for CI stability
- **E2E fix** — `pwdMixin` spread operator replaced with `Object.defineProperties` for proper getter transfer

---

- **Scan Trivy** — Scan de vulnerabilites de l'image conteneur ajoute au pipeline
- **Dependabot** — Configure pour les modules Go et GitHub Actions avec mises a jour groupees
- **Correctif Lighthouse** — Throttling simule desactive, timeouts augmentes pour la stabilite CI
- **Correctif E2E** — Operateur spread de `pwdMixin` remplace par `Object.defineProperties` pour un transfert correct des getters

## 🧪 Tests / Qualite

- **100% coverage maintained** — All 13 packages pass with 0 failures
- **New test suites** — `templates_test.go` (type-switching), `auth_internal_test.go` (session cache)
- **ResetForTest** — Added to `db` and `crypto` packages for clean test state with `sync.Once`

---

- **100% de couverture maintenue** — Les 13 packages passent avec 0 echecs
- **Nouvelles suites de tests** — `templates_test.go` (type-switching), `auth_internal_test.go` (cache session)
- **ResetForTest** — Ajoute aux packages `db` et `crypto` pour un etat de test propre avec `sync.Once`

## 📝 Documentation

- **README rewritten** — Removed completed roadmap (14 items), reorganized features into Core/Security/Quality/Design sections
- **SECURITY.md** — Updated supported version to v2.10.x

---

- **README reecrit** — Roadmap completee supprimee (14 items), fonctionnalites reorganisees en sections Core/Security/Quality/Design
- **SECURITY.md** — Version supportee mise a jour vers v2.10.x

## 🔗 Docker image / Image Docker

```bash
docker pull ghcr.io/neotoxicfr/pilot-finance:v2.10.0
# ou / or
docker pull ghcr.io/neotoxicfr/pilot-finance:latest
```

**Full Changelog**: https://github.com/neotoxicfr/pilot-finance/compare/v2.9.1...v2.10.0

---

🤖 Published with [Claude Code](https://claude.com/claude-code)
