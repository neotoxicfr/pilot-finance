# Migration Pilot Finance : Next.js → Go + HTMX

## Objectif

Réécrire l'application en Go pour obtenir :
- **RAM** : ~30MB vs ~200MB actuellement
- **Bundle** : ~15KB JS vs ~250KB
- **Sécurité** : 0 dépendances npm vulnérables
- **Déploiement** : 1 binaire vs image Docker 300MB
- **Performance** : Cold start ~50ms vs ~2s

## Compatibilité données existantes

**100% compatible** - Même base de données, mêmes clés :

| Élément | Format actuel | Équivalent Go |
|---------|---------------|---------------|
| SQLite | better-sqlite3 | `modernc.org/sqlite` |
| AES-256-GCM | Node crypto | `crypto/aes` + `crypto/cipher` |
| bcrypt | bcryptjs | `golang.org/x/crypto/bcrypt` |
| HMAC-SHA256 | Node crypto | `crypto/hmac` + `crypto/sha256` |
| JWT | jose | `github.com/golang-jwt/jwt/v5` |

## Stack cible

```
Go 1.24
├── chi ou echo          # Router HTTP
├── HTMX 2.0             # Interactivité sans JS lourd
├── Alpine.js            # Réactivité légère (thème, modals)
├── Tailwind CSS 4       # Styles (compilé en build)
├── modernc.org/sqlite   # SQLite pure Go
├── templ                # Templates type-safe
└── go-webauthn          # Passkeys
```

## Structure projet

```
pilot-go/
├── cmd/
│   └── server/
│       └── main.go           # Point d'entrée
├── internal/
│   ├── auth/                 # Authentification
│   │   ├── session.go        # JWT sessions
│   │   ├── passkey.go        # WebAuthn
│   │   ├── mfa.go            # TOTP 2FA
│   │   └── middleware.go     # Auth middleware
│   ├── crypto/               # Chiffrement
│   │   ├── aes.go            # AES-256-GCM (compatible Node)
│   │   ├── blind_index.go    # HMAC-SHA256
│   │   └── bcrypt.go         # Password hashing
│   ├── db/                   # Base de données
│   │   ├── sqlite.go         # Connexion
│   │   ├── models.go         # Structures
│   │   └── queries.go        # Requêtes SQL
│   ├── handlers/             # Routes HTTP
│   │   ├── accounts.go       # CRUD comptes
│   │   ├── recurring.go      # Opérations récurrentes
│   │   ├── auth.go           # Login/Register
│   │   └── settings.go       # Paramètres
│   ├── mail/                 # Emails
│   │   └── smtp.go
│   └── ratelimit/            # Rate limiting
│       └── limiter.go
├── templates/                # Templates templ
│   ├── layouts/
│   │   └── base.templ
│   ├── pages/
│   │   ├── login.templ
│   │   ├── accounts.templ
│   │   └── settings.templ
│   └── components/
│       ├── account_card.templ
│       └── recurring_row.templ
├── static/
│   ├── css/
│   │   └── output.css        # Tailwind compilé
│   ├── js/
│   │   ├── htmx.min.js       # ~14KB
│   │   └── alpine.min.js     # ~15KB
│   └── favicon.ico
├── Dockerfile                # Multi-stage, image finale ~20MB
├── go.mod
└── go.sum
```

## Plan de migration (phases)

### Phase 1 : Fondations ✅ TERMINÉ
- [x] Setup projet Go + structure
- [x] Connexion SQLite (même schéma)
- [x] Crypto compatible (AES-256-GCM, HMAC, bcrypt)
- [x] Module JWT pour sessions
- [x] Middleware d'authentification
- [x] Configuration via variables d'environnement
- [x] Dockerfile optimisé
- [ ] Tests de compatibilité avec données réelles

### Phase 2 : Authentification
- [ ] Sessions JWT complètes
- [ ] Login/Register avec bcrypt
- [ ] Rate limiting
- [ ] Passkeys (WebAuthn)
- [ ] 2FA TOTP

### Phase 3 : Pages principales
- [ ] Layout de base (HTMX + Alpine)
- [ ] Page login
- [ ] Dashboard (patrimoine total)
- [ ] Page comptes (CRUD)
- [ ] Opérations récurrentes (CRUD)

### Phase 4 : Fonctionnalités avancées
- [ ] Rendements et projections
- [ ] Graphiques (Chart.js ou équivalent léger)
- [ ] Page settings
- [ ] Gestion admin
- [ ] Emails (SMTP)

