package mail

import (
	"context"
	"strings"
	"testing"
)

type fakeStore struct {
	settings *Settings
	logs     []LogEntry
}

func (f *fakeStore) GetMailSettings(_ context.Context) (*Settings, error) {
	if f.settings != nil {
		return f.settings, nil
	}
	return &Settings{Provider: "log"}, nil
}

func (f *fakeStore) RecordMailLog(_ context.Context, e *LogEntry) error {
	f.logs = append(f.logs, *e)
	return nil
}

type fakeSender struct {
	sent []Message
	err  error
}

func (f *fakeSender) Send(_ context.Context, m Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

func TestRenderAllTemplates(t *testing.T) {
	for _, name := range []string{TemplateTest, TemplateVerifyEmail, TemplateResetPassword, TemplateOrgInvite} {
		msg, err := Render(name, "user@example.com", TemplateData{
			AppName:   "Openbase",
			ActionURL: "https://example.com/a?token=abc",
			OrgName:   "Acme",
			Expires:   "24 hours",
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if msg.To != "user@example.com" || msg.Subject == "" {
			t.Fatalf("%s: bad envelope: %+v", name, msg)
		}
		if !strings.Contains(msg.HTML, "https://example.com/a?token=abc") {
			t.Fatalf("%s: html missing action url", name)
		}
		// The test template is informational (no call to action); the rest
		// must carry the link in both bodies.
		if name != TemplateTest && !strings.Contains(msg.Text, "https://example.com/a?token=abc") {
			t.Fatalf("%s: text missing action url", name)
		}
		if !strings.Contains(msg.HTML, "<html>") {
			t.Fatalf("%s: html missing layout", name)
		}
	}
}

func TestRenderUnknownTemplate(t *testing.T) {
	if _, err := Render("nope", "a@b.c", TemplateData{}); err == nil {
		t.Fatal("expected unknown-template error")
	}
}

func TestRenderEscapesRecipientContent(t *testing.T) {
	msg, err := Render(TemplateOrgInvite, "a@b.c", TemplateData{
		OrgName:   `<script>alert(1)</script>`,
		ActionURL: "https://example.com/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(msg.HTML, "<script>") {
		t.Fatalf("html not escaped: %s", msg.HTML)
	}
}

func TestResolveFallsBackToLog(t *testing.T) {
	svc := &Service{Store: &fakeStore{}}
	sender, provider, err := svc.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider != "log" {
		t.Fatalf("provider = %q, want log", provider)
	}
	if _, ok := sender.(*LogSender); !ok {
		t.Fatalf("sender = %T, want *LogSender", sender)
	}
}

func TestResolveSMTPWhenConfigured(t *testing.T) {
	svc := &Service{Store: &fakeStore{settings: &Settings{
		Provider: "smtp", Host: "smtp.example.com", Port: 587,
		FromAddress: "noreply@example.com",
	}}}
	sender, provider, err := svc.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider != "smtp" {
		t.Fatalf("provider = %q, want smtp", provider)
	}
	if _, ok := sender.(*SMTPSender); !ok {
		t.Fatalf("sender = %T, want *SMTPSender", sender)
	}
}

func TestSendTemplateRecordsDelivery(t *testing.T) {
	store := &fakeStore{}
	sent := &fakeSender{}
	svc := &Service{Store: store, NewSMTPSender: func(Settings) Sender { return sent }}
	// Point at SMTP so the injected sender is exercised.
	store.settings = &Settings{Provider: "smtp", Host: "smtp.example.com", Port: 587, FromAddress: "n@e.c"}
	if err := svc.SendTemplate(context.Background(), "u@e.c", TemplateTest, TemplateData{}); err != nil {
		t.Fatal(err)
	}
	if len(sent.sent) != 1 {
		t.Fatalf("sent = %d, want 1", len(sent.sent))
	}
	if len(store.logs) != 1 || !store.logs[0].OK {
		t.Fatalf("logs = %+v, want one ok entry", store.logs)
	}
}

func TestSendTemplateRecordsFailure(t *testing.T) {
	store := &fakeStore{}
	sent := &fakeSender{err: context.DeadlineExceeded}
	svc := &Service{Store: store, NewSMTPSender: func(Settings) Sender { return sent }}
	store.settings = &Settings{Provider: "smtp", Host: "smtp.example.com", Port: 587, FromAddress: "n@e.c"}
	if err := svc.SendTemplate(context.Background(), "u@e.c", TemplateTest, TemplateData{}); err == nil {
		t.Fatal("expected send error")
	}
	if len(store.logs) != 1 || store.logs[0].OK || store.logs[0].Error == "" {
		t.Fatalf("logs = %+v, want one failed entry with error", store.logs)
	}
}
