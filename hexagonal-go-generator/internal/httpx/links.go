// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// links.go - HATEOAS link primitives (REST maturity Level 3).
//
// Every collection response carries a `links` map with at minimum:
//
//   self   -> canonical URL of this resource
//   next   -> URL of the next page (when hasNext)
//   prev   -> URL of the previous page (offset mode only)
//   create -> POST URL to make a new resource of the same type
//
// GAP CLOSED: #9 HATEOAS links
package httpx

// Link is a single HATEOAS hyperlink.
type Link struct {
	Href string `json:"href"`
}

// LinksBuilder is a tiny fluent builder so controllers don't have to spell
// out the map literal every time.
type LinksBuilder struct {
	m map[string]Link
}

func NewLinks() *LinksBuilder { return &LinksBuilder{m: map[string]Link{}} }

func (b *LinksBuilder) Self(href string) *LinksBuilder     { b.m["self"] = Link{href}; return b }
func (b *LinksBuilder) Next(href string) *LinksBuilder     { b.m["next"] = Link{href}; return b }
func (b *LinksBuilder) Prev(href string) *LinksBuilder     { b.m["prev"] = Link{href}; return b }
func (b *LinksBuilder) Create(href string) *LinksBuilder   { b.m["create"] = Link{href}; return b }
func (b *LinksBuilder) Update(href string) *LinksBuilder   { b.m["update"] = Link{href}; return b }
func (b *LinksBuilder) Delete(href string) *LinksBuilder   { b.m["delete"] = Link{href}; return b }
func (b *LinksBuilder) Collection(href string) *LinksBuilder { b.m["collection"] = Link{href}; return b }
func (b *LinksBuilder) Custom(rel, href string) *LinksBuilder { b.m[rel] = Link{href}; return b }

// Build returns the assembled map. Empty map is returned as nil so empty
// links are omitted from the JSON envelope entirely.
func (b *LinksBuilder) Build() map[string]Link {
	if len(b.m) == 0 {
		return nil
	}
	return b.m
}
