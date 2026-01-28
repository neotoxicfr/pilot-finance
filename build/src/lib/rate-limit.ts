import logger from './logger';

interface RateLimitEntry {
  count: number;
  resetTime: number;
}

interface RateLimitConfig {
  maxAttempts: number;
  windowMs: number;
}

const DEFAULT_CONFIG: RateLimitConfig = {
  maxAttempts: 5,
  windowMs: 15 * 60 * 1000,
};

const rateLimitStore = new Map<string, RateLimitEntry>();

const CLEANUP_INTERVAL = 60 * 1000;
let cleanupTimer: NodeJS.Timeout | null = null;

function startCleanup() {
  if (cleanupTimer) return;
  cleanupTimer = setInterval(() => {
    const now = Date.now();
    for (const [key, entry] of rateLimitStore) {
      if (now > entry.resetTime) {
        rateLimitStore.delete(key);
      }
    }
  }, CLEANUP_INTERVAL);
  cleanupTimer.unref();
}

export function checkRateLimit(
  identifier: string,
  action: string,
  config: Partial<RateLimitConfig> = {}
): { allowed: boolean; remaining: number; retryAfterMs: number } {
  startCleanup();

  const { maxAttempts, windowMs } = { ...DEFAULT_CONFIG, ...config };
  const key = `${action}:${identifier}`;
  const now = Date.now();

  const entry = rateLimitStore.get(key);

  if (!entry || now > entry.resetTime) {
    rateLimitStore.set(key, { count: 1, resetTime: now + windowMs });
    return { allowed: true, remaining: maxAttempts - 1, retryAfterMs: 0 };
  }

  if (entry.count >= maxAttempts) {
    const retryAfterMs = entry.resetTime - now;
    logger.warn({ identifier, action, retryAfterMs }, 'Rate limit exceeded');
    return { allowed: false, remaining: 0, retryAfterMs };
  }

  entry.count++;
  return { allowed: true, remaining: maxAttempts - entry.count, retryAfterMs: 0 };
}

export function resetRateLimit(identifier: string, action: string): void {
  const key = `${action}:${identifier}`;
  rateLimitStore.delete(key);
}

export const RATE_LIMIT_CONFIGS = {
  forgotPassword: { maxAttempts: 3, windowMs: 60 * 60 * 1000 },
  register: { maxAttempts: 5, windowMs: 60 * 60 * 1000 },
  login: { maxAttempts: 10, windowMs: 15 * 60 * 1000 },
  verifyEmail: { maxAttempts: 5, windowMs: 15 * 60 * 1000 },
} as const;
