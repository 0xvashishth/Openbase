// Package mail implements the platform's outbound email (PHASES.md 9.2):
// a Sender interface with SMTP and log-sink implementations, shared
// HTML+text templates, and a Service that resolves the operator-configured
// provider, sends, and records every attempt in the delivery log.
//
// All mail flows (verification, password reset, org invites, and later
// Phase 10 app mail) send through Service — never SMTP directly.
package mail

import (
	"context"
	"log/slog"
)

// Message is one outbound email.
type Message struct {
	To      string
	Subject string
	HTML    string
	Text    string
}

// Sender delivers one message.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Settings mirrors the operator-configured mail_settings row (plaintext
// password only in memory — storage keeps it envelope-encrypted).
type Settings struct {
	Provider    string // "smtp" | "log"
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
}

// Store is the persistence Service needs: settings + delivery log. The
// settings password arrives decrypted (plaintext in memory only); storage
// keeps it envelope-encrypted.
type Store interface {
	GetMailSettings(ctx context.Context) (*Settings, error)
	RecordMailLog(ctx context.Context, e *LogEntry) error
}

// LogEntry records one send attempt for the dashboard delivery log.
type LogEntry struct {
	To       string
	Template string
	Subject  string
	OK       bool
	Error    string
}

// Service resolves the effective sender from settings and records delivery.
type Service struct {
	Store Store
	Log   *slog.Logger
	// NewSMTPSender builds the SMTP sender; overridable in tests.
	NewSMTPSender func(s Settings) Sender
}

// Resolve returns the sender for current settings: configured SMTP, else
// the log sink (honest fallback — unconfigured instances log mail instead
// of failing sends).
func (s *Service) Resolve(ctx context.Context) (Sender, string, error) {
	st, err := s.Store.GetMailSettings(ctx)
	if err != nil {
		return nil, "", err
	}
	if st.Provider == "smtp" && st.Host != "" {
		mk := s.NewSMTPSender
		if mk == nil {
			mk = func(s Settings) Sender { return NewSMTPSender(s) }
		}
		return mk(*st), "smtp", nil
	}
	return NewLogSender(s.Log), "log", nil
}

// SendTemplate renders a named template, delivers it, and records the
// outcome. Recording failures are logged, never propagated — the delivery
// result is authoritative.
func (s *Service) SendTemplate(ctx context.Context, to, template string, data TemplateData) error {
	msg, err := Render(template, to, data)
	if err != nil {
		return err
	}
	sender, provider, err := s.Resolve(ctx)
	if err != nil {
		return err
	}
	sendErr := sender.Send(ctx, msg)
	entry := &LogEntry{
		To:       to,
		Template: template + " (via " + provider + ")",
		Subject:  msg.Subject,
		OK:       sendErr == nil,
	}
	if sendErr != nil {
		entry.Error = sendErr.Error()
	}
	if s.Store != nil {
		if err := s.Store.RecordMailLog(ctx, entry); err != nil && s.Log != nil {
			s.Log.Warn("mail: record delivery", "to", to, "template", template, "err", err)
		}
	}
	return sendErr
}
