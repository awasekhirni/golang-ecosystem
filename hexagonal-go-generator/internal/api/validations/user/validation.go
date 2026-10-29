// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// validation.go - per-field request validation.
//
// Each ValidateX returns an httpx.AppError carrying field-level details so
// the controller can hand it straight to httpx.WriteError and the client
// sees a uniform `details[]` array.
//
// Layer-4 (Validation) logging is emitted for every rejected field.
//
// GAP CLOSED: per-field validation errors with stable codes
package uservalidation

import (
        "context"

        "github.com/ori-inc/hexagonal-go/internal/api/dtos/user"
        "github.com/ori-inc/hexagonal-go/internal/httpx"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// ValidateCreate validates a CreateRequest and returns nil on success.
func ValidateCreate(ctx context.Context, r userdto.CreateRequest) error {
        var problems []httpx.FieldError

        if r.Email == "" {
                problems = append(problems, httpx.FieldError{Field: "email", Message: "required"})
        } else if !looksLikeEmail(r.Email) {
                problems = append(problems, httpx.FieldError{Field: "email", Value: r.Email, Message: "invalid email format"})
        }
        if r.FirstName == "" {
                problems = append(problems, httpx.FieldError{Field: "first_name", Message: "required"})
        }
        if len(r.FirstName) > 100 {
                problems = append(problems, httpx.FieldError{Field: "first_name", Value: r.FirstName, Message: "max 100 chars"})
        }
        if r.LastName == "" {
                problems = append(problems, httpx.FieldError{Field: "last_name", Message: "required"})
        }
        if len(r.LastName) > 100 {
                problems = append(problems, httpx.FieldError{Field: "last_name", Value: r.LastName, Message: "max 100 chars"})
        }

        for _, p := range problems {
                logger.Ctx(ctx, logger.LayerValidation).
                        Warn().
                        Str("field", p.Field).
                        Interface("value", p.Value).
                        Str("message", p.Message).
                        Msg("VALIDATION_FAILED")
        }
        if len(problems) > 0 {
                return httpx.NewError(httpx.ErrCodeValidationFailed, "validation failed", problems...)
        }
        return nil
}

// ValidateChangeStatus validates a ChangeStatusRequest.
func ValidateChangeStatus(ctx context.Context, r userdto.ChangeStatusRequest) error {
        var problems []httpx.FieldError
        switch r.Status {
        case "active", "inactive", "pending":
                // ok
        default:
                problems = append(problems, httpx.FieldError{Field: "status", Value: r.Status, Message: "must be active|inactive|pending"})
        }
        for _, p := range problems {
                logger.Ctx(ctx, logger.LayerValidation).
                        Warn().Str("field", p.Field).Str("message", p.Message).Msg("VALIDATION_FAILED")
        }
        if len(problems) > 0 {
                return httpx.NewError(httpx.ErrCodeValidationFailed, "validation failed", problems...)
        }
        return nil
}

// looksLikeEmail is a tiny RFC-ish sanity check. The full validation happens
// in valueobject.ParseEmail once we cross into the domain.
func looksLikeEmail(s string) bool {
        at := -1
        dotAfterAt := false
        for i := 0; i < len(s); i++ {
                switch s[i] {
                case '@':
                        if at != -1 {
                                return false
                        }
                        at = i
                case '.':
                        if at != -1 && i > at {
                                dotAfterAt = true
                        }
                }
        }
        return at > 0 && dotAfterAt
}
