package auth

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const userIDKey contextKey = "userID"

// NewJWTMiddleware crea un middleware JWT compatible con chi router
// que verifica tokens Bearer y valida que el userID del token coincida con el parámetro de ruta
func NewJWTMiddleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extraer token del header Authorization
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				slog.WarnContext(r.Context(), "missing authorization header")
				http.Error(w, "unauthorized: missing token", http.StatusUnauthorized)
				return
			}

			// Verificar formato Bearer
			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				slog.WarnContext(r.Context(), "invalid authorization format")
				http.Error(w, "unauthorized: invalid token format", http.StatusUnauthorized)
				return
			}

			tokenString := parts[1]

			// Parsear y verificar token
			token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
				// Verificar método de firma
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, jwt.ErrSignatureInvalid
				}
				return []byte(secret), nil
			})

			if err != nil || !token.Valid {
				slog.WarnContext(r.Context(), "invalid token", "error", err)
				http.Error(w, "unauthorized: invalid token", http.StatusUnauthorized)
				return
			}

			// Extraer claim 'sub'
			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				slog.WarnContext(r.Context(), "invalid token claims")
				http.Error(w, "unauthorized: invalid token claims", http.StatusUnauthorized)
				return
			}

			sub, ok := claims["sub"].(string)
			if !ok || sub == "" {
				slog.WarnContext(r.Context(), "missing sub claim")
				http.Error(w, "unauthorized: missing subject", http.StatusUnauthorized)
				return
			}

			// Comparar con userID de URL
			urlUserID := chi.URLParam(r, "userID")
			if urlUserID != "" && urlUserID != sub {
				slog.WarnContext(r.Context(), "userID mismatch",
					"token_sub", sub,
					"url_userID", urlUserID,
				)
				http.Error(w, "forbidden: user ID mismatch", http.StatusForbidden)
				return
			}

			// Inyectar userID en contexto
			ctx := context.WithValue(r.Context(), userIDKey, sub)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserID extrae el userID del contexto
func GetUserID(ctx context.Context) (string, bool) {
	userID, ok := ctx.Value(userIDKey).(string)
	return userID, ok
}
