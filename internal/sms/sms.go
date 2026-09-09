// Package sms sends phone OTP codes behind a pluggable Provider (Phase 10,
// A2). The log provider is the default: unconfigured instances record instead
// of failing, and callers must surface that honestly.
package sms

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Provider delivers one text message.
type Provider interface {
	Send(ctx context.Context, to, body string) error
	Name() string
}

// LogProvider records sends in application logs. Used when no SMS driver is
// configured and in tests that only assert the call path.
type LogProvider struct {
	Log *slog.Logger
}

func (p *LogProvider) Name() string { return "log" }

// Send logs the message. Numbers are debug-level (PII minimization); the fact
// of the send is info-level so delivery is auditable.
func (p *LogProvider) Send(ctx context.Context, to, body string) error {
	log := p.Log
	if log == nil {
		log = slog.Default()
	}
	log.InfoContext(ctx, "sms: send (log provider — no SMS delivered)", "body_len", len(body))
	log.DebugContext(ctx, "sms: redacted recipient", "to_suffix", suffix(to))
	return nil
}

func suffix(to string) string {
	if len(to) <= 4 {
		return "****"
	}
	return "****" + to[len(to)-4:]
}

// TwilioProvider delivers via Twilio Programmable Messaging. BaseURL defaults
// to the production API and is overridable for tests.
type TwilioProvider struct {
	AccountSID string
	AuthToken  string
	From       string
	BaseURL    string
	HTTP       *http.Client
}

func (p *TwilioProvider) Name() string { return "twilio" }

func (p *TwilioProvider) client() *http.Client {
	if p.HTTP != nil {
		return p.HTTP
	}
	return &http.Client{Timeout: 10 * time.Second}
}

// Send posts one message. Twilio answers 201 with the message SID on success;
// anything else is an error carrying the status and the (truncated) body.
func (p *TwilioProvider) Send(ctx context.Context, to, body string) error {
	if p.AccountSID == "" || p.AuthToken == "" || p.From == "" {
		return errors.New("sms: twilio credentials/from number are not configured")
	}
	base := p.BaseURL
	if base == "" {
		base = "https://api.twilio.com"
	}
	endpoint := strings.TrimSuffix(base, "/") + "/2010-04-01/Accounts/" + p.AccountSID + "/Messages.json"
	form := url.Values{"To": {to}, "From": {p.From}, "Body": {body}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.SetBasicAuth(p.AccountSID, p.AuthToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.client().Do(req)
	if err != nil {
		return fmt.Errorf("sms: twilio request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		detail, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("sms: twilio status %d: %s", resp.StatusCode, strings.TrimSpace(string(detail)))
	}
	return nil
}
