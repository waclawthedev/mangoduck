package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrChatNotFound = errors.New("chat not found")

type ChatStatus string

const (
	ChatStatusActive   ChatStatus = "active"
	ChatStatusInactive ChatStatus = "inactive"
)

type Chat struct {
	ID         int64
	TGID       int64
	Title      string
	Username   string
	Type       string
	Status     ChatStatus
	MemoryText string
	CreatedAt  time.Time
}

type ChatsRepo struct {
	db *sql.DB
}

func NewChatsRepo(db *sql.DB) *ChatsRepo {
	return &ChatsRepo{db: db}
}

func (r *ChatsRepo) Create(ctx context.Context, tgID int64, title string, username string, chatType string, status ChatStatus) (*Chat, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO chats (tg_id, title, username, type, status) VALUES (?, ?, ?, ?, ?)`,
		tgID, title, username, chatType, status,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting chat: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert id: %w", err)
	}

	return r.GetByID(ctx, id)
}

func (r *ChatsRepo) GetByID(ctx context.Context, id int64) (*Chat, error) {
	var chat Chat
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, title, username, type, status, memory_text, created_at FROM chats WHERE id = ?`,
		id,
	).Scan(&chat.ID, &chat.TGID, &chat.Title, &chat.Username, &chat.Type, &chat.Status, &chat.MemoryText, &chat.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrChatNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying chat by id: %w", err)
	}

	return &chat, nil
}

func (r *ChatsRepo) GetByTGID(ctx context.Context, tgID int64) (*Chat, error) {
	var chat Chat
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, title, username, type, status, memory_text, created_at FROM chats WHERE tg_id = ?`,
		tgID,
	).Scan(&chat.ID, &chat.TGID, &chat.Title, &chat.Username, &chat.Type, &chat.Status, &chat.MemoryText, &chat.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrChatNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying chat by tg_id: %w", err)
	}

	return &chat, nil
}

func (r *ChatsRepo) UpdateProfile(ctx context.Context, tgID int64, title string, username string, chatType string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE chats SET title = ?, username = ?, type = ? WHERE tg_id = ?`,
		title, username, chatType, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating chat profile: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrChatNotFound
	}

	return nil
}

func (r *ChatsRepo) UpdateStatus(ctx context.Context, tgID int64, status ChatStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE chats SET status = ? WHERE tg_id = ?`,
		status, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating chat status: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrChatNotFound
	}

	return nil
}

func (r *ChatsRepo) List(ctx context.Context) ([]*Chat, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tg_id, title, username, type, status, memory_text, created_at FROM chats ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing chats: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	chats := make([]*Chat, 0)
	for rows.Next() {
		var chat Chat
		err = rows.Scan(&chat.ID, &chat.TGID, &chat.Title, &chat.Username, &chat.Type, &chat.Status, &chat.MemoryText, &chat.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning chat: %w", err)
		}

		chats = append(chats, &chat)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating chats: %w", err)
	}

	return chats, nil
}

func (r *ChatsRepo) GetMemory(ctx context.Context, tgID int64) (string, error) {
	var memoryText string
	err := r.db.QueryRowContext(ctx, `SELECT memory_text FROM chats WHERE tg_id = ?`, tgID).Scan(&memoryText)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrChatNotFound
	}
	if err != nil {
		return "", fmt.Errorf("querying chat memory by tg_id: %w", err)
	}

	return memoryText, nil
}

func (r *ChatsRepo) SetMemory(ctx context.Context, tgID int64, memoryText string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE chats SET memory_text = ? WHERE tg_id = ?`, memoryText, tgID)
	if err != nil {
		return fmt.Errorf("updating chat memory: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrChatNotFound
	}

	return nil
}
