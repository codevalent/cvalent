// Package store is the storage layer for cvalent.
//
// At Rung 0 it wraps a single sqlc-generated Querier per dialect (sqlite,
// postgres). The Store wrapper itself, the migrator-aware Open() that
// runs goose on startup, and the typed query helpers all land in
// AH-0316.6 (Stage B). This file exists so that the package directory
// is buildable from Stage A and so the migration files have a Go-package
// neighbor for embedding.
//
//go:generate go run github.com/sqlc-dev/sqlc/cmd/sqlc@latest generate
package store
