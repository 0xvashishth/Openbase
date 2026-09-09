package server

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/openbase/openbase/internal/function"
	"github.com/openbase/openbase/internal/metadata"
)

// Auth hooks (Phase 10, A3.2). A project function runs on identity events:
//
//	before-user-created — may reject signup ({error: msg}) or contribute
//	    user_metadata ({user_metadata: {...}} merged before insert).
//	after-user-created  — notification-style; failures honor fail_open.
//	before-token-issued — may contribute custom access-token claims
//	    ({custom_claims: {...}} merged into the token's custom claim).
//
// The function event payload is {event, project_id, user}. Hook results must
// be JSON objects; anything else is a hook failure. Unconfigured events cost
// one indexed lookup and no execution.

// hookInvoker runs auth hooks. A nil Runner means "no runtime configured":
// configured hooks then fail per their fail_open flag (closed by default),
// and the log names the missing runtime.
type hookInvoker struct {
	store  metadata.Store
	runner function.Runner
}

func (s *Server) hooks() *hookInvoker {
	return &hookInvoker{store: s.svc.Store, runner: s.svc.Functions}
}

// errHookRejected marks a fail-closed hook failure that must surface as a
// 4xx (rejection), never a 500.
var errHookRejected = errors.New("rejected by auth hook")

// run executes the hook for event, returning the function's result object
// (or nil when unconfigured). Fail-open hooks log-and-continue on any
// failure; fail-closed hooks propagate a descriptive error.
func (h *hookInvoker) run(ctx context.Context, projectID, event string, payload map[string]any) (map[string]any, error) {
	hook, err := h.store.GetProjectAuthHook(ctx, projectID, event)
	if err != nil {
		return nil, nil // unconfigured — the common case
	}
	fail := func(format string, args ...any) (map[string]any, error) {
		msg := fmt.Sprintf("auth hook %s: %s", event, fmt.Sprintf(format, args...))
		if hook.FailOpen {
			return nil, nil // logged by the caller with context
		}
		return nil, fmt.Errorf("%w: %s", errHookRejected, msg)
	}
	fn, err := h.store.GetFunction(ctx, projectID, hook.FunctionID)
	if err != nil {
		return fail("function %s not found", hook.FunctionID)
	}
	if h.runner == nil {
		return fail("no function runtime configured")
	}
	event2 := map[string]any{"event": event, "project_id": projectID}
	for k, v := range payload {
		event2[k] = v
	}
	res, err := h.runner.Run(ctx, fn.Source, string(fn.Runtime), event2)
	if err != nil {
		return fail("execution failed: %v", err)
	}
	if res == nil {
		return map[string]any{}, nil
	}
	return res, nil
}

// hookError extracts a rejection message ({error: "..."}) from a hook result.
func hookError(res map[string]any) string {
	if res == nil {
		return ""
	}
	if msg, _ := res["error"].(string); msg != "" {
		return msg
	}
	return ""
}

// hookUserMetadata extracts a metadata merge ({user_metadata: {...}}).
func hookUserMetadata(res map[string]any) map[string]any {
	if res == nil {
		return nil
	}
	m, _ := res["user_metadata"].(map[string]any)
	return m
}

// hookCustomClaims extracts token claim contributions
// ({custom_claims: {...}}, alias "claims").
func hookCustomClaims(res map[string]any) map[string]any {
	if res == nil {
		return nil
	}
	if m, _ := res["custom_claims"].(map[string]any); m != nil {
		return m
	}
	m, _ := res["claims"].(map[string]any)
	return m
}

// ---- operator CRUD (admin+, powers the dashboard Authentication UI) ----

func (s *Server) listAuthHooks(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	hooks, err := s.svc.Store.ListProjectAuthHooks(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, hooks)
}

type upsertHookRequest struct {
	Event      string `json:"event"`
	FunctionID string `json:"function_id"`
	FailOpen   *bool  `json:"fail_open"`
}

func (s *Server) upsertAuthHook(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	var req upsertHookRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !metadata.ValidHookEvent(req.Event) {
		writeError(w, http.StatusBadRequest, "event must be before-user-created, after-user-created or before-token-issued")
		return
	}
	if req.FunctionID == "" {
		writeError(w, http.StatusBadRequest, "function_id is required")
		return
	}
	// The function must exist in this project (no dangling hooks).
	if _, err := s.svc.Store.GetFunction(r.Context(), projectID, req.FunctionID); err != nil {
		writeError(w, http.StatusBadRequest, "function not found in this project")
		return
	}
	h := &metadata.ProjectAuthHook{ProjectID: projectID, Event: req.Event, FunctionID: req.FunctionID}
	if req.FailOpen != nil {
		h.FailOpen = *req.FailOpen
	}
	if err := s.svc.Store.UpsertProjectAuthHook(r.Context(), h); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, h)
}

func (s *Server) deleteAuthHook(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	event := r.PathValue("event")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteProjectAuthHook(r.Context(), projectID, event); err != nil {
		s.writeStoreErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
