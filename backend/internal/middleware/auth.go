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
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = "dev-only-insecure-secret-change-me"
	}
	return []byte(secret)
}

func IssueToken(userID int64, role models.UserRole) (string, error) {
	claims := Claims{
		UserID: userID,
		Role:   role,
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
