// Package migrations embeds the versioned SQL migrations so the API binary
// and the migrate command carry them without extra files.
package migrations

import "embed"

// FS holds the NNNNNN_name.{up,down}.sql files.
//
//go:embed *.sql
var FS embed.FS
