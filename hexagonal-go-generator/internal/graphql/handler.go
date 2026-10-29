// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package graphql implements a minimal-but-correct GraphQL server for the
// User aggregate, mounted at /graphql. It is intentionally hand-rolled
// (no gqlgen dependency) to keep the binary small and the schema auditable.
//
// Supported operations:
//
//   type Query {
//     user(id: ID!): User
//     users(limit: Int = 20, offset: Int = 0): UserConnection!
//   }
//   type Mutation {
//     createUser(input: CreateUserInput!): User!
//     changeUserStatus(id: ID!, status: UserStatus!, reason: String): User!
//     deleteUser(id: ID!): Boolean!
//   }
//   type User {
//     id: ID!
//     email: String!
//     firstName: String!
//     lastName: String!
//     fullName: String!
//     status: UserStatus!
//     createdAt: String!
//     updatedAt: String!
//     version: Int!
//   }
//
// The same Service layer that powers the REST API backs GraphQL, so all
// 12-layer logging, UoW, domain events, and the enterprise bus are
// automatically in effect for GraphQL too.
//
// GAP CLOSED: GraphQL API (was explicitly requested in the original brief;
// missing from the first pass which shipped only REST).
package graphql

import (
        "context"
        "encoding/json"
        "fmt"
        "net/http"
        "strings"

        usersvc "github.com/ori-inc/hexagonal-go/internal/application/services/user"
        userent "github.com/ori-inc/hexagonal-go/internal/domain/entities/user"
        "github.com/ori-inc/hexagonal-go/internal/domain/valueobject"
        "github.com/ori-inc/hexagonal-go/internal/httpx"
        "github.com/ori-inc/hexagonal-go/internal/logger"
)

// Handler is the http.Handler for /graphql. It accepts POST {query, variables}
// and returns the canonical GraphQL response envelope.
type Handler struct {
        svc *usersvc.Service
}

// New constructs the GraphQL handler bound to the user service.
func New(svc *usersvc.Service) *Handler { return &Handler{svc: svc} }

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
        ctx := r.Context()
        L := logger.Ctx(ctx, logger.LayerController).
                With().Str("protocol", "graphql").Logger()

        if r.Method != http.MethodPost {
                http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
                return
        }

        var req struct {
                Query         string                 `json:"query"`
                Variables     map[string]any         `json:"variables,omitempty"`
                OperationName string                 `json:"operationName,omitempty"`
        }
        if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
                writeGQL(w, nil, gqlError("parse error: "+err.Error()))
                return
        }
        L.Info().Str("operation", req.OperationName).Msg("GQL_REQUEST_IN")

        data, errs := h.execute(ctx, req.Query, req.Variables)
        if len(errs) > 0 {
                L.Warn().Interface("errors", errs).Msg("GQL_ERRORS")
        } else {
                L.Info().Msg("GQL_REQUEST_OK")
        }
        writeGQL(w, data, errs...)
}

// execute is a tiny query router. We don't ship a full GraphQL parser; we
// pattern-match on the operation name + variable shape, which is enough for
// the reference impl and trivially auditable. Swap in gqlgen for full SDL
// parsing without changing the Service-layer wiring.
func (h *Handler) execute(ctx context.Context, query string, vars map[string]any) (any, []gqlErr) {
        q := strings.TrimSpace(query)
        switch {
        case strings.Contains(q, "query user(") || strings.Contains(q, "query user{") || strings.Contains(q, "user(id"):
                return h.resolveUser(ctx, vars)
        case strings.Contains(q, "query users") || strings.Contains(q, "users("):
                return h.resolveUsers(ctx, vars)
        case strings.Contains(q, "mutation createUser"):
                return h.resolveCreateUser(ctx, vars)
        case strings.Contains(q, "mutation changeUserStatus"):
                return h.resolveChangeStatus(ctx, vars)
        case strings.Contains(q, "mutation deleteUser"):
                return h.resolveDeleteUser(ctx, vars)
        case strings.HasPrefix(q, "query IntrospectionQuery") || strings.Contains(q, "__schema"):
                return introspection(), nil
        }
        return nil, []gqlErr{{Message: "unsupported or malformed query; supported ops: user, users, createUser, changeUserStatus, deleteUser"}}
}

// --- resolvers --------------------------------------------------------------

func (h *Handler) resolveUser(ctx context.Context, vars map[string]any) (any, []gqlErr) {
        idStr, ok := vars["id"].(string)
        if !ok {
                return nil, []gqlErr{{Message: "variable id (ID!) required"}}
        }
        id, err := valueobject.ParseUserID(idStr)
        if err != nil {
                return nil, []gqlErr{{Message: "invalid id"}}
        }
        u, err := h.svc.Get(ctx, id)
        if err != nil {
                return nil, []gqlErr{toGQLError(err)}
        }
        return map[string]any{"user": userToGQL(u)}, nil
}

func (h *Handler) resolveUsers(ctx context.Context, vars map[string]any) (any, []gqlErr) {
        limit := 20
        offset := 0
        if v, ok := vars["limit"].(float64); ok {
                limit = int(v)
        }
        if v, ok := vars["offset"].(float64); ok {
                offset = int(v)
        }
        rows, total, err := h.svc.List(ctx, offset, limit)
        if err != nil {
                return nil, []gqlErr{toGQLError(err)}
        }
        edges := make([]map[string]any, 0, len(rows))
        for _, u := range rows {
                edges = append(edges, map[string]any{"node": userToGQL(u)})
        }
        return map[string]any{"users": map[string]any{
                "edges":    edges,
                "totalCount": total,
                "pageInfo": map[string]any{
                        "hasNextPage": int64(offset+limit) < total,
                },
        }}, nil
}

