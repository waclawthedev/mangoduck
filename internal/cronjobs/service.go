package cronjobs

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/robfig/cron/v3"
	"go.uber.org/zap"

	"mangoduck/internal/logging"
	"mangoduck/internal/repo"
)

type Lister interface {
	List(ctx context.Context) ([]*repo.CronTask, error)
}

type Executor interface {
	ExecuteScheduled(ctx context.Context, chatID int64, prompt string) (string, error)
}

type Sender interface {
	Send(chatID int64, text string) error
}

type Service struct {
	repo     Lister
	executor Executor
	sender   Sender
	runner   *cron.Cron
	logger   *zap.Logger

	mu       sync.Mutex
	entryIDs map[int64]cron.EntryID
	running  map[int64]*atomic.Bool
}

type Option func(*Service)

func NewService(repo Lister, executor Executor, sender Sender, options ...Option) (*Service, error) {
	if repo == nil {
		return nil, errors.New("cron jobs repository is required")
	}
	if sender == nil {
		return nil, errors.New("cron jobs sender is required")
	}

	var service Service
	service.repo = repo
	service.executor = executor
	service.sender = sender
	service.runner = cron.New(cron.WithLocation(time.Local))
	service.logger = zap.NewNop()
	service.entryIDs = make(map[int64]cron.EntryID)
	service.running = make(map[int64]*atomic.Bool)

	for _, option := range options {
		if option == nil {
			continue
		}

		option(&service)
	}

	return &service, nil
}

func (s *Service) SetExecutor(executor Executor) {
	if s == nil {
		return
	}

	s.executor = executor
}

func WithLogger(logger *zap.Logger) Option {
	return func(service *Service) {
		if service == nil {
			return
		}

		service.logger = logging.WithComponent(logger, "cronjobs")
	}
}

func (s *Service) Start(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	tasks, err := s.repo.List(ctx)
	if err != nil {
		return fmt.Errorf("listing cron tasks: %w", err)
	}

	if len(tasks) == 0 {
		s.logger.Debug("no persisted cron tasks found")
	}

	for _, task := range tasks {
		if task == nil {
			continue
		}

		err = s.AddTask(task)
		if err != nil {
			s.logger.Error("skipping invalid persisted cron task", zap.Int64("task_id", task.ID), zap.String("schedule", task.Schedule), zap.Error(err))
			continue
		}

		s.logger.Debug("registered persisted cron task", zap.Int64("task_id", task.ID), zap.Int64("chat_id", task.ChatID), zap.Int64("created_by_tg_id", task.CreatedByTGID), zap.String("schedule", task.Schedule), zap.String("prompt", task.Prompt))
	}

	s.runner.Start()
	s.logger.Debug("cron scheduler started")

	go func() {
		<-ctx.Done()
		s.logger.Debug("cron scheduler stopping")
		stopCtx := s.runner.Stop()
		<-stopCtx.Done()
		s.logger.Debug("cron scheduler stopped")
	}()

	return nil
}

func (s *Service) AddTask(task *repo.CronTask) error {
	return s.AddTaskWithContext(context.Background(), task)
}

func (s *Service) AddTaskWithContext(ctx context.Context, task *repo.CronTask) error {
	if task == nil {
		return errors.New("cron task is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	schedule := strings.TrimSpace(task.Schedule)
	if schedule == "" {
		return errors.New("cron task schedule is required")
	}

	entryID, err := s.runner.AddFunc(schedule, s.buildJob(ctx, task))
	if err != nil {
		return fmt.Errorf("registering cron task: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if previousEntryID, ok := s.entryIDs[task.ID]; ok {
		s.runner.Remove(previousEntryID)
	}

	s.entryIDs[task.ID] = entryID
	if _, ok := s.running[task.ID]; !ok {
		s.running[task.ID] = &atomic.Bool{}
	}

	return nil
}

func (s *Service) RemoveTask(taskID int64) {
	s.mu.Lock()
	defer s.mu.Unlock()

	entryID, ok := s.entryIDs[taskID]
	if !ok {
		return
	}

	s.runner.Remove(entryID)
	delete(s.entryIDs, taskID)
	delete(s.running, taskID)
}

func (s *Service) buildJob(ctx context.Context, task *repo.CronTask) func() {
	taskID := task.ID
	chatID := task.ChatID
	prompt := strings.TrimSpace(task.Prompt)

	return func() {
		running := s.runningFlag(taskID)
		if !running.CompareAndSwap(false, true) {
			s.logger.Debug("cron task skipped because previous run is still in progress", zap.Int64("task_id", taskID))
			return
		}
		defer running.Store(false)

		s.logger.Debug("cron task started", zap.Int64("task_id", taskID), zap.Int64("chat_id", chatID))

		result, err := s.executeScheduled(ctx, chatID, prompt)
		if err != nil {
			s.logger.Error("cron task execution failed", zap.Int64("task_id", taskID), zap.Error(err))
			return
		}

		result = strings.TrimSpace(result)
		if result == "" {
			s.logger.Debug("cron task produced empty response", zap.Int64("task_id", taskID))
			return
		}

		err = s.sendResult(ctx, chatID, result)
		if err != nil {
			s.logger.Error("cron task send failed", zap.Int64("task_id", taskID), zap.Int64("chat_id", chatID), zap.Error(err))
			return
		}

		s.logger.Debug("cron task delivered message", zap.Int64("task_id", taskID), zap.Int64("chat_id", chatID))
	}
}

func (s *Service) runningFlag(taskID int64) *atomic.Bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	flag, ok := s.running[taskID]
	if ok {
		return flag
	}

	flag = &atomic.Bool{}
	s.running[taskID] = flag

	return flag
}

func (s *Service) executeScheduled(ctx context.Context, chatID int64, prompt string) (string, error) {
	if s.executor == nil {
		return "", errors.New("cron jobs executor is required")
	}

	type result struct {
		text string
		err  error
	}

	resultCh := make(chan result, 1)
	go func() {
		text, err := s.executor.ExecuteScheduled(ctx, chatID, prompt)
		resultCh <- result{text: text, err: err}
	}()

	select {
	case <-ctx.Done():
		return "", ctx.Err()
	case res := <-resultCh:
		return res.text, res.err
	}
}

func (s *Service) sendResult(ctx context.Context, chatID int64, text string) error {
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.sender.Send(chatID, text)
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}
