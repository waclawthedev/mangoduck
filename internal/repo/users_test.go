package repo_test

import (
	"context"
	"testing"

	"mangoduck/internal/repo"

	"github.com/stretchr/testify/require"
)

func TestUsersRepoCreateAndListWithoutRoleColumn(t *testing.T) {
	conn := openTestDB(t)

	repository := repo.NewUsersRepo(conn)

	createdUser, err := repository.Create(context.Background(), 42, "boss", repo.UserStatusActive)
	require.NoError(t, err)
	require.Equal(t, int64(42), createdUser.TGID)
	require.Equal(t, "boss", createdUser.Username)
	require.Equal(t, repo.UserStatusActive, createdUser.Status)

	fetchedUser, err := repository.GetByTGID(context.Background(), 42)
	require.NoError(t, err)
	require.Equal(t, createdUser.ID, fetchedUser.ID)
	require.Equal(t, "boss", fetchedUser.Username)
	require.Equal(t, repo.UserStatusActive, fetchedUser.Status)

	usersList, err := repository.List(context.Background())
	require.NoError(t, err)
	require.Len(t, usersList, 1)
	require.Equal(t, int64(42), usersList[0].TGID)
	require.Equal(t, "boss", usersList[0].Username)
	require.Equal(t, repo.UserStatusActive, usersList[0].Status)
}
