// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// auth.go - JWT + API key authentication middleware.
//
// Two schemes are supported simultaneously:
//
//   Authorization: Bearer <jwt>   -> verified against JWT_SECRET
//   X-API-Key:     <static key>   -> verified against AUTH_API_KEYS env list
//
// Either one authenticates the request. The resulting principal is stored on
// the request context as a *Principal so downstream RBAC + audit can read it.
//
// GAP CLOSED: #1 JWT auth + API key auth + RBAC (auth half)
package middleware

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/ori-inc/hexagonal-go/internal/logger"
	"github.com/ori-inc/hexagonal-go/pkg/metrics"
)

// Role is a coarse-grained RBAC role. Domain code can extend this freely; the
// RBAC middleware just compares sets.
type Role string

const (
	RoleAdmin Role = "admin"
	RoleUser  Role = "user"
	RoleGuest Role = "guest"
)

// Principal is the authenticated subject carried on the request context.
type Principal struct {
	UserID string
	Roles  []Role
	// APIKeyID is set when the caller authenticated with an API key.
	APIKeyID string
	// Subject is the JWT sub claim or the API key id.
	Subject string
}

type principalKey struct{}

// WithPrincipal stores a principal on the context.
func WithPrincipal(ctx context.Context, p *Principal) context.Context {
	return context.WithValue(ctx, principalKey{}, p)
}

// PrincipalFrom returns the principal on the context, or nil if anonymous.
func PrincipalFrom(ctx context.Context) *Principal {
	if p, ok := ctx.Value(principalKey{}).(*Principal); ok {
		return p
	}
	return nil
}

// JWTAuth returns a middleware that verifies a Bearer token. On failure it
// writes a 401 envelope and stops. On success it stamps the principal on ctx.
func JWTAuth(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			L := logger.Ctx(r.Context(), logger.LayerMiddleware)

			token := bearerToken(r)
			if token == "" {
				L.Debug().Msg("AUTH_NO_BEARER")
				next.ServeHTTP(w, r) // anonymous; RBAC downstream will 401/403 as needed
				return
			}

			p, err := verifyJWT(token, secret)
			if err != nil {
				L.Warn().Err(err).Msg("AUTH_JWT_INVALID")
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"invalid token"}}`,
					http.StatusUnauthorized)
				return
			}
			L.Info().Str("userId", p.UserID).Strs("roles", rolesAsStrings(p.Roles)).Msg("AUTH_OK_JWT")
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// APIKeyAuth returns a middleware that verifies X-API-Key against a static
// allow-list. Useful for service-to-service calls where JWT is overkill.
func APIKeyAuth(valid []string) func(http.Handler) http.Handler {
	allowed := map[string]struct{}{}
	for _, k := range valid {
		allowed[strings.TrimSpace(k)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			L := logger.Ctx(r.Context(), logger.LayerMiddleware)
			key := strings.TrimSpace(r.Header.Get("X-API-Key"))
			if key == "" {
				next.ServeHTTP(w, r) // anonymous
				return
			}
			if _, ok := allowed[key]; !ok {
				L.Warn().Str("apiKeyPrefix", prefix(key)).Msg("AUTH_APIKEY_INVALID")
				http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"invalid api key"}}`,
					http.StatusUnauthorized)
				return
			}
			p := &Principal{
				Subject:  "apikey:" + prefix(key),
				APIKeyID: prefix(key),
				Roles:    []Role{RoleAdmin}, // service accounts are admins by default
			}
			L.Info().Str("apiKeyPrefix", prefix(key)).Msg("AUTH_OK_APIKEY")
			next.ServeHTTP(w, r.WithContext(WithPrincipal(r.Context(), p)))
		})
	}
}

// bearerToken pulls the JWT out of `Authorization: Bearer ...`.
func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return ""
	}
	parts := strings.SplitN(h, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func prefix(s string) string {
	if len(s) <= 4 {
		return s
	}
	return s[:4]
}

// jwtClaims is the shape we expect in our tokens.
type jwtClaims struct {
	UserID string   `json:"uid"`
	Roles  []string `json:"roles"`
	jwt.RegisteredClaims
}

// verifyJWT parses+validates the token and returns a Principal.
func verifyJWT(tok, secret string) (*Principal, error) {
	c := jwtClaims{}
	_, err := jwt.ParseWithClaims(tok, &c, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errUnexpectedAlg
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	p := &Principal{UserID: c.UserID, Subject: c.Subject}
	for _, r := range c.Roles {
		p.Roles = append(p.Roles, Role(r))
	}
	return p, nil
}

// IssueJWT is exported for cmd/server's /auth/login demo route and for tests.
func IssueJWT(secret, issuer, userID string, roles []Role, ttl time.Duration) (string, error) {
	now := time.Now()
	claims := jwtClaims{
		UserID: userID,
		Roles:  rolesAsStrings(roles),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
	}
	t := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return t.SignedString([]byte(secret))
}

func rolesAsStrings(rs []Role) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = string(r)
	}
	return out
}

var errUnexpectedAlg = &jwtError{"unexpected signing method"}

type jwtError struct{ msg string }

func (e *jwtError) Error() string { return e.msg }

// AuditMetric bump used by both auth paths.
var _ = metrics.AuditEvents // keep import live even if unused in this file
