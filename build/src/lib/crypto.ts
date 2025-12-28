import crypto from 'crypto';

// Récupération des clés (peut être vide pendant le build)
const ENCRYPTION_KEY = process.env.ENCRYPTION_KEY || '';
const BLIND_INDEX_KEY = process.env.BLIND_INDEX_KEY || '';

// Algorithme : AES-256-GCM
const ALGO = 'aes-256-gcm';

// Fonction interne pour vérifier les clés au dernier moment
function ensureKeys() {
    if (!ENCRYPTION_KEY || !BLIND_INDEX_KEY) {
        // On ne jette l'erreur que si on essaie d'utiliser les fonctions
        throw new Error("CRITIQUE : Clés de chiffrement manquantes dans .env");
    }
}

/**
 * Chiffre un texte (réversible).
 * Format sortie : iv:authTag:content
 */
export function encrypt(text: string): string {
  if (!text) return text;
  ensureKeys(); 

  const iv = crypto.randomBytes(16);
  const cipher = crypto.createCipheriv(ALGO, Buffer.from(ENCRYPTION_KEY, 'hex'), iv);
  
  let encrypted = cipher.update(text, 'utf8', 'hex');
  encrypted += cipher.final('hex');
  const authTag = cipher.getAuthTag().toString('hex');

  return `${iv.toString('hex')}:${authTag}:${encrypted}`;
}

/**
 * Déchiffre un texte.
 */
export function decrypt(text: string): string {
  if (!text || !text.includes(':')) return text;
  ensureKeys();

  try {
      const parts = text.split(':');
      if (parts.length !== 3) return text;

      const iv = Buffer.from(parts[0], 'hex');
      const authTag = Buffer.from(parts[1], 'hex');
      const encryptedText = parts[2];

      const decipher = crypto.createDecipheriv(ALGO, Buffer.from(ENCRYPTION_KEY, 'hex'), iv);
      decipher.setAuthTag(authTag);

      let decrypted = decipher.update(encryptedText, 'hex', 'utf8');
      decrypted += decipher.final('utf8');
      return decrypted;
  } catch (error) {
      console.error("Erreur de déchiffrement:", error);
      return "[Donnée Illisible]";
  }
}

/**
 * Hash déterministe pour la recherche (Blind Index).
 */
export function computeBlindIndex(text: string): string {
  if (!text) return text;
  ensureKeys();
  
  return crypto.createHmac('sha256', Buffer.from(BLIND_INDEX_KEY, 'hex'))
    .update(text)
    .digest('hex');
}