# Pilot Finance v2.0 - Go + HTMX
# Image finale: ~20MB

# ============ BUILD STAGE ============
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Installer les dépendances de build
RUN apk add --no-cache git

# Copier les fichiers de dépendances
COPY go.mod go.sum ./
RUN go mod download

# Copier le code source
COPY . .

# Build optimisé
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-s -w -X main.Version=2.0.0" \
    -o /server ./cmd/server

# ============ PRODUCTION STAGE ============
FROM alpine:3.21

# Certificats SSL et timezone
RUN apk add --no-cache ca-certificates tzdata

# Créer un utilisateur non-root
RUN adduser -D -g '' appuser

WORKDIR /app

# Copier le binaire
COPY --from=builder /server /app/server

# Copier les fichiers statiques et templates
COPY static /app/static
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
