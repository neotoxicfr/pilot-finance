# Roadmap — Pilot Finance

## v2.5.0 — Sécurité, UX, Accessibilité, Perf (sans refactor)

### Sécurité
- [x] ~~CSP `default-src 'none'` + `strict-dynamic` + CORP~~ (v2.4.2)
- [ ] `MaxBytesReader` — limite taille body HTTP à 1MB (anti-DoS)
- [ ] Email retiré des claims JWT → UserID uniquement
- [ ] CSRF : rejeter si Origin ET Referer tous deux absents
- [ ] `ValidateOrigin` appliqué sur `/logout`
- [ ] `toJSON` dans templates : `template.JS` → `json.Marshal` + échappement

### UX / Fonctionnel
- [ ] Validation password alignée client/serveur (vérifier seuil exact)
- [ ] `hx-confirm` sur suppression de compte bancaire
- [ ] Renommage passkeys : `prompt()` → modal
- [ ] Erreurs auth retournées en HTML partiel HTMX (plus `http.Error()`)
- [ ] État vide dashboard avec CTA "Créer mon premier compte"

### Accessibilité (WCAG 2.2)
- [ ] Supprimer `user-scalable=no` / `maximum-scale=1.0` du viewport
- [ ] `aria-label` sur tous les boutons icône-seuls (thème, settings, logout, delete, move)
- [ ] `<label>` explicites sur tous les inputs des formulaires
- [ ] Skip-link `<a href="#main">` en début de body
- [ ] Audit contraste `text-muted-foreground` (Lighthouse / axe-core)

### Performance
- [ ] `MaxOpenConns(1)` → 4 en mode WAL
- [ ] Chart.js chargé conditionnellement si `len(Accounts) > 0`
- [ ] Goroutine leak rate limiter → `context.Context` ou `chan` stop

### CI / Infra
- [ ] Workflow CI `.github/workflows/test.yml` — `go test -race ./...`

### Conformité
- [ ] Page `/privacy` — politique de confidentialité

---

## v2.6.0 — Refactoring majeur, RGPD, Tests

### Sécurité
- [ ] Chiffrer `balance`, `amount`, `yield_min`, `yield_max`, `position` en BDD (AES-256-GCM)
  - Migration colonnes `REAL` → `TEXT` hex
  - Tri/agrégation déplacés en Go
  - Migration des données existantes au déploiement

### Performance
- [ ] Déchiffrement parallèle via `errgroup` (après chiffrement des soldes)

### Infra
- [ ] Migrations versionnées (table `migrations(version, applied_at)`)
- [ ] Backup SQLite automatisé (`VACUUM INTO` + volume séparé ou S3)

### RGPD
- [ ] Export données utilisateur `GET /settings/export` (JSON/CSV)
- [ ] Suppression de compte `DELETE /settings/account` avec confirmation mot de passe
- [ ] Log d'audit `audit_log(user_id, action, ip, timestamp)`

### Tests
- [ ] Tests handlers auth (login, register, 2FA)
- [ ] Tests middleware (RequireAuth, ValidateOrigin)
- [ ] Tests DB CRUD — objectif couverture >50% → >70%
