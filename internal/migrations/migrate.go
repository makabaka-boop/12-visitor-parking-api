package migrations

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
)

//go:embed *.sql
var files embed.FS

// Apply runs every embedded *.sql migration in filename order. Each file is a
// set of idempotent DDL statements (CREATE ... IF NOT EXISTS), so re-running is
// safe. Statements within a file share one ExecContext call.
func Apply(ctx context.Context, db *sql.DB) error {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("read migrations: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)
	for _, name := range names {
		b, err := files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read %s: %w", name, err)
		}
		if _, err := db.ExecContext(ctx, string(b)); err != nil {
			return fmt.Errorf("apply %s: %w", name, err)
		}
	}
	return nil
}
