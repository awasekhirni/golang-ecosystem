// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// user_ops.go - registers the User resource's REST operations into the
// OpenAPI builder. Called once at boot from server.BuildOpenAPI().
package openapi

// RegisterUserOps adds every /api/v1/users route to the spec along with the
// User schema, request bodies, and error responses.
func RegisterUserOps(b *Builder) {
	// --- schemas ----------------------------------------------------------
	b.RegisterSchema("UserCreateRequest", &Schema{
		Type: "object",
		Required: []string{"email", "first_name", "last_name"},
		Properties: map[string]*Schema{
			"email":      {Type: "string", Format: "email", Example: "alice@example.com"},
			"first_name": {Type: "string", Example: "Alice"},
			"last_name":  {Type: "string", Example: "Smith"},
		},
	})
	b.RegisterSchema("UserChangeStatusRequest", &Schema{
		Type: "object",
		Required: []string{"status"},
		Properties: map[string]*Schema{
			"status": {Type: "string", Enum: []string{"active", "inactive", "pending"}},
			"reason": {Type: "string"},
		},
	})
	b.RegisterSchema("UserResponse", &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"id":         {Type: "string", Format: "uuid"},
			"email":      {Type: "string", Format: "email"},
			"first_name": {Type: "string"},
			"last_name":  {Type: "string"},
			"full_name":  {Type: "string"},
			"status":     {Type: "string", Enum: []string{"active", "inactive", "pending"}},
			"created_at": {Type: "string", Format: "date-time"},
			"updated_at": {Type: "string", Format: "date-time"},
			"version":    {Type: "integer"},
		},
	})
	b.RegisterSchema("SuccessEnvelope", &Schema{
		Type: "object",
		Properties: map[string]*Schema{
			"data":  {},
			"meta":  {Ref: "#/components/schemas/Meta"},
			"links": {Type: "object", AdditionalProperties: &Schema{Ref: "#/components/schemas/Link"}},
		},
	})

	// --- POST /api/v1/users ------------------------------------------------
	post := b.AddPath("/api/v1/users")
	post.Post = &Operation{
		Tags:        []string{"users"},
		Summary:     "Create a user",
		OperationID: "createUser",
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]*MediaType{
				"application/json": {Schema: &Schema{Ref: "#/components/schemas/UserCreateRequest"}},
			},
		},
		Responses: map[string]*Response{
			"201": {Description: "Created", Content: jsonContent(&Schema{Ref: "#/components/schemas/SuccessEnvelope"}),
				Headers: map[string]*Header{"Location": {Description: "URL of the new resource", Schema: &Schema{Type: "string"}}}},
			"400": refErrorResponse("Bad request / validation failed"),
			"409": refErrorResponse("Email already registered"),
			"500": refErrorResponse("Internal error"),
		},
	}

	// --- GET /api/v1/users -------------------------------------------------
	get := post // same path
	get.Get = &Operation{
		Tags:        []string{"users"},
		Summary:     "List users (offset pagination)",
		OperationID: "listUsers",
		Parameters: []Parameter{
			{Name: "limit", In: "query", Schema: &Schema{Type: "integer", Default: 20}},
			{Name: "offset", In: "query", Schema: &Schema{Type: "integer", Default: 0}},
		},
		Responses: map[string]*Response{
			"200": {Description: "OK", Content: jsonContent(&Schema{Ref: "#/components/schemas/SuccessEnvelope"})},
			"401": refErrorResponse("Unauthorized"),
			"500": refErrorResponse("Internal error"),
		},
	}

	// --- GET /api/v1/users/{id} --------------------------------------------
	byID := b.AddPath("/api/v1/users/{id}")
	byID.Get = &Operation{
		Tags:        []string{"users"},
		Summary:     "Get a user by id",
		OperationID: "getUser",
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
			{Name: "If-None-Match", In: "header", Description: "ETag from a previous response", Schema: &Schema{Type: "string"}},
		},
		Responses: map[string]*Response{
			"200": {Description: "OK", Content: jsonContent(&Schema{Ref: "#/components/schemas/SuccessEnvelope"}),
				Headers: map[string]*Header{"ETag": {Schema: &Schema{Type: "string"}}}},
			"304": {Description: "Not Modified"},
			"404": refErrorResponse("Not found"),
		},
	}

	// --- PATCH /api/v1/users/{id}/status -----------------------------------
	status := b.AddPath("/api/v1/users/{id}/status")
	status.Patch = &Operation{
		Tags:        []string{"users"},
		Summary:     "Change a user's status",
		OperationID: "changeUserStatus",
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
			{Name: "Idempotency-Key", In: "header", Description: "Deduplicates the request for 24h", Schema: &Schema{Type: "string"}},
		},
		RequestBody: &RequestBody{
			Required: true,
			Content: map[string]*MediaType{
				"application/json": {Schema: &Schema{Ref: "#/components/schemas/UserChangeStatusRequest"}},
			},
		},
		Responses: map[string]*Response{
			"200": {Description: "OK", Content: jsonContent(&Schema{Ref: "#/components/schemas/SuccessEnvelope"})},
			"409": refErrorResponse("Invalid status transition"),
			"404": refErrorResponse("Not found"),
		},
	}

	// --- DELETE /api/v1/users/{id} -----------------------------------------
	del := b.AddPath("/api/v1/users/{id}")
	del.Delete = &Operation{
		Tags:        []string{"users"},
		Summary:     "Soft-delete a user",
		OperationID: "deleteUser",
		Parameters: []Parameter{
			{Name: "id", In: "path", Required: true, Schema: &Schema{Type: "string", Format: "uuid"}},
		},
		Responses: map[string]*Response{
			"204": {Description: "No Content"},
			"404": refErrorResponse("Not found"),
		},
	}
}

// jsonContent is the canonical 200 response media type for success envelopes.
func jsonContent(schema *Schema) map[string]*MediaType {
	return map[string]*MediaType{"application/json": {Schema: schema}}
}

// refErrorResponse builds a standard error response referencing the
// ErrorEnvelope schema. Centralized so every operation sees the same shape.
func refErrorResponse(desc string) *Response {
	return &Response{
		Description: desc,
		Content:     jsonContent(&Schema{Ref: "#/components/schemas/ErrorEnvelope"}),
	}
}

// AdditionalProperties is a helper kept here because openapi.go doesn't
// export it; reused above via &Schema{... AdditionalProperties: ...}.
// (We add the field to Schema in spec.go; this is just a marker comment.)
