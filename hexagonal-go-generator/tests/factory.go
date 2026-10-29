// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// factory.go - test data factories for the User aggregate.
//
// Factories centralise "make me a valid user" so tests don't reinvent it.
// Each builder method overrides one field; the defaults always produce a
// valid object so a test that doesn't care about a particular field can
// ignore it.
//
// Pattern: the Builder stores mutations and the Build() method applies them
// last, so partial overrides don't fight each other.
//
// GAP CLOSED: #19 Test factories + integration tests (Testing pyramid)
package tests

import (
	"time"

	"github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
)

// UserBuilder mints valid User instances for tests.
type UserBuilder struct {
	mutators []func(*user.User)
}

// NewUserBuilder returns a builder pre-loaded with sane defaults.
func NewUserBuilder() *UserBuilder {
	return &UserBuilder{
		mutators: []func(*user.User){
			func(u *user.User) {
				u.ID = valueobject.NewUserID()
				email, _ := valueobject.ParseEmail("test-" + u.ID.String()[:8] + "@example.com")
				u.Email = email
				u.FirstName = "Test"
				u.LastName = "User"
				u.Status = user.StatusPending
				u.CreatedAt = time.Now().UTC()
				u.UpdatedAt = time.Now().UTC()
				u.Version = 1
			},
		},
	}
}

// WithEmail overrides the email.
func (b *UserBuilder) WithEmail(email string) *UserBuilder {
	b.mutators = append(b.mutators, func(u *user.User) {
		if e, err := valueobject.ParseEmail(email); err == nil {
			u.Email = e
		}
	})
	return b
}

// WithStatus overrides the status.
func (b *UserBuilder) WithStatus(s user.Status) *UserBuilder {
	b.mutators = append(b.mutators, func(u *user.User) { u.Status = s })
	return b
}

// WithName overrides first/last name.
func (b *UserBuilder) WithName(first, last string) *UserBuilder {
	b.mutators = append(b.mutators, func(u *user.User) {
		u.FirstName = first
		u.LastName = last
	})
	return b
}

// Build returns the assembled User.
func (b *UserBuilder) Build() *user.User {
	u := &user.User{}
	for _, m := range b.mutators {
		m(u)
	}
	return u
}
