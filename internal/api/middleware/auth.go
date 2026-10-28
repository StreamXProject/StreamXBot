package middleware

import (
	"context"
	"net/http"
	"strings"

	"streamgo/internal/api"
	"streamgo/internal/services"
)

type contextKey string

const (
	userIDKey  contextKey = "userID"
	isGuestKey contextKey = "isGuest"
)

// WithUserID injects a numeric user ID into context.
func WithUserID(ctx context.Context, userID int64) context.Context {
	return context.WithValue(ctx, userIDKey, userID)
}

// GetUserID extracts the user ID from context, returning ok=false if not found.
func GetUserID(ctx context.Context) (int64, bool) {
	val := ctx.Value(userIDKey)
	if val == nil {
		return 0, false
	}
	uid, ok := val.(int64)
	return uid, ok && uid > 0
}

// WithGuest marks context as a guest session.
func WithGuest(ctx context.Context, isGuest bool) context.Context {
	return context.WithValue(ctx, isGuestKey, isGuest)
}

// IsGuest checks if current request is authenticated as guest.
func IsGuest(ctx context.Context) bool {
	val := ctx.Value(isGuestKey)
	if val == nil {
		return false
	}
	g, ok := val.(bool)
	return ok && g
}

// ExtractToken extracts auth token from headers, query string, or cookies.
func ExtractToken(r *http.Request) string {
	// 1. Authorization: Bearer <token>
	authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
	if authHeader != "" {
		if strings.HasPrefix(strings.ToLower(authHeader), "bearer ") {
			return strings.TrimSpace(authHeader[7:])
		}
		return authHeader
	}

	// 2. X-Auth-Token header
	if xAuth := strings.TrimSpace(r.Header.Get("X-Auth-Token")); xAuth != "" {
		return xAuth
	}

	// 3. Query string (?token=... or ?auth=...)
	if qToken := strings.TrimSpace(r.URL.Query().Get("token")); qToken != "" {
		return qToken
	}
	if qAuth := strings.TrimSpace(r.URL.Query().Get("auth")); qAuth != "" {
		return qAuth
	}

	// 4. Cookies (auth_token or token)
	if c, err := r.Cookie("auth_token"); err == nil && strings.TrimSpace(c.Value) != "" {
		return strings.TrimSpace(c.Value)
	}
	if c, err := r.Cookie("token"); err == nil && strings.TrimSpace(c.Value) != "" {
		return strings.TrimSpace(c.Value)
	}

	return ""
}

func extractGuestPassword(r *http.Request) string {
	if gp := strings.TrimSpace(r.Header.Get("X-Guest-Password")); gp != "" {
		return gp
	}
	if gp := strings.TrimSpace(r.URL.Query().Get("guest_password")); gp != "" {
		return gp
	}
	return ""
}

// RequireAuth enforces a valid authentication token or valid guest password.
func RequireAuth(authSvc *services.AuthService) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 1. Check direct guest password bypass
			if guestPwd := extractGuestPassword(r); guestPwd != "" {
				if authSvc.VerifyGuestPassword(r.Context(), guestPwd) {
					ctx := WithGuest(r.Context(), true)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}

			// 2. Check token
			token := ExtractToken(r)
			if token == "" {
				api.RespondError(w, http.StatusUnauthorized, "Authentication required")
				return
			}

			claims, err := authSvc.VerifyTokenClaims(token)
			if err != nil {
				api.RespondError(w, http.StatusUnauthorized, "Invalid or expired authentication token")
				return
			}

			if claims.IsGuest {
				ctx := WithGuest(r.Context(), true)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}

			if claims.UserID <= 0 {
				api.RespondError(w, http.StatusUnauthorized, "Invalid user ID in token")
				return
			}

			ctx := WithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// OptionalAuth parses authentication token or guest password without rejecting unauthenticated requests.
func OptionalAuth(authSvc *services.AuthService) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			if guestPwd := extractGuestPassword(r); guestPwd != "" {
				if authSvc.VerifyGuestPassword(ctx, guestPwd) {
					ctx = WithGuest(ctx, true)
				}
			}

			token := ExtractToken(r)
			if token != "" {
				if claims, err := authSvc.VerifyTokenClaims(token); err == nil {
					if claims.IsGuest {
						ctx = WithGuest(ctx, true)
					} else if claims.UserID > 0 {
						ctx = WithUserID(ctx, claims.UserID)
					}
				}
			}

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
