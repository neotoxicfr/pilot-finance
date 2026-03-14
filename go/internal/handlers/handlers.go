// Package handlers contient les handlers HTTP
package handlers

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"runtime"
	"time"
)

// Version est définie par ldflags au build
var Version = "dev"

// AssetVersion est un hash court des fichiers statiques, calculé au démarrage.
// Utilisé pour le cache-busting des CSS/JS (?v=xxx).
var AssetVersion = "dev"

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

	// Test de la base de données (timeout 2s)
	dbStatus := "ok"
	if err := hookPingDB(r.Context()); err != nil {
		dbStatus = "error"
	}

	response := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now(),
		Version:   Version,
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

	jsonSuccess(w, response)
}

// CSPReport reçoit les rapports de violation CSP et les log
func CSPReport(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 10240))
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	if len(body) > 0 && json.Valid(body) {
		slog.Warn("csp-violation", "report", string(body), "ip", r.RemoteAddr, "ua", r.UserAgent())
	}
	w.WriteHeader(http.StatusNoContent)
}
