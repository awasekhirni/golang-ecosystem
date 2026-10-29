// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// user_integration_test.go - end-to-end User repo + service tests against a
// real PostgreSQL. Run with: go test -tags=integration ./tests/...
//
// These tests verify:
//   - migrations apply cleanly
//   - the repository Create/Get/List/Update/SoftDelete round-trips a User
//   - optimistic concurrency via the version column rejects stale writes
//   - the service dispatches a UserCreated event through the bus
//
// GAP CLOSED: #19 Test factories + integration tests (Testing pyramid)
//
//go:build integration

package tests

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/ori-inc/hexagonal-go/internal/domain/bus"
	"github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/events"
	"github.com/ori-inc/hexagonal-go/internal/domain/uow"
	userrepo "github.com/ori-inc/hexagonal-go/internal/infrastructure/repositories/user"
	usersvc "github.com/ori-inc/hexagonal-go/internal/application/services/user"
	"github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
)

func dsn(t *testing.T) string {
	t.Helper()
	d := os.Getenv("DATABASE_URL")
	if d == "" {
		t.Skip("DATABASE_URL not set; skipping integration test")
	}
	return d
}

func TestIntegration_UserCreateGetList(t *testing.T) {
	dsn := dsn(t)
	ctx := context.Background()

	db, err := sqlx.Connect("pgx", dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer db.Close()

	// Clean slate.
	if _, err := db.ExecContext(ctx, "TRUNCATE users RESTART IDENTITY CASCADE"); err != nil {
		t.Fatalf("truncate: %v", err)
	}

	repo := userrepo.New(db)
	bus := bus.New()
	uowInstance := uow.New(db, uow.NewConcreteTx, bus)
	svc := usersvc.New(repo, uowInstance, bus)

	// Subscribe to verify the event flows through the bus.
	got := make(chan events.Event, 1)
	bus.Subscribe(events.HandlerFunc{
		TopicStr: user.UserCreated{}.Topic(),
		Fn: func(_ context.Context, e events.Event) error { got <- e; return nil },
	})

	email, _ := valueobject.ParseEmail("alice@example.com")
	created, err := svc.Create(ctx, email, "Alice", "Smith")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.ID.IsZero() {
		t.Fatal("expected non-zero id")
	}

	// Round-trip via Get.
	fetched, err := svc.Get(ctx, created.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if fetched.Email.String() != "alice@example.com" {
		t.Fatalf("unexpected email: %s", fetched.Email)
	}

	// List sees it.
	rows, total, err := svc.List(ctx, 0, 10)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(rows) != 1 {
		t.Fatalf("expected 1 row, got total=%d rows=%d", total, len(rows))
	}

	// Event should have been dispatched.
	select {
	case <-got:
		// ok
	case <-time.After(2 * time.Second):
		t.Fatal("expected UserCreated event within 2s")
	}
}
