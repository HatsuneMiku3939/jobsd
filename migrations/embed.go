package migrations

import "embed"

// Files contains embedded SQL migrations.
//
//go:embed sqlite/*.sql
var Files embed.FS
