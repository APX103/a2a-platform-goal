package handler

import (
	"encoding/json"
	"net/http"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/pathvar"

	"a2a-platform/internal/svc"
)

// writeJSON marshals data as JSON and writes it to the response.
func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}

// pathParam extracts a named path parameter from the URL using go-zero's pathvar.
func pathParam(r *http.Request, name string) string {
	vars := pathvar.Vars(r)
	return vars[name]
}

// RegisterHandlers registers all HTTP routes on the go-zero server.
func RegisterHandlers(server *rest.Server, svcCtx *svc.ServiceContext) {
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/agents", Handler: GetAgents(svcCtx)},
		{Method: http.MethodGet, Path: "/api/agents/:name", Handler: GetAgent(svcCtx)},
		{Method: http.MethodGet, Path: "/api/capabilities", Handler: GetCapabilities(svcCtx)},
		{Method: http.MethodGet, Path: "/api/tasks", Handler: GetTasks(svcCtx)},
		{Method: http.MethodGet, Path: "/api/tasks/:id/trace", Handler: GetTaskTrace(svcCtx)},
		{Method: http.MethodPost, Path: "/api/agents/register", Handler: RegisterAgent(svcCtx)},
		{Method: http.MethodPost, Path: "/api/agents", Handler: AddAgent(svcCtx)},
		{Method: http.MethodDelete, Path: "/api/agents/:name", Handler: DeleteAgent(svcCtx)},
		{Method: http.MethodPost, Path: "/agent/:name", Handler: ProxyAgent(svcCtx)},
		{Method: http.MethodPost, Path: "/api/chat", Handler: Chat(svcCtx)},
	})
}
