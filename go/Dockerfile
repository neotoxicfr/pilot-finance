# Pilot Finance - Go + HTMX
# Image finale: ~28MB

# ============ CSS BUILD STAGE ============
FROM node:25-alpine AS css

WORKDIR /app

# Installer Tailwind CSS v4 (framework + CLI)
RUN npm install -g tailwindcss@4.1.7 @tailwindcss/cli@4.1.7

# Copier les fichiers nécessaires pour la compilation CSS
COPY static/css/input.css static/css/
COPY templates templates/

# Compiler Tailwind CSS (NODE_PATH expose les globals au résolveur CSS)
RUN NODE_PATH=$(npm root -g) npx @tailwindcss/cli -i static/css/input.css -o static/css/tailwind.css --minify

# ============ GO BUILD STAGE ============
FROM golang:1.26.1-alpine AS builder

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

# ============ PRODUCTION STAGE ============
FROM alpine:3.23

# Certificats SSL et timezone
RUN apk add --no-cache ca-certificates tzdata

# Créer un utilisateur non-root + répertoire data
RUN adduser -D -g '' appuser && mkdir -p /data && chown appuser:appuser /data

WORKDIR /app

# Copier avec --chown pour éviter un layer chown séparé (économise ~18MB)
COPY --from=builder --chown=appuser:appuser /server /app/server
COPY --from=builder --chown=appuser:appuser /app/static /app/static
COPY --from=builder --chown=appuser:appuser /app/templates /app/templates
COPY --from=builder --chown=appuser:appuser /app/locales /app/locales

USER appuser

# Port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:3000/api/health || exit 1

# Démarrage
CMD ["/app/server"]
