package migrations
import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
)
//go:embed 001_init.sql
var schema string
func Apply(ctx context.Context, db *sql.DB) error {
	if _, err := db.ExecContext(ctx, schema); err != nil {
		return fmt.Errorf("apply migrations: %w", err)
	}
	return nil
}
