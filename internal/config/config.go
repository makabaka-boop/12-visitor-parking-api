package config
import (
	"fmt"
	"os"
	"strings"
)
type Config struct {
	HTTPAddr      string // listen address, default ":8117"
	DatabaseURL   string // PostgreSQL DSN, e.g. postgres://user:pass@host:5432/db?sslmode=disable
	StorageDriver string // "postgres" (production) or "memory" (tests)
	LogLevel      string // "info" by default
}
func Load() Config {
	return Config{
		HTTPAddr:      envOr("HTTP_ADDR", ":8117"),
		DatabaseURL:   envOr("DATABASE_URL", ""),
		StorageDriver: strings.ToLower(envOr("STORAGE_DRIVER", "postgres")),
		LogLevel:      envOr("LOG_LEVEL", "info"),
	}
}
func (c Config) Validate() error {
	if c.StorageDriver == "postgres" && c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required when STORAGE_DRIVER=postgres")
	}
	if c.StorageDriver != "postgres" && c.StorageDriver != "memory" {
		return fmt.Errorf("unsupported STORAGE_DRIVER %q (want postgres or memory)", c.StorageDriver)
	}
	return nil
}
func envOr(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
