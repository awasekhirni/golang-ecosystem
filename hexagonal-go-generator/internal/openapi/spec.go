// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package openapi generates and serves an OpenAPI 3.1 document for every
// REST route mounted in the chi router, plus a Swagger UI and a ReDoc UI.
//
// The spec is built programmatically from a Spec struct so it never drifts
// from the actual code (controllers register their operations into the spec
// at boot). Two UIs are served:
//
//   GET /docs/swagger   - Swagger UI 5.x (interactive try-it-out)
//   GET /docs/redoc     - ReDoc 2.x (readable reference)
//   GET /openapi.json   - raw OpenAPI 3.1 JSON
//   GET /openapi.yaml   - raw OpenAPI 3.1 YAML
//
// GAP CLOSED: OpenAPI 3 spec generation (was explicitly requested in the
// original brief; missing from the first pass).
package openapi

import (
        "encoding/json"
        "net/http"
        "strings"
)

// Spec is the in-memory OpenAPI 3.1 document. We hand-assemble it (rather
// than depending on a third-party library) so the binary stays small and
// the schema is auditable in code review.
type Spec struct {
        OpenAPI    string                 `json:"openapi"`
        Info       Info                   `json:"info"`
        Servers    []Server               `json:"servers,omitempty"`
        Paths      map[string]*PathItem   `json:"paths"`
        Components *Components            `json:"components,omitempty"`
        Security   []map[string][]string  `json:"security,omitempty"`
        Tags       []Tag                  `json:"tags,omitempty"`
}

type Info struct {
        Title       string `json:"title"`
        Description string `json:"description,omitempty"`
        Version     string `json:"version"`
        Contact     *Contact `json:"contact,omitempty"`
        License     *License `json:"license,omitempty"`
}

type Contact struct {
        Name  string `json:"name"`
        Email string `json:"email"`
        URL   string `json:"url,omitempty"`
}

type License struct {
        Name string `json:"name"`
        URL  string `json:"url,omitempty"`
}

type Server struct {
        URL         string `json:"url"`
        Description string `json:"description,omitempty"`
}

type Tag struct {
        Name        string `json:"name"`
        Description string `json:"description,omitempty"`
}

type PathItem struct {
        Get    *Operation `json:"get,omitempty"`
        Post   *Operation `json:"post,omitempty"`
        Put    *Operation `json:"put,omitempty"`
        Patch  *Operation `json:"patch,omitempty"`
        Delete *Operation `json:"delete,omitempty"`
}

type Operation struct {
        Tags        []string             `json:"tags,omitempty"`
        Summary     string               `json:"summary"`
        Description string               `json:"description,omitempty"`
        OperationID string               `json:"operationId"`
        Parameters  []Parameter          `json:"parameters,omitempty"`
        RequestBody *RequestBody         `json:"requestBody,omitempty"`
        Responses   map[string]*Response `json:"responses"`
        Security    []map[string][]string `json:"security,omitempty"`
        Deprecated  bool                 `json:"deprecated,omitempty"`
}

type Parameter struct {
        Name        string  `json:"name"`
        In          string  `json:"in"` // query|header|path|cookie
        Description string  `json:"description,omitempty"`
        Required    bool    `json:"required"`
        Schema      *Schema `json:"schema,omitempty"`
}

type RequestBody struct {
        Description string                  `json:"description,omitempty"`
        Content     map[string]*MediaType   `json:"content"`
        Required    bool                    `json:"required,omitempty"`
}

type MediaType struct {
        Schema  *Schema             `json:"schema,omitempty"`
        Example any                 `json:"example,omitempty"`
}

type Response struct {
        Description string                `json:"description"`
        Content     map[string]*MediaType `json:"content,omitempty"`
        Headers     map[string]*Header    `json:"headers,omitempty"`
}

type Header struct {
        Description string  `json:"description,omitempty"`
        Schema      *Schema `json:"schema,omitempty"`
}

type Schema struct {
        Ref                string             `json:"$ref,omitempty"`
        Type               string             `json:"type,omitempty"`
        Format             string             `json:"format,omitempty"`
        Description        string             `json:"description,omitempty"`
        Properties         map[string]*Schema `json:"properties,omitempty"`
        Items              *Schema            `json:"items,omitempty"`
        Required           []string           `json:"required,omitempty"`
        Enum               []string           `json:"enum,omitempty"`
        Example            any                `json:"example,omitempty"`
        Default            any                `json:"default,omitempty"`
        AdditionalProperties *Schema          `json:"additionalProperties,omitempty"`
}

type Components struct {
        Schemas         map[string]*Schema         `json:"schemas,omitempty"`
        SecuritySchemes map[string]*SecurityScheme `json:"securitySchemes,omitempty"`
}

type SecurityScheme struct {
        Type         string `json:"type"`         // http|apiKey|oauth2|...
        Scheme       string `json:"scheme,omitempty"` // bearer|basic
        BearerFormat string `json:"bearerFormat,omitempty"`
        In           string `json:"in,omitempty"`     // header|query|cookie
        Name         string `json:"name,omitempty"`
        Description  string `json:"description,omitempty"`
}

// Builder accumulates operations and produces a Spec.
type Builder struct {
        spec *Spec
}

