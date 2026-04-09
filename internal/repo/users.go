package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrUserNotFound = errors.New("user not found")

type UserStatus string

const (
	UserStatusActive   UserStatus = "active"
	UserStatusInactive UserStatus = "inactive"
)

type User struct {
	ID        int64
	TGID      int64
	Username  string
	Status    UserStatus
	CreatedAt time.Time
}

type UsersRepo struct {
	db *sql.DB
}

func NewUsersRepo(db *sql.DB) *UsersRepo {
	return &UsersRepo{db: db}
}

func (r *UsersRepo) Create(ctx context.Context, tgID int64, username string, status UserStatus) (*User, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO users (tg_id, username, status) VALUES (?, ?, ?)`,
		tgID, username, status,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting user: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert id: %w", err)
	}

	return r.GetByID(ctx, id)
}

func (r *UsersRepo) GetByID(ctx context.Context, id int64) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, username, status, created_at FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.TGID, &u.Username, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by id: %w", err)
	}

	return &u, nil
}

func (r *UsersRepo) GetByTGID(ctx context.Context, tgID int64) (*User, error) {
	var u User
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, username, status, created_at FROM users WHERE tg_id = ?`, tgID,
	).Scan(&u.ID, &u.TGID, &u.Username, &u.Status, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying user by tg_id: %w", err)
	}

	return &u, nil
}

func (r *UsersRepo) UpdateProfile(ctx context.Context, tgID int64, username string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET username = ? WHERE tg_id = ?`,
		username, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating user profile: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (r *UsersRepo) UpdateStatus(ctx context.Context, tgID int64, status UserStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET status = ? WHERE tg_id = ?`,
		status, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating user status: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrUserNotFound
	}

	return nil
}

func (r *UsersRepo) List(ctx context.Context) ([]*User, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tg_id, username, status, created_at FROM users ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing users: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	users := make([]*User, 0)
	for rows.Next() {
		var user User
		err = rows.Scan(&user.ID, &user.TGID, &user.Username, &user.Status, &user.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}

		users = append(users, &user)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating users: %w", err)
	}

	return users, nil
}
