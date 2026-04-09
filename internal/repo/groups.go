package repo

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

var ErrGroupNotFound = errors.New("group not found")

type GroupStatus string

const (
	GroupStatusActive   GroupStatus = "active"
	GroupStatusInactive GroupStatus = "inactive"
)

type Group struct {
	ID        int64
	TGID      int64
	Title     string
	Type      string
	Status    GroupStatus
	CreatedAt time.Time
}

type GroupsRepo struct {
	db *sql.DB
}

func NewGroupsRepo(db *sql.DB) *GroupsRepo {
	return &GroupsRepo{db: db}
}

func (r *GroupsRepo) Create(ctx context.Context, tgID int64, title string, chatType string, status GroupStatus) (*Group, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO groups (tg_id, title, type, status) VALUES (?, ?, ?, ?)`,
		tgID, title, chatType, status,
	)
	if err != nil {
		return nil, fmt.Errorf("inserting group: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("getting last insert id: %w", err)
	}

	return r.GetByID(ctx, id)
}

func (r *GroupsRepo) GetByID(ctx context.Context, id int64) (*Group, error) {
	var group Group
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, title, type, status, created_at FROM groups WHERE id = ?`, id,
	).Scan(&group.ID, &group.TGID, &group.Title, &group.Type, &group.Status, &group.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGroupNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying group by id: %w", err)
	}

	return &group, nil
}

func (r *GroupsRepo) GetByTGID(ctx context.Context, tgID int64) (*Group, error) {
	var group Group
	err := r.db.QueryRowContext(ctx,
		`SELECT id, tg_id, title, type, status, created_at FROM groups WHERE tg_id = ?`, tgID,
	).Scan(&group.ID, &group.TGID, &group.Title, &group.Type, &group.Status, &group.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGroupNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("querying group by tg_id: %w", err)
	}

	return &group, nil
}

func (r *GroupsRepo) UpdateProfile(ctx context.Context, tgID int64, title string, chatType string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE groups SET title = ?, type = ? WHERE tg_id = ?`,
		title, chatType, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating group profile: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrGroupNotFound
	}

	return nil
}

func (r *GroupsRepo) UpdateStatus(ctx context.Context, tgID int64, status GroupStatus) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE groups SET status = ? WHERE tg_id = ?`,
		status, tgID,
	)
	if err != nil {
		return fmt.Errorf("updating group status: %w", err)
	}

	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("getting rows affected: %w", err)
	}
	if rows == 0 {
		return ErrGroupNotFound
	}

	return nil
}

func (r *GroupsRepo) List(ctx context.Context) ([]*Group, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, tg_id, title, type, status, created_at FROM groups ORDER BY created_at ASC, id ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("listing groups: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	groups := make([]*Group, 0)
	for rows.Next() {
		var group Group
		err = rows.Scan(&group.ID, &group.TGID, &group.Title, &group.Type, &group.Status, &group.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("scanning group: %w", err)
		}

		groups = append(groups, &group)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating groups: %w", err)
	}

	return groups, nil
}
