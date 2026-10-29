// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// pagination.go - cursor + offset pagination primitives.
//
// Two strategies are supported simultaneously because real APIs need both:
//
//   - OFFSET pagination: simple, page-based, fine for small tables.
//   - CURSOR pagination: stable under writes, scales to millions of rows;
//     the cursor is opaque base64 of "<last_seen_value>:<last_seen_id>".
//
// GAP CLOSED: #8 Cursor + offset pagination envelope
package httpx

import (
        "encoding/base64"
        "encoding/json"
        "fmt"
        "net/http"
        "net/url"
        "strconv"
        "strings"
)

// PageReq is parsed from query string. Either Offset/Cursor mode is auto-detected.
type PageReq struct {
        Mode   PageMode
        Offset int
        Limit  int
        Cursor string // opaque
}

type PageMode int

const (
        PageModeOffset PageMode = iota
        PageModeCursor
)

const (
        defaultLimit  = 20
        maxLimit      = 200
        defaultOffset = 0
)

// ParsePage reads `page`, `limit` (offset mode) or `cursor`, `limit`
// (cursor mode) from the request URL.
func ParsePage(r *http.Request) (PageReq, error) {
        q := r.URL.Query()
        limit := atoiDefault(q.Get("limit"), defaultLimit)
        if limit < 1 {
                limit = defaultLimit
        }
        if limit > maxLimit {
                limit = maxLimit
        }

        cursor := strings.TrimSpace(q.Get("cursor"))
        if cursor != "" {
                if _, _, err := DecodeCursor(cursor); err != nil {
                        return PageReq{}, fmt.Errorf("invalid cursor: %w", err)
                }
                return PageReq{Mode: PageModeCursor, Limit: limit, Cursor: cursor}, nil
        }

        offset := atoiDefault(q.Get("offset"), defaultOffset)
        if offset < 0 {
                offset = 0
        }
        return PageReq{Mode: PageModeOffset, Offset: offset, Limit: limit}, nil
}

// EncodeCursor packs a (sortValue, id) pair into an opaque base64 token.
// Callers pass whatever they last sorted by — e.g. ("2026-01-01T12:00:00Z",
// "550e8400-...").
func EncodeCursor(sortValue string, id string) string {
        raw, _ := json.Marshal([2]string{sortValue, id})
        return base64.URLEncoding.EncodeToString(raw)
}

// DecodeCursor inverts EncodeCursor.
func DecodeCursor(c string) (sortValue string, id string, err error) {
        b, err := base64.URLEncoding.DecodeString(c)
        if err != nil {
                return "", "", err
        }
        var pair [2]string
        if err := json.Unmarshal(b, &pair); err != nil {
                return "", "", err
        }
        return pair[0], pair[1], nil
}

// PageMetaFromReq builds the Meta.Page block to embed in the success envelope.
// nextCursor is computed by the caller (it knows the sort key of the last row).
func PageMetaFromReq(req PageReq, total int64, hasNext bool, nextCursor string) *PageMeta {
        m := &PageMeta{Total: total, Limit: req.Limit, HasNext: hasNext}
        if req.Mode == PageModeOffset {
                m.Offset = req.Offset
        } else {
                m.NextCursor = nextCursor
        }
        return m
}

// atoiDefault parses an int, returning def on error/empty.
func atoiDefault(s string, def int) int {
        if s == "" {
                return def
        }
        n, err := strconv.Atoi(s)
        if err != nil {
                return def
        }
        return n
}

// BuildCursorLink constructs a `?cursor=...&limit=...` URL for HATEOAS `next`.
func BuildCursorLink(base string, cursor string, limit int) string {
        u, err := url.Parse(base)
        if err != nil {
                return base
        }
        q := u.Query()
        q.Set("cursor", cursor)
        q.Set("limit", strconv.Itoa(limit))
        u.RawQuery = q.Encode()
        return u.String()
}

// BuildOffsetLink constructs a `?offset=...&limit=...` URL for HATEOAS `next`.
func BuildOffsetLink(base string, offset, limit int) string {
        u, err := url.Parse(base)
        if err != nil {
                return base
        }
        q := u.Query()
        q.Set("offset", strconv.Itoa(offset))
        q.Set("limit", strconv.Itoa(limit))
        u.RawQuery = q.Encode()
        return u.String()
}
