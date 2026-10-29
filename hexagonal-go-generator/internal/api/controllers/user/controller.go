// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// controller.go - HTTP handlers for /api/v1/users.
//
// The controller is intentionally thin: parse -> validate -> call service ->
// write envelope. All cross-cutting concerns (auth, rbac, audit, request id,
// logging, metrics) are handled by middleware before the request reaches
// here, so the controller can focus on orchestration.
//
// Layer-3 (Controller) logging: action + masked body + response status.
//
// Routes (mounted in server.go):
//
//   GET    /api/v1/users            list (offset pagination)
//   POST   /api/v1/users            create
//   GET    /api/v1/users/{id}       get one
//   PATCH  /api/v1/users/{id}/status change status
//   DELETE /api/v1/users/{id}       soft delete
//
// All routes are behind JWT/RBAC middleware (RoleAdmin or RoleUser).
package usercontroller

import (
        "context"
        "encoding/json"
        "net/http"
        "strings"

        "github.com/go-chi/chi/v5"
        "github.com/ori-inc/hexagonal-go/internal/api/dtos/user"
        usermapper "github.com/ori-inc/hexagonal-go/internal/api/mappers/user"
        uservalidation "github.com/ori-inc/hexagonal-go/internal/api/validations/user"
        "github.com/ori-inc/hexagonal-go/internal/application/services/user"
        "github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
        "github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
        "github.com/ori-inc/hexagonal-go/internal/httpx"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// Controller wires the user service to HTTP routes.
type Controller struct {
        svc *usersvc.Service
}

// New constructs the controller.
func New(svc *usersvc.Service) *Controller { return &Controller{svc: svc} }

// Register mounts the controller's routes on the given router.
func (c *Controller) Register(r chi.Router) {
        r.Route("/users", func(r chi.Router) {
                r.Get("/", c.list)
                r.Post("/", c.create)
                r.Get("/{id}", c.get)
                r.Patch("/{id}/status", c.changeStatus)
                r.Delete("/{id}", c.softDelete)
        })
}

// --- handlers ---------------------------------------------------------------

func (c *Controller) create(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        logCtrl(ctx, "create", "IN")

        var req userdto.CreateRequest
        if err := decodeJSON(r, &req); err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid JSON body"))
                return
        }
        if err := uservalidation.ValidateCreate(ctx, req); err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }

        email, err := usermapper.ToEmail(req)
        if err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeValidationFailed, "invalid email",
                        httpx.FieldError{Field: "email", Value: req.Email, Message: err.Error()}))
                return
        }

        u, err := c.svc.Create(ctx, email, req.FirstName, req.LastName)
        if err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }

        loc := "/api/v1/users/" + u.ID.String()
        httpx.WriteCreated(ctx, w, loc, usermapper.ToResponse(u))
        logCtrl(ctx, "create", "OUT 201")
}

func (c *Controller) list(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        logCtrl(ctx, "list", "IN")

        page, err := httpx.ParsePage(r)
        if err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid pagination: "+err.Error()))
                return
        }
        if page.Mode != httpx.PageModeOffset {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "this endpoint uses offset pagination; drop cursor"))
                return
        }

        rows, total, err := c.svc.List(ctx, page.Offset, page.Limit)
        if err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }
        hasNext := int64(page.Offset+page.Limit) < total
        res := httpx.PagedResult[userdto.Response]{
                Rows:    usermapper.ToResponseList(rows),
                Total:   total,
                HasNext: hasNext,
        }
        httpx.WritePaged(ctx, w, r, "/api/v1/users", page, res)
        logCtrl(ctx, "list", "OUT 200")
}

func (c *Controller) get(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        logCtrl(ctx, "get", "IN")

        id, err := parseID(chi.URLParam(r, "id"))
        if err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid id"))
                return
        }
        u, err := c.svc.Get(ctx, id)
        if err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }
        httpx.WriteJSON(ctx, w, usermapper.ToResponse(u))
        logCtrl(ctx, "get", "OUT 200")
}

func (c *Controller) changeStatus(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        logCtrl(ctx, "changeStatus", "IN")

        id, err := parseID(chi.URLParam(r, "id"))
        if err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid id"))
                return
        }
        var req userdto.ChangeStatusRequest
        if err := decodeJSON(r, &req); err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid JSON body"))
                return
        }
        if err := uservalidation.ValidateChangeStatus(ctx, req); err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }
        u, err := c.svc.ChangeStatus(ctx, id, user.Status(req.Status), req.Reason)
        if err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }
        httpx.WriteJSON(ctx, w, usermapper.ToResponse(u))
        logCtrl(ctx, "changeStatus", "OUT 200")
}

func (c *Controller) softDelete(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        logCtrl(ctx, "softDelete", "IN")

        id, err := parseID(chi.URLParam(r, "id"))
        if err != nil {
                httpx.WriteError(ctx, w, httpx.NewError(httpx.ErrCodeBadRequest, "invalid id"))
                return
        }
        if err := c.svc.SoftDelete(ctx, id); err != nil {
                httpx.WriteError(ctx, w, err)
                return
        }
        httpx.WriteNoContent(ctx, w)
        logCtrl(ctx, "softDelete", "OUT 204")
}

// --- helpers ----------------------------------------------------------------

func parseID(s string) (valueobject.UserID, error) {
        s = strings.TrimSpace(s)
        return valueobject.ParseUserID(s)
}

func decodeJSON(r *http.Request, v any) error {
        dec := json.NewDecoder(r.Body)
        dec.DisallowUnknownFields()
        return dec.Decode(v)
}

func logCtrl(ctx context.Context, action, marker string) {
        logger.Ctx(ctx, logger.LayerController).
                Info().
                Str("action", action).
                Str("marker", marker).
                Msg("CONTROLLER_TRACE")
}
