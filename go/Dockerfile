# Pilot Finance - Go + HTMX
# Image finale: ~8MB (scratch + UPX)

# ============ CSS BUILD STAGE ============
FROM node:26-alpine AS css

WORKDIR /app

# Chaîne d'outils d'assets verrouillée (audit FIN-5) :
#  - versions exactes déclarées dans package.json (aucune plage) ;
#  - arbre transitif figé par package-lock.json → `npm ci` ;
#  - `--ignore-scripts` : aucun script d'installation tiers n'est exécuté
#    (esbuild résout son binaire via optionalDependencies, pas via postinstall).
# Le build échoue si le lockfile manque ou diverge de package.json : c'est le
# seul chemin par lequel du code tiers atteint les assets servis en production.
# Voir CONTRIBUTING.md § "Asset toolchain lockfile".
COPY package.json package-lock.json ./
RUN npm ci --ignore-scripts --no-audit --no-fund

# Copier les fichiers nécessaires pour la compilation CSS
COPY static/css/input.css static/css/
COPY static/css/app.css static/css/
COPY static/js/charts.js static/js/
COPY static/js/passkey.js static/js/
COPY templates templates/

# Compiler Tailwind CSS + minifier les assets custom
RUN ./node_modules/.bin/tailwindcss -i static/css/input.css -o static/css/tailwind.css --minify \
    && ./node_modules/.bin/esbuild static/css/app.css --minify --outfile=static/css/app.css --allow-overwrite \
    && ./node_modules/.bin/esbuild static/js/charts.js --minify --outfile=static/js/charts.js --allow-overwrite \
    && ./node_modules/.bin/esbuild static/js/passkey.js --minify --outfile=static/js/passkey.js --allow-overwrite

# ============ GO BUILD STAGE ============
FROM golang:1.27-alpine AS builder

ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app

# Installer les dépendances de build + UPX pour compression binaire
RUN apk add --no-cache git upx

# Copier les fichiers de dépendances en premier (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copier le reste du code source
COPY . .

# Copier les assets compilés depuis le stage CSS — DOIT rester APRÈS `COPY . .`,
# sinon les sources non minifiées réécrasent le résultat du stage css (audit S-38).
COPY --from=css /app/static/css/tailwind.css static/css/tailwind.css
COPY --from=css /app/static/css/app.css static/css/app.css
COPY --from=css /app/static/js/charts.js static/js/charts.js
COPY --from=css /app/static/js/passkey.js static/js/passkey.js

# Build + compress
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -trimpath \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /server ./cmd/server \
    && upx --best --lzma /server

# Certificats CA, timezone et compte non privilégié (scratch n'a pas d'outil
# de création d'utilisateur : /etc/passwd et /etc/group sont fabriqués ici).
FROM alpine:3.24 AS certs
RUN apk add --no-cache ca-certificates tzdata \
    && addgroup -g 65532 -S nonroot \
    && adduser -u 65532 -S -D -H -G nonroot -s /sbin/nologin nonroot

# ============ PRODUCTION STAGE ============
FROM scratch

# Certificats SSL et timezone (copiés depuis alpine)
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo

# Comptes système (pour que l'uid 65532 ait un nom résolvable)
COPY --from=certs /etc/passwd /etc/passwd
COPY --from=certs /etc/group /etc/group

# Copier le binaire et les assets
COPY --from=builder /server /app/server
COPY --from=builder /app/static /app/static
COPY --from=builder /app/templates /app/templates
COPY --from=builder /app/locales /app/locales

WORKDIR /app

# Port
EXPOSE 3000

# Health check via le binaire intégré (pas de wget dans scratch)
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/app/server", "healthcheck"]

# Exécution non privilégiée (audit S-34). Forme numérique obligatoire : sans
# NSS, l'uid doit être littéral pour que le runtime puisse l'appliquer.
# Le volume de données doit appartenir à cet uid : `chown -R 65532:65532 ./data`
# (voir README.md § Upgrading from v2.23.0 or earlier).
USER 65532:65532

# Démarrage
ENTRYPOINT ["/app/server"]
