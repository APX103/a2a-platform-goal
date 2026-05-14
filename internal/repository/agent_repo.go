package repository

import (
	"database/sql"
	"fmt"

	"a2a-platform/internal/model"
)

// AgentRepository defines the interface for agent persistence operations.
type AgentRepository interface {
	Create(agent *model.AgentRecord) (*model.AgentRecord, error)
	GetByName(name string) (*model.AgentRecord, error)
	List() ([]*model.AgentRecord, error)
	Upsert(agent *model.AgentRecord) (*model.AgentRecord, error)
	UpdateStatus(name string, status model.AgentStatus, errorMessage string) error
	Delete(name string) error
}

// sqliteAgentRepo implements AgentRepository using SQLite.
type sqliteAgentRepo struct {
	db *sql.DB
}

// NewAgentRepository creates a new AgentRepository backed by the given database.
func NewAgentRepository(db *sql.DB) AgentRepository {
	return &sqliteAgentRepo{db: db}
}

func (r *sqliteAgentRepo) Create(agent *model.AgentRecord) (*model.AgentRecord, error) {
	result, err := r.db.Exec(
		`INSERT INTO agents (name, url, description, version, type, status, skills_json, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.Name, agent.URL, agent.Description, agent.Version, agent.AgentType,
		agent.Status, agent.SkillsJSON, agent.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("create agent: get last insert id: %w", err)
	}

	created := *agent
	created.ID = id
	return &created, nil
}

func (r *sqliteAgentRepo) GetByName(name string) (*model.AgentRecord, error) {
	row := r.db.QueryRow(
		`SELECT id, name, url, description, version, type, status, skills_json, error_message, created_at, updated_at
		 FROM agents WHERE name = ?`, name,
	)

	var agent model.AgentRecord
	var skillsJSON, errorMessage sql.NullString

	err := row.Scan(
		&agent.ID, &agent.Name, &agent.URL, &agent.Description, &agent.Version,
		&agent.AgentType, &agent.Status, &skillsJSON, &errorMessage,
		&agent.CreatedAt, &agent.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get agent by name: %w", err)
	}

	if skillsJSON.Valid {
		agent.SkillsJSON = skillsJSON.String
	}
	if errorMessage.Valid {
		agent.ErrorMessage = errorMessage.String
	}

	return &agent, nil
}

func (r *sqliteAgentRepo) List() ([]*model.AgentRecord, error) {
	rows, err := r.db.Query(
		`SELECT id, name, url, description, version, type, status, skills_json, error_message, created_at, updated_at
		 FROM agents ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list agents: %w", err)
	}
	defer rows.Close()

	var agents []*model.AgentRecord
	for rows.Next() {
		var agent model.AgentRecord
		var skillsJSON, errorMessage sql.NullString

		err := rows.Scan(
			&agent.ID, &agent.Name, &agent.URL, &agent.Description, &agent.Version,
			&agent.AgentType, &agent.Status, &skillsJSON, &errorMessage,
			&agent.CreatedAt, &agent.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("list agents: scan row: %w", err)
		}

		if skillsJSON.Valid {
			agent.SkillsJSON = skillsJSON.String
		}
		if errorMessage.Valid {
			agent.ErrorMessage = errorMessage.String
		}

		agents = append(agents, &agent)
	}

	return agents, nil
}

func (r *sqliteAgentRepo) Upsert(agent *model.AgentRecord) (*model.AgentRecord, error) {
	result, err := r.db.Exec(
		`INSERT INTO agents (name, url, description, version, type, status, skills_json, error_message)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(name) DO UPDATE SET
			url = excluded.url,
			description = excluded.description,
			version = excluded.version,
			type = excluded.type,
			status = excluded.status,
			skills_json = excluded.skills_json,
			error_message = excluded.error_message,
			updated_at = CURRENT_TIMESTAMP`,
		agent.Name, agent.URL, agent.Description, agent.Version, agent.AgentType,
		agent.Status, agent.SkillsJSON, agent.ErrorMessage,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert agent: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("upsert agent: get last insert id: %w", err)
	}

	// Re-fetch to get the correct created_at for existing rows.
	var upserted model.AgentRecord
	var skillsJSON, errorMessage sql.NullString

	err = r.db.QueryRow(
		`SELECT id, name, url, description, version, type, status, skills_json, error_message, created_at, updated_at
		 FROM agents WHERE id = ?`, id,
	).Scan(
		&upserted.ID, &upserted.Name, &upserted.URL, &upserted.Description, &upserted.Version,
		&upserted.AgentType, &upserted.Status, &skillsJSON, &errorMessage,
		&upserted.CreatedAt, &upserted.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("upsert agent: re-fetch: %w", err)
	}

	if skillsJSON.Valid {
		upserted.SkillsJSON = skillsJSON.String
	}
	if errorMessage.Valid {
		upserted.ErrorMessage = errorMessage.String
	}

	return &upserted, nil
}

func (r *sqliteAgentRepo) UpdateStatus(name string, status model.AgentStatus, errorMessage string) error {
	_, err := r.db.Exec(
		`UPDATE agents SET status = ?, error_message = ?, updated_at = CURRENT_TIMESTAMP WHERE name = ?`,
		status, errorMessage, name,
	)
	if err != nil {
		return fmt.Errorf("update agent status: %w", err)
	}
	return nil
}

func (r *sqliteAgentRepo) Delete(name string) error {
	_, err := r.db.Exec(`DELETE FROM agents WHERE name = ?`, name)
	if err != nil {
		return fmt.Errorf("delete agent: %w", err)
	}
	return nil
}
