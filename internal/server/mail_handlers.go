package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/openbase/openbase/internal/mail"
	"github.com/openbase/openbase/internal/metadata"
)

// ---- Platform mail settings (Phase 9.2, dashboard-managed BYOC SMTP) ----

// requireAnyOrgOwner gates platform-global settings: the caller must hold the
// owner role in at least one org. V1 simplification, documented in PHASES.md
// 9.2 — self-hosted instances have one operator; instance roles come later.
func (s *Server) requireAnyOrgOwner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := userIDFromContext(r.Context())
		if userID == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		orgs, err := s.svc.Store.ListOrganizationsForUser(r.Context(), userID)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		for _, o := range orgs {
			m, err := s.svc.Store.GetMembership(r.Context(), o.ID, userID)
			if err != nil {
				continue
			}
			if m.Role == metadata.RoleOwner {
				next.ServeHTTP(w, r)
				return
			}
		}
		writeError(w, http.StatusForbidden, "owner role required")
	})
}

type mailSettingsView struct {
	Provider     string `json:"provider"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	PasswordSet  bool   `json:"password_set"`
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name"`
}

// getMailSettings serves the singleton config with the password redacted —
// the API never returns it (PasswordSet tells the UI whether one is stored).
func (s *Server) getMailSettings(w http.ResponseWriter, r *http.Request) {
	m, err := s.svc.Store.GetMailSettings(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, mailSettingsView{
		Provider:     m.Provider,
		SMTPHost:     m.SMTPHost,
		SMTPPort:     m.SMTPPort,
		SMTPUsername: m.SMTPUsername,
		PasswordSet:  len(m.EncryptedPassword) > 0,
		FromAddress:  m.FromAddress,
		FromName:     m.FromName,
	})
}

type updateMailSettingsRequest struct {
	Provider     string `json:"provider"`
	SMTPHost     string `json:"smtp_host"`
	SMTPPort     int    `json:"smtp_port"`
	SMTPUsername string `json:"smtp_username"`
	// SMTPPassword is write-only: empty/absent keeps the stored secret.
	SMTPPassword string `json:"smtp_password,omitempty"`
	FromAddress  string `json:"from_address"`
	FromName     string `json:"from_name"`
}

// updateMailSettings upserts the singleton. Validation is fail-fast so a
// typo'd hostname is caught at save time, not at 2am when an invite fires.
func (s *Server) updateMailSettings(w http.ResponseWriter, r *http.Request) {
	var req updateMailSettingsRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Provider != "smtp" && req.Provider != "log" {
		writeError(w, http.StatusBadRequest, "provider must be smtp or log")
		return
	}
	if req.Provider == "smtp" {
		if strings.TrimSpace(req.SMTPHost) == "" {
			writeError(w, http.StatusBadRequest, "smtp_host is required")
			return
		}
		if req.SMTPPort < 1 || req.SMTPPort > 65535 {
			writeError(w, http.StatusBadRequest, "smtp_port must be 1-65535")
			return
		}
		if !strings.Contains(req.FromAddress, "@") {
			writeError(w, http.StatusBadRequest, "from_address must be an email address")
			return
		}
	}
	current, err := s.svc.Store.GetMailSettings(r.Context())
	if err != nil {
		s.writeErr(w, err)
		return
	}
	next := &metadata.MailSettings{
		Provider:          req.Provider,
		SMTPHost:          strings.TrimSpace(req.SMTPHost),
		SMTPPort:          req.SMTPPort,
		SMTPUsername:      req.SMTPUsername,
		EncryptedPassword: current.EncryptedPassword,
		EncryptionKeyID:   current.EncryptionKeyID,
		FromAddress:       strings.TrimSpace(req.FromAddress),
		FromName:          req.FromName,
	}
	if req.SMTPPassword != "" {
		if s.svc.Secrets == nil {
			writeError(w, http.StatusServiceUnavailable, "encryption not configured; cannot store SMTP password")
			return
		}
		ct, keyID, err := s.svc.Secrets.EncryptValue(req.SMTPPassword)
		if err != nil {
			s.writeErr(w, err)
			return
		}
		next.EncryptedPassword = ct
		next.EncryptionKeyID = keyID
	}
	if err := s.svc.Store.UpdateMailSettings(r.Context(), next); err != nil {
		s.writeErr(w, err)
		return
	}
	s.getMailSettings(w, r)
}

