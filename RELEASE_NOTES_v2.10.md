# 🔬 Pilot Finance v2.10.0 — Quality Pass, Centimes Migration & Live Updates

## 💰 Centimes Migration

- **int64 centimes** — All monetary values (`Balance`, `Amount`) migrated from `float64` to `int64` centimes for perfect precision
- **Zero-downtime migration** — No database migration needed; `DecryptCents` auto-detects legacy float format and converts on read
- **Template type-switching** — All money functions (`formatMoney`, `formatBalance`, etc.) accept both `int64` (centimes) and `float64` (direct)

---

- **Centimes int64** — Toutes les valeurs monétaires (`Balance`, `Amount`) migrées de `float64` vers `int64` centimes pour une précision parfaite
- **Migration sans interruption** — Aucune migration de base de données nécessaire ; `DecryptCents` détecte automatiquement l'ancien format float et convertit à la lecture
- **Type-switching templates** — Toutes les fonctions monétaires (`formatMoney`, `formatBalance`, etc.) acceptent `int64` (centimes) et `float64` (direct)

## 🔒 Security / Sécurité

- **CSRF always active** — No longer skipped when `HOST` env is unset; falls back to request `Host` header
- **Metrics endpoint protected** — `/metrics` now requires authentication + admin role
- **MFA disable re-auth** — Disabling 2FA requires entering current password
- **Session cache** — 30s TTL cache reduces DB queries on authenticated requests
- **Client IP hardening** — Prefers `RemoteAddr` over spoofable `X-Forwarded-For` / `X-Real-IP`
- **Password Unicode** — Character count uses rune length instead of byte length
- **Audit anonymization** — User deletion anonymizes audit logs instead of deleting them

---

- **CSRF toujours actif** — Plus ignoré quand `HOST` n'est pas défini ; fallback sur le header `Host` de la requête
- **Endpoint metrics protégé** — `/metrics` requiert désormais authentification + rôle admin
- **Re-auth pour désactiver MFA** — Désactiver la 2FA exige le mot de passe actuel
- **Cache de session** — Cache avec TTL 30s réduit les requêtes DB sur les requêtes authentifiées
- **Durcissement IP client** — Préfère `RemoteAddr` aux headers `X-Forwarded-For` / `X-Real-IP` falsifiables
- **Mot de passe Unicode** — Le comptage de caractères utilise la longueur en runes au lieu des octets
- **Anonymisation audit** — La suppression d'un utilisateur anonymise les logs d'audit au lieu de les supprimer

## 🎨 UI/UX

- **Live summary card** — Income/expenses/net card updates instantly on every account or recurring operation change (HTMX OOB swap)
- **Edit form fix** — Account and recurring edit forms correctly display euros instead of raw centimes
- **Button contrast** — Primary buttons upgraded to WCAG AA compliance on white backgrounds
- **Reduced motion** — `prefers-reduced-motion` disables all CSS transitions
- **Passkey toasts** — Blocking `alert()` calls replaced with non-blocking toast notifications
- **Consent checkbox** — Registration form includes consent checkbox with privacy policy link
- **Language detection** — `Accept-Language` header sets user language automatically at registration

---

- **Summary card en direct** — La carte revenus/dépenses/solde se met à jour instantanément à chaque modification de compte ou d'opération récurrente (HTMX OOB swap)
- **Correction formulaires d'édition** — Les formulaires de modification de compte et d'opération affichent correctement les euros au lieu des centimes bruts
- **Contraste boutons** — Boutons principaux mis en conformité WCAG AA sur fond blanc
- **Mouvement réduit** — `prefers-reduced-motion` désactive toutes les transitions CSS
- **Toasts passkey** — Les `alert()` bloquants remplacés par des notifications toast non-bloquantes
- **Checkbox de consentement** — Le formulaire d'inscription inclut une case de consentement avec lien vers la politique de confidentialité
- **Détection de langue** — Le header `Accept-Language` définit automatiquement la langue à l'inscription

## ⚡ Performance

- **Crypto init once** — AES cipher block pre-computed once at startup
- **DB init once** — Prevents double-init and goroutine leaks on concurrent calls
- **Connection pool** — `MaxOpenConns=10`, `MaxIdleConns=10`, `ConnMaxLifetime=1h`
- **Parallel audit decrypt** — Bulk audit log decryption parallelized (8 goroutines)

---

- **Crypto init unique** — Bloc chiffrement AES pré-calculé une seule fois au démarrage
- **DB init unique** — Empêche la double-initialisation et les fuites de goroutines
- **Pool de connexions** — `MaxOpenConns=10`, `MaxIdleConns=10`, `ConnMaxLifetime=1h`
- **Déchiffrement audit parallèle** — Déchiffrement en masse des logs d'audit parallélisé (8 goroutines)

## ⚖️ Legal / Juridique

- **Privacy policy rewritten** — Updated for simulation/projection tool context (not banking)
- **Cookie section** — New section describing session cookie usage
- **Legal notices** — Updated date, contact information and simulation tool framing

---

- **Politique de confidentialité réécrite** — Mise à jour pour le contexte d'outil de simulation/projection (pas bancaire)
- **Section cookies** — Nouvelle section décrivant l'utilisation des cookies de session
- **Mentions légales** — Date, informations de contact et cadrage outil de simulation mis à jour

## ⚙️ CI/CD

- **Trivy scanning** — Container image vulnerability scanning added to pipeline
- **Dependabot** — Configured for Go modules and GitHub Actions with grouped updates
- **Lighthouse fix** — Disabled simulated throttling, increased timeouts for CI stability
- **E2E fix** — `pwdMixin` spread operator replaced with `Object.defineProperties` for proper getter transfer
- **100% coverage maintained** — All 13 packages pass with 0 failures

---

- **Scan Trivy** — Scan de vulnérabilités de l'image conteneur ajouté au pipeline
- **Dependabot** — Configuré pour les modules Go et GitHub Actions avec mises à jour groupées
- **Correctif Lighthouse** — Throttling simulé désactivé, timeouts augmentés pour la stabilité CI
- **Correctif E2E** — Opérateur spread de `pwdMixin` remplacé par `Object.defineProperties` pour un transfert correct des getters
- **100% de couverture maintenue** — Les 13 packages passent avec 0 échecs

## 📝 Documentation

- **README rewritten** — Removed completed roadmap (14 items), reorganized features into Core/Security/Quality/Design sections
- **SECURITY.md** — Updated supported version to v2.10.x

---

- **README réécrit** — Roadmap complétée supprimée (14 items), fonctionnalités réorganisées en sections Core/Sécurité/Qualité/Design
- **SECURITY.md** — Version supportée mise à jour vers v2.10.x

## 🔗 Docker image / Image Docker

```bash
docker pull ghcr.io/neotoxicfr/pilot-finance:v2.10.0
# ou / or
docker pull ghcr.io/neotoxicfr/pilot-finance:latest
```

**Full Changelog**: https://github.com/neotoxicfr/pilot-finance/compare/v2.9.1...v2.10.0

---

🤖 Published with [Claude Code](https://claude.com/claude-code)
