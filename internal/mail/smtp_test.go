package mail

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"fmt"
	"math/big"
	"net"
	"strings"
	"testing"
	"time"
)
// fakeSMTPServer speaks just enough SMTP to prove the sender's wire flow:
// EHLO → STARTTLS → EHLO → AUTH PLAIN → MAIL/RCPT/DATA → QUIT, capturing
// the envelope and body.
type fakeSMTPServer struct {
	t      *testing.T
	addr   string
	user   string
	pass   string
	from   string
	to     string
	body   string
	gotTLS bool
	done   chan struct{}
}

func startFakeSMTP(t *testing.T, user, pass string) *fakeSMTPServer {
	t.Helper()
	cert, err := selfSignedCert()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeSMTPServer{t: t, addr: ln.Addr().String(), user: user, pass: pass, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		defer ln.Close()
		s.serve(conn, cert)
	}()
	return s
}

func pemEncode(block string, der []byte) []byte {
	// Minimal PEM writer without importing encoding/pem into the helper path:
	// kept explicit so the test stays dependency-free.
	var out strings.Builder
	out.WriteString("-----BEGIN " + block + "-----\n")
	enc := base64.StdEncoding.EncodeToString(der)
	for i := 0; i < len(enc); i += 64 {
		end := i + 64
		if end > len(enc) {
			end = len(enc)
		}
		out.WriteString(enc[i:end] + "\n")
	}
	out.WriteString("-----END " + block + "-----\n")
	return []byte(out.String())
}

func mustMarshalEC(key *ecdsa.PrivateKey) []byte {
	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		panic(err)
	}
	return der
}

func TestSMTPSenderSTARTTLSFlow(t *testing.T) {
	srv := startFakeSMTP(t, "op", "secret")
	host, portStr, _ := net.SplitHostPort(srv.addr)
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)

	sender := &SMTPSender{
		Host: host, Port: port, Username: "op", Password: "secret",
		FromAddress: "noreply@example.com", FromName: "Openbase",
		tlsConfig:   &tls.Config{InsecureSkipVerify: true},
	}
	msg := Message{To: "user@example.com", Subject: "Hi", HTML: "<p>Hi</p>", Text: "Hi"}
	if err := sender.Send(context.Background(), msg); err != nil {
		t.Fatalf("send: %v", err)
	}
	<-srv.done
	if !srv.gotTLS {
		t.Fatal("expected STARTTLS upgrade")
	}
	if srv.from != "noreply@example.com" || srv.to != "user@example.com" {
		t.Fatalf("envelope from=%q to=%q", srv.from, srv.to)
	}
	for _, want := range []string{"Subject: Hi", "To: user@example.com", "text/plain", "text/html", "<p>Hi</p>"} {
		if !strings.Contains(srv.body, want) {
			t.Fatalf("body missing %q:\n%s", want, srv.body)
		}
	}
}

func TestSMTPSenderRefusesPlaintext(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
		rw.WriteString("220 fake\r\n")
		rw.Flush()
		for {
			line, err := rw.ReadString('\n')
			if err != nil {
				return
			}
			if strings.HasPrefix(strings.ToUpper(line), "EHLO") {
				// Deliberately no STARTTLS advertisement.
				rw.WriteString("250 fake\r\n")
				rw.Flush()
			}
		}
	}()
	host, portStr, _ := net.SplitHostPort(ln.Addr().String())
	var port int
	_, _ = fmt.Sscanf(portStr, "%d", &port)
	sender := &SMTPSender{Host: host, Port: port, FromAddress: "n@e.c"}
	err = sender.Send(context.Background(), Message{To: "u@e.c", Subject: "x", Text: "x"})
	if err == nil || !strings.Contains(err.Error(), "STARTTLS") {
		t.Fatalf("expected STARTTLS refusal, got %v", err)
	}
}

func selfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.X509KeyPair(
		pemEncode("CERTIFICATE", der),
		pemEncode("EC PRIVATE KEY", mustMarshalEC(key)),
	)
}

func (s *fakeSMTPServer) serve(raw net.Conn, cert tls.Certificate) {
	rw := bufio.NewReadWriter(bufio.NewReader(raw), bufio.NewWriter(raw))
	say := func(line string) {
		rw.WriteString(line + "\r\n")
		rw.Flush()
	}
	read := func() string {
		line, err := rw.ReadString('\n')
		if err != nil {
			s.t.Errorf("fake smtp read: %v", err)
		}
		return strings.TrimRight(line, "\r\n")
	}
	say("220 fake ESMTP")
	tlsUp := false
	for {
		line := read()
		upper := strings.ToUpper(line)
		switch {
		case strings.HasPrefix(upper, "EHLO"):
			if !tlsUp {
				say("250-fake")
				say("250-STARTTLS")
				say("250 AUTH PLAIN")
			} else {
				say("250-fake")
				say("250 AUTH PLAIN")
			}
		case strings.HasPrefix(upper, "STARTTLS"):
			say("220 ready")
			tlsConn := tls.Server(raw, &tls.Config{Certificates: []tls.Certificate{cert}})
			if err := tlsConn.Handshake(); err != nil {
				s.t.Errorf("fake smtp tls handshake: %v", err)
				return
			}
			s.gotTLS = true
			rw = bufio.NewReadWriter(bufio.NewReader(tlsConn), bufio.NewWriter(tlsConn))
			// Rebind say/read to the TLS stream.
			say = func(line string) {
				rw.WriteString(line + "\r\n")
				rw.Flush()
			}
			read = func() string {
				line, err := rw.ReadString('\n')
				if err != nil {
					s.t.Errorf("fake smtp read: %v", err)
				}
				return strings.TrimRight(line, "\r\n")
			}
			tlsUp = true
		case strings.HasPrefix(upper, "AUTH PLAIN"):
			parts := strings.SplitN(line, " ", 3)
			if len(parts) != 3 {
				say("535 bad auth")
				continue
			}
			raw, err := base64.StdEncoding.DecodeString(parts[2])
			if err != nil || string(raw) != "\x00"+s.user+"\x00"+s.pass {
				say("535 bad auth")
				continue
			}
			say("235 ok")
		case strings.HasPrefix(upper, "MAIL FROM:"):
			s.from = strings.Trim(line[len("MAIL FROM:"):], " <>")
			say("250 ok")
		case strings.HasPrefix(upper, "RCPT TO:"):
			s.to = strings.Trim(line[len("RCPT TO:"):], " <>")
			say("250 ok")
		case strings.HasPrefix(upper, "DATA"):
			say("354 go")
			var body strings.Builder
			for {
				l := read()
				if l == "." {
					break
				}
				body.WriteString(l + "\n")
			}
			s.body = body.String()
			say("250 queued")
		case strings.HasPrefix(upper, "QUIT"):
			say("221 bye")
			return
		default:
			say("502 unimplemented")
		}
	}
}
