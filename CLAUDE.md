# Pilot Finance - Contexte pour Claude

## Description du projet

**Pilot Finance** est un cockpit financier personnel auto-hébergé. Application minimaliste pour :
- Noter les soldes de chaque compte bancaire/épargne
- Suivre la rentabilité des placements
- Gérer les opérations récurrentes (revenus/dépenses)
- Faire des projections d'évolution du patrimoine

**Ce n'est PAS** un outil de pointage de transactions bancaires - l'utilisateur prévoit ses opérations pour estimer l'évolution de ses comptes.

## Stack technique

- **Framework** : Next.js 16 (App Router, Server Actions)
- **Base de données** : SQLite avec better-sqlite3 (mode WAL)
- **CSS** : Tailwind CSS 4.1.18 (moteur Rust)
- **Auth** : Session JWT + Passkeys (WebAuthn) + 2FA TOTP
- **Chiffrement** : AES-256-GCM pour données sensibles, Argon2id pour passwords
- **Conteneurisation** : Docker multi-stage avec node:22-alpine
- **Reverse Proxy** : Traefik + Cloudflare

## Structure du projet

```
pilot-finance/
├── build/                    # Code source Next.js
│   ├── src/
│   │   ├── app/             # Pages et API routes
│   │   ├── components/      # Composants React
│   │   ├── actions/         # Server Actions
│   │   ├── lib/             # Utilitaires (auth, db, mail...)
│   │   └── db.ts            # Connexion SQLite
│   ├── public/              # Assets statiques
│   ├── middleware.ts        # Auth + CSP + Security headers
│   ├── Dockerfile
│   └── package.json
├── docker-compose.yml       # Config Docker prod
├── .claude/                  # Config Claude Code
└── README.md
```

## Branches Git

- **main** : Production stable
- **develop** : Développement actif (merger ici d'abord)

## Variables d'environnement clés

- `AUTH_SECRET` : Signature JWT (32 bytes hex)
- `ENCRYPTION_KEY` : Chiffrement AES (32 bytes hex)
- `BLIND_INDEX_KEY` : Index recherche chiffré (32 bytes hex)
- `DB_ENCRYPTION_KEY` : Optionnel, chiffrement SQLCipher complet
- `HOST` : Domaine sans https (ex: pilot.neotoxic.net)

## Conventions

- **Langue** : Documentation et commits en français
- **Commits** : Format conventionnel (feat, fix, docs, perf, chore)
- **Co-author** : Ajouter `Co-Authored-By: Claude Sonnet 4.5 <noreply@anthropic.com>`
- **Workflow Git** : Développer sur `develop`, merger vers `main`, créer tag + release

## Versioning actuel

- **Version** : 1.4.0
- **Dernières optimisations** :
  - Migration Tailwind 4 (-74% build time)
  - Lazy loading Recharts (-30% bundle)
  - Garbage collection manuel (-58% RAM)
  - Health endpoint enrichi (/api/health)
  - Fix CSP pour compatibilité Cloudflare

## Points d'attention

1. **CSP** : Utilise `'unsafe-inline'` pour compatibilité Cloudflare (pas de nonces sur scripts)
2. **Docker** : `@tailwindcss/postcss` doit être dans dependencies (pas devDependencies)
3. **Bundle analyzer** : Import conditionnel pour ne pas casser le build Docker
4. **Passkeys** : Nécessitent HTTPS et un domaine valide

## Roadmap

- [x] v1.4.0 - Tailwind 4, optimisations performance
- [ ] Support multi-langues (i18n)
- [ ] Graphiques et statistiques avancées

## Déploiement serveur

Le serveur de production est à `192.168.1.69`. Pour déployer :
```bash
ssh neo@192.168.1.69
cd /chemin/vers/pilot
git pull
docker compose build --no-cache && docker compose up -d
```

## Commandes utiles

```bash
# Dev local
cd build && npm run dev

# Build local
npm run build

# Tests
npm run test

# Analyse bundle
ANALYZE=true npm run build
```
