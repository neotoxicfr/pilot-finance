package handlers

import (
	"log/slog"
	"net/http"
	"strconv"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
	"pilot-finance/internal/templates"
)

const auditPageSize = 50

// AuditPage affiche la page d'audit log paginée (admin uniquement)
func AuditPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil || user.Role != "ADMIN" {
		http.Error(w, "Non autorisé", http.StatusForbidden)
		return
	}

	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	entries, err := hookGetAuditLog(page, auditPageSize)
	if err != nil {
		slog.Error("AuditPage: GetAuditLog", "err", err)
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	total, err := hookCountAuditLog()
	if err != nil {
		slog.Error("AuditPage: CountAuditLog", "err", err)
	}

	// Résoudre les emails par user_id (lookup unique par ID)
	emailCache := make(map[int64]string)
	for _, e := range entries {
		if _, ok := emailCache[e.UserID]; ok {
			continue
		}
		u, err := db.GetUserByID(e.UserID)
		if err == nil && u != nil {
			if email, err := crypto.Decrypt(u.EmailEncrypted); err == nil {
				emailCache[e.UserID] = email
			}
		}
		if emailCache[e.UserID] == "" {
			emailCache[e.UserID] = strconv.FormatInt(e.UserID, 10)
		}
	}

	type auditRow struct {
		db.AuditEntry
		Email string
	}
	rows := make([]auditRow, len(entries))
	for i, e := range entries {
		rows[i] = auditRow{AuditEntry: e, Email: emailCache[e.UserID]}
	}

	totalPages := (total + auditPageSize - 1) / auditPageSize

	data := baseData(r, user)
	data["Title"] = "Audit Log"
	data["Entries"] = rows
	data["Page"] = page
	data["TotalPages"] = totalPages
	data["Total"] = total

	templates.Render(w, "admin-audit.html", data)
}
