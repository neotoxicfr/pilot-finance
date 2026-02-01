// Package handlers contient les handlers HTTP
package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"pilot-finance/internal/db"
)

// HealthResponse représente la réponse du health check
type HealthResponse struct {
	Status    string    `json:"status"`
	Timestamp time.Time `json:"timestamp"`
	Version   string    `json:"version"`
	Database  string    `json:"database"`
	Memory    struct {
		Alloc      uint64 `json:"alloc_mb"`
		TotalAlloc uint64 `json:"total_alloc_mb"`
		Sys        uint64 `json:"sys_mb"`
		NumGC      uint32 `json:"num_gc"`
	} `json:"memory"`
}

// HealthCheck retourne l'état de santé de l'application
func HealthCheck(w http.ResponseWriter, r *http.Request) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Test de la base de données
	dbStatus := "ok"
	if err := db.DB.Ping(); err != nil {
		dbStatus = "error"
	}

	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   "2.0.0",
		Database:  dbStatus,
	}
	response.Memory.Alloc = m.Alloc / 1024 / 1024
	response.Memory.TotalAlloc = m.TotalAlloc / 1024 / 1024
	response.Memory.Sys = m.Sys / 1024 / 1024
	response.Memory.NumGC = m.NumGC

	if dbStatus != "ok" {
		response.Status = "degraded"
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Placeholder handlers - à implémenter avec les templates HTMX

func LoginPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render login template
	w.Write([]byte("Login Page - TODO"))
}

func LoginSubmit(w http.ResponseWriter, r *http.Request) {
	// TODO: Handle login
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func RegisterPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render register template
	w.Write([]byte("Register Page - TODO"))
}

func RegisterSubmit(w http.ResponseWriter, r *http.Request) {
	// TODO: Handle registration
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func Logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func Dashboard(w http.ResponseWriter, r *http.Request) {
	// TODO: Render dashboard template
	w.Write([]byte("Dashboard - TODO"))
}

func AccountsPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render accounts template
	w.Write([]byte("Accounts Page - TODO"))
}

func CreateAccount(w http.ResponseWriter, r *http.Request) {
	// TODO: Create account
	w.WriteHeader(http.StatusCreated)
}

func UpdateAccount(w http.ResponseWriter, r *http.Request) {
	// TODO: Update account
	w.WriteHeader(http.StatusOK)
}

func DeleteAccount(w http.ResponseWriter, r *http.Request) {
	// TODO: Delete account
	w.WriteHeader(http.StatusNoContent)
}

func UpdateBalance(w http.ResponseWriter, r *http.Request) {
	// TODO: Update balance
	w.WriteHeader(http.StatusOK)
}

func RecurringPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render recurring template
	w.Write([]byte("Recurring Page - TODO"))
}

func CreateRecurring(w http.ResponseWriter, r *http.Request) {
	// TODO: Create recurring
	w.WriteHeader(http.StatusCreated)
}

func UpdateRecurring(w http.ResponseWriter, r *http.Request) {
	// TODO: Update recurring
	w.WriteHeader(http.StatusOK)
}

func DeleteRecurring(w http.ResponseWriter, r *http.Request) {
	// TODO: Delete recurring
	w.WriteHeader(http.StatusNoContent)
}

func SettingsPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render settings template
	w.Write([]byte("Settings Page - TODO"))
}

func ChangePassword(w http.ResponseWriter, r *http.Request) {
	// TODO: Change password
	w.WriteHeader(http.StatusOK)
}

func AdminPage(w http.ResponseWriter, r *http.Request) {
	// TODO: Render admin template
	w.Write([]byte("Admin Page - TODO"))
}

func DeleteUser(w http.ResponseWriter, r *http.Request) {
	// TODO: Delete user
	w.WriteHeader(http.StatusNoContent)
}
