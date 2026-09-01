package db

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"pilot-finance/internal/crypto"
)

// auditDecryptConcurrency limits parallel decryption goroutines for audit logs
const auditDecryptConcurrency = 8

// Constantes d'action pour l'audit log
const (
	AuditLoginSuccess  = "LOGIN_SUCCESS"
	AuditLoginFail     = "LOGIN_FAIL"
	AuditLogout        = "LOGOUT"
	AuditPasswordChange = "PASSWORD_CHANGE"
	AuditMFAEnable     = "MFA_ENABLE"
	AuditMFADisable    = "MFA_DISABLE"
	// Codes de récupération 2FA (audit S-22). MFA_RECOVERY_USED est
	// délibérément journalisé : la consommation d'un code contourne le
	// téléphone de l'utilisateur, elle doit rester visible dans l'audit.
	AuditMFARecoveryRegen = "MFA_RECOVERY_REGENERATE"
	AuditMFARecoveryUsed  = "MFA_RECOVERY_USED"
	AuditPasskeyAdd    = "PASSKEY_ADD"
	AuditPasskeyRemove = "PASSKEY_REMOVE"
	AuditPasskeyRename = "PASSKEY_RENAME"
	AuditAccountCreate = "ACCOUNT_CREATE"
	AuditAccountUpdate = "ACCOUNT_UPDATE"
	AuditAccountDelete = "ACCOUNT_DELETE"
	AuditImportBalances     = "IMPORT_BALANCES"
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

// auditWG suit les écritures audit en vol pour FlushAuditLog au shutdown.
var auditWG sync.WaitGroup

// auditWriteConcurrency borne le nombre de goroutines d'écriture audit afin de
// ne pas saturer le pool de 10 connexions sous une rafale d'évènements.
const auditWriteConcurrency = 8

// auditJob représente une écriture audit à effectuer par le worker pool.
type auditJob struct {
	userID    int64
	action    string
	ip        string
	userAgent string
	createdAt int64
}

// auditQueue est le canal tamponné alimentant le worker pool. Initialisé
// paresseusement par startAuditWorkers (via sync.Once) au premier LogAudit.
var (
	auditQueue chan auditJob
	auditOnce  sync.Once
)

// startAuditWorkers démarre un pool fixe de workers consommant auditQueue.
// Appelé paresseusement et une seule fois.
func startAuditWorkers() {
	auditQueue = make(chan auditJob, 256)
	for i := 0; i < auditWriteConcurrency; i++ {
		go func() {
			for job := range auditQueue {
				writeAuditEntry(job)
				auditWG.Done()
			}
		}()
	}
}

// writeAuditEntry chiffre IP/UserAgent et insère une entrée d'audit.
//
// Audit S-24 : en cas d'échec de chiffrement, ces deux champs retombaient sur
// la valeur EN CLAIR — soit exactement l'inverse du contrat affiché (« IP et
// UserAgent sont chiffrés AES-256-GCM avant stockage »), et sans aucune erreur
// visible. Le repli est désormais la chaîne VIDE : la trace d'audit
// (utilisateur, action, horodatage) est conservée, mais aucune donnée
// personnelle n'est jamais écrite en clair.
func writeAuditEntry(job auditJob) {
	ipEnc, err := crypto.Encrypt(job.ip)
	if err != nil {
		slog.Error("audit log: chiffrement ip échoué, champ vidé (jamais stocké en clair)", "err", err)
		ipEnc = ""
	}
	uaEnc, err := crypto.Encrypt(job.userAgent)
	if err != nil {
		slog.Error("audit log: chiffrement user_agent échoué, champ vidé (jamais stocké en clair)", "err", err)
		uaEnc = ""
	}
	if _, err := DB.Exec(`INSERT INTO audit_log (user_id, action, ip, user_agent, created_at) VALUES (?, ?, ?, ?, ?)`,
		job.userID, job.action, ipEnc, uaEnc, job.createdAt); err != nil {
		slog.Warn("audit log insert failed", "action", job.action, "userID", job.userID, "err", err)
	}
}

// LogAudit enregistre une action dans le journal d'audit (vraie fire-and-forget).
// M6 fix : 2 chiffrements AES + 1 INSERT exécutés hors du handler appelant pour
// ne pas le bloquer (~2-5ms gagnés par action sensible). Le travail est confié à
// un pool de workers borné (auditWriteConcurrency) afin de ne pas saturer le pool
// de connexions sous une rafale d'évènements. Le timestamp est capturé de manière
// synchrone pour préserver l'ordre logique des événements. IP et UserAgent sont
// chiffrés AES-256-GCM avant stockage.
func LogAudit(userID int64, action, ip, userAgent string) {
	auditOnce.Do(startAuditWorkers)
	createdAt := time.Now().Unix()
	auditWG.Add(1)
	auditQueue <- auditJob{
		userID:    userID,
		action:    action,
		ip:        ip,
		userAgent: userAgent,
		createdAt: createdAt,
	}
}

// FlushAuditLog attend que toutes les écritures audit en vol soient terminées.
// À appeler pendant le shutdown gracieux pour ne pas perdre d'entrées en cours.
func FlushAuditLog() {
	auditWG.Wait()
}

// GetAuditLogByUserID retourne toutes les entrées d'audit d'un utilisateur (export GDPR).
// IP et UserAgent sont déchiffrés en parallèle (semaphore-limited).
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Parallel decryption with semaphore
	decryptAuditEntries(entries)

	return entries, nil
}

// GetAuditLog retourne les entrées d'audit paginées, les plus récentes en premier.
// IP et UserAgent sont déchiffrés en parallèle (semaphore-limited).
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
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Parallel decryption with semaphore
	decryptAuditEntries(entries)

	return entries, nil
}

// decryptAuditEntries decrypts IP and UserAgent fields in parallel using a semaphore.
func decryptAuditEntries(entries []AuditEntry) {
	if len(entries) == 0 {
		return
	}

	sem := make(chan struct{}, auditDecryptConcurrency)
	var wg sync.WaitGroup

	for i := range entries {
		wg.Add(1)
		sem <- struct{}{} // acquire semaphore slot
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore slot
			if dec, err := crypto.Decrypt(entries[idx].IP); err == nil {
				entries[idx].IP = dec
			}
			if dec, err := crypto.Decrypt(entries[idx].UserAgent); err == nil {
				entries[idx].UserAgent = dec
			}
		}(i)
	}

	wg.Wait()
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

// auditRotationStopped est un hook de test (nil en production) appelé lorsque la
// goroutine de rotation se termine sur annulation du contexte. Permet aux tests
// d'observer l'arrêt propre sans dépendre d'un time.Sleep ni changer l'API.
var auditRotationStopped func()

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
		if auditRotationStopped != nil {
			defer auditRotationStopped()
		}
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
