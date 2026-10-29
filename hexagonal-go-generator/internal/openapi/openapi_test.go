// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// openapi_test.go - unit tests for the OpenAPI request-body validator.
// Run with: go test -short ./internal/openapi/...
package openapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequestValidator_RejectsMissingRequired(t *testing.T) {
	spec := &Spec{
		OpenAPI: "3.1.0",
		Info:    Info{Title: "t", Version: "1"},
		Paths: map[string]*PathItem{
			"/api/v1/users": {
				Post: &Operation{
					RequestBody: &RequestBody{
						Required: true,
						Content: map[string]*MediaType{
							"application/json": {Schema: &Schema{Ref: "#/components/schemas/UserCreateRequest"}},
						},
					},
					Responses: map[string]*Response{"201": {Description: "ok"}},
				},
			},
		},
		Components: &Components{
			Schemas: map[string]*Schema{
				"UserCreateRequest": {
					Type:     "object",
					Required: []string{"email", "first_name", "last_name"},
					Properties: map[string]*Schema{
						"email":      {Type: "string", Format: "email"},
						"first_name": {Type: "string"},
						"last_name":  {Type: "string"},
					},
				},
			},
		},
	}

	called := false
	h := RequestValidator(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(201)
	}))

	// Missing required field "email".
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/users", strings.NewReader(`{"first_name":"A","last_name":"B"}`))
	h.ServeHTTP(rec, req)
	if called {
		t.Fatal("validator should have blocked the request; handler should not be called")
	}
	if rec.Code != 400 {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "VALIDATION_FAILED") {
		t.Fatalf("expected VALIDATION_FAILED in body, got %s", body)
	}
	if !strings.Contains(body, "email") {
		t.Fatalf("expected 'email' field in details, got %s", body)
	}
}

func TestRequestValidator_AcceptsValid(t *testing.T) {
	spec := &Spec{
		OpenAPI: "3.1.0",
		Info:    Info{Title: "t", Version: "1"},
		Paths: map[string]*PathItem{
			"/api/v1/users": {
				Post: &Operation{
					RequestBody: &RequestBody{
						Required: true,
						Content: map[string]*MediaType{
							"application/json": {Schema: &Schema{Ref: "#/components/schemas/UserCreateRequest"}},
						},
					},
					Responses: map[string]*Response{"201": {Description: "ok"}},
				},
			},
		},
		Components: &Components{
			Schemas: map[string]*Schema{
				"UserCreateRequest": {
					Type:     "object",
					Required: []string{"email", "first_name", "last_name"},
					Properties: map[string]*Schema{
						"email":      {Type: "string", Format: "email"},
						"first_name": {Type: "string"},
						"last_name":  {Type: "string"},
					},
				},
			},
		},
	}

	called := false
	h := RequestValidator(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(201)
	}))

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("POST", "/api/v1/users",
		strings.NewReader(`{"email":"alice@example.com","first_name":"Alice","last_name":"Smith"}`))
	h.ServeHTTP(rec, req)
	if !called {
		t.Fatal("validator should have passed the request through to the handler")
	}
	if rec.Code != 201 {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestRequestValidator_PassThroughForGet(t *testing.T) {
	spec := &Spec{
		OpenAPI: "3.1.0",
		Info:    Info{Title: "t", Version: "1"},
		Paths:   map[string]*PathItem{},
	}
	called := false
	h := RequestValidator(spec)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/api/v1/users", nil))
	if !called {
		t.Fatal("GET should pass through without validation")
	}
}

func TestValidateValue_Enum(t *testing.T) {
	spec := &Spec{Components: &Components{Schemas: map[string]*Schema{}}}
	schema := &Schema{Type: "string", Enum: []string{"active", "inactive"}}
	problems := validateValue(spec, "status", "bogus", schema)
	if len(problems) == 0 {
		t.Fatal("expected enum validation to fail for bogus value")
	}
	problems = validateValue(spec, "status", "active", schema)
	if len(problems) != 0 {
		t.Fatalf("expected no problems for valid enum value, got %v", problems)
	}
}
