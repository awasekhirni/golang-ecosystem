// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// dto.go - request/response DTOs for the User aggregate.
//
// DTOs are intentionally distinct from the domain entity so the wire format
// can evolve independently of the domain. The mapper (mapper.go) translates
// between them.
package userdto

import "time"

// CreateRequest is the body of POST /api/v1/users.
type CreateRequest struct {
	Email     string `json:"email"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// UpdateRequest is the body of PATCH /api/v1/users/{id}. All fields optional.
type UpdateRequest struct {
	FirstName *string `json:"first_name,omitempty"`
	LastName  *string `json:"last_name,omitempty"`
}

// ChangeStatusRequest is the body of POST /api/v1/users/{id}/status.
type ChangeStatusRequest struct {
	Status string `json:"status"`
	Reason string `json:"reason,omitempty"`
}

// Response is the body of every 2xx that returns a single user.
type Response struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FirstName string    `json:"first_name"`
	LastName  string    `json:"last_name"`
	FullName  string    `json:"full_name"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
	Version   int       `json:"version"`
}
