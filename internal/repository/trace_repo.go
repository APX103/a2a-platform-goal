package repository

import (
	"database/sql"
	"fmt"
	"strings"

	"a2a-platform/internal/model"
)

// TraceRepository defines the interface for trace event persistence operations.
type TraceRepository interface {
	Record(event *model.TraceEventRecord) (*model.TraceEventRecord, error)
	GetTimeline(taskID string) ([]*model.TraceEventRecord, error)
	GetDebugTrace(taskID string) (string, error)
	GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error)
}

// sqliteTraceRepo implements TraceRepository using SQLite.
type sqliteTraceRepo struct {
	db *sql.DB
}

// NewTraceRepository creates a new TraceRepository backed by the given database.
func NewTraceRepository(db *sql.DB) TraceRepository {
	return &sqliteTraceRepo{db: db}
}

func (r *sqliteTraceRepo) Record(event *model.TraceEventRecord) (*model.TraceEventRecord, error) {
	result, err := r.db.Exec(
		`INSERT INTO trace_events (task_id, context_id, agent_name, target_agent, event_type, data_json)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		event.TaskID, event.ContextID, event.AgentName, event.TargetAgent,
		event.EventType, event.DataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("record trace event: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("record trace event: get last insert id: %w", err)
	}

	var created model.TraceEventRecord
	err = r.db.QueryRow(
		`SELECT id, task_id, context_id, agent_name, target_agent, event_type, data_json, created_at
		 FROM trace_events WHERE id = ?`, id,
	).Scan(
		&created.ID, &created.TaskID, &created.ContextID, &created.AgentName,
		&created.TargetAgent, &created.EventType, &created.DataJSON, &created.CreatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("record trace event: re-fetch: %w", err)
	}

	return &created, nil
}

func (r *sqliteTraceRepo) GetTimeline(taskID string) ([]*model.TraceEventRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, task_id, context_id, agent_name, target_agent, event_type, data_json, created_at
		 FROM trace_events WHERE task_id = ? ORDER BY created_at ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("get timeline: %w", err)
	}
	defer rows.Close()

	var events []*model.TraceEventRecord
	for rows.Next() {
		var event model.TraceEventRecord
		err := rows.Scan(
			&event.ID, &event.TaskID, &event.ContextID, &event.AgentName,
			&event.TargetAgent, &event.EventType, &event.DataJSON, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("get timeline: scan row: %w", err)
		}
		events = append(events, &event)
	}

	return events, nil
}

func (r *sqliteTraceRepo) GetDebugTrace(taskID string) (string, error) {
	events, err := r.GetTimeline(taskID)
	if err != nil {
		return "", err
	}

	if len(events) == 0 {
		return "(no trace yet)", nil
	}

	var lines []string
	for _, e := range events {
		lines = append(lines, fmt.Sprintf("[%s] %s %s", e.CreatedAt.Format("15:04:05.000"), e.EventType, e.DataJSON))
	}

	return strings.Join(lines, "\n"), nil
}

func (r *sqliteTraceRepo) GetRecentByAgent(agentName string, limit int) ([]*model.TraceEventRecord, error) {
	if limit <= 0 {
		limit = 10
	}

	rows, err := r.db.Query(
		`SELECT id, task_id, context_id, agent_name, target_agent, event_type, data_json, created_at
		 FROM trace_events WHERE agent_name = ? ORDER BY id DESC LIMIT ?`,
		agentName, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("get recent by agent: %w", err)
	}
	defer rows.Close()

	var events []*model.TraceEventRecord
	for rows.Next() {
		var event model.TraceEventRecord
		err := rows.Scan(
			&event.ID, &event.TaskID, &event.ContextID, &event.AgentName,
			&event.TargetAgent, &event.EventType, &event.DataJSON, &event.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("get recent by agent: scan row: %w", err)
		}
		events = append(events, &event)
	}

	return events, nil
}
