package db

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"net/url"
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

	if err := configureSQLite(conn, dbPath); err != nil {
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
	if strings.TrimSpace(dbPath) == "" {
		return errors.New("database path is empty")
	}

	if dbPath == ":memory:" {
		return errors.New("in-memory sqlite databases are not supported; use a file-backed database path")
	}

	dsnURL, ok := sqliteURI(dbPath)
	if !ok {
		return nil
	}

	query := dsnURL.Query()
	if strings.EqualFold(query.Get("mode"), "memory") {
		return errors.New("in-memory sqlite databases are not supported; use a file-backed database path")
	}
	if strings.EqualFold(query.Get("vfs"), "memdb") {
		return errors.New("in-memory sqlite databases are not supported; use a file-backed database path")
	}

	return nil
}

func sqliteDSN(dbPath string) string {
	query := url.Values{}
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMillis))

	if strings.HasPrefix(dbPath, "file:") {
		dsnURL, ok := sqliteURI(dbPath)
		if !ok {
			return dbPath
		}

		mergedQuery := dsnURL.Query()
		for key, values := range query {
			for _, value := range values {
				mergedQuery.Add(key, value)
			}
		}
		dsnURL.RawQuery = mergedQuery.Encode()

		return dsnURL.String()
	}

	return dbPath + "?" + query.Encode()
}

func configureSQLite(conn *sql.DB, dbPath string) error {
	if !sqliteSupportsWAL(dbPath) {
		return nil
	}

	journalMode, err := sqliteJournalMode(conn, "WAL")
	if err != nil {
		// WAL improves concurrency for file-backed databases, but the bot still
		// works correctly with SQLite's default journal mode.
		return nil
	}
	if !strings.EqualFold(journalMode, "wal") {
		return nil
	}

	return nil
}

func sqliteSupportsWAL(dbPath string) bool {
	if dbPath == ":memory:" {
		return false
	}

	dsnURL, ok := sqliteURI(dbPath)
	if !ok {
		return true
	}

	query := dsnURL.Query()

	if strings.EqualFold(query.Get("mode"), "memory") {
		return false
	}
	if strings.EqualFold(query.Get("vfs"), "memdb") {
		return false
	}
	if strings.EqualFold(query.Get("immutable"), "1") {
		return false
	}
	if strings.EqualFold(query.Get("mode"), "ro") {
		return false
	}

	return true
}

func sqliteJournalMode(conn *sql.DB, mode string) (string, error) {
	var journalMode string
	err := conn.QueryRowContext(context.Background(), fmt.Sprintf(`PRAGMA journal_mode = %s`, mode)).Scan(&journalMode)
	if err != nil {
		return "", err
	}

	return journalMode, nil
}

func sqliteURI(dbPath string) (url.URL, bool) {
	if !strings.HasPrefix(dbPath, "file:") {
		return url.URL{}, false
	}

	dsnURL, err := url.Parse(dbPath)
	if err != nil {
		return url.URL{}, false
	}

	return *dsnURL, true
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
