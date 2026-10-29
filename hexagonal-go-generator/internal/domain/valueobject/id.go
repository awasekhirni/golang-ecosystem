// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package valueobject provides DDD value objects for identifiers and other
// invariants. The flagship types are:
//
//   UserID  - typed UUID wrapper so you can't pass an OrderID where a UserID
//             is expected (the classic primitive-obsession bug).
//   Email   - validates RFC-ish shape, normalizes to lowercase, refuses "".
//   Slug    - URL-safe identifier with case+dash normalization.
//
// GAP CLOSED: #13 Value Objects for IDs
package valueobject

import (
	"errors"
	"regexp"
	"strings"

	"github.com/google/uuid"
)

// ID is the generic typed identifier. Embed it in any domain entity so the
// entity's id becomes a strongly-typed value object rather than a bare string.
type ID[T any] struct {
	value uuid.UUID
}

// NewID mints a fresh random ID.
func NewID[T any]() ID[T] { return ID[T]{value: uuid.New()} }

// ParseID converts a string to a typed ID, returning an error on bad format.
func ParseID[T any](s string) (ID[T], error) {
	u, err := uuid.Parse(s)
	if err != nil {
		return ID[T]{}, err
	}
	return ID[T]{value: u}, nil
}

func (i ID[T]) String() string        { return i.value.String() }
func (i ID[T]) IsZero() bool          { return i.value == uuid.UUID{} }
func (i ID[T]) Equal(o ID[T]) bool    { return i.value == o.value }

// --- Concrete typed IDs ----------------------------------------------------
//
// Phantom types used to specialize ID[T]. They never hold state; their only
// purpose is to make ID[UserMarker] a different compile-time type than
// ID[OrderMarker].

type UserMarker struct{}
type OrderMarker struct{}
type ProductMarker struct{}

// UserID, OrderID, ProductID are the convenience aliases used across the
// codebase. Add more as you grow the domain.
type UserID = ID[UserMarker]
type OrderID = ID[OrderMarker]
type ProductID = ID[ProductMarker]

// NewUserID, ParseUserID are the constructors callers actually use.
func NewUserID() UserID                { return NewID[UserMarker]() }
func ParseUserID(s string) (UserID, error) { return ParseID[UserMarker](s) }

// --- Email value object ----------------------------------------------------

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)

// Email is a validated, normalized email address.
type Email struct{ value string }

// ParseEmail validates and lowercases the input.
func ParseEmail(s string) (Email, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" {
		return Email{}, errors.New("email is required")
	}
	if !emailRE.MatchString(s) {
		return Email{}, errors.New("invalid email format")
	}
	return Email{value: s}, nil
}

func (e Email) String() string  { return e.value }
func (e Email) IsZero() bool    { return e.value == "" }
func (e Email) Equal(o Email) bool { return e.value == o.value }

// --- Slug value object -----------------------------------------------------

var slugRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Slug is a URL-safe kebab-case identifier.
type Slug struct{ value string }

// ParseSlug lowercases, trims, collapses spaces to dashes, and validates.
func ParseSlug(s string) (Slug, error) {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	if !slugRE.MatchString(s) {
		return Slug{}, errors.New("invalid slug")
	}
	return Slug{value: s}, nil
}

func (s Slug) String() string { return s.value }
