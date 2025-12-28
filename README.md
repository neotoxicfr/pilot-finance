# ✈️ Pilot Finance

**Pilot Finance** est un cockpit financier personnel conçu pour l'auto-hébergement. Une application simple et sécurisée pour suivre votre patrimoine net, vos rendements et vos opérations récurrentes en toute confidentialité.

![Logo](build/public/logo.svg)

---

## ✨ Fonctionnalités

* 💰 **Suivi de patrimoine** : Visualisez l'évolution globale de vos actifs.
* 📈 **Simulation de rendements** : Gérez vos intérêts composés et projetez vos gains sur plusieurs années.
* 🔄 **Opérations récurrentes** : Automatisez le suivi de vos revenus et dépenses mensuelles.
* 🔐 **Sécurité avancée** : 
    * Chiffrement des données sensibles (mail, noms de comptes, transactions) en base de données.
    * Support natif des **Passkeys** (WebAuthn) pour une connexion sans mot de passe.
    * Double authentification (2FA/TOTP).
* 📧 **Gestion des Emails** (Optionnel) : Validation des comptes à l'inscription et récupération de mot de passe.
* 📱 **Interface Responsive** : Expérience fluide sur tous les supports (mobile, tablette et ordinateur).

---

## 🚀 Installation avec Docker

La méthode recommandée est d'utiliser **Docker Compose**.

### 1. Prérequis
* Un nom de domaine (indispensable pour les Passkeys et la validation SSL).
* Un reverse-proxy déjà configuré (Traefik, Nginx Proxy Manager, Cloudflare Tunnel, etc.).

### 2. Configuration (`docker-compose.yml`)

Créez un fichier `docker-compose.yml` dans votre dossier de travail :

```yaml
services:
  pilot:
    image: ghcr.io/neotoxicfr/pilot-finance:latest
    container_name: pilot
    restart: unless-stopped
    environment:
      - TZ=Europe/Paris
      - HOST=pilot.votre-domaine.tld # Votre domaine sans https (ex: pilot.exemple.com)
      - ALLOW_REGISTER=true          # Mettre à false après votre inscription initiale
      - ENABLE_MAIL=false            # Passer à true pour activer les emails (SMTP requis)
      - SMTP_HOST=
      - SMTP_PORT=587
      - SMTP_USER=
      - SMTP_PASS=
      - SMTP_FROM=
      - DATABASE_URL=file:/data/pilot.db
      - ENCRYPTION_KEY=             # Générez une clé : openssl rand -hex 32
      - BLIND_INDEX_KEY=            # Générez une clé : openssl rand -hex 32
      - AUTH_SECRET=                # Générez une clé : openssl rand -hex 32
    volumes:
      - ./data:/data
    healthcheck:
      test: ["CMD", "wget", "--no-verbose", "--tries=1", "--spider", "[http://127.0.0.1:3000/login](http://127.0.0.1:3000/login)"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 15s
```
### 3. Démarrage

Lancez le conteneur avec la commande suivante :
```bash
docker compose up -d
```
L'application écoute sur le port **3000** à l'intérieur du conteneur.

---

## 🛠️ Variables d'environnement

| Variable | Description |
| :--- | :--- |
| **HOST** | Votre nom de domaine complet sans le protocole (ex: `pilot.exemple.com`). Indispensable pour les Passkeys et les liens de mail. |
| **ENCRYPTION_KEY** | Clé de 32 octets utilisée pour le chiffrement AES des données sensibles en base de données. Générée via `openssl rand -hex 32`. |
| **BLIND_INDEX_KEY** | Clé secrète servant à générer des index de recherche hachés pour vos emails sans les stocker en clair. |
| **AUTH_SECRET** | Clé de sécurité pour la gestion des sessions d'authentification NextAuth. |
| **ENABLE_MAIL** | Active la sécurité SMTP au démarrage et les fonctions de validation d'email / mot de passe oublié. |
| **ALLOW_REGISTER** | Permet ou bloque la création de nouveaux comptes. Il est conseillé de la passer à `false` après votre inscription. |
| **DATABASE_URL** | Chemin vers votre base de données SQLite (ex: `file:/data/pilot.db`). |
| **TZ** | Fuseau horaire du conteneur (ex: `Europe/Paris`) pour la précision des dates d'opérations. |

---

## 🛡️ Sécurité et Confidentialité

Pilot Finance a été construit avec la sécurité par défaut :

* **Zéro stockage en clair** : Les noms de comptes et libellés de transactions sont chiffrés. Seul votre serveur avec sa clé unique peut les lire.
* **Vérification au démarrage** : Le système refuse de démarrer si `ENABLE_MAIL` est actif mais que la configuration SMTP est incomplète, évitant les erreurs silencieuses.
* **Protection Passkeys** : L'utilisation des Passkeys offre une protection robuste contre le phishing et élimine le besoin de mémoriser des mots de passe complexes.

---

## 🤖 Crédits & Conception

Ce projet a été conçu avec l'assistance d'une Intelligence Artificielle pour la structure et l'optimisation du code. Toutefois, **le code final est purement applicatif** et n'utilise aucun algorithme d'IA ou service tiers de traitement de données lors de son exécution. Votre cockpit reste 100% local et privé.

---

## 📝 Licence

Ce projet est distribué sous licence **MIT**.