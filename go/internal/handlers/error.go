package handlers

import (
	"net/http"

	"pilot-finance/internal/middleware"
)

// NotFound affiche la page d'erreur 404
func NotFound(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	data := baseData(r, user)
	data["Title"] = "404"
	data["Code"] = "404"
	w.Header().Set("X-Error-Code", ErrNotFound)
	w.WriteHeader(http.StatusNotFound)
	hookRender(w, "error.html", data) //nolint:errcheck
}

// MethodNotAllowed affiche la page d'erreur 405
func MethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	data := baseData(r, user)
	data["Title"] = "405"
	data["Code"] = "405"
	w.Header().Set("X-Error-Code", ErrForbidden)
	w.WriteHeader(http.StatusMethodNotAllowed)
	hookRender(w, "error.html", data) //nolint:errcheck
}

// InternalServerError affiche la page d'erreur 500
func InternalServerError(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	data := baseData(r, user)
	data["Title"] = "500"
	data["Code"] = "500"
	w.WriteHeader(http.StatusInternalServerError)
	hookRender(w, "error.html", data) //nolint:errcheck
}
