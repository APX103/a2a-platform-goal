package a2a

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client is an A2A protocol HTTP client for communicating with agents.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient creates a new A2A client targeting the given agent base URL.
func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
	}
}

// FetchAgentCard retrieves the agent card from the well-known endpoint.
func (c *Client) FetchAgentCard() (*AgentCard, error) {
	url := fmt.Sprintf("%s/.well-known/agent.json", c.baseURL)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("fetch agent card: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("fetch agent card: status %d", resp.StatusCode)
	}
	var card AgentCard
	if err := json.NewDecoder(resp.Body).Decode(&card); err != nil {
		return nil, fmt.Errorf("decode agent card: %w", err)
	}
	return &card, nil
}

// SendStreamingMessage sends a JSON-RPC message to the agent and returns the
// response events. If the server responds with SSE, events are parsed from the
// stream. Otherwise, the response body is wrapped as a single event.
func (c *Client) SendStreamingMessage(role, text, taskID string) ([]SSEEventEnvelope, error) {
	req := JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "1",
		Method:  "SendStreamingMessage",
		Params: SendStreamingMessageParams{
			Message: Message{Role: role, Parts: []Part{{Kind: "text", Text: text}}},
			TaskID:  taskID,
		},
	}
	body, _ := json.Marshal(req)
	url := fmt.Sprintf("%s/", c.baseURL)
	httpReq, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("A2A-Version", "1.0")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("send streaming message: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("send streaming message: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	if IsSSEContentType(resp.Header.Get("Content-Type")) {
		return ParseSSEStream(resp.Body)
	}
	// Non-SSE response: wrap as single event
	respBody, _ := io.ReadAll(resp.Body)
	return []SSEEventEnvelope{{
		Type: "message",
		Data: json.RawMessage(respBody),
	}}, nil
}

// PostRaw sends a raw HTTP POST request (for tool calls).
func (c *Client) PostRaw(path string, body io.Reader, contentType string) (*http.Response, error) {
	url := c.baseURL + path
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return c.httpClient.Do(req)
}

// GetRaw sends a raw HTTP GET request.
func (c *Client) GetRaw(path string) (*http.Response, error) {
	return c.httpClient.Get(c.baseURL + path)
}
