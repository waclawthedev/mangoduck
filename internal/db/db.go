package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	// Register the modernc SQLite driver for database/sql.
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

const (
	squashedInitialMigrationVersion = 1
	legacyInitialMigrationVersion   = 10
	sqliteBusyTimeoutMillis         = 5000
	sqliteMaxOpenConns              = 1
)

func Open(dbPath string) (*sql.DB, error) {
	if err := validateDBPath(dbPath); err != nil {
		return nil, err
	}

	conn, err := sql.Open("sqlite", sqliteDSN(dbPath))
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	conn.SetMaxOpenConns(sqliteMaxOpenConns)
	conn.SetMaxIdleConns(sqliteMaxOpenConns)

	if err := conn.PingContext(context.Background()); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	if err := configureSQLite(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("configuring database: %w", err)
	}

	if err := runMigrations(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return conn, nil
}

func validateDBPath(dbPath string) error {
	dbPath = strings.TrimSpace(dbPath)
	if dbPath == "" {
		return errors.New("database path is empty")
	}
	if strings.Contains(dbPath, "?") || strings.Contains(dbPath, "#") || strings.HasPrefix(dbPath, "file:") {
		return errors.New("database path must be a sqlite file path, not a URI")
	}

	return nil
}

func sqliteDSN(dbPath string) string {
	return fmt.Sprintf("%s?_pragma=busy_timeout(%d)", dbPath, sqliteBusyTimeoutMillis)
}

func configureSQLite(conn *sql.DB) error {
	// WAL improves concurrency for file-backed databases, but the bot still
	// works correctly with SQLite's default journal mode.
	_, _ = sqliteJournalMode(conn)
	return nil
}

func sqliteJournalMode(conn *sql.DB) (string, error) {
	var journalMode string
	err := conn.QueryRowContext(context.Background(), `PRAGMA journal_mode = WAL`).Scan(&journalMode)
	if err != nil {
		return "", err
	}

	return journalMode, nil
}

func runMigrations(conn *sql.DB) error {
	source, err := iofs.New(migrations, "migrations")
	if err != nil {
		return fmt.Errorf("creating iofs source: %w", err)
	}

	driver, err := sqlite.WithInstance(conn, &sqlite.Config{})
	if err != nil {
		return fmt.Errorf("creating sqlite driver: %w", err)
	}

	m, err := migrate.NewWithInstance("iofs", source, "sqlite", driver)
	if err != nil {
		return fmt.Errorf("creating migrate instance: %w", err)
	}

	if err := normalizeSquashedInitialMigration(m); err != nil {
		return err
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}

func normalizeSquashedInitialMigration(m *migrate.Migrate) error {
	version, dirty, err := m.Version()
	if errors.Is(err, migrate.ErrNilVersion) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("reading migration version: %w", err)
	}
	if dirty {
		return nil
	}

	if version == legacyInitialMigrationVersion {
		if err := m.Force(squashedInitialMigrationVersion); err != nil {
			return fmt.Errorf("forcing squashed initial migration version: %w", err)
		}

		return nil
	}

	if version > squashedInitialMigrationVersion && version < legacyInitialMigrationVersion {
		return fmt.Errorf("database is on unsupported pre-release migration version %d; recreate the database or migrate it to version %d before upgrading", version, legacyInitialMigrationVersion)
	}

	return nil
}
