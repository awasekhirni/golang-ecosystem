// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package openapi - request_body_validation.go
//
// Runtime request-body validation against the OpenAPI schema. The middleware
// looks up the operation by method+path in the spec, finds the requestBody
// schema, and validates the incoming JSON against it. Mismatches return a
// 400 with field-level details in the standard error envelope.
//
// This closes the loop between "spec says X" and "code enforces X" — the
// controller never sees an invalid body.
//
// GAP CLOSED: OpenAPI request-body validation middleware (contract enforcement)
package openapi

import (
        "encoding/json"
        "io"
        "net/http"
        "regexp"
        "strings"

        "github.com/ori-inc/hexagonal-go/internal/httpx"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// pathPattern converts an OpenAPI path template like "/api/v1/users/{id}"
// into a regex with named capture groups, plus a chi-style pattern matcher.
type compiledPath struct {
        template string
        regex    *regexp.Regexp
        method   string
        op       *Operation
}

// compilePaths pre-compiles every (method, path) pair in the spec for fast
// lookup at request time.
func (s *Spec) compilePaths() []compiledPath {
        var out []compiledPath
        for path, item := range s.Paths {
                regexStr := "^" + regexp.QuoteMeta(path) + "$"
                // Replace escaped \{...\} with capture groups.
                regexStr = strings.ReplaceAll(regexStr, `\{`, "{")
                regexStr = strings.ReplaceAll(regexStr, `\}`, "}")
                // Now substitute {param} with a capture group.
                paramRE := regexp.MustCompile(`\{[^}]+\}`)
                regexStr = paramRE.ReplaceAllString(regexStr, `([^/]+)`)
                re := regexp.MustCompile(regexStr)

                if item.Get != nil {
                        out = append(out, compiledPath{template: path, regex: re, method: http.MethodGet, op: item.Get})
                }
                if item.Post != nil {
                        out = append(out, compiledPath{template: path, regex: re, method: http.MethodPost, op: item.Post})
                }
                if item.Put != nil {
                        out = append(out, compiledPath{template: path, regex: re, method: http.MethodPut, op: item.Put})
                }
                if item.Patch != nil {
                        out = append(out, compiledPath{template: path, regex: re, method: http.MethodPatch, op: item.Patch})
                }
                if item.Delete != nil {
                        out = append(out, compiledPath{template: path, regex: re, method: http.MethodDelete, op: item.Delete})
                }
        }
        return out
}

// RequestValidator returns a middleware that validates request bodies against
// the OpenAPI spec. Mount it AFTER the auth middleware so only authenticated
// traffic is validated (cheaper) and BEFORE the controller.
func RequestValidator(spec *Spec) func(http.Handler) http.Handler {
        compiled := spec.compilePaths()
        return func(next http.Handler) http.Handler {
                return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                        // Only validate methods with a body.
                        if r.Method != http.MethodPost && r.Method != http.MethodPut && r.Method != http.MethodPatch {
                                next.ServeHTTP(w, r)
                                return
                        }
                        op := findOp(compiled, r)
                        if op == nil || op.RequestBody == nil {
                                // No schema for this route — pass through.
                                next.ServeHTTP(w, r)
                                return
                        }
                        mt := op.RequestBody.Content["application/json"]
                        if mt == nil || mt.Schema == nil {
                                next.ServeHTTP(w, r)
                                return
                        }

                        // Read + restore the body.
                        var body any
                        dec := json.NewDecoder(r.Body)
                        if err := dec.Decode(&body); err != nil {
                                httpx.WriteError(r.Context(), w, httpx.NewError(httpx.ErrCodeBadRequest,
                                        "invalid JSON body: "+err.Error()))
                                return
                        }
                        // Re-buffer the body so the downstream controller can read it.
                        r.Body = ioNopCloser(body)

                        // Resolve $ref if needed.
                        schema := mt.Schema
                        if schema.Ref != "" {
                                schema = resolveRef(spec, schema.Ref)
                        }

                        problems := validateValue(spec, "", body, schema)
                        if len(problems) > 0 {
                                fields := make([]httpx.FieldError, 0, len(problems))
                                for _, p := range problems {
                                        fields = append(fields, httpx.FieldError{
                                                Field: p.path, Message: p.msg,
                                        })
                                        logger.Ctx(r.Context(), logger.LayerValidation).
                                                Warn().
                                                Str("field", p.path).
                                                Str("message", p.msg).
                                                Msg("OPENAPI_VALIDATION_FAILED")
                                }
                                httpx.WriteError(r.Context(), w, httpx.NewError(httpx.ErrCodeValidationFailed,
                                        "request body failed schema validation", fields...))
                                return
                        }
                        next.ServeHTTP(w, r)
                })
        }
}

