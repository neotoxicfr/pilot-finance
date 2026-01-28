import { describe, it, expect, beforeAll } from 'vitest';
import { encrypt, decrypt, computeBlindIndex, hashToken } from './crypto';

beforeAll(() => {
    process.env.ENCRYPTION_KEY = '0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef';
    process.env.BLIND_INDEX_KEY = 'fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210';
});

describe('Crypto Module', () => {
    it('should encrypt and decrypt correctly', () => {
        const secret = "SuperSecretData";
        const encrypted = encrypt(secret);
        
        expect(encrypted).not.toBe(secret);
        expect(encrypted).toContain(':');

        const decrypted = decrypt(encrypted);
        expect(decrypted).toBe(secret);
    });

    it('should produce consistent blind indexes', () => {
        const email = "test@example.com";
        const idx1 = computeBlindIndex(email);
        const idx2 = computeBlindIndex(email);
        
        expect(idx1).toBe(idx2);
        expect(idx1).not.toBe(email);
    });

    it('should hash tokens deterministically', () => {
        const token = "my-verification-token";
        const hash1 = hashToken(token);
        const hash2 = hashToken(token);

        expect(hash1).toBe(hash2);
        expect(hash1.length).toBe(64);
    });
});