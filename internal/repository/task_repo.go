package repository

import (
	"database/sql"
	"fmt"

	"a2a-platform/internal/model"

	"github.com/google/uuid"
)

// TaskRepository defines the interface for task persistence operations.
type TaskRepository interface {
	Create(task *model.TaskRecord) (*model.TaskRecord, error)
	Get(localTaskID string) (*model.TaskRecord, error)
	UpdateState(localTaskID string, state model.TaskState) error
	List(agentName string, state model.TaskState) ([]*model.TaskRecord, error)
}

// sqliteTaskRepo implements TaskRepository using SQLite.
type sqliteTaskRepo struct {
	db *sql.DB
}

// NewTaskRepository creates a new TaskRepository backed by the given database.
func NewTaskRepository(db *sql.DB) TaskRepository {
	return &sqliteTaskRepo{db: db}
}

func (r *sqliteTaskRepo) Create(task *model.TaskRecord) (*model.TaskRecord, error) {
	localTaskID := uuid.New().String()
	if task.LocalTaskID != "" {
		localTaskID = task.LocalTaskID
	}

	result, err := r.db.Exec(
		`INSERT INTO tasks (local_task_id, agent_name, context_id, state, display_id)
		 VALUES (?, ?, ?, ?, ?)`,
		localTaskID, task.AgentName, task.ContextID, task.State, task.DisplayID,
	)
	if err != nil {
		return nil, fmt.Errorf("create task: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create task: get last insert id: %w", err)
	}

	var created model.TaskRecord
	err = r.db.QueryRow(
		`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
		 FROM tasks WHERE id = ?`, id,
	).Scan(
		&created.ID, &created.LocalTaskID, &created.AgentName, &created.ContextID,
		&created.State, &created.DisplayID, &created.CreatedAt, &created.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("create task: re-fetch: %w", err)
	}

	return &created, nil
}

func (r *sqliteTaskRepo) Get(localTaskID string) (*model.TaskRecord, error) {
	var task model.TaskRecord
	err := r.db.QueryRow(
		`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
		 FROM tasks WHERE local_task_id = ?`, localTaskID,
	).Scan(
		&task.ID, &task.LocalTaskID, &task.AgentName, &task.ContextID,
		&task.State, &task.DisplayID, &task.CreatedAt, &task.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get task: %w", err)
	}

	return &task, nil
}

func (r *sqliteTaskRepo) UpdateState(localTaskID string, state model.TaskState) error {
	_, err := r.db.Exec(
		`UPDATE tasks SET state = ?, updated_at = CURRENT_TIMESTAMP WHERE local_task_id = ?`,
		state, localTaskID,
	)
	if err != nil {
		return fmt.Errorf("update task state: %w", err)
	}
	return nil
}

func (r *sqliteTaskRepo) List(agentName string, state model.TaskState) ([]*model.TaskRecord, error) {
	var rows *sql.Rows
	var err error

	switch {
	case agentName != "" && state != "":
		rows, err = r.db.Query(
			`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
			 FROM tasks WHERE agent_name = ? AND state = ? ORDER BY created_at DESC`,
			agentName, state,
		)
	case agentName != "":
		rows, err = r.db.Query(
			`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
			 FROM tasks WHERE agent_name = ? ORDER BY created_at DESC`,
			agentName,
		)
	case state != "":
		rows, err = r.db.Query(
			`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
			 FROM tasks WHERE state = ? ORDER BY created_at DESC`,
			state,
		)
	default:
		rows, err = r.db.Query(
			`SELECT id, local_task_id, agent_name, context_id, state, display_id, created_at, updated_at
			 FROM tasks ORDER BY created_at DESC`,
		)
	}
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []*model.TaskRecord
	for rows.Next() {
		var task model.TaskRecord
		err := rows.Scan(
			&task.ID, &task.LocalTaskID, &task.AgentName, &task.ContextID,
			&task.State, &task.DisplayID, &task.CreatedAt, &task.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("list tasks: scan row: %w", err)
		}
		tasks = append(tasks, &task)
	}

	return tasks, nil
}
