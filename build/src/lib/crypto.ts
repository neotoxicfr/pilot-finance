import { createCipheriv, createDecipheriv, createHmac, randomBytes, createHash } from 'crypto';
import { ENV } from './env';
import logger from './logger';

const ALGORITHM = 'aes-256-gcm';
const IV_LENGTH = 12;

const SECRET_KEY = Buffer.from(ENV.ENCRYPTION_KEY, 'hex');
const BLIND_INDEX_KEY = Buffer.from(ENV.BLIND_INDEX_KEY, 'hex');

if (SECRET_KEY.length !== 32) {
    throw new Error(`CRITICAL: ENCRYPTION_KEY invalid length. Got ${SECRET_KEY.length} bytes, expected 32.`);
}

if (BLIND_INDEX_KEY.length !== 32) {
    throw new Error(`CRITICAL: BLIND_INDEX_KEY invalid length. Got ${BLIND_INDEX_KEY.length} bytes, expected 32.`);
}

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
    logger.error({ err: error }, "Decryption failed");
    return text;
  }
}

export function computeBlindIndex(input: string): string {
  return createHmac('sha256', BLIND_INDEX_KEY).update(input).digest('hex');
}

export function hashToken(token: string): string {
  return createHash('sha256').update(token).digest('hex');
}