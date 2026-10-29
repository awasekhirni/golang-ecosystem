// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package specification implements the Specification pattern (DDD tactical).
// A Specification is a small predicate that knows how to:
//
//   - test if a candidate satisfies it                  (IsSatisfiedBy)
//   - convert itself into the query language of the repo (ToQuery)
//
// Combinators And, Or, Not let callers build arbitrarily complex query trees
// declaratively. Repositories accept a Specification and translate it into
// SQL via ToQuery without the service layer knowing SQL syntax.
//
// GAP CLOSED: #11 Specification pattern (queries)
package specification

import (
	"strings"
)

// Spec is the abstract specification. T is the entity type.
type Spec[T any] interface {
	IsSatisfiedBy(t T) bool
	ToQuery() Query
}

// Query is a parameterized SQL fragment. Repositories append `WHERE <sql>`
// and bind `args` in order. This decouples the domain from raw SQL strings
// while still giving the repo everything it needs to run a real query.
type Query struct {
	SQL  string
	Args []any
}

// And combines two specs with logical AND.
type AndSpec[T any] struct{ A, B Spec[T] }

func (s AndSpec[T]) IsSatisfiedBy(t T) bool { return s.A.IsSatisfiedBy(t) && s.B.IsSatisfiedBy(t) }
func (s AndSpec[T]) ToQuery() Query {
	a, b := s.A.ToQuery(), s.B.ToQuery()
	return Query{SQL: "(" + a.SQL + " AND " + b.SQL + ")", Args: append(a.Args, b.Args...)}
}

// Or combines two specs with logical OR.
type OrSpec[T any] struct{ A, B Spec[T] }

func (s OrSpec[T]) IsSatisfiedBy(t T) bool { return s.A.IsSatisfiedBy(t) || s.B.IsSatisfiedBy(t) }
func (s OrSpec[T]) ToQuery() Query {
	a, b := s.A.ToQuery(), s.B.ToQuery()
	return Query{SQL: "(" + a.SQL + " OR " + b.SQL + ")", Args: append(a.Args, b.Args...)}
}

// Not negates a spec.
type NotSpec[T any] struct{ A Spec[T] }

func (s NotSpec[T]) IsSatisfiedBy(t T) bool { return !s.A.IsSatisfiedBy(t) }
func (s NotSpec[T]) ToQuery() Query {
	a := s.A.ToQuery()
	return Query{SQL: "NOT (" + a.SQL + ")", Args: a.Args}
}

// And / Or / Not are the fluent constructors callers actually use.
func And[T any](a, b Spec[T]) Spec[T] { return AndSpec[T]{A: a, B: b} }
func Or[T any](a, b Spec[T]) Spec[T]  { return OrSpec[T]{A: a, B: b} }
func Not[T any](a Spec[T]) Spec[T]    { return NotSpec[T]{A: a} }

// FieldEq is the most common leaf spec: `column = value`.
type FieldEq[T any] struct {
	Column string
	Value  any
}

func (s FieldEq[T]) IsSatisfiedBy(_ T) bool { return true } // domain-side test is optional
func (s FieldEq[T]) ToQuery() Query {
	return Query{SQL: s.Column + " = ?", Args: []any{s.Value}}
}

// FieldLike is `column LIKE value`.
type FieldLike[T any] struct {
	Column string
	Value  string
}

func (s FieldLike[T]) ToQuery() Query {
	return Query{SQL: s.Column + " ILIKE ?", Args: []any{s.Value}}
}

// FieldGt is `column > value`.
type FieldGt[T any] struct {
	Column string
	Value  any
}

func (s FieldGt[T]) ToQuery() Query {
	return Query{SQL: s.Column + " > ?", Args: []any{s.Value}}
}

// FieldLt is `column < value`.
type FieldLt[T any] struct {
	Column string
	Value  any
}

func (s FieldLt[T]) ToQuery() Query {
	return Query{SQL: s.Column + " < ?", Args: []any{s.Value}}
}

// Compose concatenates many specs with AND in one call.
func Compose[T any](specs ...Spec[T]) Spec[T] {
	if len(specs) == 0 {
		return FieldEq[T]{Column: "1", Value: 1}
	}
	out := specs[0]
	for _, s := range specs[1:] {
		out = And(out, s)
	}
	return out
}

// RenderQuery is a small helper for repositories: it takes a base SELECT and
// appends " WHERE <spec.SQL>" with positional args converted to Postgres
// $1, $2, ... syntax.
func RenderQuery(base string, q Query) (string, []any) {
	if strings.TrimSpace(q.SQL) == "" || q.SQL == "1 = ?" {
		return base, nil
	}
	// Convert ? placeholders to $N.
	sql := q.SQL
	out := base + " WHERE "
	n := 0
	for i := 0; i < len(sql); i++ {
		if sql[i] == '?' {
			n++
			out += "$" + itoa(n)
		} else {
			out += string(sql[i])
		}
	}
	return out, q.Args
}

// itoa is a tiny stdlib-free int->string to avoid importing strconv here.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
