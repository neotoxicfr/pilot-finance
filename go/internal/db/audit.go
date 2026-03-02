package db

import (
	"context"
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
	AuditGDPRExport         = "GDPR_EXPORT"
	AuditGDPRDelete         = "GDPR_DELETE"
	AuditAdminDeleteUser    = "ADMIN_DELETE_USER"
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
		SELECT id, user_id, action, COALESCE(ip, ''), COALESCE(user_agent, ''), created_at
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
		SELECT id, user_id, action, COALESCE(ip, ''), COALESCE(user_agent, ''), created_at
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

// PurgeAuditLog supprime les entrées d'audit plus anciennes que retentionDays jours.
// Retourne le nombre d'entrées supprimées.
func PurgeAuditLog(retentionDays int) (int64, error) {
	cutoff := time.Now().AddDate(0, 0, -retentionDays).Unix()
	result, err := DB.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// StartAuditRotation lance une goroutine qui purge les entrées d'audit
// plus anciennes que 90 jours, toutes les 24h.
func StartAuditRotation(ctx context.Context) {
	const retentionDays = 90

	// Purge initiale au démarrage
	if deleted, err := PurgeAuditLog(retentionDays); err != nil {
		slog.Warn("audit rotation initiale échouée", "err", err)
	} else if deleted > 0 {
		slog.Info("audit rotation initiale", "deleted", deleted)
	}

	go func() {
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if deleted, err := PurgeAuditLog(retentionDays); err != nil {
					slog.Warn("audit rotation échouée", "err", err)
				} else if deleted > 0 {
					slog.Info("audit rotation", "deleted", deleted)
				}
			}
		}
	}()
}
