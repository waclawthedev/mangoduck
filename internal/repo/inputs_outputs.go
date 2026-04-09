package repo

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
)

type InputsOutputsRepo struct {
	db *sql.DB
}

func NewInputsOutputsRepo(db *sql.DB) *InputsOutputsRepo {
	return &InputsOutputsRepo{db: db}
}

func (r *InputsOutputsRepo) Append(ctx context.Context, chatID int64, items []json.RawMessage) error {
	if len(items) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin inputs_outputs append transaction: %w", err)
	}

	for _, item := range items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO inputs_outputs (chat_id, item_json) VALUES (?, ?)`,
			chatID, string(item),
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("inserting inputs_outputs item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit inputs_outputs append transaction: %w", err)
	}

	return nil
}

func (r *InputsOutputsRepo) List(ctx context.Context, chatID int64) ([]json.RawMessage, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT item_json FROM inputs_outputs WHERE chat_id = ? ORDER BY id ASC`,
		chatID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing inputs_outputs items: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	items := make([]json.RawMessage, 0)
	for rows.Next() {
		var itemJSON string
		if err := rows.Scan(&itemJSON); err != nil {
			return nil, fmt.Errorf("scanning inputs_outputs item: %w", err)
		}

		items = append(items, json.RawMessage(itemJSON))
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating inputs_outputs items: %w", err)
	}

	return items, nil
}

func (r *InputsOutputsRepo) ReplaceHistory(ctx context.Context, chatID int64, items []json.RawMessage) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin inputs_outputs replace transaction: %w", err)
	}

	_, err = tx.ExecContext(ctx, `DELETE FROM inputs_outputs WHERE chat_id = ?`, chatID)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("deleting inputs_outputs items: %w", err)
	}

	for _, item := range items {
		_, err = tx.ExecContext(ctx,
			`INSERT INTO inputs_outputs (chat_id, item_json) VALUES (?, ?)`,
			chatID, string(item),
		)
		if err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("reinserting inputs_outputs item: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit inputs_outputs replace transaction: %w", err)
	}

	return nil
}

func (r *InputsOutputsRepo) Clear(ctx context.Context, chatID int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM inputs_outputs WHERE chat_id = ?`, chatID)
	if err != nil {
		return fmt.Errorf("clearing inputs_outputs items: %w", err)
	}

	return nil
}
