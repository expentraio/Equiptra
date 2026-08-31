package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/crypto/bcrypt"

	"equiptra/internal/middleware"
	"equiptra/internal/models"
)

const userSelectCols = `id, name, email, role, active, must_change_password, created_at, updated_at`

func scanUser(row pgx.Row) (models.User, error) {
	var u models.User
	err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

// UserListItem adds whether a user has any allocation history, so the
// frontend can proactively grey out Delete the same way project Delete is
// greyed out when booking_requests exist — Deactivate is the safe action
// once that history exists.
type UserListItem struct {
	models.User
	HasAllocationHistory bool `json:"has_allocation_history"`
}

func (a *API) ListUsers(w http.ResponseWriter, r *http.Request) {
	rows, err := a.DB.Query(r.Context(), `
		SELECT u.id, u.name, u.email, u.role, u.active, u.must_change_password, u.created_at, u.updated_at,
		       EXISTS(
		         SELECT 1 FROM booking_allocations ba
		         WHERE ba.checked_out_by = u.id OR ba.checked_in_by = u.id
		       ) AS has_allocation_history
		FROM users u
		ORDER BY u.name`)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	users := []UserListItem{}
	for rows.Next() {
		var item UserListItem
		u := &item.User
		if err := rows.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.Active, &u.MustChangePassword, &u.CreatedAt, &u.UpdatedAt, &item.HasAllocationHistory); err != nil {
			writeError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		users = append(users, item)
	}
	writeJSON(w, http.StatusOK, users)
}

type createUserRequest struct {
	Name     string          `json:"name"`
	Email    string          `json:"email"`
	Password string          `json:"password"`
	Role     models.UserRole `json:"role"`
}

func (a *API) CreateUser(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Name == "" || req.Email == "" {
		writeError(w, http.StatusBadRequest, "name and email are required")
		return
	}
	if len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "password must be at least 8 characters")
		return
	}
	if req.Role == "" {
		req.Role = models.UserRoleStandard
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	u, err := scanUser(a.DB.QueryRow(r.Context(), `
		INSERT INTO users (name, email, password_hash, role)
		VALUES ($1, $2, $3, $4)
		RETURNING `+userSelectCols,
		req.Name, req.Email, string(hash), req.Role,
	))
	if isUniqueViolation(err) {
		writeError(w, http.StatusConflict, "a user with this email already exists")
		return
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "insert failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, u)
}

type updateUserRequest struct {
	Active *bool            `json:"active"`
	Role   *models.UserRole `json:"role"`
}

func (a *API) UpdateUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req updateUserRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Active == nil && req.Role == nil {
		writeError(w, http.StatusBadRequest, "nothing to update")
		return
	}

	claims, ok := middleware.UserFromContext(r.Context())
	if ok && claims.UserID == id && req.Active != nil && !*req.Active {
		writeError(w, http.StatusBadRequest, "cannot deactivate your own account")
		return
	}

	existing, err := scanUser(a.DB.QueryRow(r.Context(), `SELECT `+userSelectCols+` FROM users WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}

	active := existing.Active
	if req.Active != nil {
		active = *req.Active
	}
	role := existing.Role
	if req.Role != nil {
		role = *req.Role
	}

	u, err := scanUser(a.DB.QueryRow(r.Context(), `
		UPDATE users SET active = $1, role = $2, updated_at = now()
		WHERE id = $3
		RETURNING `+userSelectCols,
		active, role, id,
	))
	if err != nil {
		writeError(w, http.StatusBadRequest, "update failed: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func (a *API) DeleteUser(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}

	claims, ok := middleware.UserFromContext(r.Context())
	if ok && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot delete your own account")
		return
	}

	var allocationCount int
	err = a.DB.QueryRow(r.Context(),
		`SELECT COUNT(*) FROM booking_allocations WHERE checked_out_by = $1 OR checked_in_by = $1`, id,
	).Scan(&allocationCount)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "query failed")
		return
	}
	if allocationCount > 0 {
		writeError(w, http.StatusConflict, "user has allocation history — deactivate instead of deleting")
		return
	}

	tag, err := a.DB.Exec(r.Context(), `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "delete failed")
		return
	}
	if tag.RowsAffected() == 0 {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type changeOwnPasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// ChangeOwnPassword is the self-service flow (Settings page) and also how a
// forced reset gets resolved — RequirePasswordSet exempts this exact path so
// a must_change_password session can reach it. Verifies the current
// password even in the forced-reset case: the "current" password there is
// the admin-set temporary one, and requiring it here confirms the caller
// actually knows it rather than just holding a still-valid cookie.
func (a *API) ChangeOwnPassword(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeError(w, http.StatusUnauthorized, "not authenticated")
		return
	}
	var req changeOwnPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	var currentHash string
	err := a.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id = $1`, claims.UserID).Scan(&currentHash)
	if err != nil {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(currentHash), []byte(req.CurrentPassword)); err != nil {
		writeError(w, http.StatusBadRequest, "current password is incorrect")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	u, err := scanUser(a.DB.QueryRow(r.Context(), `
		UPDATE users SET password_hash = $1, must_change_password = false, updated_at = now()
		WHERE id = $2
		RETURNING `+userSelectCols,
		string(newHash), claims.UserID,
	))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}

	// Land straight in the app rather than forcing a fresh login: the old
	// token (if this came from a forced reset) still carries
	// must_change_password=true, so a new one is issued and swapped in.
	token, err := middleware.IssueToken(u.ID, u.Role, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not issue session")
		return
	}
	middleware.SetSessionCookie(w, token)
	writeJSON(w, http.StatusOK, u)
}

type adminResetPasswordRequest struct {
	NewPassword string `json:"new_password"`
}

// AdminResetPassword sets a temporary password for a locked-out user and
// flags must_change_password so they're forced to set their own on next
// login — the admin never ends up as the long-term holder of a teammate's
// real password. Self-lockout carve-out matches UpdateUser/DeleteUser: an
// admin resetting their own password should use the self-service flow
// instead (they already know their current password).
func (a *API) AdminResetPassword(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	claims, ok := middleware.UserFromContext(r.Context())
	if ok && claims.UserID == id {
		writeError(w, http.StatusBadRequest, "cannot reset your own password this way — use Settings instead")
		return
	}
	var req adminResetPasswordRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(req.NewPassword) < 8 {
		writeError(w, http.StatusBadRequest, "new password must be at least 8 characters")
		return
	}

	newHash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not hash password")
		return
	}

	u, err := scanUser(a.DB.QueryRow(r.Context(), `
		UPDATE users SET password_hash = $1, must_change_password = true, updated_at = now()
		WHERE id = $2
		RETURNING `+userSelectCols,
		string(newHash), id,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "update failed")
		return
	}
	writeJSON(w, http.StatusOK, u)
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
