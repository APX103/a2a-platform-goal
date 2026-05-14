package repository

import (
	"database/sql"
	"fmt"

	"a2a-platform/internal/model"
)

// MessageRepository defines the interface for message persistence operations.
type MessageRepository interface {
	Create(msg *model.MessageRecord) (*model.MessageRecord, error)
	ListByTaskID(taskID int64) ([]*model.MessageRecord, error)
}

// sqliteMessageRepo implements MessageRepository using SQLite.
type sqliteMessageRepo struct {
	db *sql.DB
}

// NewMessageRepository creates a new MessageRepository backed by the given database.
func NewMessageRepository(db *sql.DB) MessageRepository {
	return &sqliteMessageRepo{db: db}
}

func (r *sqliteMessageRepo) Create(msg *model.MessageRecord) (*model.MessageRecord, error) {
	result, err := r.db.Exec(
		`INSERT INTO messages (task_id, role, content) VALUES (?, ?, ?)`,
		msg.TaskID, msg.Role, msg.Content,
	)
	if err != nil {
		return nil, fmt.Errorf("create message: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create message: get last insert id: %w", err)
	}

	var created model.MessageRecord
	err = r.db.QueryRow(
		`SELECT id, task_id, role, content, created_at FROM messages WHERE id = ?`, id,
	).Scan(&created.ID, &created.TaskID, &created.Role, &created.Content, &created.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("create message: re-fetch: %w", err)
	}

	return &created, nil
}

func (r *sqliteMessageRepo) ListByTaskID(taskID int64) ([]*model.MessageRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, role, content, created_at FROM messages WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list messages by task id: %w", err)
	}
	defer rows.Close()

	var messages []*model.MessageRecord
	for rows.Next() {
		var msg model.MessageRecord
		err := rows.Scan(&msg.ID, &msg.TaskID, &msg.Role, &msg.Content, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("list messages: scan row: %w", err)
		}
		messages = append(messages, &msg)
	}

	return messages, nil
}
