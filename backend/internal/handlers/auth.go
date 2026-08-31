package handlers

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"

	"equiptra/internal/middleware"
	"equiptra/internal/models"
)

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (a *API) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	var user models.User
	err := a.DB.QueryRow(r.Context(),
		`SELECT id, name, email, role, active, must_change_password, password_hash FROM users WHERE lower(email) = lower($1)`,
		req.Email,
	).Scan(&user.ID, &user.Name, &user.Email, &user.Role, &user.Active, &user.MustChangePassword, &user.PasswordHash)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	// Same generic message as a bad password — don't reveal that a disabled
	// account exists.
	if !user.Active {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}

	token, err := middleware.IssueToken(user.ID, user.Role, user.MustChangePassword)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	middleware.SetSessionCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"id":                   user.ID,
		"name":                 user.Name,
		"email":                user.Email,
		"role":                 user.Role,
		"must_change_password": user.MustChangePassword,
	})
}

func (a *API) Logout(w http.ResponseWriter, r *http.Request) {
	middleware.ClearSessionCookie(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}

	var user models.User
	err := a.DB.QueryRow(r.Context(),
		`SELECT id, name, email, role, active, must_change_password FROM users WHERE id = $1`,
		claims.UserID,
	).Scan(&user.ID, &user.Name, &user.Email, &user.Role, &user.Active, &user.MustChangePassword)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	writeJSON(w, http.StatusOK, user)
}
