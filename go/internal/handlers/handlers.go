// Package handlers contient les handlers HTTP
package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"pilot-finance/internal/db"
)

// Version est définie par ldflags au build
var Version = "dev"

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
	pingCtx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	if err := db.DB.PingContext(pingCtx); err != nil {
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
