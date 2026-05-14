package repository

import (
	"database/sql"
	"fmt"
	"testing"

	"a2a-platform/internal/model"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := InitDB(db); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return db
}

func TestCreateAndGet(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAgentRepository(db)

	agent := &model.AgentRecord{
		Name:        "test-agent",
		URL:         "http://localhost:8080",
		Description: "A test agent",
		Version:     "1.0.0",
		AgentType:   "chat",
		Status:      model.AgentStatusConnected,
		SkillsJSON:  `["coding","testing"]`,
	}

	created, err := repo.Create(agent)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID")
	}
	if created.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", created.Name)
	}

	got, err := repo.GetByName("test-agent")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected agent, got nil")
	}
	if got.URL != "http://localhost:8080" {
		t.Errorf("expected url 'http://localhost:8080', got '%s'", got.URL)
	}
	if got.SkillsJSON != `["coding","testing"]` {
		t.Errorf("expected skills_json '[\"coding\",\"testing\"]', got '%s'", got.SkillsJSON)
	}
}

func TestGetByNameMissing(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAgentRepository(db)

	got, err := repo.GetByName("nonexistent")
	if err != nil {
		t.Fatalf("GetByName failed for missing agent: %v", err)
	}
	if got != nil {
		t.Error("expected nil for missing agent, got non-nil")
	}
}

func TestUpsert(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAgentRepository(db)

	original := &model.AgentRecord{
		Name:        "upsert-agent",
		URL:         "http://localhost:8080",
		Description: "Original description",
		Version:     "1.0.0",
		AgentType:   "chat",
		Status:      model.AgentStatusDisconnected,
	}

	// Insert
	created, err := repo.Upsert(original)
	if err != nil {
		t.Fatalf("Upsert (insert) failed: %v", err)
	}
	if created.ID == 0 {
		t.Error("expected non-zero ID on insert")
	}

	// Update with same name but different fields
	updated := &model.AgentRecord{
		Name:        "upsert-agent",
		URL:         "http://localhost:9090",
		Description: "Updated description",
		Version:     "2.0.0",
		AgentType:   "search",
		Status:      model.AgentStatusConnected,
	}

	result, err := repo.Upsert(updated)
	if err != nil {
		t.Fatalf("Upsert (update) failed: %v", err)
	}
	if result.ID != created.ID {
		t.Errorf("expected same ID %d, got %d", created.ID, result.ID)
	}
	if result.URL != "http://localhost:9090" {
		t.Errorf("expected updated url, got '%s'", result.URL)
	}
	if result.Description != "Updated description" {
		t.Errorf("expected updated description, got '%s'", result.Description)
	}
	if result.Version != "2.0.0" {
		t.Errorf("expected updated version, got '%s'", result.Version)
	}

	// Verify count is still 1
	agents, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestList(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAgentRepository(db)

	for i := 0; i < 3; i++ {
		_, err := repo.Create(&model.AgentRecord{
			Name:  fmt.Sprintf("agent-%d", i),
			URL:   fmt.Sprintf("http://localhost:808%d", i),
			Status: model.AgentStatusConnected,
		})
		if err != nil {
			t.Fatalf("Create agent-%d failed: %v", i, err)
		}
	}

	agents, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(agents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(agents))
	}
}

func TestUpdateStatus(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	repo := NewAgentRepository(db)

	_, err := repo.Create(&model.AgentRecord{
		Name:   "status-agent",
		URL:    "http://localhost:8080",
		Status: model.AgentStatusConnected,
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	err = repo.UpdateStatus("status-agent", model.AgentStatusError, "connection refused")
	if err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	got, err := repo.GetByName("status-agent")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if got.Status != model.AgentStatusError {
		t.Errorf("expected status 'error', got '%s'", got.Status)
	}
	if got.ErrorMessage != "connection refused" {
		t.Errorf("expected error_message 'connection refused', got '%s'", got.ErrorMessage)
	}
}
