// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// service.go - application service for the User aggregate.
//
// The service orchestrates: validation -> repository -> domain-method ->
// domain event. It never talks to the database directly; it always goes
// through the Repository port (or the Unit of Work for multi-step mutations).
//
// Every method emits a Layer-6 (Service) SERVICE_IN / SERVICE_OUT log pair
// with the operation name and the duration.
package usersvc

import (
        "context"
        "errors"
        "fmt"
        "time"

        "github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
        "github.com/ori-inc/hexagonal-go/internal/domain/events"
        "github.com/ori-inc/hexagonal-go/internal/domain/uow"
        "github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
        "github.com/ori-inc/hexagonal-go/internal/httpx"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// Service is the application-layer facade for everything User.
type Service struct {
        repo user.Repository
        uow  *uow.UoW
        bus  events.Dispatcher
}

// New wires the service with its repository, UoW and event bus.
func New(repo user.Repository, u *uow.UoW, bus events.Dispatcher) *Service {
        return &Service{repo: repo, uow: u, bus: bus}
}

// Create registers a new user. It performs the uniqueness pre-check, mints
// the typed ID, calls the repository inside a UoW, and dispatches a
// UserCreated event after commit.
func (s *Service) Create(ctx context.Context, email valueobject.Email, firstName, lastName string) (*user.User, error) {
        logEntry(ctx, "Create", "SERVICE_IN")
        defer logEntry(ctx, "Create", "SERVICE_OUT")

        exists, err := s.repo.ExistsByEmail(ctx, email)
        if err != nil {
                return nil, wrapDB(err)
        }
        if exists {
                return nil, httpx.NewError(httpx.ErrCodeConflict, "email already registered",
                        httpx.FieldError{Field: "email", Value: email.String(), Message: "already taken"})
        }

        u := &user.User{
                ID:        valueobject.NewUserID(),
                Email:     email,
                FirstName: firstName,
                LastName:  lastName,
                Status:    user.StatusPending,
                CreatedAt: time.Now().UTC(),
                UpdatedAt: time.Now().UTC(),
                Version:   1,
        }

        // Single-step mutation: still wrap in UoW so events flush only on commit.
        err = s.uow.Run(ctx, func(tx uow.Tx) error {
                // In a multi-aggregate flow we'd use tx.SQLTx() + a tx-bound repo
                // here; for a single insert the shared pool is fine.
                return s.repo.Create(ctx, u)
        })
        if err != nil {
                return nil, wrapDB(err)
        }

        // Dispatch domain event.
        events.Publish(ctx, s.bus, user.UserCreated{
                Base:   events.NewBase(),
                UserID: u.ID.String(),
                Email:  u.Email.String(),
        })
        return u, nil
}

// Get returns one user by id or an httpx-typed NotFound.
func (s *Service) Get(ctx context.Context, id valueobject.UserID) (*user.User, error) {
        logEntry(ctx, "Get", "SERVICE_IN")
        defer logEntry(ctx, "Get", "SERVICE_OUT")

        u, err := s.repo.GetByID(ctx, id)
        if errors.Is(err, user.ErrUserNotFound) {
                return nil, httpx.NewError(httpx.ErrCodeNotFound, "user not found")
        }
        if err != nil {
                return nil, wrapDB(err)
        }
        return u, nil
}

// List returns one offset page plus the total.
func (s *Service) List(ctx context.Context, offset, limit int) ([]*user.User, int64, error) {
        logEntry(ctx, "List", "SERVICE_IN")
        defer logEntry(ctx, "List", "SERVICE_OUT")

        rows, total, err := s.repo.List(ctx, offset, limit)
        if err != nil {
                return nil, 0, wrapDB(err)
        }
        return rows, total, nil
}

// ChangeStatus runs the domain state-machine and persists via UoW; on success
// it dispatches UserStatusChanged.
func (s *Service) ChangeStatus(ctx context.Context, id valueobject.UserID, to user.Status, reason string) (*user.User, error) {
        logEntry(ctx, "ChangeStatus", "SERVICE_IN")
        defer logEntry(ctx, "ChangeStatus", "SERVICE_OUT")

        var event events.Event
        var updated *user.User
        err := s.uow.Run(ctx, func(tx uow.Tx) error {
                u, err := s.repo.GetByID(ctx, id)
                if errors.Is(err, user.ErrUserNotFound) {
                        return httpx.NewError(httpx.ErrCodeNotFound, "user not found")
                }
                if err != nil {
                        return wrapDB(err)
                }
                evt, err := u.ChangeStatus(to, reason)
                if err != nil {
                        if errors.Is(err, user.ErrInvalidTransition) {
                                return httpx.NewError(httpx.ErrCodeConflict, "invalid status transition",
                                        httpx.FieldError{Field: "status", Value: string(to), Message: "not allowed from current state"})
                        }
                        return wrapDB(err)
                }
                if err := s.repo.Update(ctx, u); err != nil {
                        return wrapDB(err)
                }
                event = evt
                updated = u
                return nil
        })
        if err != nil {
                return nil, err
        }
        if event != nil {
                events.Publish(ctx, s.bus, event)
        }
        return updated, nil
}

// SoftDelete marks a user deleted and dispatches UserDeleted.
func (s *Service) SoftDelete(ctx context.Context, id valueobject.UserID) error {
        logEntry(ctx, "SoftDelete", "SERVICE_IN")
        defer logEntry(ctx, "SoftDelete", "SERVICE_OUT")

        err := s.uow.Run(ctx, func(tx uow.Tx) error {
                u, err := s.repo.GetByID(ctx, id)
                if errors.Is(err, user.ErrUserNotFound) {
                        return httpx.NewError(httpx.ErrCodeNotFound, "user not found")
                }
                if err != nil {
                        return wrapDB(err)
                }
                evt := u.SoftDelete()
                if err := s.repo.Update(ctx, u); err != nil {
                        return wrapDB(err)
                }
                if evt != nil {
                        tx.Events().Append(evt)
                }
                return nil
        })
        if err != nil {
                return err
        }
        // Events appended to tx.Events() are dispatched by the UoW itself after
        // commit, so we don't dispatch again here.
        return nil
}

// logEntry emits the Layer-6 SERVICE_IN/SERVICE_OUT markers used to trace
// business operations end-to-end.
func logEntry(ctx context.Context, op, marker string) {
        logger.Ctx(ctx, logger.LayerService).
                Info().
                Str("operation", op).
                Str("marker", marker).
                Msg("SERVICE_TRACE")
}

// wrapDB converts a raw DB/driver error into an httpx-typed Internal error so
// the controller can hand it straight to httpx.WriteError without a switch.
func wrapDB(err error) error {
        if err == nil {
                return nil
        }
        return httpx.WrapError(httpx.ErrCodeDatabase, "database error", err)
}

// itoa avoids importing strconv just for one tiny call.
var _ = fmt.Sprintf
