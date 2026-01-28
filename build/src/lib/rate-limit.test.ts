import { describe, it, expect, beforeEach } from 'vitest';
import { checkRateLimit, resetRateLimit, RATE_LIMIT_CONFIGS } from './rate-limit';

describe('Rate Limit Module', () => {
  beforeEach(() => {
    resetRateLimit('test-ip', 'test-action');
  });

  it('should allow requests under the limit', () => {
    const result1 = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(result1.allowed).toBe(true);
    expect(result1.remaining).toBe(2);

    const result2 = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(result2.allowed).toBe(true);
    expect(result2.remaining).toBe(1);

    const result3 = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(result3.allowed).toBe(true);
    expect(result3.remaining).toBe(0);
  });

  it('should block requests over the limit', () => {
    for (let i = 0; i < 3; i++) {
      checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    }

    const blocked = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(blocked.allowed).toBe(false);
    expect(blocked.remaining).toBe(0);
    expect(blocked.retryAfterMs).toBeGreaterThan(0);
  });

  it('should track different identifiers separately', () => {
    for (let i = 0; i < 3; i++) {
      checkRateLimit('ip-1', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    }

    const ip1Blocked = checkRateLimit('ip-1', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(ip1Blocked.allowed).toBe(false);

    const ip2Allowed = checkRateLimit('ip-2', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(ip2Allowed.allowed).toBe(true);
  });

  it('should track different actions separately', () => {
    for (let i = 0; i < 3; i++) {
      checkRateLimit('test-ip', 'action-1', { maxAttempts: 3, windowMs: 60000 });
    }

    const action1Blocked = checkRateLimit('test-ip', 'action-1', { maxAttempts: 3, windowMs: 60000 });
    expect(action1Blocked.allowed).toBe(false);

    const action2Allowed = checkRateLimit('test-ip', 'action-2', { maxAttempts: 3, windowMs: 60000 });
    expect(action2Allowed.allowed).toBe(true);
  });

  it('should reset rate limit correctly', () => {
    for (let i = 0; i < 3; i++) {
      checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    }

    const blocked = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(blocked.allowed).toBe(false);

    resetRateLimit('test-ip', 'test-action');

    const afterReset = checkRateLimit('test-ip', 'test-action', { maxAttempts: 3, windowMs: 60000 });
    expect(afterReset.allowed).toBe(true);
    expect(afterReset.remaining).toBe(2);
  });

  it('should have correct predefined configs', () => {
    expect(RATE_LIMIT_CONFIGS.forgotPassword.maxAttempts).toBe(3);
    expect(RATE_LIMIT_CONFIGS.forgotPassword.windowMs).toBe(60 * 60 * 1000);

    expect(RATE_LIMIT_CONFIGS.register.maxAttempts).toBe(5);
    expect(RATE_LIMIT_CONFIGS.login.maxAttempts).toBe(10);
    expect(RATE_LIMIT_CONFIGS.verifyEmail.maxAttempts).toBe(5);
  });
});
