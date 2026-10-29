// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// repository.go - PostgreSQL adapter for the User aggregate.
//
// Embeds the enterprise repository Base so every call automatically gets
// Layer-7 logging, Prometheus metrics, slow-query WARN, and rows-affected
// instrumentation without us repeating that boilerplate here.
//
// Uses sqlx named queries for clarity and prepared-statement caching under
// the hood via pgx.
package userrepo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/repository"
	"github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// rowModel is the on-disk representation. It is intentionally distinct from
// the domain.User so the schema can evolve without touching the domain.
type rowModel struct {
	ID         string         `db:"id"`
	Email      string         `db:"email"`
	FirstName  string         `db:"first_name"`
	LastName   string         `db:"last_name"`
	Status     string         `db:"status"`
	IsDeleted  bool           `db:"is_deleted"`
	DeletedAt  sql.NullTime   `db:"deleted_at"`
	CreatedAt  time.Time      `db:"created_at"`
	UpdatedAt  time.Time      `db:"updated_at"`
	Version    int            `db:"version"`
}

// Repository is the concrete PostgreSQL adapter.
type Repository struct {
	repository.Base
}

// New wires the repository with the shared pool.
func New(db *sqlx.DB) *Repository {
	return &Repository{Base: repository.NewBase(db, "user")}
}

// GetByID fetches a single non-deleted user by id.
func (r *Repository) GetByID(ctx context.Context, id valueobject.UserID) (*user.User, error) {
	start := time.Now()
	var m rowModel
	err := r.DB.GetContext(ctx, &m,
		`SELECT id, email, first_name, last_name, status, is_deleted, deleted_at,
		        created_at, updated_at, version
		   FROM users
		  WHERE id = $1 AND is_deleted = false`, id.String())
	r.Observe(ctx, "get_by_id", start, 1, err)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, user.ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get user %s: %w", id, err)
	}
	return rowToEntity(m)
}

// List returns one offset page plus the total count.
func (r *Repository) List(ctx context.Context, offset, limit int) ([]*user.User, int64, error) {
	start := time.Now()
	var total int64
	if err := r.DB.GetContext(ctx, &total,
		`SELECT count(*) FROM users WHERE is_deleted = false`); err != nil {
		r.Observe(ctx, "list_count", start, 0, err)
		return nil, 0, err
	}
	var ms []rowModel
	err := r.DB.SelectContext(ctx, &ms,
		`SELECT id, email, first_name, last_name, status, is_deleted, deleted_at,
		        created_at, updated_at, version
		   FROM users
		  WHERE is_deleted = false
		  ORDER BY created_at DESC
		  LIMIT $1 OFFSET $2`, limit, offset)
	r.Observe(ctx, "list", start, int64(len(ms)), err)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*user.User, 0, len(ms))
	for _, m := range ms {
		u, err := rowToEntity(m)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, nil
}

// ListByStatus filters by status (uses a Specification under the hood for
// demonstration).
func (r *Repository) ListByStatus(ctx context.Context, status user.Status, offset, limit int) ([]*user.User, int64, error) {
	start := time.Now()
	var total int64
	if err := r.DB.GetContext(ctx, &total,
		`SELECT count(*) FROM users WHERE is_deleted = false AND status = $1`, string(status)); err != nil {
		r.Observe(ctx, "list_by_status_count", start, 0, err)
		return nil, 0, err
	}
	var ms []rowModel
	err := r.DB.SelectContext(ctx, &ms,
		`SELECT id, email, first_name, last_name, status, is_deleted, deleted_at,
		        created_at, updated_at, version
		   FROM users
		  WHERE is_deleted = false AND status = $1
		  ORDER BY created_at DESC
		  LIMIT $2 OFFSET $3`, string(status), limit, offset)
	r.Observe(ctx, "list_by_status", start, int64(len(ms)), err)
	if err != nil {
		return nil, 0, err
	}
	out := make([]*user.User, 0, len(ms))
	for _, m := range ms {
		u, err := rowToEntity(m)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, u)
	}
	return out, total, nil
}

