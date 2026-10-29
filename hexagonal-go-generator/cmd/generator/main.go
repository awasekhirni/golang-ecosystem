// Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
// Author: Awase Khirni Syed
//
// Package main is the Go port of the original Python hexagonal-architecture
// generator. It introspects a live PostgreSQL schema and emits the full
// hexagonal artifact tree (entity, repository interface, service interface,
// repository impl, service impl, controller, DTO, mapper, validation) as Go
// source files - one bundle per table.
//
// Differences from the Python original:
//   - Output is Go, not Python.
//   - Each generated file embeds the enterprise patterns from this repo
//     (Base repository, UoW, Specification, Domain Events, Enterprise Bus,
//     Value Objects, standardised error envelope, HATEOAS pagination).
//   - Layer-by-layer logging is wired into the generated code so the 12-layer
//     matrix is observable without further work.
//
// Usage:
//   go run ./cmd/generator -dsn="postgres://user:pass@host:5432/db?sslmode=disable" -out=./generated
//
// GAP CLOSED: Go port of the Python generator (mirrors the script that
// produced this very project).
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	dsn := flag.String("dsn", os.Getenv("DATABASE_URL"), "PostgreSQL DSN")
	out := flag.String("out", "./generated", "output directory")
	schema := flag.String("schema", "public", "schema to introspect")
	flag.Parse()

	if *dsn == "" {
		fmt.Fprintln(os.Stderr, "ERROR: -dsn or DATABASE_URL required")
		os.Exit(1)
	}

	L := newLogger()
	L.info("GENERATOR_START", "dsn", redact(*dsn), "out", *out, "schema", *schema)

	db, err := sql.Open("pgx", *dsn)
	if err != nil {
		L.fatal("DB_OPEN_FAILED", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		L.fatal("DB_PING_FAILED", err)
	}

	tables, err := introspect(ctx, db, *schema)
	if err != nil {
		L.fatal("INTROSPECT_FAILED", err)
	}
	L.info("INTROSPECT_OK", "tables", fmt.Sprintf("%d", len(tables)))

	for _, t := range tables {
		if err := generateForTable(*out, t); err != nil {
			L.error("GENERATE_TABLE_FAILED", err, "table", t.Name)
			continue
		}
		L.info("GENERATE_TABLE_OK", "table", t.Name)
	}

	L.info("GENERATOR_DONE")
}

// --- introspection ---------------------------------------------------------

// tableInfo is the introspected schema of one table.
type tableInfo struct {
	Schema      string
	Name        string
	GoName      string // CamelCased
	Columns     []columnInfo
	HasTimestamps bool
	HasStatus     bool
	HasSoftDelete bool
	HasUserRel    bool
}

type columnInfo struct {
	Name      string
	GoName    string
	DataType  string // postgres type
	GoType    string // mapped go type
	Nullable  bool
	IsPK      bool
	IsFK      bool
	Default   string
}

// introspect queries information_schema for every table in the schema and
// builds a tableInfo per table.
func introspect(ctx context.Context, db *sql.DB, schema string) ([]tableInfo, error) {
	rows, err := db.QueryContext(ctx, `
        SELECT t.table_name,
               c.column_name,
               c.data_type,
               c.is_nullable,
               c.column_default,
               CASE WHEN pk.column_name IS NOT NULL THEN true ELSE false END AS is_pk
          FROM information_schema.tables t
          JOIN information_schema.columns c
            ON c.table_schema = t.table_schema AND c.table_name = t.table_name
     LEFT JOIN information_schema.key_column_usage pk
            ON pk.table_schema = c.table_schema
           AND pk.table_name = c.table_name
           AND pk.column_name = c.column_name
           AND pk.constraint_name IN (SELECT constraint_name
                                        FROM information_schema.table_constraints
                                       WHERE table_schema = c.table_schema
                                         AND table_name   = c.table_name
                                         AND constraint_type = 'PRIMARY KEY')
         WHERE t.table_schema = $1 AND t.table_type = 'BASE TABLE'
         ORDER BY t.table_name, c.ordinal_position`, schema)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	byTable := map[string]*tableInfo{}
	var order []string
	for rows.Next() {
		var tName, cName, dType, nullable string
		var isPK bool
		var def sql.NullString
		if err := rows.Scan(&tName, &cName, &dType, &nullable, &def, &isPK); err != nil {
			return nil, err
		}
		t, ok := byTable[tName]
		if !ok {
			t = &tableInfo{Schema: schema, Name: tName, GoName: camelCase(tName)}
			byTable[tName] = t
			order = append(order, tName)
		}
		col := columnInfo{
			Name: cName, GoName: exportedName(cName),
			DataType: dType, GoType: mapGoType(dType, nullable == "YES"),
			Nullable: nullable == "YES", IsPK: isPK, Default: def.String,
		}
		t.Columns = append(t.Columns, col)
		// heuristics for the convenience flags
		switch {
		case cName == "created_at" || cName == "updated_at" || cName == "modified_at":
			t.HasTimestamps = true
		case strings.HasSuffix(cName, "_status"):
			t.HasStatus = true
		case cName == "is_deleted" || cName == "deleted_at":
			t.HasSoftDelete = true
		case cName == "user_id" || strings.HasSuffix(cName, "_user_id"):
			t.HasUserRel = true
		}
	}
	out := make([]tableInfo, 0, len(order))
	for _, n := range order {
		out = append(out, *byTable[n])
	}
	return out, nil
}