// validationProblem is one field-level failure.
type validationProblem struct {
        path string
        msg  string
}

// findOp locates the operation for the incoming request.
func findOp(compiled []compiledPath, r *http.Request) *Operation {
        for _, c := range compiled {
                if c.method == r.Method && c.regex.MatchString(r.URL.Path) {
                        return c.op
                }
        }
        return nil
}

// resolveRef turns "#/components/schemas/UserCreateRequest" into the actual Schema.
func resolveRef(spec *Spec, ref string) *Schema {
        const prefix = "#/components/schemas/"
        if !strings.HasPrefix(ref, prefix) {
                return nil
        }
        name := strings.TrimPrefix(ref, prefix)
        if spec.Components == nil || spec.Components.Schemas == nil {
                return nil
        }
        return spec.Components.Schemas[name]
}

// validateValue recursively validates a decoded JSON value against a schema.
// Returns a list of field-level problems (empty = valid).
func validateValue(spec *Spec, path string, val any, schema *Schema) []validationProblem {
        if schema == nil {
                return nil
        }
        if schema.Ref != "" {
                schema = resolveRef(spec, schema.Ref)
                if schema == nil {
                        return nil
                }
        }
        var problems []validationProblem

        switch schema.Type {
        case "object":
                obj, ok := val.(map[string]any)
                if !ok {
                        return []validationProblem{{path: path, msg: "expected object"}}
                }
                // Required fields
                for _, req := range schema.Required {
                        if _, present := obj[req]; !present {
                                p := path
                                if p == "" {
                                        p = req
                                } else {
                                        p = p + "." + req
                                }
                                problems = append(problems, validationProblem{path: p, msg: "required field missing"})
                        }
                }
                // Properties
                for k, v := range obj {
                        childSchema := schema.Properties[k]
                        if childSchema == nil {
                                // Unknown field — allow through (OpenAPI default is open model)
                                continue
                        }
                        childPath := k
                        if path != "" {
                                childPath = path + "." + k
                        }
                        problems = append(problems, validateValue(spec, childPath, v, childSchema)...)
                }
        case "string":
                s, ok := val.(string)
                if !ok {
                        return []validationProblem{{path: path, msg: "expected string"}}
                }
                if schema.Format == "email" && !looksLikeEmail(s) {
                        problems = append(problems, validationProblem{path: path, msg: "invalid email format"})
                }
                if len(schema.Enum) > 0 {
                        ok := false
                        for _, e := range schema.Enum {
                                if e == s {
                                        ok = true
                                        break
                                }
                        }
                        if !ok {
                                problems = append(problems, validationProblem{
                                        path: path, msg: "must be one of: " + strings.Join(schema.Enum, ", "),
                                })
                        }
                }
        case "integer":
                if _, ok := val.(float64); !ok {
                        return []validationProblem{{path: path, msg: "expected integer"}}
                }
        case "boolean":
                if _, ok := val.(bool); !ok {
                        return []validationProblem{{path: path, msg: "expected boolean"}}
                }
        }
        return problems
}

// looksLikeEmail is a tiny RFC-ish check (the value object does the strict one).
func looksLikeEmail(s string) bool {
        at := strings.IndexByte(s, '@')
        return at > 0 && strings.IndexByte(s[at+1:], '.') >= 0
}

// ioNopCloser re-serializes the decoded body so the controller can re-read it.
// We intentionally round-trip through JSON so the downstream decoder sees the
// canonical form.
func ioNopCloser(v any) io.ReadCloser {
        b, _ := json.Marshal(v)
        return io.NopCloser(bytesReader(b))
}

// bytesReader is a tiny reader over a byte slice.
type bytesReaderImpl struct {
        b   []byte
        pos int
}

func (c *bytesReaderImpl) Read(p []byte) (int, error) {
        if c.pos >= len(c.b) {
                return 0, io.EOF
        }
        n := copy(p, c.b[c.pos:])
        c.pos += n
        return n, nil
}
func (c *bytesReaderImpl) Close() error { return nil }

func bytesReader(b []byte) io.ReadCloser { return &bytesReaderImpl{b: b} }