### Phase 5 : Finalisation
- [ ] Health endpoint ✅
- [ ] Dockerfile optimisé ✅
- [ ] Tests complets
- [ ] Documentation

## Fichiers créés (Phase 1)

```
go/
├── cmd/server/main.go           # Point d'entrée + routing
├── internal/
│   ├── auth/jwt.go              # Génération/validation JWT
│   ├── config/config.go         # Chargement config env
│   ├── crypto/crypto.go         # AES-256-GCM compatible Node.js
│   ├── crypto/crypto_test.go    # Tests unitaires crypto
│   ├── db/models.go             # Structures de données
│   ├── db/sqlite.go             # Connexion + requêtes SQLite
│   ├── handlers/handlers.go     # Handlers HTTP (stubs)
│   └── middleware/auth.go       # Middleware auth + session
├── scripts/
│   └── test_crypto_compat.js    # Script test compatibilité
├── static/css/.gitkeep
├── static/js/.gitkeep
├── Dockerfile                   # Multi-stage (~20MB final)
├── go.mod
└── go.sum
```

## Points techniques critiques

### 1. Compatibilité chiffrement AES-256-GCM

Le format Node actuel :
```
IV (12 bytes) || Ciphertext || AuthTag (16 bytes)
```

Code Go équivalent :
```go
func Decrypt(ciphertext []byte, key []byte) ([]byte, error) {
    block, _ := aes.NewCipher(key)
    gcm, _ := cipher.NewGCM(block)

    nonceSize := gcm.NonceSize() // 12
    nonce, ciphertext := ciphertext[:nonceSize], ciphertext[nonceSize:]

    return gcm.Open(nil, nonce, ciphertext, nil)
}
```

### 2. Blind Index compatible

```go
func ComputeBlindIndex(data string, key []byte) string {
    h := hmac.New(sha256.New, key)
    h.Write([]byte(strings.ToLower(strings.TrimSpace(data))))
    return hex.EncodeToString(h.Sum(nil))
}
```

### 3. HTMX pour l'interactivité

Exemple : Mise à jour solde compte
```html
<form hx-post="/accounts/1/balance" hx-swap="outerHTML">
    <input name="balance" value="1500.00" />
    <button>Sauvegarder</button>
</form>
```

Le serveur renvoie le HTML mis à jour, pas de JSON.

## Variables d'environnement (identiques)

```env
HOST=pilot.exemple.com
AUTH_SECRET=...          # Même clé JWT
ENCRYPTION_KEY=...       # Même clé AES
BLIND_INDEX_KEY=...      # Même clé HMAC
DATABASE_URL=file:/data/pilot.db
ENABLE_MAIL=false
SMTP_HOST=...
```

## Dockerfile final

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY go.* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o server ./cmd/server

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
COPY --from=builder /app/server /server
COPY --from=builder /app/static /static
EXPOSE 3000
CMD ["/server"]
```

Image finale : **~20MB** (vs ~300MB actuel)

## Avantages attendus

| Métrique | Next.js actuel | Go + HTMX |
|----------|----------------|-----------|
| Image Docker | ~300MB | ~20MB |
| RAM runtime | ~200MB | ~30MB |
| Cold start | ~2s | ~50ms |
| Bundle JS | ~250KB | ~30KB |
| Vulnérabilités | 23 npm | 0 |
| Temps build | ~45s | ~5s |

## Risques et mitigations

| Risque | Mitigation |
|--------|------------|
| Incompatibilité crypto | Tests unitaires avec données réelles |
| Passkeys complexes | Librairie go-webauthn éprouvée |
| Courbe apprentissage | Go est simple, HTMX intuitif |
| Perte fonctionnalités | Migration feature par feature |

## Optimisations Go vs Node.js

Avantages supplémentaires découverts pendant l'implémentation :

1. **Garbage Collection** : Go GC plus efficace, pas besoin de `gc()` manuel
2. **Concurrence native** : Goroutines pour requêtes parallèles BDD
3. **Typage strict** : Erreurs détectées à la compilation
4. **Binaire unique** : Pas de node_modules (0 dépendances runtime)
5. **SQLite pure Go** : Pas de compilation native (CGO_ENABLED=0)

## Prochaine étape

Implémenter Phase 2 : Authentification complète avec Passkeys et 2FA.
