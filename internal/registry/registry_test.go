package registry

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"a2a-platform/internal/model"
	"a2a-platform/internal/repository"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	if err := repository.InitDB(db); err != nil {
		t.Fatalf("failed to init db: %v", err)
	}
	return db
}

func newFakeAgentServer(t *testing.T) *httptest.Server {
	t.Helper()
	card := map[string]interface{}{
		"name":        "test-agent",
		"description": "A test agent",
		"version":     "1.0.0",
		"url":         "",
		"capabilities": map[string]bool{
			"streaming": true,
		},
		"skills": []map[string]string{
			{"id": "s1", "name": "coding", "description": "Code assistance"},
		},
	}
	cardJSON, _ := json.Marshal(card)

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/agent.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(cardJSON)
	})

	return httptest.NewServer(mux)
}

func TestConnectAndDisconnect(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := newFakeAgentServer(t)
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	// Connect
	conn, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("ConnectByURL failed: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	if conn.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", conn.Name)
	}
	if conn.Card == nil {
		t.Fatal("expected non-nil AgentCard")
	}
	if conn.Card.Version != "1.0.0" {
		t.Errorf("expected version '1.0.0', got '%s'", conn.Card.Version)
	}

	// Should be in memory
	got := reg.GetClient("test-agent")
	if got == nil {
		t.Fatal("expected client in memory after connect")
	}

	// Should be in DB
	agent, err := reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent in DB")
	}
	if agent.Status != model.AgentStatusConnected {
		t.Errorf("expected status 'connected', got '%s'", agent.Status)
	}

	// Disconnect
	reg.Disconnect("test-agent")

	// Should be gone from memory
	got = reg.GetClient("test-agent")
	if got != nil {
		t.Error("expected nil client after disconnect")
	}

	// Should be disconnected in DB
	agent, err = reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("GetAgent after disconnect failed: %v", err)
	}
	if agent == nil {
		t.Fatal("expected agent still in DB after disconnect")
	}
	if agent.Status != model.AgentStatusDisconnected {
		t.Errorf("expected status 'disconnected', got '%s'", agent.Status)
	}
}

func TestReconnect(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := newFakeAgentServer(t)
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	// Connect
	_, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	// Disconnect
	reg.Disconnect("test-agent")

	// Reconnect
	conn2, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("reconnect failed: %v", err)
	}

	if conn2 == nil {
		t.Fatal("expected non-nil connection after reconnect")
	}
	if conn2.Name != "test-agent" {
		t.Errorf("expected name 'test-agent', got '%s'", conn2.Name)
	}

	// Should be connected in DB
	agent, err := reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("GetAgent failed: %v", err)
	}
	if agent.Status != model.AgentStatusConnected {
		t.Errorf("expected status 'connected', got '%s'", agent.Status)
	}
}

func TestConnectDuplicate(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := newFakeAgentServer(t)
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	_, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("first connect failed: %v", err)
	}

	// Connect same agent again - should overwrite
	_, err = reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("duplicate connect failed: %v", err)
	}

	// Should still be only one agent in DB
	agents := reg.ListAgents()
	if len(agents) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(agents))
	}
}

func TestListAgents(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := newFakeAgentServer(t)
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	// Connect an agent
	_, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("ConnectByURL failed: %v", err)
	}

	// Create a DB-only agent (no live connection)
	err = reg.UpsertAgent(&model.AgentRecord{
		Name:        "db-only-agent",
		URL:         "http://localhost:9999",
		Description: "DB only",
		Status:      model.AgentStatusDisconnected,
	})
	if err != nil {
		t.Fatalf("UpsertAgent failed: %v", err)
	}

	agents := reg.ListAgents()
	if len(agents) != 2 {
		t.Fatalf("expected 2 agents, got %d", len(agents))
	}

	// Find the connected agent - description should come from AgentCard
	var connected *model.AgentRecord
	var dbOnly *model.AgentRecord
	for _, a := range agents {
		if a.Name == "test-agent" {
			connected = a
		}
		if a.Name == "db-only-agent" {
			dbOnly = a
		}
	}

	if connected == nil {
		t.Fatal("expected to find test-agent")
	}
	if connected.Description != "A test agent" {
		t.Errorf("expected connected description from card, got '%s'", connected.Description)
	}
	if connected.Status != model.AgentStatusConnected {
		t.Errorf("expected connected status, got '%s'", connected.Status)
	}

	if dbOnly == nil {
		t.Fatal("expected to find db-only-agent")
	}
	if dbOnly.Description != "DB only" {
		t.Errorf("expected 'DB only' description, got '%s'", dbOnly.Description)
	}
}

func TestDeleteAgent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	server := newFakeAgentServer(t)
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	_, err := reg.ConnectByURL(server.URL, "chat", "")
	if err != nil {
		t.Fatalf("ConnectByURL failed: %v", err)
	}

	err = reg.DeleteAgent("test-agent")
	if err != nil {
		t.Fatalf("DeleteAgent failed: %v", err)
	}

	// Gone from memory
	if reg.GetClient("test-agent") != nil {
		t.Error("expected nil client after delete")
	}

	// Gone from DB
	agent, err := reg.GetAgent("test-agent")
	if err != nil {
		t.Fatalf("GetAgent after delete failed: %v", err)
	}
	if agent != nil {
		t.Error("expected nil agent after delete")
	}
}

func TestGetClientNonexistent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	reg := New(repository.NewAgentRepository(db))

	if reg.GetClient("nonexistent") != nil {
		t.Error("expected nil for nonexistent client")
	}
}

func TestConnectWithRetry(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			http.Error(w, "not ready", 503)
			return
		}
		card := map[string]interface{}{
			"name":    "retry-agent",
			"version": "2.0.0",
			"capabilities": map[string]bool{
				"streaming": false,
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(card)
	}))
	defer server.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 3, 10*time.Millisecond)

	conn, err := reg.ConnectByURL(server.URL, "task", "")
	if err != nil {
		t.Fatalf("ConnectByURL with retry failed: %v", err)
	}
	if conn.Name != "retry-agent" {
		t.Errorf("expected name 'retry-agent', got '%s'", conn.Name)
	}
	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestConnectFailure(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	reg := NewWithRetry(repository.NewAgentRepository(db), 1, 10*time.Millisecond)

	// Server that returns 500
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "internal error", 500)
	}))
	defer server.Close()

	_, err := reg.ConnectByURL(server.URL, "chat", "")
	if err == nil {
		t.Fatal("expected error when server returns 500")
	}
}