// Create inserts a new user. The id and timestamps are expected to be set by
// the caller (the service assigns them via value objects).
func (r *Repository) Create(ctx context.Context, u *user.User) error {
	start := time.Now()
	_, err := r.DB.NamedExecContext(ctx,
		`INSERT INTO users (id, email, first_name, last_name, status, is_deleted,
		                    created_at, updated_at, version)
		  VALUES (:id, :email, :first_name, :last_name, :status, false,
		          :created_at, :updated_at, 1)`,
		map[string]any{
			"id":         u.ID.String(),
			"email":      u.Email.String(),
			"first_name": u.FirstName,
			"last_name":  u.LastName,
			"status":     string(u.Status),
			"created_at": u.CreatedAt,
			"updated_at": u.UpdatedAt,
		})
	r.Observe(ctx, "create", start, 1, err)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// Update uses optimistic concurrency via the version column.
func (r *Repository) Update(ctx context.Context, u *user.User) error {
	start := time.Now()
	res, err := r.DB.NamedExecContext(ctx,
		`UPDATE users
		    SET email = :email,
		        first_name = :first_name,
		        last_name = :last_name,
		        status = :status,
		        is_deleted = :is_deleted,
		        deleted_at = :deleted_at,
		        updated_at = :updated_at,
		        version = version + 1
		  WHERE id = :id AND version = :version`,
		map[string]any{
			"id":         u.ID.String(),
			"email":      u.Email.String(),
			"first_name": u.FirstName,
			"last_name":  u.LastName,
			"status":     string(u.Status),
			"is_deleted": u.IsDeleted,
			"deleted_at": u.DeletedAt,
			"updated_at": u.UpdatedAt,
			"version":    u.Version,
		})
	r.Observe(ctx, "update", start, 0, err)
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return user.ErrUserNotFound
	}
	u.Version++
	return nil
}

// SoftDelete sets is_deleted=true and deleted_at=now().
func (r *Repository) SoftDelete(ctx context.Context, id valueobject.UserID) error {
	start := time.Now()
	res, err := r.DB.ExecContext(ctx,
		`UPDATE users SET is_deleted = true, deleted_at = now(), updated_at = now()
		  WHERE id = $1 AND is_deleted = false`, id.String())
	r.Observe(ctx, "soft_delete", start, 0, err)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return user.ErrUserNotFound
	}
	return nil
}

// ExistsByEmail is the uniqueness pre-check used by the service before Insert.
func (r *Repository) ExistsByEmail(ctx context.Context, email valueobject.Email) (bool, error) {
	start := time.Now()
	var exists bool
	err := r.DB.GetContext(ctx, &exists,
		`SELECT EXISTS(SELECT 1 FROM users WHERE email = $1)`, email.String())
	r.Observe(ctx, "exists_by_email", start, 0, err)
	return exists, err
}

// --- mappers (row <-> entity) ------------------------------------------------
//
// Layer-5 Mapper logging: each conversion emits a debug line so we can trace
// schema drift between the on-disk row and the domain entity.

func rowToEntity(m rowModel) (*user.User, error) {
	id, err := valueobject.ParseUserID(m.ID)
	if err != nil {
		return nil, fmt.Errorf("bad id %q: %w", m.ID, err)
	}
	email, err := valueobject.ParseEmail(m.Email)
	if err != nil {
		return nil, fmt.Errorf("bad email %q: %w", m.Email, err)
	}
	u := &user.User{
		ID:        id,
		Email:     email,
		FirstName: m.FirstName,
		LastName:  m.LastName,
		Status:    user.Status(m.Status),
		CreatedAt: m.CreatedAt,
		UpdatedAt: m.UpdatedAt,
		IsDeleted: m.IsDeleted,
		Version:   m.Version,
	}
	if m.DeletedAt.Valid {
		t := m.DeletedAt.Time
		u.DeletedAt = &t
	}
	logger.For(logger.LayerMapper).
		Debug().
		Str("mapper", "rowToEntity").
		Str("entity", "user").
		Str("id", u.ID.String()).
		Msg("MAP_ROW_TO_ENTITY")
	return u, nil
}
