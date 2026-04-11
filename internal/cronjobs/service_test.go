package cronjobs

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"mangoduck/internal/repo"
)

type stubRepository struct{}

func (s *stubRepository) List(ctx context.Context) ([]*repo.CronTask, error) {
	return nil, nil
}

type blockingExecutor struct {
	called chan struct{}
}

func (s *blockingExecutor) ExecuteScheduled(ctx context.Context, chatID int64, prompt string) (string, error) {
	close(s.called)
	<-ctx.Done()
	return "", ctx.Err()
}

type immediateExecutor struct{}

func (s *immediateExecutor) ExecuteScheduled(ctx context.Context, chatID int64, prompt string) (string, error) {
	return "scheduled reply", nil
}

type blockingSender struct {
	called chan struct{}
}

func (s *blockingSender) Send(chatID int64, text string) error {
	close(s.called)
	select {}
}

type noopSender struct{}

func (s *noopSender) Send(chatID int64, text string) error {
	return nil
}

func TestBuildJobStopsWaitingForExecutorWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	var repository stubRepository
	var executor blockingExecutor
	executor.called = make(chan struct{})
	var sender noopSender

	service, err := NewService(&repository, &executor, &sender)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	service.setLifecycleDone(ctx.Done())

	var task repo.CronTask
	task.ID = 1
	task.ChatID = 10
	task.Prompt = "run"

	done := make(chan struct{})
	go func() {
		defer close(done)
		service.buildJob(&task)()
	}()

	<-executor.called
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cron job to stop after executor cancellation")
	}
}

func TestBuildJobStopsWaitingForSenderWhenContextIsCancelled(t *testing.T) {
	t.Parallel()

	var repository stubRepository
	var executor immediateExecutor
	var sender blockingSender
	sender.called = make(chan struct{})

	service, err := NewService(&repository, &executor, &sender)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	service.setLifecycleDone(ctx.Done())

	var task repo.CronTask
	task.ID = 1
	task.ChatID = 10
	task.Prompt = "run"

	done := make(chan struct{})
	go func() {
		defer close(done)
		service.buildJob(&task)()
	}()

	<-sender.called
	cancel()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for cron job to stop after sender cancellation")
	}
}

func TestExecuteScheduledReturnsContextErrorWhenCancelled(t *testing.T) {
	t.Parallel()

	var repository stubRepository
	var executor blockingExecutor
	executor.called = make(chan struct{})
	var sender noopSender

	service, err := NewService(&repository, &executor, &sender)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		_, execErr := service.executeScheduled(ctx, 10, "run")
		done <- execErr
	}()

	<-executor.called
	cancel()

	select {
	case execErr := <-done:
		require.ErrorIs(t, execErr, context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for executeScheduled to return")
	}
}

func TestNewJobContextUsesLifecycleCancellation(t *testing.T) {
	t.Parallel()

	var repository stubRepository
	var sender noopSender

	service, err := NewService(&repository, nil, &sender)
	require.NoError(t, err)

	lifecycleCtx, cancel := context.WithCancel(context.Background())
	service.setLifecycleDone(lifecycleCtx.Done())

	ctx, ctxCancel := service.newJobContext()
	defer ctxCancel()

	cancel()

	select {
	case <-ctx.Done():
		require.ErrorIs(t, ctx.Err(), context.Canceled)
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for job context to observe lifecycle cancellation")
	}
}

func TestSetExecutorAllowsDeferredBinding(t *testing.T) {
	t.Parallel()

	var repository stubRepository
	var sender noopSender

	service, err := NewService(&repository, nil, &sender)
	require.NoError(t, err)

	_, err = service.executeScheduled(context.Background(), 10, "run")
	require.EqualError(t, err, "cron jobs executor is required")

	var executor immediateExecutor
	service.SetExecutor(&executor)

	text, err := service.executeScheduled(context.Background(), 10, "run")
	require.NoError(t, err)
	require.Equal(t, "scheduled reply", text)
}
