import argon2 from 'argon2';
import bcrypt from 'bcryptjs';
import logger from './logger';

const ARGON2_OPTIONS: argon2.Options = {
  type: argon2.argon2id,
  memoryCost: 65536,
  timeCost: 3,
  parallelism: 4,
  saltLength: 16,
};

const BCRYPT_PREFIX = '$2a$';
const ARGON2_PREFIX = '$argon2';

function isArgon2Hash(hash: string): boolean {
  return hash.startsWith(ARGON2_PREFIX);
}

function isBcryptHash(hash: string): boolean {
  return hash.startsWith(BCRYPT_PREFIX) || hash.startsWith('$2b$');
}

export async function hashPassword(password: string): Promise<string> {
  return await argon2.hash(password, ARGON2_OPTIONS);
}

export async function verifyPassword(
  password: string,
  hash: string
): Promise<{ valid: boolean; needsRehash: boolean }> {
  try {
    if (isArgon2Hash(hash)) {
      const valid = await argon2.verify(hash, password);
      const needsRehash = valid && argon2.needsRehash(hash, ARGON2_OPTIONS);
      return { valid, needsRehash };
    }

    if (isBcryptHash(hash)) {
      const valid = await bcrypt.compare(password, hash);
      return { valid, needsRehash: valid };
    }

    logger.error({ hashPrefix: hash.substring(0, 10) }, 'Unknown password hash format');
    return { valid: false, needsRehash: false };
  } catch (error) {
    logger.error({ err: error }, 'Password verification failed');
    return { valid: false, needsRehash: false };
  }
}

export async function verifyAndRehash(
  password: string,
  currentHash: string
): Promise<{ valid: boolean; newHash: string | null }> {
  const { valid, needsRehash } = await verifyPassword(password, currentHash);

  if (!valid) {
    return { valid: false, newHash: null };
  }

  if (needsRehash) {
    const newHash = await hashPassword(password);
    logger.info('Password rehashed to Argon2id');
    return { valid: true, newHash };
  }

  return { valid: true, newHash: null };
}
