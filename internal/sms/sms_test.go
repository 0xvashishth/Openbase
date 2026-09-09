package sms

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestLogProviderAlwaysSucceeds(t *testing.T) {
	p := &LogProvider{}
	if p.Name() != "log" {
		t.Fatal("name")
	}
	if err := p.Send(context.Background(), "+15550001", "code 123456"); err != nil {
		t.Fatal(err)
	}
}

func TestTwilioProviderPostsFormWithBasicAuth(t *testing.T) {
	var gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, pw, _ := r.BasicAuth()
		gotAuth = u + ":" + pw
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"sid":"SMx"}`))
	}))
	defer srv.Close()

	p := &TwilioProvider{AccountSID: "AC1", AuthToken: "tok", From: "+15559999", BaseURL: srv.URL, HTTP: srv.Client()}
	if err := p.Send(context.Background(), "+15550001", "hello"); err != nil {
		t.Fatal(err)
	}
	if gotAuth != "AC1:tok" {
		t.Fatalf("basic auth = %q", gotAuth)
	}
	form, _ := url.ParseQuery(gotBody)
	if form.Get("To") != "+15550001" || form.Get("From") != "+15559999" || form.Get("Body") != "hello" {
		t.Fatalf("form = %v", form)
	}
}

func TestTwilioProviderSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"code":21211,"message":"bad to"}`))
	}))
	defer srv.Close()

	p := &TwilioProvider{AccountSID: "AC1", AuthToken: "tok", From: "+15559999", BaseURL: srv.URL, HTTP: srv.Client()}
	err := p.Send(context.Background(), "bad", "hello")
	if err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("expected status error, got %v", err)
	}
}

func TestTwilioProviderRequiresConfig(t *testing.T) {
	p := &TwilioProvider{}
	if err := p.Send(context.Background(), "+1", "x"); err == nil {
		t.Fatal("expected config error")
	}
}