// mailStoreAdapter bridges metadata.MailSettings (encrypted at rest) to the
// mail.Service store contract (plaintext in memory only).
type mailStoreAdapter struct {
	store   metadata.Store
	secrets SecretsProvider
}

func (a *mailStoreAdapter) GetMailSettings(ctx context.Context) (*mail.Settings, error) {
	m, err := a.store.GetMailSettings(ctx)
	if err != nil {
		return nil, err
	}
	out := &mail.Settings{
		Provider:    m.Provider,
		Host:        m.SMTPHost,
		Port:        m.SMTPPort,
		Username:    m.SMTPUsername,
		FromAddress: m.FromAddress,
		FromName:    m.FromName,
	}
	if len(m.EncryptedPassword) > 0 {
		if a.secrets == nil {
			return nil, fmt.Errorf("mail: SMTP password stored but encryption is not configured")
		}
		pw, err := a.secrets.DecryptValue(m.EncryptedPassword, m.EncryptionKeyID)
		if err != nil {
			return nil, fmt.Errorf("mail: decrypt SMTP password: %w", err)
		}
		out.Password = pw
	}
	return out, nil
}

func (a *mailStoreAdapter) RecordMailLog(ctx context.Context, e *mail.LogEntry) error {
	return a.store.RecordMailLog(ctx, &metadata.MailLog{
		ToAddress: e.To,
		Template:  e.Template,
		Subject:   e.Subject,
		OK:        e.OK,
		Error:     e.Error,
	})
}

// mailService builds the sender service for handlers.
func (s *Server) mailService() *mail.Service {
	return &mail.Service{
		Store: &mailStoreAdapter{store: s.svc.Store, secrets: s.svc.Secrets},
		Log:   s.svc.Log,
	}
}

type testMailRequest struct {
	To string `json:"to"`
}

type testMailResult struct {
	OK       bool   `json:"ok"`
	Provider string `json:"provider"`
	Error    string `json:"error,omitempty"`
}

// testMailSend delivers the sample template to an arbitrary address and
// reports the outcome inline — the dashboard's "Send test email" button.
// Always 200: a failed send is a diagnostic result, not an API error.
func (s *Server) testMailSend(w http.ResponseWriter, r *http.Request) {
	var req testMailRequest
	if err := decodeBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !strings.Contains(req.To, "@") {
		writeError(w, http.StatusBadRequest, "to must be an email address")
		return
	}
	sendErr := s.mailService().SendTemplate(r.Context(), req.To, mail.TemplateTest, mail.TemplateData{})
	provider := "log"
	if st, err := s.svc.Store.GetMailSettings(r.Context()); err == nil && st.Provider == "smtp" && st.SMTPHost != "" {
		provider = "smtp"
	}
	res := testMailResult{OK: sendErr == nil, Provider: provider}
	if sendErr != nil {
		res.Error = sendErr.Error()
	}
	writeJSON(w, http.StatusOK, res)
}

// listMailLog serves recent send attempts, newest first.
func (s *Server) listMailLog(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if _, err := fmt.Sscanf(v, "%d", &limit); err != nil {
			limit = 50
		}
	}
	entries, err := s.svc.Store.ListMailLog(r.Context(), limit)
	if err != nil {
		s.writeErr(w, err)
		return
	}
	if entries == nil {
		entries = []metadata.MailLog{}
	}
	writeJSON(w, http.StatusOK, entries)
}
