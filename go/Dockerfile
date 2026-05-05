# Pilot Finance - Go + HTMX
# Image finale: ~8MB (scratch + UPX)

# ============ CSS BUILD STAGE ============
FROM node:25-alpine AS css

WORKDIR /app

# Installer Tailwind CSS v4 (framework + CLI) + esbuild pour minification
RUN npm install -g tailwindcss@4.2.4 @tailwindcss/cli@4.2.4 esbuild@0.28.0

# Copier les fichiers nécessaires pour la compilation CSS
COPY static/css/input.css static/css/
COPY static/css/app.css static/css/
COPY static/js/charts.js static/js/
COPY static/js/passkey.js static/js/
COPY templates templates/

# Compiler Tailwind CSS + minifier les assets custom
RUN NODE_PATH=$(npm root -g) npx @tailwindcss/cli -i static/css/input.css -o static/css/tailwind.css --minify \
    && esbuild static/css/app.css --minify --outfile=static/css/app.css --allow-overwrite \
    && esbuild static/js/charts.js --minify --outfile=static/js/charts.js --allow-overwrite \
    && esbuild static/js/passkey.js --minify --outfile=static/js/passkey.js --allow-overwrite

# ============ GO BUILD STAGE ============
FROM golang:1.26.2-alpine AS builder

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

# Copier le CSS compilé depuis le stage CSS
COPY --from=css /app/static/css/tailwind.css static/css/tailwind.css

# Build + compress
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /server ./cmd/server \
    && upx --best --lzma /server

# Copier les certificats CA et timezone depuis alpine
FROM alpine:3.23 AS certs
RUN apk add --no-cache ca-certificates tzdata

# ============ PRODUCTION STAGE ============
FROM scratch

# Certificats SSL et timezone (copiés depuis alpine)
COPY --from=certs /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=certs /usr/share/zoneinfo /usr/share/zoneinfo

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

# Démarrage
ENTRYPOINT ["/app/server"]
