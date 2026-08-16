package migrations
import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)
//go:embed 001_init.sql
var schema001 string
//go:embed 002_extension_applications.sql
var schema002 string
// migrations applied in order; each is idempotent (IF NOT EXISTS).
var migrations = []struct {
	name string
	sql  string
}{
	{"001_init", schema001},
	{"002_extension_applications", schema002},
}
func Apply(ctx context.Context, db *sql.DB) error {
	for _, m := range migrations {
		if _, err := db.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.name, err)
		}
	}
	return nil
}
