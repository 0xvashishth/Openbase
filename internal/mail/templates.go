package mail

import (
	"bytes"
	"fmt"
	htmltemplate "html/template"
	texttemplate "text/template"
)

// TemplateData carries the variables templates may use. ActionURL is the
// single call-to-action link (verify, reset, accept); renderer-escaped.
type TemplateData struct {
	AppName   string
	Recipient string
	ActionURL string
	OrgName   string
	Expires   string
}

// Template names.
const (
	TemplateTest          = "test"
	TemplateVerifyEmail   = "verify-email"
	TemplateResetPassword = "reset-password"
	TemplateOrgInvite     = "org-invite"
)

type mailTemplate struct {
	subject string
	html    string
	text    string
}

const layoutHTML = `<!doctype html><html><body style="font-family:sans-serif;max-width:560px;margin:0 auto;padding:24px;color:#111;">
<h2 style="margin:0 0 16px;">{{.Title}}</h2>
<div>{{.Body}}</div>
<p style="margin-top:24px;">
<a href="{{.ActionURL}}" style="display:inline-block;padding:10px 20px;background:#111;color:#fff;text-decoration:none;border-radius:6px;">{{.ActionLabel}}</a>
</p>
<p style="color:#666;font-size:13px;">Or paste this link: {{.ActionURL}}</p>
<hr style="border:none;border-top:1px solid #eee;margin:24px 0;" />
<p style="color:#999;font-size:12px;">Sent by {{.AppName}}. If you didn't ask for this, ignore it.</p>
</body></html>`

var templates = map[string]mailTemplate{
	TemplateTest: {
		subject: "Openbase test email",
		html:    `<p>This is a test message from {{.AppName}}. Your mail provider is configured correctly.</p>`,
		text:    "This is a test message from {{.AppName}}. Your mail provider is configured correctly.",
	},
	TemplateVerifyEmail: {
		subject: "Verify your email",
		html:    `<p>Welcome to {{.AppName}} — confirm this address to finish signing up.</p>`,
		text:    "Welcome to {{.AppName}} — confirm this address to finish signing up.\n\nVerify: {{.ActionURL}}",
	},
	TemplateResetPassword: {
		subject: "Reset your password",
		html:    `<p>A password reset was requested for your {{.AppName}} account.{{if .Expires}} This link expires {{.Expires}}.{{end}}</p>`,
		text:    "A password reset was requested for your {{.AppName}} account.\n\nReset: {{.ActionURL}}",
	},
	TemplateOrgInvite: {
		subject: "You've been invited",
		html:    `<p>You've been invited to join <strong>{{.OrgName}}</strong> on {{.AppName}}.{{if .Expires}} This invite expires {{.Expires}}.{{end}}</p>`,
		text:    "You've been invited to join {{.OrgName}} on {{.AppName}}.\n\nAccept: {{.ActionURL}}",
	},
}

// Render builds the Message for a template. Unknown template names are an
// error — callers must not format user input as a template name.
func Render(template, to string, data TemplateData) (Message, error) {
	t, ok := templates[template]
	if !ok {
		return Message{}, fmt.Errorf("mail: unknown template %q", template)
	}
	if data.AppName == "" {
		data.AppName = "Openbase"
	}
	var htmlBody, textBody bytes.Buffer
	if err := htmltemplate.Must(htmltemplate.New("body").Parse(t.html)).Execute(&htmlBody, data); err != nil {
		return Message{}, fmt.Errorf("mail: render html: %w", err)
	}
	if err := texttemplate.Must(texttemplate.New("body").Parse(t.text)).Execute(&textBody, data); err != nil {
		return Message{}, fmt.Errorf("mail: render text: %w", err)
	}
	var htmlFull bytes.Buffer
	layoutData := map[string]any{
		"Title":       t.subject,
		"Body":        htmltemplate.HTML(htmlBody.String()),
		"ActionURL":   data.ActionURL,
		"ActionLabel": actionLabel(template),
		"AppName":     data.AppName,
	}
	if err := htmltemplate.Must(htmltemplate.New("layout").Parse(layoutHTML)).Execute(&htmlFull, layoutData); err != nil {
		return Message{}, fmt.Errorf("mail: render layout: %w", err)
	}
	return Message{To: to, Subject: data.AppName + " — " + t.subject, HTML: htmlFull.String(), Text: textBody.String()}, nil
}

func actionLabel(template string) string {
	switch template {
	case TemplateVerifyEmail:
		return "Verify email"
	case TemplateResetPassword:
		return "Reset password"
	case TemplateOrgInvite:
		return "Accept invite"
	default:
		return "Open dashboard"
	}
}
