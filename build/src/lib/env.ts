export function getEnv(key: string, minLength: number = 0): string {
  const value = process.env[key];

  if (!value && (process.env.npm_lifecycle_event === 'build' || process.env.NEXT_PHASE === 'phase-production-build')) {
    return '0000000000000000000000000000000000000000000000000000000000000000';
  }

  if (!value) {
    throw new Error(`Missing environment variable: ${key}`);
  }
  if (value.length < minLength && process.env.npm_lifecycle_event !== 'build') {
    throw new Error(`Variable ${key} is too short (min ${minLength} chars required).`);
  }
  return value;
}

export const ENV = {
  AUTH_SECRET: getEnv('AUTH_SECRET', 32),
  ENCRYPTION_KEY: getEnv('ENCRYPTION_KEY', 64), // 32 bytes = 64 hex chars
  BLIND_INDEX_KEY: getEnv('BLIND_INDEX_KEY', 64),
  HOST: process.env.HOST || 'localhost',
};