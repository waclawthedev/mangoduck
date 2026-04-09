package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	_ "modernc.org/sqlite"
)

func TestSQLiteDSN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		dbPath string
		want   string
	}{
		{
			name:   "relative path",
			dbPath: "mangoduck.db",
			want:   "mangoduck.db?_pragma=busy_timeout%285000%29",
		},
		{
			name:   "absolute path",
			dbPath: "/tmp/mangoduck.db",
			want:   "/tmp/mangoduck.db?_pragma=busy_timeout%285000%29",
		},
		{
			name:   "file uri preserves query",
			dbPath: "file:mangoduck.db?cache=shared",
			want:   "file:mangoduck.db",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := sqliteDSN(tt.dbPath)
			if !strings.HasPrefix(tt.dbPath, "file:") {
				require.Equal(t, tt.want, got)
				return
			}

			parsed, err := url.Parse(got)
			require.NoError(t, err)
			require.Equal(t, tt.want, parsed.Scheme+":"+parsed.Opaque)
			require.Equal(t, "shared", parsed.Query().Get("cache"))
			require.Equal(t, []string{fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMillis)}, parsed.Query()["_pragma"])
		})
	}
}

func TestOpenRejectsInMemorySQLiteDatabase(t *testing.T) {
	_, err := Open(":memory:")
	require.Error(t, err)
	require.ErrorContains(t, err, "in-memory sqlite databases are not supported")
}

func TestOpenAppliesSquashedInitialMigration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "fresh.db")

	conn, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	require.Equal(t, squashedInitialMigrationVersion, currentMigrationVersion(t, conn))
	require.Equal(t, []string{"id", "tg_id", "username", "status", "created_at"}, tableColumns(t, conn, "users"))
	require.Equal(t, []string{"id", "tg_id", "title", "type", "status", "created_at"}, tableColumns(t, conn, "groups"))
	require.Equal(t, []string{"id", "chat_id", "item_json", "created_at"}, tableColumns(t, conn, "inputs_outputs"))
	require.Equal(t, []string{"id", "chat_id", "created_by_tg_id", "schedule", "prompt", "created_at"}, tableColumns(t, conn, "cron_tasks"))
	require.Equal(t, []string{"id", "tg_id", "title", "username", "type", "status", "memory_text", "created_at"}, tableColumns(t, conn, "chats"))
}

func TestOpenNormalizesLegacyInitialMigrationVersion(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "legacy.db")

	conn, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)

	schema, err := migrations.ReadFile("migrations/001_init.up.sql")
	require.NoError(t, err)

	_, err = conn.ExecContext(context.Background(), string(schema))
	require.NoError(t, err)

	_, err = conn.ExecContext(context.Background(), `
CREATE TABLE schema_migrations (version uint64, dirty bool);
INSERT INTO schema_migrations (version, dirty) VALUES (10, false);
`)
	require.NoError(t, err)
	require.NoError(t, conn.Close())

	migratedConn, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, migratedConn.Close())
	})

	require.Equal(t, squashedInitialMigrationVersion, currentMigrationVersion(t, migratedConn))
	require.Equal(t, []string{"id", "tg_id", "title", "username", "type", "status", "memory_text", "created_at"}, tableColumns(t, migratedConn, "chats"))
}

func TestOpenConfiguresSQLiteForConcurrentAccess(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "concurrency.db")

	conn, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	stats := conn.Stats()
	require.Equal(t, sqliteMaxOpenConns, stats.MaxOpenConnections)

	var journalMode string
	err = conn.QueryRowContext(context.Background(), `PRAGMA journal_mode`).Scan(&journalMode)
	require.NoError(t, err)
	require.Equal(t, "wal", strings.ToLower(journalMode))

	var busyTimeoutMillis int
	err = conn.QueryRowContext(context.Background(), `PRAGMA busy_timeout`).Scan(&busyTimeoutMillis)
	require.NoError(t, err)
	require.Equal(t, sqliteBusyTimeoutMillis, busyTimeoutMillis)
}

func TestOpenRejectsInMemorySQLiteURI(t *testing.T) {
	_, err := Open("file:shared-memory?mode=memory&cache=shared")
	require.Error(t, err)
	require.ErrorContains(t, err, "in-memory sqlite databases are not supported")
}

func TestOpenHandlesConcurrentWrites(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "writes.db")

	conn, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	const workers = 24

	start := make(chan struct{})
	errs := make(chan error, workers)

	var ready sync.WaitGroup
	ready.Add(workers)

	for i := range workers {
		go func(i int) {
			ready.Done()
			<-start

			_, execErr := conn.ExecContext(
				context.Background(),
				`INSERT INTO users (tg_id, username, status) VALUES (?, ?, ?)`,
				int64(i+1),
				fmt.Sprintf("user-%d", i+1),
				"active",
			)
			errs <- execErr
		}(i)
	}

	ready.Wait()
	close(start)

	for range workers {
		require.NoError(t, <-errs)
	}

	var count int
	err = conn.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM users`).Scan(&count)
	require.NoError(t, err)
	require.Equal(t, workers, count)
}

func currentMigrationVersion(t *testing.T, conn *sql.DB) int {
	t.Helper()

	var version int
	err := conn.QueryRowContext(context.Background(), `SELECT version FROM schema_migrations LIMIT 1`).Scan(&version)
	require.NoError(t, err)

	return version
}

func tableColumns(t *testing.T, conn *sql.DB, table string) []string {
	t.Helper()

	rows, err := conn.QueryContext(context.Background(), `SELECT name FROM pragma_table_info(?) ORDER BY cid ASC`, table)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, rows.Close())
	}()

	columns := make([]string, 0)
	for rows.Next() {
		var name string
		err = rows.Scan(&name)
		require.NoError(t, err)
		columns = append(columns, name)
	}

	require.NoError(t, rows.Err())

	return columns
}
