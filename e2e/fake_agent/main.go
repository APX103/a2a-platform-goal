package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
)

var (
	port = flag.Int("port", 0, "port to listen on")
	name = flag.String("name", "fake-agent", "agent name")
)

func main() {
	flag.Parse()

	p := *port
	if p == 0 {
		pStr := os.Getenv("PORT")
		if pStr != "" {
			fmt.Sscanf(pStr, "%d", &p)
		}
		if p == 0 {
			p = 10001
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleRequest)

	fmt.Printf("Fake agent '%s' starting on :%d\n", *name, p)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", p), mux); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}

// handleRequest routes incoming requests to the appropriate handler.
// GET /.well-known/agent.json -> AgentCard
// POST / -> A2A JSON-RPC request with mode-specific behavior
func handleRequest(w http.ResponseWriter, r *http.Request) {
	mode := parseMode(r)
	agentName := parseName(r)

	if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/.well-known/agent.json") {
		handleAgentCard(w, r, agentName, mode)
		return
	}

	if r.Method == http.MethodPost {
		handleAgentRequest(w, r, mode)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusMethodNotAllowed)
	json.NewEncoder(w).Encode(map[string]string{
		"error": "method not allowed",
	})
}
