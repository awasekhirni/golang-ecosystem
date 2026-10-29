// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// rbac.go - role-based access control.
//
// Require is used as `r.With(middleware.Require(middleware.RoleAdmin))` on
// any sub-router whose routes need a particular role. The middleware also
// enforces "must be authenticated" (anonymous -> 401).
//
// GAP CLOSED: #1 JWT auth + API key auth + RBAC (rbac half)
package middleware

import (
	"net/http"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// Require returns a middleware that allows only principals whose roles
// intersect the required set. Anonymous requests get 401, mismatched roles
// get 403.
func Require(allowed ...Role) func(http.Handler) http.Handler {
	allowSet := map[Role]struct{}{}
	for _, r := range allowed {
		allowSet[r] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := PrincipalFrom(r.Context())
			if p == nil {
				logger.Ctx(r.Context(), logger.LayerMiddleware).
					Warn().Str("path", r.URL.Path).Msg("RBAC_ANONYMOUS")
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"authentication required"}}`,
					http.StatusUnauthorized)
				return
			}
			ok := false
			for _, role := range p.Roles {
				if _, has := allowSet[role]; has {
					ok = true
					break
				}
			}
			if !ok {
				logger.Ctx(r.Context(), logger.LayerMiddleware).
					Warn().
					Str("userId", p.UserID).
					Strs("roles", rolesAsStrings(p.Roles)).
					Msg("RBAC_FORBIDDEN")
				http.Error(w, `{"error":{"code":"FORBIDDEN","message":"insufficient role"}}`,
					http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireUser stamps the user id on the context (so logger.Ctx picks it up)
// for every authenticated request. Mount this just after the auth middlewares.
func RequireUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p := PrincipalFrom(r.Context()); p != nil {
			ctx := logger.WithUserID(r.Context(), p.UserID)
			r = r.WithContext(ctx)
		}
		next.ServeHTTP(w, r)
	})
}
