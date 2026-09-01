package server

import (
	"errors"
	"net/http"
)

var (
	errForbidden = errors.New("forbidden")
	errNotFound  = errors.New("not found")
)

// writeErr maps package errors and sentinels to appropriate HTTP statuses and
// writes a JSON error body.
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, errForbidden.Error())
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, errNotFound.Error())
	default:
		s.svc.Log.Error("handler error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
