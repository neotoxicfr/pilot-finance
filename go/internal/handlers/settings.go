package handlers

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"pilot-finance/internal/crypto"
	"pilot-finance/internal/db"
	"pilot-finance/internal/middleware"
)

// ChangePassword change le mot de passe de l'utilisateur
func ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Non authentifie", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Donnees invalides", http.StatusBadRequest)
		return
	}

	currentPassword := r.FormValue("currentPassword")
	newPassword := r.FormValue("newPassword")
	confirmPassword := r.FormValue("confirmPassword")

	if currentPassword == "" || newPassword == "" {
		http.Error(w, "Tous les champs sont requis", http.StatusBadRequest)
		return
	}

	if newPassword != confirmPassword {
		http.Error(w, "Les mots de passe ne correspondent pas", http.StatusBadRequest)
		return
	}

	if len(newPassword) < 8 {
		http.Error(w, "Mot de passe trop court (min 8 caracteres)", http.StatusBadRequest)
		return
	}

	// Verifier le mot de passe actuel
	if !crypto.VerifyPassword(currentPassword, user.Password) {
		http.Error(w, "Mot de passe actuel incorrect", http.StatusUnauthorized)
		return
	}

	// Hasher le nouveau mot de passe
	hashedPassword, err := crypto.HashPassword(newPassword)
	if err != nil {
		http.Error(w, "Erreur serveur", http.StatusInternalServerError)
		return
	}

	// Mettre a jour
	err = db.UpdatePassword(user.ID, hashedPassword)
	if err != nil {
		http.Error(w, "Erreur mise a jour", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

// DeleteUser supprime un utilisateur (admin uniquement)
func DeleteUser(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil || user.Role != "ADMIN" {
		http.Error(w, "Non autorise", http.StatusForbidden)
		return
	}

	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "ID invalide", http.StatusBadRequest)
		return
	}

	// Ne pas permettre de supprimer un admin
	targetUser, err := db.GetUserByID(id)
	if err != nil {
		http.Error(w, "Utilisateur non trouve", http.StatusNotFound)
		return
	}

	if targetUser.Role == "ADMIN" {
		http.Error(w, "Impossible de supprimer un administrateur", http.StatusForbidden)
		return
	}

	err = db.DeleteUser(id)
	if err != nil {
		http.Error(w, "Erreur suppression", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
