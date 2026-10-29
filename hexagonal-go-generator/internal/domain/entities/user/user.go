// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package user is the reference domain aggregate. It demonstrates every
// tactical DDD building block in the codebase:
//
//   - UserID value object (typed, can't be confused with OrderID)
//   - Email value object (validated, normalized)
//   - Domain events (UserCreated, UserStatusChanged)
//   - Aggregate methods that produce events (the entity is in charge of its
//     own invariants; the service just calls the method)
//
// The repository + service interfaces live in this same package so the
// domain stays self-contained (hexagonal "ports" pattern).
package user

import (
        "context"
        "errors"
        "time"

        "github.com/ori-inc/hexagonal-go/internal/domain/events"
        "github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
)

// Status enumerates the lifecycle states of a user.
type Status string

const (
        StatusActive   Status = "active"
        StatusInactive Status = "inactive"
        StatusPending  Status = "pending"
)

// User is the aggregate root.
type User struct {
        ID        valueobject.UserID
        Email     valueobject.Email
        FirstName string
        LastName  string
        Status    Status
        CreatedAt time.Time
        UpdatedAt time.Time
        // IsDeleted + DeletedAt implement soft-delete.
        IsDeleted  bool
        DeletedAt  *time.Time
        // version is the optimistic-concurrency token; repositories use it in
        // UPDATE ... WHERE version = $old.
        Version int
}

// --- Aggregate methods (own their invariants) ------------------------------

// ChangeStatus transitions the user to a new status and records a domain
// event. Illegal transitions return an error without mutating the aggregate.
func (u *User) ChangeStatus(new Status, reason string) (events.Event, error) {
        if u.IsDeleted {
                return nil, ErrDeletedUser
        }
        if !isValidTransition(u.Status, new) {
                return nil, ErrInvalidTransition
        }
        old := u.Status
        u.Status = new
        u.UpdatedAt = time.Now().UTC()
        return UserStatusChanged{
                Base:   events.NewBase(),
                UserID: u.ID.String(),
                From:   string(old),
                To:     string(new),
                Reason: reason,
        }, nil
}

// SoftDelete marks the user deleted. Idempotent.
func (u *User) SoftDelete() events.Event {
        if u.IsDeleted {
                return nil
        }
        now := time.Now().UTC()
        u.IsDeleted = true
        u.DeletedAt = &now
        u.UpdatedAt = now
        return UserDeleted{
                Base:   events.NewBase(),
                UserID: u.ID.String(),
        }
}

// FullName is a tiny derived value.
func (u *User) FullName() string { return u.FirstName + " " + u.LastName }

// isValidTransition encodes the state machine.
//
//   pending  -> active   (admin approves signup)
//   active   -> inactive (admin suspends)
//   inactive -> active   (admin restores)
//
// Rejection of a signup is modelled as SoftDelete, not as a transition to
// inactive - so pending->inactive is intentionally forbidden.
func isValidTransition(from, to Status) bool {
        switch from {
        case StatusPending:
                return to == StatusActive
        case StatusActive:
                return to == StatusInactive
        case StatusInactive:
                return to == StatusActive
        }
        return false
}

// Domain errors. These implement httpx.AppError indirectly via the
// service-layer wrappers in service.go.
var (
        ErrDeletedUser       = errors.New("user is deleted")
        ErrInvalidTransition = errors.New("invalid status transition")
        ErrEmailTaken        = errors.New("email already taken")
        ErrUserNotFound      = errors.New("user not found")
)

// --- Domain events ----------------------------------------------------------

// UserCreated is published when a new user signs up.
type UserCreated struct {
        events.Base
        UserID string `json:"userId"`
        Email  string `json:"email"`
}

func (UserCreated) Topic() string { return "user.created" }

// UserStatusChanged is published on every status transition.
type UserStatusChanged struct {
        events.Base
        UserID string `json:"userId"`
        From   string `json:"from"`
        To     string `json:"to"`
        Reason string `json:"reason"`
}

func (UserStatusChanged) Topic() string { return "user.status_changed" }

// UserDeleted is published when a user is soft-deleted.
type UserDeleted struct {
        events.Base
        UserID string `json:"userId"`
}

func (UserDeleted) Topic() string { return "user.deleted" }

// --- Ports (repository interface) ------------------------------------------
//
// The interface lives in the domain layer. The infrastructure layer provides
// the concrete PostgreSQL implementation. This is the heart of the hexagonal
// "ports and adapters" pattern.

type Repository interface {
        GetByID(ctx context.Context, id valueobject.UserID) (*User, error)
        List(ctx context.Context, offset, limit int) ([]*User, int64, error)
        ListByStatus(ctx context.Context, status Status, offset, limit int) ([]*User, int64, error)
        Create(ctx context.Context, u *User) error
        Update(ctx context.Context, u *User) error
        SoftDelete(ctx context.Context, id valueobject.UserID) error
        ExistsByEmail(ctx context.Context, email valueobject.Email) (bool, error)
}
