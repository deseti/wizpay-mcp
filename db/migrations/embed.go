// Package migrations embeds the forward-only WizPay MCP database migrations.
package migrations

import "embed"

// Files contains only versioned .up.sql migrations. Destructive down
// migrations are intentionally unsupported.
//
//go:embed *.up.sql
var Files embed.FS