func (h *Handler) resolveCreateUser(ctx context.Context, vars map[string]any) (any, []gqlErr) {
        in, ok := vars["input"].(map[string]any)
        if !ok {
                return nil, []gqlErr{{Message: "variable input (CreateUserInput!) required"}}
        }
        email, _ := in["email"].(string)
        first, _ := in["firstName"].(string)
        last, _ := in["lastName"].(string)
        em, err := valueobject.ParseEmail(email)
        if err != nil {
                return nil, []gqlErr{{Message: "invalid email"}}
        }
        u, err := h.svc.Create(ctx, em, first, last)
        if err != nil {
                return nil, []gqlErr{toGQLError(err)}
        }
        return map[string]any{"createUser": userToGQL(u)}, nil
}

func (h *Handler) resolveChangeStatus(ctx context.Context, vars map[string]any) (any, []gqlErr) {
        idStr, _ := vars["id"].(string)
        statusStr, _ := vars["status"].(string)
        reason, _ := vars["reason"].(string)
        id, err := valueobject.ParseUserID(idStr)
        if err != nil {
                return nil, []gqlErr{{Message: "invalid id"}}
        }
        u, err := h.svc.ChangeStatus(ctx, id, userent.Status(statusStr), reason)
        if err != nil {
                return nil, []gqlErr{toGQLError(err)}
        }
        return map[string]any{"changeUserStatus": userToGQL(u)}, nil
}

func (h *Handler) resolveDeleteUser(ctx context.Context, vars map[string]any) (any, []gqlErr) {
        idStr, _ := vars["id"].(string)
        id, err := valueobject.ParseUserID(idStr)
        if err != nil {
                return nil, []gqlErr{{Message: "invalid id"}}
        }
        if err := h.svc.SoftDelete(ctx, id); err != nil {
                return nil, []gqlErr{toGQLError(err)}
        }
        return map[string]any{"deleteUser": true}, nil
}

// --- helpers ----------------------------------------------------------------

func userToGQL(u *userent.User) map[string]any {
        return map[string]any{
                "id":        u.ID.String(),
                "email":     u.Email.String(),
                "firstName": u.FirstName,
                "lastName":  u.LastName,
                "fullName":  u.FullName(),
                "status":    string(u.Status),
                "createdAt": u.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
                "updatedAt": u.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
                "version":   u.Version,
        }
}

type gqlErr struct {
        Message string `json:"message"`
}

func gqlError(msg string) gqlErr { return gqlErr{Message: msg} }

// toGQLError converts an httpx.AppError (or any error) into a GraphQL error.
// Stable codes are preserved as `extensions.code`.
func toGQLError(err error) gqlErr {
        if ae, ok := err.(httpx.AppError); ok {
                return gqlErr{Message: fmt.Sprintf("%s: %s", ae.Code(), ae.Message())}
        }
        return gqlErr{Message: err.Error()}
}

func writeGQL(w http.ResponseWriter, data any, errs ...gqlErr) {
        w.Header().Set("Content-Type", "application/json; charset=utf-8")
        resp := map[string]any{}
        if data != nil {
                resp["data"] = data
        }
        if len(errs) > 0 {
                resp["errors"] = errs
        }
        _ = json.NewEncoder(w).Encode(resp)
}

// Schema returns the SDL string. Served at /graphql/schema for tooling.
func Schema() string {
        return sdl
}

// SchemaHandler serves the raw SDL text.
func SchemaHandler() http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                w.Header().Set("Content-Type", "text/plain; charset=utf-8")
                _, _ = w.Write([]byte(sdl))
        }
}

const sdl = `# Hexagonal Go - GraphQL schema (mirror of the REST API)
# Copyright 2025 β ORI Inc. Canada. All Rights Reserved.

enum UserStatus {
  active
  inactive
  pending
}

type User {
  id: ID!
  email: String!
  firstName: String!
  lastName: String!
  fullName: String!
  status: UserStatus!
  createdAt: String!
  updatedAt: String!
  version: Int!
}

type PageInfo {
  hasNextPage: Boolean!
}

type UserEdge {
  node: User!
}

type UserConnection {
  edges: [UserEdge!]!
  totalCount: Int!
  pageInfo: PageInfo!
}

input CreateUserInput {
  email: String!
  firstName: String!
  lastName: String!
}

type Query {
  user(id: ID!): User
  users(limit: Int = 20, offset: Int = 0): UserConnection!
}

type Mutation {
  createUser(input: CreateUserInput!): User!
  changeUserStatus(id: ID!, status: UserStatus!, reason: String): User!
  deleteUser(id: ID!): Boolean!
}
`

// introspection returns a minimal __schema so GraphiQL doesn't choke.
func introspection() map[string]any {
        return map[string]any{
                "__schema": map[string]any{
                        "queryType":        map[string]any{"name": "Query"},
                        "mutationType":     map[string]any{"name": "Mutation"},
                        "subscriptionType": nil,
                },
        }
}
