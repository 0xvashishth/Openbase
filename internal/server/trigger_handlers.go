package server

import (
	"net/http"

	"github.com/openbase/openbase/internal/metadata"
)

// ---- Triggers ----

type triggerView struct {
	ID           string `json:"id"`
	ProjectID    string `json:"project_id"`
	Name         string `json:"name"`
	Collection   string `json:"collection"`
	Event        string `json:"event"`
	ActionType   string `json:"action_type"`
	ActionTarget string `json:"action_target"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
}

func toTriggerView(t metadata.Trigger) triggerView {
	return triggerView{
		ID:           t.ID,
		ProjectID:    t.ProjectID,
		Name:         t.Name,
		Collection:   t.Collection,
		Event:        string(t.Event),
		ActionType:   string(t.ActionType),
		ActionTarget: t.ActionTarget,
		Enabled:      t.Enabled,
		CreatedAt:    t.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

type createTriggerRequest struct {
	Name         string `json:"name"`
	Collection   string `json:"collection"`
	Event        string `json:"event"`
	ActionType   string `json:"action_type"`
	ActionTarget string `json:"action_target"`
	Enabled      *bool  `json:"enabled,omitempty"`
}

func (s *Server) listTriggers(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	trigs, err := s.svc.Store.ListTriggers(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]triggerView, 0, len(trigs))
	for _, t := range trigs {
		out = append(out, toTriggerView(t))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createTrigger(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	var req createTriggerRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" || req.Collection == "" || req.ActionTarget == "" {
		writeError(w, http.StatusBadRequest, "name, collection and action_target are required")
		return
	}
	ev := metadata.TriggerEvent(req.Event)
	if ev != metadata.TriggerInsert && ev != metadata.TriggerUpdate && ev != metadata.TriggerDelete {
		writeError(w, http.StatusBadRequest, "event must be insert, update or delete")
		return
	}
	at := metadata.TriggerActionType(req.ActionType)
	if at != metadata.ActionFunction && at != metadata.ActionWebhook {
		writeError(w, http.StatusBadRequest, "action_type must be function or webhook")
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	trig := &metadata.Trigger{
		ProjectID:    projectID,
		Name:         req.Name,
		Collection:   req.Collection,
		Event:        ev,
		ActionType:   at,
		ActionTarget: req.ActionTarget,
		Enabled:      enabled,
	}
	if err := s.svc.Store.CreateTrigger(r.Context(), trig); err != nil {
		s.writeErr(w, err)
		return
	}
	// Reload the project's adapter wiring so the new trigger takes effect.
	s.resyncTriggers(r, projectID)
	writeJSON(w, http.StatusCreated, toTriggerView(*trig))
}

type updateTriggerRequest struct {
	Name         *string `json:"name,omitempty"`
	Collection   *string `json:"collection,omitempty"`
	Event        *string `json:"event,omitempty"`
	ActionType   *string `json:"action_type,omitempty"`
	ActionTarget *string `json:"action_target,omitempty"`
	Enabled      *bool   `json:"enabled,omitempty"`
}

func (s *Server) updateTrigger(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	triggerID := r.PathValue("triggerID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	t, err := s.svc.Store.GetTrigger(r.Context(), projectID, triggerID)
	if err != nil {
		s.writeStoreNotFound(w, err)
		return
	}
	var req updateTriggerRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name != nil {
		t.Name = *req.Name
	}
	if req.Collection != nil {
		t.Collection = *req.Collection
	}
	if req.Event != nil {
		t.Event = metadata.TriggerEvent(*req.Event)
	}
	if req.ActionType != nil {
		t.ActionType = metadata.TriggerActionType(*req.ActionType)
	}
	if req.ActionTarget != nil {
		t.ActionTarget = *req.ActionTarget
	}
	if req.Enabled != nil {
		t.Enabled = *req.Enabled
	}
	if err := s.svc.Store.UpdateTrigger(r.Context(), t); err != nil {
		s.writeErr(w, err)
		return
	}
	s.resyncTriggers(r, projectID)
	writeJSON(w, http.StatusOK, toTriggerView(*t))
}

func (s *Server) deleteTrigger(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	triggerID := r.PathValue("triggerID")
	if _, _, _, err := s.projectAndOrgRole(r, projectID, metadata.RoleAdmin); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteTrigger(r.Context(), projectID, triggerID); err != nil {
		s.writeStoreNotFound(w, err)
		return
	}
	s.resyncTriggers(r, projectID)
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// resyncTriggers re-registers the project's DB-level triggers after any change.
func (s *Server) resyncTriggers(r *http.Request, projectID string) {
	if s.svc.TriggerService == nil {
		return
	}
	conn, err := s.svc.Store.GetConnectionByProject(r.Context(), projectID)
	if err != nil || conn.Status != metadata.StatusConnected {
		return
	}
	if s.svc.Secrets == nil {
		return
	}
	secret, err := s.svc.Secrets.DecryptConnection(conn)
	if err != nil {
		return
	}
	s.svc.TriggerService.RegisterProject(r.Context(), *conn, secret)
}

// writeStoreNotFound maps a not-found store error to 404.
func (s *Server) writeStoreNotFound(w http.ResponseWriter, err error) {
	if err != nil {
		s.writeErr(w, err)
	}
}
