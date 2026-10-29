// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package auth bundles the JWT issuer + API key validator. The verifier side
// lives in middleware/auth.go; this package holds the issuer side so the
// /auth/login route can mint tokens.
package auth

import (
        "errors"
        "time"

        "github.com/golang-jwt/jwt/v5"
        "github.com/ori-inc/hexagonal-go/internal/middleware"
)

// Issuer mints JWTs.
type Issuer struct {
        secret  []byte
        issuer  string
        accessT time.Duration
}

// NewIssuer constructs an Issuer from config.
func NewIssuer(secret, issuer string, accessTTL time.Duration) *Issuer {
        return &Issuer{secret: []byte(secret), issuer: issuer, accessT: accessTTL}
}

// Issue produces a signed access token for the given user+roles.
func (i *Issuer) Issue(userID string, roles []middleware.Role) (string, error) {
        if userID == "" {
                return "", errors.New("userID required")
        }
        return middleware.IssueJWT(string(i.secret), i.issuer, userID, roles, i.accessT)
}

// Issuer secret length guard - useful at boot.
func (i *Issuer) Healthy() bool { return len(i.secret) >= 32 }

// jwtClaims is duplicated locally so middleware doesn't have to export it.
var _ = jwt.RegisteredClaims{}
