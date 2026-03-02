@echo off
set AUTH_SECRET=dev-secret-key-that-is-at-least-32chars
set ENCRYPTION_KEY=0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
set BLIND_INDEX_KEY=fedcba9876543210fedcba9876543210fedcba9876543210fedcba9876543210
set ALLOW_REGISTER=true
set DISABLE_RATE_LIMIT=true
go run ./cmd/server
