// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// mapper.go - translates between the domain.User entity and the wire DTOs.
//
// Layer-5 (Mapper) logging: every conversion emits a debug line so we can
// trace schema drift between the domain entity and the DTO.
package usermapper

import (
	"github.com/ori-inc/hexagonal-go/internal/api/dtos/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// ToResponse converts a domain.User into a wire Response DTO.
func ToResponse(u *user.User) userdto.Response {
	logger.For(logger.LayerMapper).
		Debug().
		Str("mapper", "user.ToResponse").
		Str("id", u.ID.String()).
		Msg("MAP_ENTITY_TO_DTO")
	return userdto.Response{
		ID:        u.ID.String(),
		Email:     u.Email.String(),
		FirstName: u.FirstName,
		LastName:  u.LastName,
		FullName:  u.FullName(),
		Status:    string(u.Status),
		CreatedAt: u.CreatedAt,
		UpdatedAt: u.UpdatedAt,
		Version:   u.Version,
	}
}

// ToResponseList maps a slice in one call.
func ToResponseList(us []*user.User) []userdto.Response {
	out := make([]userdto.Response, 0, len(us))
	for _, u := range us {
		out = append(out, ToResponse(u))
	}
	return out
}

// ToEntity converts a CreateRequest DTO into a brand-new domain.User (without
// persisting it). The id+timestamps are minted here so the entity is fully
// populated before reaching the repository.
//
// Returned errors are httpx-typed ValidationFailed so the controller can pass
// them straight to WriteError.
func ToEmail(r userdto.CreateRequest) (valueobject.Email, error) {
	return valueobject.ParseEmail(r.Email)
}
