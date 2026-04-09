package repo_test

import (
	"context"
	"testing"

	"mangoduck/internal/repo"

	"github.com/stretchr/testify/require"
)

func TestChatsRepoGetAndSetMemory(t *testing.T) {
	t.Parallel()

	conn := openTestDB(t)

	repository := repo.NewChatsRepo(conn)
	_, err := repository.Create(context.Background(), 42, "Test", "test_chat", "private", repo.ChatStatusActive)
	require.NoError(t, err)

	memoryText, err := repository.GetMemory(context.Background(), 42)
	require.NoError(t, err)
	require.Empty(t, memoryText)

	err = repository.SetMemory(context.Background(), 42, "Speak Ukrainian and remember project deadlines.")
	require.NoError(t, err)

	chatRecord, err := repository.GetByTGID(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "Speak Ukrainian and remember project deadlines.", chatRecord.MemoryText)

	memoryText, err = repository.GetMemory(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, "Speak Ukrainian and remember project deadlines.", memoryText)
}

func TestChatsRepoMemoryReturnsNotFound(t *testing.T) {
	t.Parallel()

	conn := openTestDB(t)

	repository := repo.NewChatsRepo(conn)

	_, err := repository.GetMemory(context.Background(), 404)
	require.ErrorIs(t, err, repo.ErrChatNotFound)

	err = repository.SetMemory(context.Background(), 404, "remember this")
	require.ErrorIs(t, err, repo.ErrChatNotFound)
}
