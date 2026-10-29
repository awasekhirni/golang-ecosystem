// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// response.go - high-level write helpers that bind envelope + pagination +
// HATEOAS together so controllers stay one-liners.
package httpx

import (
	"context"
	"net/http"

	"github.com/ori-inc/hexagonal-go/internal/logger"
)

// PagedResult is what services return for collection queries.
// RowsT is the concrete row type the controller will marshal.
type PagedResult[T any] struct {
	Rows        []T
	Total       int64
	HasNext     bool
	NextCursor  string
}

// WritePaged writes a paginated collection envelope with HATEOAS links.
// `baseURL` is the canonical collection URL (e.g. /api/v1/users) used to
// construct `self`, `next`, `prev`, `create` links.
func WritePaged[T any](ctx context.Context, w http.ResponseWriter, r *http.Request,
	baseURL string, req PageReq, res PagedResult[T]) {

	meta := Meta{
		Page:      PageMetaFromReq(req, res.Total, res.HasNext, res.NextCursor),
		ServerTime: nowISO(),
		RequestID:  logger.RequestIDFrom(ctx),
	}

	links := NewLinks().
		Self(r.URL.String()).
		Create(baseURL)
	if res.HasNext {
		if req.Mode == PageModeCursor {
			links.Next(BuildCursorLink(baseURL, res.NextCursor, req.Limit))
		} else {
			links.Next(BuildOffsetLink(baseURL, req.Offset+req.Limit, req.Limit))
			if req.Offset > 0 {
				prev := req.Offset - req.Limit
				if prev < 0 {
					prev = 0
				}
				links.Prev(BuildOffsetLink(baseURL, prev, req.Limit))
			}
		}
	}

	env := SuccessEnvelope{
		Data:  res.Rows,
		Meta:  meta,
		Links: links.Build(),
	}
	writeJSON(w, http.StatusOK, env)
}