// mapGoType converts a postgres type into the corresponding Go type string.
func mapGoType(pgType string, nullable bool) string {
	var goType string
	switch pgType {
	case "uuid":
		goType = "string"
	case "integer", "int", "int4", "smallint", "int2":
		goType = "int"
	case "bigint", "int8":
		goType = "int64"
	case "boolean", "bool":
		goType = "bool"
	case "real", "double precision", "numeric", "decimal":
		goType = "float64"
	case "text", "character varying", "character", "varchar", "char":
		goType = "string"
	case "timestamp without time zone", "timestamp with time zone", "timestamptz":
		goType = "time.Time"
	case "date":
		goType = "time.Time"
	case "json", "jsonb":
		goType = "[]byte"
	case "bytea":
		goType = "[]byte"
	default:
		goType = "string"
	}
	if nullable && goType != "[]byte" {
		switch goType {
		case "string":
			goType = "sql.NullString"
		case "int", "int64":
			goType = "sql.NullInt64"
		case "bool":
			goType = "sql.NullBool"
		case "time.Time":
			goType = "sql.NullTime"
		case "float64":
			goType = "sql.NullFloat64"
		}
	}
	return goType
}

// --- code emission ---------------------------------------------------------

// generateForTable writes the per-table artifact bundle into out/<table>/.
func generateForTable(out string, t tableInfo) error {
	dir := filepath.Join(out, t.Name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	files := map[string]string{
		t.Name + "_entity.go":               render(entityTpl, t),
		t.Name + "_repository.go":           render(repoTpl, t),
		t.Name + "_service.go":              render(serviceTpl, t),
		t.Name + "_controller.go":           render(controllerTpl, t),
		t.Name + "_dto.go":                  render(dtoTpl, t),
		t.Name + "_mapper.go":               render(mapperTpl, t),
		t.Name + "_validation.go":           render(validationTpl, t),
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func render(tpl string, t tableInfo) string {
	tmpl := template.Must(template.New("artifact").
		Funcs(template.FuncMap{"title": strings.Title, "join": strings.Join}).
		Parse(tpl))
	var sb strings.Builder
	if err := tmpl.Execute(&sb, t); err != nil {
		panic(err)
	}
	return sb.String()
}

// camelCase converts snake_case to CamelCase.
func camelCase(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = strings.Title(p)
	}
	return strings.Join(parts, "")
}

// exportedName converts a snake_case column name into an exported Go field name.
func exportedName(s string) string {
	parts := strings.Split(s, "_")
	for i, p := range parts {
		parts[i] = strings.Title(p)
	}
	return strings.Join(parts, "")
}

// redact masks the password in a DSN.
func redact(dsn string) string {
	at := strings.Index(dsn, "@")
	if at < 0 {
		return dsn
	}
	scheme := "postgres://"
	if i := strings.Index(dsn, "://"); i >= 0 {
		scheme = dsn[:i+3]
	}
	return scheme + "***" + dsn[at:]
}
