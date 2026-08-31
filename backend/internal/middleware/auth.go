package middleware

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"equiptra/internal/models"
)

const CookieName = "equiptra_session"

type contextKey string

const userContextKey contextKey = "user"

type Claims struct {
	UserID int64           `json:"uid"`
	Role   models.UserRole `json:"role"`
	// MustChangePassword mirrors users.must_change_password at the moment
	// the token was issued — RequirePasswordSet uses it to restrict a
	// forced-reset session to only the self-service password-change
	// endpoint until a new password is set.
	MustChangePassword bool `json:"must_change_password"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-insecure-secret-change-me"
	}
	return []byte(secret)
}

func IssueToken(userID int64, role models.UserRole, mustChangePassword bool) (string, error) {
	claims := Claims{
		UserID:             userID,
		Role:               role,
		MustChangePassword: mustChangePassword,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(7 * 24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(jwtSecret())
}

func SetSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   os.Getenv("COOKIE_SECURE") == "true",
		Expires:  time.Now().Add(7 * 24 * time.Hour),
	})
}

func ClearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   os.Getenv("COOKIE_SECURE") == "true",
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func parseToken(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (interface{}, error) {
		return jwtSecret(), nil
	})
	if err != nil || !token.Valid {
		return nil, err
	}
	return claims, nil
}

// RequireAuth verifies the session cookie and injects the caller's claims into
// the request context. Responds 401 if missing/invalid.
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			http.Error(w, `{"error":"not authenticated"}`, http.StatusUnauthorized)
			return
		}
		claims, err := parseToken(cookie.Value)
		if err != nil {
			http.Error(w, `{"error":"invalid session"}`, http.StatusUnauthorized)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequirePasswordSet is the enforced side of the forced "set a new
// password" screen — not just a frontend redirect. Chained right after
// RequireAuth, it blocks every /api route except the two a locked session
// still needs (fetching who's logged in, and setting the new password
// itself) whenever the caller's token was issued with
// must_change_password = true. A normal session (the common case) is
// completely unaffected.
func RequirePasswordSet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		exempt := r.URL.Path == "/api/me" || r.URL.Path == "/api/users/me/password"
		if ok && claims.MustChangePassword && !exempt {
			http.Error(w, `{"error":"password change required","must_change_password":true}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// OptionalAuth injects the caller's claims into context if a valid session
// cookie happens to be present, but never rejects the request — for the one
// or two genuinely public endpoints (the fault-report form) that need to
// tell a logged-in staff member from an anonymous freelancer without
// requiring a session either way.
func OptionalAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(CookieName)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		claims, err := parseToken(cookie.Value)
		if err != nil {
			next.ServeHTTP(w, r)
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAdmin must be chained after RequireAuth.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := UserFromContext(r.Context())
		if !ok || claims.Role != models.UserRoleAdmin {
			http.Error(w, `{"error":"admin role required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func UserFromContext(ctx context.Context) (*Claims, bool) {
	claims, ok := ctx.Value(userContextKey).(*Claims)
	return claims, ok
}

func UserIDString(ctx context.Context) string {
	claims, ok := UserFromContext(ctx)
	if !ok {
		return ""
	}
	return strconv.FormatInt(claims.UserID, 10)
}
