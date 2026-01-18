export function getEnv(key: string, minLength: number = 0): string {
  const value = process.env[key];
  if (!value && process.env.npm_lifecycle_event === 'build') {
    return 'build-mode-dummy-secret-key-change-me-in-prod-min-32-chars';
  }
  if (!value) {
    if (process.env.NODE_ENV === 'development') {
       console.warn(`Missing env var: ${key}`);
       return 'dev-fallback-secret-key-change-me-immediately'; 
    }
    throw new Error(`Missing environment variable: ${key}`);
  }
  if (value.length < minLength && process.env.npm_lifecycle_event !== 'build') {
    throw new Error(`Variable ${key} is too short (min ${minLength} chars required).`);
  }
  return value;
}
export const ENV = {
  AUTH_SECRET: getEnv('AUTH_SECRET', 32),
  ENCRYPTION_KEY: getEnv('ENCRYPTION_KEY', 32),
  BLIND_INDEX_KEY: getEnv('BLIND_INDEX_KEY', 32),
  HOST: process.env.HOST || 'localhost',
};