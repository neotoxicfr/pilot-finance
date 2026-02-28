package db

import (
	"log/slog"
	"time"
)

// Constantes d'action pour l'audit log
const (
	AuditLoginSuccess  = "LOGIN_SUCCESS"
	AuditLoginFail     = "LOGIN_FAIL"
	AuditLogout        = "LOGOUT"
	AuditPasswordChange = "PASSWORD_CHANGE"
	AuditMFAEnable     = "MFA_ENABLE"
	AuditMFADisable    = "MFA_DISABLE"
	AuditPasskeyAdd    = "PASSKEY_ADD"
	AuditPasskeyRemove = "PASSKEY_REMOVE"
	AuditAccountCreate = "ACCOUNT_CREATE"
	AuditAccountUpdate = "ACCOUNT_UPDATE"
	AuditAccountDelete = "ACCOUNT_DELETE"
	AuditGDPRExport    = "GDPR_EXPORT"
	AuditGDPRDelete    = "GDPR_DELETE"
)

// AuditEntry représente une entrée dans le journal d'audit
type AuditEntry struct {
	ID        int64
	UserID    int64
	Action    string
	IP        string
	UserAgent string
	CreatedAt time.Time
}

// LogAudit enregistre une action dans le journal d'audit (fire-and-forget).
func LogAudit(userID int64, action, ip, userAgent string) {
	if _, err := DB.Exec(`INSERT INTO audit_log (user_id, action, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		userID, action, ip, userAgent, time.Now().Unix()); err != nil {
		slog.Warn("audit log insert failed", "action", action, "userID", userID, "err", err)
	}
}

// GetAuditLogByUserID retourne toutes les entrées d'audit d'un utilisateur (export GDPR).
func GetAuditLogByUserID(userID int64) ([]AuditEntry, error) {
	rows, err := DB.Query(`
		SELECT id, user_id, action, ip, user_agent, created_at
		FROM audit_log WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var ts int64
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.IP, &e.UserAgent, &ts); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(ts, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetAuditLog retourne les entrées d'audit paginées, les plus récentes en premier.
func GetAuditLog(page, limit int) ([]AuditEntry, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * limit

	rows, err := DB.Query(`
		SELECT id, user_id, action, ip, user_agent, created_at
		FROM audit_log
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var createdAt int64
		if err := rows.Scan(&e.ID, &e.UserID, &e.Action, &e.IP, &e.UserAgent, &createdAt); err != nil {
			return nil, err
		}
		e.CreatedAt = time.Unix(createdAt, 0)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CountAuditLog retourne le nombre total d'entrées dans le journal d'audit.
func CountAuditLog() (int, error) {
	var count int
	err := DB.QueryRow(`SELECT COUNT(*) FROM audit_log`).Scan(&count)
	return count, err
}
