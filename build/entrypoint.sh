#!/bin/sh

# 1. Mise à jour schéma
npx drizzle-kit push --force > /dev/null 2>&1

# 2. Sécurité SMTP
if [ "$ENABLE_MAIL" = "true" ]; then
    echo "🔍 Vérification de la configuration SMTP..."
    if [ -z "$SMTP_HOST" ] || [ -z "$SMTP_USER" ] || [ -z "$SMTP_PASS" ] || [ -z "$HOST" ]; then
        echo "❌ ERREUR FATALE : ENABLE_MAIL est à true mais les variables SMTP ou HOST sont absentes !"
        exit 1
    fi
    echo "📧 Service mail validé."
fi

echo "✅ Pilot Finance est prêt."

# 3. Lancement serveur
exec node server.js