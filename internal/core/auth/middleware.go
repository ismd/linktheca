package auth

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/ismd/linktheca/internal/core/httpx"
)

// RequireUser parses a Bearer token with the given issuer, validates it, and attaches userID + isAdmin to the request context. Responds 401 on failure.
func RequireUser(issuer *JWTIssuer) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "missing bearer token")
				return
			}
			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")

			claims, err := issuer.Parse(tokenStr)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
				return
			}

			uid, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil {
				httpx.WriteError(w, http.StatusUnauthorized, "unauthorized", "malformed subject")
				return
			}

			ctx := WithUser(r.Context(), uid, claims.IsAdmin)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// RequireAdmin must be composed INSIDE RequireUser. It responds 403 if the user on the context is not an admin.
func RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !IsAdmin(r.Context()) {
			httpx.WriteError(w, http.StatusForbidden, "forbidden", "admin only")
			return
		}

		next.ServeHTTP(w, r)
	})
}