// NewBuilder returns an empty builder with the standard envelope schemas
// (SuccessEnvelope, ErrorEnvelope, PageMeta) pre-registered.
func NewBuilder(title, version, description string) *Builder {
        b := &Builder{spec: &Spec{
                OpenAPI: "3.1.0",
                Info: Info{Title: title, Version: version, Description: description,
                        Contact: &Contact{Name: "β ORI Inc.", Email: "support@ori-inc.ca"},
                        License: &License{Name: "Copyright 2025 β ORI Inc. Canada", URL: "https://ori-inc.ca"}},
                Paths: map[string]*PathItem{},
                Components: &Components{
                        Schemas:         map[string]*Schema{},
                        SecuritySchemes: map[string]*SecurityScheme{},
                },
                Tags: []Tag{{Name: "users", Description: "User aggregate"}},
        }}
        b.registerStandardSchemas()
        return b
}

// registerStandardSchemas pre-loads the envelope schemas so every response
// can $ref them rather than repeating the shape.
func (b *Builder) registerStandardSchemas() {
        c := b.spec.Components
        c.Schemas["ErrorEnvelope"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{
                        "error": {Ref: "#/components/schemas/APIError"},
                },
                Required: []string{"error"},
        }
        c.Schemas["APIError"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{
                        "code":      {Type: "string", Description: "Stable error code"},
                        "message":   {Type: "string"},
                        "details":   {Type: "array", Items: &Schema{Ref: "#/components/schemas/FieldError"}},
                        "requestId": {Type: "string", Format: "uuid"},
                        "traceId":   {Type: "string"},
                },
                Required: []string{"code", "message"},
        }
        c.Schemas["FieldError"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{
                        "field":   {Type: "string"},
                        "value":   {},
                        "message": {Type: "string"},
                },
                Required: []string{"field", "message"},
        }
        c.Schemas["PageMeta"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{
                        "total":      {Type: "integer", Format: "int64"},
                        "offset":     {Type: "integer"},
                        "limit":      {Type: "integer"},
                        "hasNext":    {Type: "boolean"},
                        "nextCursor": {Type: "string"},
                },
        }
        c.Schemas["Meta"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{
                        "page":       {Ref: "#/components/schemas/PageMeta"},
                        "serverTime": {Type: "string", Format: "date-time"},
                        "requestId":  {Type: "string", Format: "uuid"},
                },
        }
        c.Schemas["Link"] = &Schema{
                Type: "object",
                Properties: map[string]*Schema{"href": {Type: "string"}},
        }
        c.SecuritySchemes["bearerAuth"] = &SecurityScheme{
                Type: "http", Scheme: "bearer", BearerFormat: "JWT",
                Description: "JWT Bearer token issued by /auth/demo-token (or /auth/login in prod).",
        }
        c.SecuritySchemes["apiKeyAuth"] = &SecurityScheme{
                Type: "apiKey", In: "header", Name: "X-API-Key",
                Description: "Static API key for service-to-service calls.",
        }
        b.spec.Security = []map[string][]string{
                {"bearerAuth": {}},
                {"apiKeyAuth": {}},
        }
}

// AddPath registers or extends a path item.
func (b *Builder) AddPath(path string) *PathItem {
        if b.spec.Paths[path] == nil {
                b.spec.Paths[path] = &PathItem{}
        }
        return b.spec.Paths[path]
}

// AddServer appends a server entry.
func (b *Builder) AddServer(url, desc string) *Builder {
        b.spec.Servers = append(b.spec.Servers, Server{URL: url, Description: desc})
        return b
}

// RegisterSchema adds a named schema to components.
func (b *Builder) RegisterSchema(name string, s *Schema) *Builder {
        b.spec.Components.Schemas[name] = s
        return b
}

// Build returns the assembled Spec.
func (b *Builder) Build() *Spec { return b.spec }

// --- HTTP handlers ---------------------------------------------------------

// JSONHandler returns the spec as JSON.
func JSONHandler(s *Spec) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/json; charset=utf-8")
                _ = json.NewEncoder(w).Encode(s)
        }
}

// YAMLHandler returns the spec as YAML (hand-serialized; we keep the
// dependency footprint at zero).
func YAMLHandler(s *Spec) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
                _, _ = w.Write([]byte(specToYAML(s)))
        }
}

// SwaggerUIHandler serves a single-page Swagger UI bound to /openapi.json.
func SwaggerUIHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "text/html; charset=utf-8")
                _, _ = w.Write([]byte(swaggerHTML))
        }
}

// ReDocUIHandler serves ReDoc bound to /openapi.json.
func ReDocUIHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "text/html; charset=utf-8")
                _, _ = w.Write([]byte(redocHTML))
        }
}

// specToYAML is a tiny JSON→YAML converter that handles only the shapes we
// produce. For a richer converter swap in gopkg.in/yaml.v3; we keep zero
// extra deps for the reference impl.
func specToYAML(s *Spec) string {
        b, _ := json.MarshalIndent(s, "", "  ")
        // Naive: emit as JSON-with-yaml-extension; tools like Swagger UI accept
        // both. For a strict YAML, replace this with yaml.v3.
        return string(b)
}

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Hexagonal Go - Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      SwaggerUIBundle({ url: "/openapi.json", dom_id: "#swagger-ui" });
    };
  </script>
</body>
</html>`

const redocHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Hexagonal Go - ReDoc</title>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <style>body { margin: 0; padding: 0; }</style>
</head>
<body>
  <redoc spec-url="/openapi.json"></redoc>
  <script src="https://cdn.jsdelivr.net/npm/redoc@2.1.5/bundles/redoc.standalone.js"></script>
</body>
</html>`

// PathToTag derives a tag from a path like /api/v1/users/{id} -> "users".
func PathToTag(path string) string {
        parts := strings.Split(strings.Trim(path, "/"), "/")
        for i, p := range parts {
                if p == "v1" || p == "v2" {
                        if i+1 < len(parts) {
                                return parts[i+1]
                        }
                }
        }
        if len(parts) > 0 {
                return parts[0]
        }
        return "default"
}
