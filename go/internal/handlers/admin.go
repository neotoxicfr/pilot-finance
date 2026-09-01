package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

const auditPageSize = 50

// AuditPage affiche la page d'audit log paginée (admin uniquement)
func AuditPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil || user.Role != "ADMIN" {
		clientErrorT(w, r, ErrForbidden, "error.forbidden", http.StatusForbidden)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	entries, err := hookGetAuditLog(page, auditPageSize)
	if err != nil {
		serverError(w, r, "AuditPage: GetAuditLog", err)
		return
	}
	total, err := hookCountAuditLog()
	if err != nil {
		slog.Error("AuditPage: CountAuditLog", "err", err)
	}

	// Résoudre les emails en une seule requête (évite N+1)
	allUsers, usersErr := hookGetAllUsers()
	emailCache := make(map[int64]string)
	if usersErr != nil {
		slog.Error("AuditPage: GetAllUsers", "err", usersErr)
	} else {
		for _, u := range allUsers {
			if email, err := hookDecryptStr(u.EmailEncrypted); err == nil {
				emailCache[u.ID] = email
			}
		}
	}
	for _, e := range entries {
		if emailCache[e.UserID] == "" {
			emailCache[e.UserID] = strconv.FormatInt(e.UserID, 10)
		}
	}

	data := baseData(r, user)
	t := data["T"].(map[string]string)

	type auditRow struct {
		db.AuditEntry
		Email       string
		ActionLabel string
	}
	rows := make([]auditRow, len(entries))
	for i, e := range entries {
		label := t["audit.action."+e.Action]
		if label == "" {
			label = e.Action
		}
		rows[i] = auditRow{AuditEntry: e, Email: emailCache[e.UserID], ActionLabel: label}
	}

	totalPages := (total + auditPageSize - 1) / auditPageSize

	data["Title"] = t["page.title_audit_log"]
	data["User"] = map[string]interface{}{"ID": user.ID, "Email": user.Email, "Role": user.Role, "EmailVerified": user.EmailVerified}
	data["Entries"] = rows
	data["Page"] = page
	data["TotalPages"] = totalPages
	data["Total"] = total

	hookRender(w, "admin-audit.html", data) //nolint:errcheck
}
