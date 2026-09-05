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
//
// Forbidden and not-found errors keep their wrapped message, so a 403 raised by
// authorizeOrgRole can say *which* role was required instead of a bare
// "forbidden" the dashboard cannot explain. Everything else collapses to a
// generic 500 — internal errors are logged, never echoed.
func (s *Server) writeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errForbidden):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, errNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		s.svc.Log.Error("handler error", "err", err)
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}
