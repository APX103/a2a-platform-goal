package repository

import "database/sql"

// InitDB creates the database schema (tables and indexes) for the A2A platform.
func InitDB(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS agents (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL UNIQUE,
		url TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		version TEXT NOT NULL DEFAULT '',
		type TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'disconnected',
		skills_json TEXT NOT NULL DEFAULT '[]',
		error_message TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS tasks (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		local_task_id TEXT NOT NULL,
		agent_name TEXT NOT NULL,
		context_id TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'SUBMITTED',
		display_id TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id INTEGER NOT NULL REFERENCES tasks(id),
		role TEXT NOT NULL,
		content TEXT NOT NULL DEFAULT '',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS trace_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL,
		context_id TEXT NOT NULL DEFAULT '',
		agent_name TEXT NOT NULL DEFAULT '',
		target_agent TEXT NOT NULL DEFAULT '',
		event_type TEXT NOT NULL,
		data_json TEXT NOT NULL DEFAULT '{}',
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_context_id ON tasks(context_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_agent_name ON tasks(agent_name);
	CREATE INDEX IF NOT EXISTS idx_messages_task_id ON messages(task_id);
	CREATE INDEX IF NOT EXISTS idx_traces_task_id ON trace_events(task_id);
	CREATE INDEX IF NOT EXISTS idx_traces_agent_name ON trace_events(agent_name);
	`
	_, err := db.Exec(schema)
	return err
}
