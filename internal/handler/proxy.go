package handler

import (
	"io"
	"net/http"
	"strings"

	"a2a-platform/internal/svc"
)

// hopByHopHeaders lists headers that must not be forwarded by a proxy.
var hopByHopHeaders = []string{
	"connection",
	"keep-alive",
	"proxy-authenticate",
	"proxy-authorization",
	"te",
	"trailers",
	"transfer-encoding",
	"upgrade",
	"host",
	"content-length",
	"accept-encoding",
}

// ProxyAgent proxies requests to a connected agent, supporting SSE streaming.
func ProxyAgent(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		agentName := pathParam(r, "name")
		if agentName == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "agent name is required"})
			return
		}

		conn := svcCtx.Registry.GetClient(agentName)
		if conn == nil {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "Agent '" + agentName + "' not connected"})
			return
		}

		// Build target URL from remaining path and query string
		targetURL := conn.URL
		remaining := strings.TrimPrefix(r.URL.Path, "/agent/"+agentName)
		if remaining == "" {
			remaining = "/"
		}
		targetURL += remaining
		if r.URL.RawQuery != "" {
			targetURL += "?" + r.URL.RawQuery
		}

		// Read request body
		var bodyReader io.Reader = r.Body
		if r.Body != nil {
			bodyReader = r.Body
		}

		// Build the proxy request
		proxyReq, err := http.NewRequest(r.Method, targetURL, bodyReader)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create proxy request: " + err.Error()})
			return
		}

		// Copy headers, stripping hop-by-hop
		for key, vals := range r.Header {
			lower := strings.ToLower(key)
			skip := false
			for _, h := range hopByHopHeaders {
				if lower == h {
					skip = true
					break
				}
			}
			if !skip {
				for _, val := range vals {
					proxyReq.Header.Add(key, val)
				}
			}
		}

		// Inject A2A-Version if missing
		if proxyReq.Header.Get("A2A-Version") == "" {
			proxyReq.Header.Set("A2A-Version", "1.0")
		}

		// Execute using RoundTrip (avoids redirect following)
		resp, err := http.DefaultTransport.RoundTrip(proxyReq)
		if err != nil {
			writeJSON(w, http.StatusBadGateway, map[string]string{"error": "proxy request failed: " + err.Error()})
			return
		}
		defer resp.Body.Close()

		// Copy response headers
		for key, vals := range resp.Header {
			w.Header()[key] = vals
		}

		// Copy status code
		w.WriteHeader(resp.StatusCode)

		// Stream body with flushing for SSE
		flusher, canFlush := w.(http.Flusher)
		buf := make([]byte, 4096)
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				if canFlush {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
	}
}
