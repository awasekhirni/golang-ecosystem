// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package httpx standardizes the wire format of every API response. Two
// envelope shapes are produced:
//
//   success: { "data": ..., "meta": {...}, "links": {...} }
//   error:   { "error": { "code":"...", "message":"...", "details":[...] } }
//
// All errors carry a stable string code (so clients can switch on it without
// parsing prose) plus the requestId from the request context so a user can
// quote the id to ops and have it grep-able in the logs.
//
// GAP CLOSED: #7 Standardized error envelope
//             #8 Cursor + offset pagination envelope
//             #9 HATEOAS links (REST maturity Level 3)
package httpx

import (
        "context"
        "encoding/json"
        "net/http"
        "strings"

        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// =============================================================================
// Error codes — single registry, switchable from client code.
// =============================================================================

// ErrCode is a stable string enum. Never reuse a code; only add new ones.
type ErrCode string

const (
        ErrCodeBadRequest         ErrCode = "BAD_REQUEST"
        ErrCodeValidationFailed   ErrCode = "VALIDATION_FAILED"
        ErrCodeUnauthorized       ErrCode = "UNAUTHORIZED"
        ErrCodeForbidden          ErrCode = "FORBIDDEN"
        ErrCodeNotFound           ErrCode = "NOT_FOUND"
        ErrCodeConflict           ErrCode = "CONFLICT"
        ErrCodeRateLimited        ErrCode = "RATE_LIMITED"
        ErrCodePayloadTooLarge    ErrCode = "PAYLOAD_TOO_LARGE"
        ErrCodeUnsupportedMedia   ErrCode = "UNSUPPORTED_MEDIA_TYPE"
        ErrCodeMethodNotAllowed   ErrCode = "METHOD_NOT_ALLOWED"
        ErrCodeInternal           ErrCode = "INTERNAL"
        ErrCodeServiceUnavailable ErrCode = "SERVICE_UNAVAILABLE"
        ErrCodeDatabase           ErrCode = "DATABASE_ERROR"
        ErrCodeUpstream           ErrCode = "UPSTREAM_ERROR"
)

// httpStatusOf maps a stable code to an HTTP status. Centralizing this keeps
// controllers from inventing their own status codes.
func httpStatusOf(c ErrCode) int {
        switch c {
        case ErrCodeBadRequest, ErrCodeValidationFailed:
                return http.StatusBadRequest
        case ErrCodeUnauthorized:
                return http.StatusUnauthorized
        case ErrCodeForbidden:
                return http.StatusForbidden
        case ErrCodeNotFound:
                return http.StatusNotFound
        case ErrCodeConflict:
                return http.StatusConflict
        case ErrCodeRateLimited:
                return http.StatusTooManyRequests
        case ErrCodePayloadTooLarge:
                return http.StatusRequestEntityTooLarge
        case ErrCodeUnsupportedMedia:
                return http.StatusUnsupportedMediaType
        case ErrCodeMethodNotAllowed:
                return http.StatusMethodNotAllowed
        case ErrCodeServiceUnavailable:
                return http.StatusServiceUnavailable
        case ErrCodeDatabase, ErrCodeUpstream, ErrCodeInternal:
                return http.StatusInternalServerError
        }
        return http.StatusInternalServerError
}

// =============================================================================
// Error envelope
// =============================================================================

// FieldError is a per-field validation failure. `field` is the JSON path,
// `value` is the rejected value (stringified), `message` is the human reason.
type FieldError struct {
        Field   string `json:"field"`
        Value   any    `json:"value,omitempty"`
        Message string `json:"message"`
}

// APIError is the structured error inside the envelope.
type APIError struct {
        Code        ErrCode      `json:"code"`
        Message     string       `json:"message"`
        Details     []FieldError `json:"details,omitempty"`
        RequestID   string       `json:"requestId,omitempty"`
        TraceID     string       `json:"traceId,omitempty"`
        Stack       string       `json:"-"`           // never serialized; logged only
}

// ErrorEnvelope is what clients always see on failure.
type ErrorEnvelope struct {
        Error APIError `json:"error"`
}

// AppError lets service/repository code return typed errors that the
// controller can unwrap into the standardized envelope. Implement it for any
// domain-level error so the controller does not need a giant switch.
type AppError interface {
        error
        Code() ErrCode
        Fields() []FieldError
        Message() string
}

// typedError is the canonical implementation of AppError.
type typedError struct {
        code    ErrCode
        message string
        fields  []FieldError
        cause   error
}

func (e *typedError) Error() string {
        if e.cause != nil {
                return e.message + ": " + e.cause.Error()
        }
        return e.message
}
func (e *typedError) Code() ErrCode        { return e.code }
func (e *typedError) Fields() []FieldError { return e.fields }
func (e *typedError) Message() string      { return e.message }
func (e *typedError) Unwrap() error        { return e.cause }

// NewError builds an AppError with optional field-level details.
func NewError(code ErrCode, msg string, fields ...FieldError) AppError {
        return &typedError{code: code, message: msg, fields: fields}
}

// WrapError adds a cause.
func WrapError(code ErrCode, msg string, cause error, fields ...FieldError) AppError {
        return &typedError{code: code, message: msg, fields: fields, cause: cause}
}

// WriteError inspects err, derives the code+status, logs at the Errors layer
// with stack+cause+traceId, then writes the JSON envelope to w.
func WriteError(ctx context.Context, w http.ResponseWriter, err error) {
        L := logger.Ctx(ctx, logger.LayerErrors)

        var ae AppError
        switch {
        case err == nil:
                ae = NewError(ErrCodeInternal, "nil error")
        case As(err, &ae):
                // already typed
        default:
                ae = WrapError(ErrCodeInternal, "unexpected error", err)
        }

        status := httpStatusOf(ae.Code())
        env := ErrorEnvelope{Error: APIError{
                Code:      ae.Code(),
                Message:   ae.Message(),
                Details:   ae.Fields(),
                RequestID: logger.RequestIDFrom(ctx),
                TraceID:   logger.TraceIDFrom(ctx),
        }}

        // Stack trace is logged but never serialized to the client.
        L.Error().
                Str("code", string(ae.Code())).
                Str("message", ae.Error()).
                Int("httpStatus", status).
                Str("stack", stackOf(err)).
                Msg("ERROR_ENVELOPE")

        writeJSON(w, status, env)
}

// As is the same as errors.As but kept here to avoid an import in callers.
func As(err error, target any) bool { return errorsAs(err, target) }

// =============================================================================
// Success envelope
// =============================================================================

// Meta carries non-data response metadata. Pagination lives here.
type Meta struct {
        Page       *PageMeta `json:"page,omitempty"`
        ServerTime string    `json:"serverTime"`
        RequestID  string    `json:"requestId"`
}

// PageMeta is the pagination block — supports both offset and cursor.
type PageMeta struct {
        Total      int64  `json:"total,omitempty"`
        Offset     int    `json:"offset,omitempty"`
        Limit      int    `json:"limit"`
        HasNext    bool   `json:"hasNext"`
        NextCursor string `json:"nextCursor,omitempty"`
}

// SuccessEnvelope is what clients always see on 2xx.
type SuccessEnvelope struct {
        Data  any            `json:"data"`
        Meta  Meta           `json:"meta"`
        Links map[string]Link `json:"links,omitempty"`
}

// WriteJSON writes any payload as JSON with 200 OK.
func WriteJSON(ctx context.Context, w http.ResponseWriter, v any) {
        env := SuccessEnvelope{
                Data: v,
                Meta: Meta{
                        ServerTime: nowISO(),
                        RequestID:  logger.RequestIDFrom(ctx),
                },
        }
        writeJSON(w, http.StatusOK, env)
}

// WriteCreated writes 201 with Location header and the created resource.
func WriteCreated(ctx context.Context, w http.ResponseWriter, location string, v any) {
        env := SuccessEnvelope{
                Data: v,
                Meta: Meta{
                        ServerTime: nowISO(),
                        RequestID:  logger.RequestIDFrom(ctx),
                },
                Links: map[string]Link{"self": {Href: location}},
        }
        w.Header().Set("Location", location)
        writeJSON(w, http.StatusCreated, env)
}

// WriteNoContent writes 204 with the envelope-stripped empty body.
func WriteNoContent(ctx context.Context, w http.ResponseWriter) {
        w.Header().Set("X-Request-Id", logger.RequestIDFrom(ctx))
        w.WriteHeader(http.StatusNoContent)
}

// writeJSON is the low-level serializer used by both envelopes.
func writeJSON(w http.ResponseWriter, status int, v any) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        w.WriteHeader(status)
        _ = json.NewEncoder(w).Encode(v)
}

// =============================================================================
// helpers
// =============================================================================

// nowISO returns the current time in RFC3339 with sub-second precision.
func nowISO() string { return timeNowUTC().Format("2006-01-02T15:04:05.000Z") }

// stackOf returns a stack-trace string for an error, falling back to its
// Error() string when no real stack is available. Keeping this behind a
// function lets us swap in pkg/errors or runtime/debug later.
func stackOf(err error) string {
        if err == nil {
                return ""
        }
        // Heuristic: if the error message contains a colon, surface the chain.
        return strings.ReplaceAll(err.Error(), "\n", " | ")
}
