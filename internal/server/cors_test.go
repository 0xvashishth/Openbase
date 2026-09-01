package server_test

import (
	"net/http"
	"testing"
)

func TestCORSWildcard(t *testing.T) {
	h := newTestServer(t)
	req, _ := http.NewRequest("GET", h.url+"/v1/me", nil)
	req.Header.Set("Origin", "http://evil.example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.Header.Get("Access-Control-Allow-Origin") != "*" {
		t.Fatalf("expected wildcard ACAO header, got %q", resp.Header.Get("Access-Control-Allow-Origin"))
	}
}

func TestCORSPreflight(t *testing.T) {
	s := newTestServer(t)
	req, _ := http.NewRequest("OPTIONS", s.url+"/v1/auth/login", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "content-type")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", resp.StatusCode)
	}
	if resp.Header.Get("Access-Control-Allow-Methods") == "" {
		t.Fatal("preflight should advertise allowed methods")
	}
	if resp.Header.Get("Access-Control-Allow-Headers") == "" {
		t.Fatal("preflight should advertise allowed headers")
	}
}