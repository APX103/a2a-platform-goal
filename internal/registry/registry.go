package registry

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"
	"a2a-platform/pkg/a2a"
)

// AgentConnection represents a live connection to an agent.
type AgentConnection struct {
	Name   string
	URL    string
	Card   *a2a.AgentCard
	Type   string
	Client *a2a.Client
}

// Registry manages agent connections with in-memory cache and database persistence.
type Registry struct {
	repo        repository.AgentRepository
	connections map[string]*AgentConnection
	mu          sync.RWMutex
	retryMax    int
	baseDelay   time.Duration
}

// New creates a new Registry with the given repository.
func New(repo repository.AgentRepository) *Registry {
	return &Registry{
		repo:        repo,
		connections: make(map[string]*AgentConnection),
		retryMax:    3,
		baseDelay:   time.Second,
	}
}

// NewWithRetry creates a new Registry with custom retry settings.
func NewWithRetry(repo repository.AgentRepository, retryMax int, baseDelay time.Duration) *Registry {
	return &Registry{
		repo:        repo,
		connections: make(map[string]*AgentConnection),
		retryMax:    retryMax,
		baseDelay:   baseDelay,
	}
}

// ConnectByURL fetches an AgentCard from the given URL, retries with exponential
// backoff on failure, and stores the connection in memory and the database.
func (r *Registry) ConnectByURL(url, agentType string) (*AgentConnection, error) {
	client := a2a.NewClient(url)

	var card *a2a.AgentCard
	var err error

	for attempt := 0; attempt < r.retryMax; attempt++ {
		card, err = client.FetchAgentCard()
		if err == nil {
			break
		}
		if attempt < r.retryMax-1 {
			delay := r.baseDelay * time.Duration(1<<uint(attempt))
			time.Sleep(delay)
		}
	}
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", url, err)
	}

	name := card.Name
	if name == "" {
		name = url
	}

	skillsJSON := "[]"
	if len(card.Skills) > 0 {
		skills, _ := json.Marshal(card.Skills)
		skillsJSON = string(skills)
	}

	record := &model.AgentRecord{
		Name:        name,
		URL:         url,
		Description: card.Description,
		Version:     card.Version,
		AgentType:   agentType,
		Status:      model.AgentStatusConnected,
		SkillsJSON:  skillsJSON,
	}

	_, err = r.repo.Upsert(record)
	if err != nil {
		return nil, fmt.Errorf("persist agent %s: %w", name, err)
	}

	conn := &AgentConnection{
		Name:   name,
		URL:    url,
		Card:   card,
		Type:   agentType,
		Client: client,
	}

	r.mu.Lock()
	r.connections[name] = conn
	r.mu.Unlock()

	return conn, nil
}

// Disconnect removes an agent from memory and marks it as disconnected in the database.
func (r *Registry) Disconnect(name string) {
	r.mu.Lock()
	delete(r.connections, name)
	r.mu.Unlock()

	_ = r.repo.UpdateStatus(name, model.AgentStatusDisconnected, "")
}

// GetClient returns the in-memory AgentConnection for the given agent name, or nil.
func (r *Registry) GetClient(name string) *AgentConnection {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.connections[name]
}

// GetAgent returns the database record for the given agent name.
func (r *Registry) GetAgent(name string) (*model.AgentRecord, error) {
	agent, err := r.repo.GetByName(name)
	if err != nil {
		return nil, fmt.Errorf("get agent %s: %w", name, err)
	}
	return agent, nil
}

// ListAgents returns all agents from the database, overlaying live connection info
// (description, version) from AgentCards of connected agents.
func (r *Registry) ListAgents() []*model.AgentRecord {
	agents, err := r.repo.List()
	if err != nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range agents {
		if conn, ok := r.connections[agent.Name]; ok {
			if conn.Card != nil {
				if conn.Card.Description != "" {
					agent.Description = conn.Card.Description
				}
				if conn.Card.Version != "" {
					agent.Version = conn.Card.Version
				}
			}
			agent.Status = model.AgentStatusConnected
		}
	}

	return agents
}

// UpsertAgent creates or updates an agent record in the database.
func (r *Registry) UpsertAgent(record *model.AgentRecord) error {
	_, err := r.repo.Upsert(record)
	if err != nil {
		return fmt.Errorf("upsert agent %s: %w", record.Name, err)
	}
	return nil
}

// DeleteAgent removes an agent from memory and the database.
func (r *Registry) DeleteAgent(name string) error {
	r.mu.Lock()
	delete(r.connections, name)
	r.mu.Unlock()

	err := r.repo.Delete(name)
	if err != nil {
		return fmt.Errorf("delete agent %s: %w", name, err)
	}
	return nil
}
