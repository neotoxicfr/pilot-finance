# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 1.3.x   | :white_check_mark: |
| 1.2.x   | :white_check_mark: (security fixes only) |
| < 1.2   | :x:                |

## Security Features

Pilot Finance implements multiple layers of security:

### Authentication & Authorization
- **Password Hashing**: Argon2id (memory-hard, GPU/ASIC resistant)
- **Session Management**: JWT with `__Host-` cookie prefix, HttpOnly, Secure, SameSite=Lax
- **Multi-Factor Authentication**: TOTP (RFC 6238) with encrypted secret storage
- **Passkeys/WebAuthn**: FIDO2 passwordless authentication support
- **Brute-Force Protection**: Account lockout after 5 failed attempts (15 min)

### Data Protection
- **Encryption at Rest**: AES-256-GCM for sensitive data (emails, descriptions)
- **Database Encryption**: SQLCipher with AES-256 (optional)
- **Blind Index**: HMAC-SHA256 for searchable encrypted fields
- **Secure Deletion**: SQLite `secure_delete` pragma enabled

### Network Security
- **Content Security Policy**: Strict CSP with nonces
- **Rate Limiting**: Per-IP and per-action limits
- **HTTPS Only**: Enforced via reverse proxy

### Infrastructure
- **Non-root Container**: Runs as unprivileged user (UID 1001)
- **Read-only Filesystem**: Supported via Docker `--read-only`
- **Health Checks**: Built-in endpoint for monitoring

## Reporting a Vulnerability

We take security vulnerabilities seriously. If you discover a security issue, please report it responsibly.

### Preferred Method
**GitHub Private Vulnerability Reporting**
1. Go to the [Security tab](https://github.com/neotoxicfr/pilot-finance/security)
2. Click "Report a vulnerability"
3. Fill in the details

### Alternative Method
If you cannot use GitHub's reporting:
- Email: Create an issue requesting secure contact

### What to Include
- Description of the vulnerability
- Steps to reproduce
- Potential impact assessment
- Suggested fix (if any)

### Response Timeline
| Stage | Timeframe |
|-------|-----------|
| Initial acknowledgment | 48 hours |
| Preliminary assessment | 5 business days |
| Fix development | Depends on severity |
| Public disclosure | 90 days (coordinated) |

### Severity Classification

| Severity | Description | Example |
|----------|-------------|---------|
| **Critical** | Remote code execution, auth bypass | SQL injection, JWT secret leak |
| **High** | Data exposure, privilege escalation | Broken access control |
| **Medium** | Limited data exposure, DoS | Rate limit bypass |
| **Low** | Information disclosure | Version exposure |

## Security Best Practices for Deployment

### Environment Variables
```bash
# Generate strong secrets (minimum 32 characters)
AUTH_SECRET=$(openssl rand -hex 32)
ENCRYPTION_KEY=$(openssl rand -hex 32)

# Optional: Database encryption
DB_ENCRYPTION_KEY=$(openssl rand -hex 32)
```

### Docker Deployment
```bash
docker run -d \
  --read-only \
  --cap-drop ALL \
  --security-opt no-new-privileges:true \
  -v pilot-data:/data:rw \
  ghcr.io/neotoxicfr/pilot-finance:latest
```

### Reverse Proxy (Traefik recommended)
- Enable HSTS with preload
- Configure rate limiting
- Use CrowdSec for threat detection
- Restrict to trusted IPs if possible

## Acknowledgments

We thank all security researchers who responsibly disclose vulnerabilities.

## Updates

This security policy was last updated on **January 2025**.
