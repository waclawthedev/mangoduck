package repo_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"

	"mangoduck/internal/db"
	"mangoduck/internal/repo"

	"github.com/stretchr/testify/require"
)

func TestInputsOutputsRepoAppendAndList(t *testing.T) {
	conn := openTestDB(t)

	repository := repo.NewInputsOutputsRepo(conn)
	err := repository.Append(context.Background(), 10, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user"}`),
		json.RawMessage(`{"type":"message","role":"assistant"}`),
	})
	require.NoError(t, err)

	items, err := repository.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.JSONEq(t, `{"type":"message","role":"user"}`, string(items[0]))
	require.JSONEq(t, `{"type":"message","role":"assistant"}`, string(items[1]))
}

func TestInputsOutputsRepoSeparatesChats(t *testing.T) {
	conn := openTestDB(t)

	repository := repo.NewInputsOutputsRepo(conn)
	err := repository.Append(context.Background(), 10, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"chat-10"}`),
	})
	require.NoError(t, err)

	err = repository.Append(context.Background(), 11, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"chat-11"}`),
	})
	require.NoError(t, err)

	items, err := repository.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.JSONEq(t, `{"type":"message","role":"user","content":"chat-10"}`, string(items[0]))
}

func TestInputsOutputsRepoReplaceHistory(t *testing.T) {
	conn := openTestDB(t)

	repository := repo.NewInputsOutputsRepo(conn)
	err := repository.Append(context.Background(), 10, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"old"}`),
		json.RawMessage(`{"type":"message","role":"assistant","content":"old answer"}`),
	})
	require.NoError(t, err)

	err = repository.ReplaceHistory(context.Background(), 10, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"new question"}`),
		json.RawMessage(`{"type":"message","role":"assistant","content":"new answer"}`),
	})
	require.NoError(t, err)

	items, err := repository.List(context.Background(), 10)
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.JSONEq(t, `{"type":"message","role":"user","content":"new question"}`, string(items[0]))
	require.JSONEq(t, `{"type":"message","role":"assistant","content":"new answer"}`, string(items[1]))
}

func TestInputsOutputsRepoClear(t *testing.T) {
	conn := openTestDB(t)

	repository := repo.NewInputsOutputsRepo(conn)
	err := repository.Append(context.Background(), 10, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"chat-10"}`),
	})
	require.NoError(t, err)

	err = repository.Append(context.Background(), 11, []json.RawMessage{
		json.RawMessage(`{"type":"message","role":"user","content":"chat-11"}`),
	})
	require.NoError(t, err)

	err = repository.Clear(context.Background(), 10)
	require.NoError(t, err)

	items, err := repository.List(context.Background(), 10)
	require.NoError(t, err)
	require.Empty(t, items)

	items, err = repository.List(context.Background(), 11)
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.JSONEq(t, `{"type":"message","role":"user","content":"chat-11"}`, string(items[0]))
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()

	conn, err := db.Open(t.TempDir() + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, conn.Close())
	})

	return conn
}
