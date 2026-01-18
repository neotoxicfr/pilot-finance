import { createCipheriv, createDecipheriv, createHmac, randomBytes, createHash } from 'crypto';
import { ENV } from './env';
const ALGORITHM = 'aes-256-gcm';
const IV_LENGTH = 12;
const SECRET_KEY = Buffer.from(ENV.ENCRYPTION_KEY, 'hex').length === 32 
    ? Buffer.from(ENV.ENCRYPTION_KEY, 'hex') 
    : createHash('sha256').update(ENV.ENCRYPTION_KEY).digest();
const BLIND_INDEX_KEY = Buffer.from(ENV.BLIND_INDEX_KEY, 'hex').length >= 32
    ? Buffer.from(ENV.BLIND_INDEX_KEY, 'hex')
    : createHash('sha256').update(ENV.BLIND_INDEX_KEY).digest();
export function encrypt(text: string): string {
  const iv = randomBytes(IV_LENGTH);
  const cipher = createCipheriv(ALGORITHM, SECRET_KEY, iv);
  let encrypted = cipher.update(text, 'utf8', 'hex');
  encrypted += cipher.final('hex');
  const authTag = cipher.getAuthTag().toString('hex');
  return `${iv.toString('hex')}:${authTag}:${encrypted}`;
}
export function decrypt(text: string): string {
  if (!text.includes(':')) return text; 
  const [ivHex, authTagHex, encryptedHex] = text.split(':');
  if (!ivHex || !authTagHex || !encryptedHex) return text;
  try {
    const decipher = createDecipheriv(ALGORITHM, SECRET_KEY, Buffer.from(ivHex, 'hex'));
    decipher.setAuthTag(Buffer.from(authTagHex, 'hex'));
    let decrypted = decipher.update(encryptedHex, 'hex', 'utf8');
    decrypted += decipher.final('utf8');
    return decrypted;
  } catch (error) {
    console.error("Decryption failed:", error);
    return text;
  }
}
export function computeBlindIndex(input: string): string {
  return createHmac('sha256', BLIND_INDEX_KEY).update(input).digest('hex');
}
export function hashToken(token: string): string {
  return createHash('sha256').update(token).digest('hex');
}