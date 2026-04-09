package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrCronTaskNotFound = errors.New("cron task not found")

type CronTask struct {
	ID            int64
	ChatID        int64
	CreatedByTGID int64
	Schedule      string
	Prompt        string
	CreatedAt     time.Time
}

type CronTasksRepo struct {
	db *sql.DB
}

func NewCronTasksRepo(db *sql.DB) *CronTasksRepo {
	return &CronTasksRepo{db: db}
}

func (r *CronTasksRepo) Create(ctx context.Context, chatID int64, createdByTGID int64, schedule string, prompt string) (*CronTask, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO cron_tasks (chat_id, created_by_tg_id, schedule, prompt) VALUES (?, ?, ?, ?)`,
		chatID, createdByTGID, schedule, prompt,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting cron task: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting cron task last insert id: %w", err)
	}

	return r.GetByID(ctx, id)
}

func (r *CronTasksRepo) GetByID(ctx context.Context, id int64) (*CronTask, error) {
	var task CronTask
	err := r.db.QueryRowContext(ctx,
		`SELECT id, chat_id, created_by_tg_id, schedule, prompt, created_at FROM cron_tasks WHERE id = ?`,
		id,
	).Scan(&task.ID, &task.ChatID, &task.CreatedByTGID, &task.Schedule, &task.Prompt, &task.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCronTaskNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying cron task by id: %w", err)
	}

	return &task, nil
}

func (r *CronTasksRepo) List(ctx context.Context) ([]*CronTask, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, chat_id, created_by_tg_id, schedule, prompt, created_at FROM cron_tasks ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing cron tasks: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	tasks := make([]*CronTask, 0)
	for rows.Next() {
		var task CronTask
		err = rows.Scan(&task.ID, &task.ChatID, &task.CreatedByTGID, &task.Schedule, &task.Prompt, &task.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning cron task: %w", err)
		}

		tasks = append(tasks, &task)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating cron tasks: %w", err)
	}

	return tasks, nil
}

func (r *CronTasksRepo) ListByChatID(ctx context.Context, chatID int64) ([]*CronTask, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, chat_id, created_by_tg_id, schedule, prompt, created_at FROM cron_tasks WHERE chat_id = ? ORDER BY created_at ASC, id ASC`,
		chatID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing cron tasks by chat id: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	tasks := make([]*CronTask, 0)
	for rows.Next() {
		var task CronTask
		err = rows.Scan(&task.ID, &task.ChatID, &task.CreatedByTGID, &task.Schedule, &task.Prompt, &task.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning cron task: %w", err)
		}

		tasks = append(tasks, &task)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating cron tasks: %w", err)
	}

	return tasks, nil
}

func (r *CronTasksRepo) DeleteByID(ctx context.Context, id int64) (*CronTask, error) {
	task, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM cron_tasks WHERE id = ?`, id)
	if err != nil {
		return nil, fmt.Errorf("deleting cron task: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("getting cron task rows affected: %w", err)
	}
	if rows == 0 {
		return nil, ErrCronTaskNotFound
	}

	return task, nil
}
