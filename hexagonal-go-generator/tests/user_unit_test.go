// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// user_unit_test.go - pure unit tests that don't need a database.
// Build with: go test -short ./...
//
// These cover the aggregate invariants (state machine) and the test factory
// itself. Integration tests live in user_integration_test.go and run only
// under the integration build tag.
package tests

import (
	"testing"

	"github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
)

func TestUserBuilder_Defaults(t *testing.T) {
	u := NewUserBuilder().Build()
	if u.ID.IsZero() {
		t.Fatal("expected non-zero ID")
	}
	if u.Email.IsZero() {
		t.Fatal("expected non-zero email")
	}
	if u.Status != user.StatusPending {
		t.Fatalf("expected pending, got %s", u.Status)
	}
}

func TestUserBuilder_Overrides(t *testing.T) {
	u := NewUserBuilder().WithEmail("alice@example.com").WithStatus(user.StatusActive).Build()
	if u.Email.String() != "alice@example.com" {
		t.Fatalf("unexpected email %s", u.Email)
	}
	if u.Status != user.StatusActive {
		t.Fatalf("unexpected status %s", u.Status)
	}
}

func TestUser_ChangeStatus_AllowedTransitions(t *testing.T) {
	u := NewUserBuilder().Build()

	// pending -> active is allowed.
	evt, err := u.ChangeStatus(user.StatusActive, "approved")
	if err != nil {
		t.Fatalf("pending->active should succeed, got %v", err)
	}
	if evt == nil {
		t.Fatal("expected event")
	}
	if u.Status != user.StatusActive {
		t.Fatalf("expected active, got %s", u.Status)
	}

	// active -> inactive is allowed.
	_, err = u.ChangeStatus(user.StatusInactive, "vacation")
	if err != nil {
		t.Fatalf("active->inactive should succeed, got %v", err)
	}
}

func TestUser_ChangeStatus_IllegalTransition(t *testing.T) {
	u := NewUserBuilder().Build()
	_, err := u.ChangeStatus(user.StatusInactive, "")
	if err == nil {
		t.Fatal("pending->inactive should be rejected")
	}
}

func TestUser_SoftDelete_Idempotent(t *testing.T) {
	u := NewUserBuilder().Build()
	evt := u.SoftDelete()
	if evt == nil {
		t.Fatal("expected event on first delete")
	}
	evt2 := u.SoftDelete()
	if evt2 != nil {
		t.Fatal("expected no event on second delete")
	}
	if !u.IsDeleted || u.DeletedAt == nil {
		t.Fatal("expected IsDeleted + DeletedAt set")
	}
}
