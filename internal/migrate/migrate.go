package migrate

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"sort"
	"strings"
)

//go:embed sql/*.sql
var migrationFiles embed.FS

const migrateLockName = "sportsbook_migrate"

// Apply runs embedded migrations in lexical order. Each SQL file runs at most once.
// Version keys are paths such as "sql/0001_init.sql" (see schema_migrations.version).
// Uses a connection-scoped MySQL advisory lock so parallel callers (api + worker) are safe.
func Apply(ctx context.Context, db *sql.DB) error {
	log := slog.Default()
	log.Debug("starting migration process", "lock", migrateLockName)

	conn, err := db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("migrate conn: %w", err)
	}
	defer conn.Close()

	var got int
	if err := conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, 30)`, migrateLockName).Scan(&got); err != nil {
		return fmt.Errorf("migrate GET_LOCK: %w", err)
	}
	if got != 1 {
		return fmt.Errorf("migrate GET_LOCK: not acquired (result=%d)", got)
	}
	defer func() {
		_, _ = conn.ExecContext(context.Background(), `SELECT RELEASE_LOCK(?)`, migrateLockName)
	}()

	if _, err := conn.ExecContext(ctx, `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version VARCHAR(255) NOT NULL PRIMARY KEY,
  applied_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
)`); err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	names, err := collectSQLNames(migrationFiles)
	if err != nil {
		return err
	}

	for _, name := range names {
		var n int
		if err := conn.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&n); err != nil {
			return fmt.Errorf("check migration %q: %w", name, err)
		}
		if n > 0 {
			continue
		}

		log.Info("applying migration", "version", name)
		body, err := migrationFiles.ReadFile(name)
		if err != nil {
			return fmt.Errorf("read migration %q: %w", name, err)
		}

		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("begin tx %q: %w", name, err)
		}

		if err := execStatements(ctx, tx, string(body)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("exec migration %q: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version) VALUES (?)`, name); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %q: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %q: %w", name, err)
		}
	}
	log.Debug("migrations complete")
	return nil
}

func collectSQLNames(files embed.FS) ([]string, error) {
	var names []string
	err := fs.WalkDir(files, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".sql") {
			names = append(names, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	return names, nil
}

// execStatements splits on semicolons and runs non-empty statements.
// Assumes migration SQL does not embed semicolons inside string literals.
func execStatements(ctx context.Context, tx *sql.Tx, script string) error {
	parts := strings.Split(script, ";")
	for _, p := range parts {
		stmt := strings.TrimSpace(p)
		if stmt == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("statement %q: %w", truncate(stmt, 120), err)
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
