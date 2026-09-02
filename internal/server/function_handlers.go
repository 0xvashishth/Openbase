package server

import (
	"net/http"

	"github.com/openbase/openbase/internal/metadata"
)

// ---- Functions ----

type functionView struct {
	ID        string `json:"id"`
	ProjectID string `json:"project_id"`
	Name      string `json:"name"`
	Runtime   string `json:"runtime"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

func toFunctionView(f metadata.Function) functionView {
	return functionView{
		ID:        f.ID,
		ProjectID: f.ProjectID,
		Name:      f.Name,
		Runtime:   string(f.Runtime),
		Source:    f.Source,
		CreatedAt: f.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
}

type createFunctionRequest struct {
	Name    string `json:"name"`
	Runtime string `json:"runtime"`
	Source  string `json:"source"`
}

func (s *Server) listFunctions(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	fns, err := s.svc.Store.ListFunctions(r.Context(), projectID)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	out := make([]functionView, 0, len(fns))
	for _, f := range fns {
		out = append(out, toFunctionView(f))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createFunction(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	var req createFunctionRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	rt := metadata.FunctionRuntime(req.Runtime)
	if rt == "" {
		rt = metadata.RuntimeNode
	}
	if rt != metadata.RuntimeNode && rt != metadata.RuntimePython {
		writeError(w, http.StatusBadRequest, "runtime must be node or python")
		return
	}
	fn := &metadata.Function{
		ProjectID: projectID,
		Name:      req.Name,
		Runtime:   rt,
		Source:    req.Source,
	}
	if err := s.svc.Store.CreateFunction(r.Context(), fn); err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, toFunctionView(*fn))
}

func (s *Server) getFunction(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	fnID := r.PathValue("fnID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	fn, err := s.svc.Store.GetFunction(r.Context(), projectID, fnID)
	if err != nil {
		s.writeStoreNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toFunctionView(*fn))
}

func (s *Server) deleteFunction(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectID")
	fnID := r.PathValue("fnID")
	if _, _, err := s.projectAndOrg(r, projectID); err != nil {
		s.writeErr(w, err)
		return
	}
	if err := s.svc.Store.DeleteFunction(r.Context(), projectID, fnID); err != nil {
		s.writeStoreNotFound(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
