#!/usr/bin/env node
/**
 * Script de test de compatibilité crypto Node.js <-> Go
 * Génère des données chiffrées que Go doit pouvoir déchiffrer
 */

const crypto = require('crypto');

// Clés de test (identiques dans crypto_test.go)
const ENCRYPTION_KEY = Buffer.from('0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef', 'hex');
const BLIND_INDEX_KEY = Buffer.from('fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210', 'hex');

function encrypt(text) {
  const iv = crypto.randomBytes(12);
  const cipher = crypto.createCipheriv('aes-256-gcm', ENCRYPTION_KEY, iv);
  let encrypted = cipher.update(text, 'utf8', 'hex');
  encrypted += cipher.final('hex');
  const authTag = cipher.getAuthTag().toString('hex');
  return `${iv.toString('hex')}:${authTag}:${encrypted}`;
}

function decrypt(text) {
  if (!text.includes(':')) return text;
  const [ivHex, authTagHex, encryptedHex] = text.split(':');
  const decipher = crypto.createDecipheriv('aes-256-gcm', ENCRYPTION_KEY, Buffer.from(ivHex, 'hex'));
  decipher.setAuthTag(Buffer.from(authTagHex, 'hex'));
  let decrypted = decipher.update(encryptedHex, 'hex', 'utf8');
  decrypted += decipher.final('utf8');
  return decrypted;
}

function computeBlindIndex(input) {
  return crypto.createHmac('sha256', BLIND_INDEX_KEY).update(input).digest('hex');
}

function hashToken(token) {
  return crypto.createHash('sha256').update(token).digest('hex');
}

// Tests
console.log('=== Tests de compatibilité Crypto Node.js -> Go ===\n');

const testCases = [
  'Hello, World!',
  'test@example.com',
  'Données sensibles avec accénts',
  '12345.67',
  '',
];

console.log('--- Encrypt/Decrypt ---');
testCases.forEach(text => {
  const encrypted = encrypt(text);
  const decrypted = decrypt(encrypted);
  console.log(`Original: "${text}"`);
  console.log(`Encrypted: ${encrypted}`);
  console.log(`Decrypted: "${decrypted}"`);
  console.log(`Match: ${text === decrypted ? '✓' : '✗'}\n`);
});

console.log('--- Blind Index ---');
testCases.filter(t => t).forEach(text => {
  const index = computeBlindIndex(text);
  console.log(`Input: "${text}"`);
  console.log(`Index: ${index}\n`);
});

console.log('--- Hash Token ---');
const token = 'abc123xyz';
console.log(`Token: "${token}"`);
console.log(`Hash: ${hashToken(token)}`);

console.log('\n=== Valeurs pour tests Go ===');
console.log('Copier ces valeurs dans crypto_test.go pour validation:\n');

// Générer une valeur chiffrée fixe pour le test Go
const fixedIV = Buffer.from('000102030405060708090a0b', 'hex');
const fixedCipher = crypto.createCipheriv('aes-256-gcm', ENCRYPTION_KEY, fixedIV);
let fixedEncrypted = fixedCipher.update('test@example.com', 'utf8', 'hex');
fixedEncrypted += fixedCipher.final('hex');
const fixedTag = fixedCipher.getAuthTag().toString('hex');
const fixedResult = `${fixedIV.toString('hex')}:${fixedTag}:${fixedEncrypted}`;

console.log(`Encrypted "test@example.com" with fixed IV:`);
console.log(`  ${fixedResult}`);
console.log(`\nBlind index "test@example.com":`);
console.log(`  ${computeBlindIndex('test@example.com')}`);
console.log(`\nHash token "abc123xyz":`);
console.log(`  ${hashToken('abc123xyz')}`);
