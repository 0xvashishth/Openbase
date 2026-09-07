package mail

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"
)

// Dial and exchange timeouts: a hung mail server must fail fast, not wedge
// the request that triggered the send.
const (
	smtpDialTimeout     = 10 * time.Second
	smtpExchangeTimeout = 30 * time.Second
)

// SMTPSender delivers via an operator-provided SMTP relay (BYOC credentials
// from the dashboard Email page). Port 465 uses implicit TLS; any other port
// requires STARTTLS — plaintext fallback is refused so credentials and mail
// content never traverse the network unencrypted.
type SMTPSender struct {
	Host        string
	Port        int
	Username    string
	Password    string
	FromAddress string
	FromName    string
	// tlsConfig, when non-nil, overrides the TLS client config (tests use it
	// to trust a self-signed fake server; production always verifies).
	tlsConfig *tls.Config
}

// NewSMTPSender builds an SMTP sender from resolved settings.
func NewSMTPSender(s Settings) Sender {
	return &SMTPSender{
		Host:        s.Host,
		Port:        s.Port,
		Username:    s.Username,
		Password:    s.Password,
		FromAddress: s.FromAddress,
		FromName:    s.FromName,
	}
}

func (s *SMTPSender) addr() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func (s *SMTPSender) from() string {
	if s.FromName != "" {
		return fmt.Sprintf("%s <%s>", s.FromName, s.FromAddress)
	}
	return s.FromAddress
}

// Send delivers one message, enforcing encrypted transport throughout.
func (s *SMTPSender) Send(ctx context.Context, msg Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Host == "" || s.FromAddress == "" || msg.To == "" {
		return fmt.Errorf("mail: incomplete SMTP configuration or recipient")
	}
	tlsCfg := s.tlsConfig
	if tlsCfg == nil {
		tlsCfg = &tls.Config{ServerName: s.Host}
	}

	var client *smtp.Client
	if s.Port == 465 {
		conn, err := tls.DialWithDialer(
			&net.Dialer{Timeout: smtpDialTimeout}, "tcp", s.addr(), tlsCfg)
		if err != nil {
			return fmt.Errorf("mail: dial %s: %w", s.addr(), err)
		}
		_ = conn.SetDeadline(time.Now().Add(smtpExchangeTimeout))
		defer conn.Close()
		client, err = smtp.NewClient(conn, s.Host)
		if err != nil {
			return fmt.Errorf("mail: smtp handshake: %w", err)
		}
	} else {
		conn, err := net.DialTimeout("tcp", s.addr(), smtpDialTimeout)
		if err != nil {
			return fmt.Errorf("mail: dial %s: %w", s.addr(), err)
		}
		_ = conn.SetDeadline(time.Now().Add(smtpExchangeTimeout))
		defer conn.Close()
		client, err = smtp.NewClient(conn, s.Host)
		if err != nil {
			return fmt.Errorf("mail: smtp handshake: %w", err)
		}
		if ok, _ := client.Extension("STARTTLS"); !ok {
			_ = client.Close()
			return fmt.Errorf("mail: server %s does not offer STARTTLS; refusing plaintext", s.addr())
		}
		if err := client.StartTLS(tlsCfg); err != nil {
			_ = client.Close()
			return fmt.Errorf("mail: STARTTLS: %w", err)
		}
	}
	defer client.Close()

	if s.Username != "" {
		auth := smtp.PlainAuth("", s.Username, s.Password, s.Host)
		if err := client.Auth(auth); err != nil {
			return fmt.Errorf("mail: auth: %w", err)
		}
	}
	if err := client.Mail(s.FromAddress); err != nil {
		return fmt.Errorf("mail: MAIL FROM: %w", err)
	}
	if err := client.Rcpt(msg.To); err != nil {
		return fmt.Errorf("mail: RCPT TO: %w", err)
	}
	wc, err := client.Data()
	if err != nil {
		return fmt.Errorf("mail: DATA: %w", err)
	}
	if _, err := wc.Write(formatMessage(s.from(), msg)); err != nil {
		_ = wc.Close()
		return fmt.Errorf("mail: write body: %w", err)
	}
	if err := wc.Close(); err != nil {
		return fmt.Errorf("mail: commit: %w", err)
	}
	return client.Quit()
}

// formatMessage builds a minimal RFC 5322 message with plain + HTML
// alternatives. Subjects are operator-controlled template strings; the
// recipient and content are escaped by construction (headers contain no user
// input beyond the validated address).
func formatMessage(from string, msg Message) []byte {
	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + msg.To + "\r\n")
	b.WriteString("Subject: " + msg.Subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	b.WriteString("Content-Type: multipart/alternative; boundary=openbase-mail-boundary\r\n")
	b.WriteString("\r\n")
	b.WriteString("--openbase-mail-boundary\r\n")
	b.WriteString("Content-Type: text/plain; charset=utf-8\r\n\r\n")
	b.WriteString(msg.Text + "\r\n")
	b.WriteString("--openbase-mail-boundary\r\n")
	b.WriteString("Content-Type: text/html; charset=utf-8\r\n\r\n")
	b.WriteString(msg.HTML + "\r\n")
	b.WriteString("--openbase-mail-boundary--\r\n")
	return []byte(b.String())
}
