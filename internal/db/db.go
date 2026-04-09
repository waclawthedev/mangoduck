package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/sqlite"
	"github.com/golang-migrate/migrate/v4/source/iofs"
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

	if err := runMigrations(conn); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return conn, nil
}

func sqliteDSN(dbPath string) string {
	var dsnURL url.URL
	if dbPath == ":memory:" {
		dsnURL.Scheme = "file"
		dsnURL.Opaque = ":memory:"
	} else if strings.HasPrefix(dbPath, "file:") {
		parsed, err := url.Parse(dbPath)
		if err == nil {
			dsnURL = *parsed
		}
	}

	if dsnURL.Scheme == "" {
		dsnURL.Scheme = "file"
		dsnURL.Path = filepath.ToSlash(dbPath)
	}

	query := dsnURL.Query()
	query.Add("_pragma", "journal_mode(WAL)")
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMillis))
	dsnURL.RawQuery = query.Encode()

	return dsnURL.String()
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
