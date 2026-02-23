# Pilot Finance v2.0 - Go + HTMX
# Image finale: ~20MB

# ============ CSS BUILD STAGE ============
FROM node:25-alpine AS css

WORKDIR /app

# Installer Tailwind CSS v3
RUN npm install -g tailwindcss@3

# Copier les fichiers nécessaires pour la compilation CSS
COPY tailwind.config.js .
COPY static/css/input.css static/css/
COPY templates templates/

# Compiler Tailwind CSS
RUN npx tailwindcss -i static/css/input.css -o static/css/tailwind.css --minify -c tailwind.config.js

# ============ GO BUILD STAGE ============
FROM golang:1.26-alpine AS builder

ARG TARGETARCH
ARG VERSION=dev

WORKDIR /app

# Installer les dépendances de build
RUN apk add --no-cache git

# Copier les fichiers de dépendances en premier (cache layer)
COPY go.mod go.sum ./
RUN go mod download

# Copier le reste du code source
COPY . .

# Copier le CSS compilé depuis le stage CSS
COPY --from=css /app/static/css/tailwind.css static/css/tailwind.css

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=${TARGETARCH} go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /server ./cmd/server

# ============ PRODUCTION STAGE ============
FROM alpine:3.23

# Certificats SSL et timezone
RUN apk add --no-cache ca-certificates tzdata

# Créer un utilisateur non-root
RUN adduser -D -g '' appuser

WORKDIR /app

# Copier le binaire
COPY --from=builder /server /app/server

# Copier les fichiers statiques (avec tailwind.css compilé) et templates
COPY --from=builder /app/static /app/static
COPY templates /app/templates

# Permissions
RUN chown -R appuser:appuser /app

USER appuser

# Port
EXPOSE 3000

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:3000/api/health || exit 1

# Démarrage
CMD ["/app/server"]
