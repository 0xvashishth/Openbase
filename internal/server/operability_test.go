package server_test

import (
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/openbase/openbase/internal/testutil"
)

// TestHealthz proves the liveness probe answers without credentials and
// carries a request ID.
func TestHealthz(t *testing.T) {
	ts := newTestServer(t)
	resp, js := ts.do(t, "GET", "/healthz", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz status = %d body=%v", resp.StatusCode, js)
	}
	if js["status"] != "ok" {
		t.Fatalf("healthz body = %v, want status ok", js)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Fatal("healthz must set X-Request-ID")
	}
}

// TestReadyzReady proves readiness against the real metadata DB: reachable
// pool + fully applied migrations.
func TestReadyzReady(t *testing.T) {
	ts := newTestServer(t)
	resp, js := ts.do(t, "GET", "/readyz", "", nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("readyz status = %d body=%v", resp.StatusCode, js)
	}
	if js["status"] != "ready" {
		t.Fatalf("readyz body = %v, want status ready", js)
	}
}

// TestMetricsCountsRequests proves /metrics exposes Prometheus counters that
// move with traffic.
func TestMetricsCountsRequests(t *testing.T) {
	ts := newTestServer(t)
	for i := 0; i < 3; i++ {
		resp, _ := ts.do(t, "GET", "/healthz", "", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("healthz %d status = %d", i, resp.StatusCode)
		}
	}
	resp, err := http.Get(ts.url + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	text := string(body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("metrics status = %d body=%s", resp.StatusCode, text)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.Contains(ct, "text/plain") {
		t.Fatalf("metrics content-type = %q, want Prometheus text", ct)
	}
	for _, want := range []string{
		"openbase_http_requests_total",
		"openbase_http_requests_by_route",
		"openbase_http_request_duration_nanoseconds_sum",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("metrics missing series %q:\n%s", want, text)
		}
	}
}

// TestAuthRateLimit proves per-IP limiting on /v1/auth/*: the N+1st request
// in a minute is rejected with 429.
func TestAuthRateLimit(t *testing.T) {
	ts := newTestServer(t)
	ts.svc.AuthRateLimitPerMin = 3
	bad := map[string]string{"email": "nobody@example.com", "password": "wrong"}
	for i := 0; i < 3; i++ {
		resp, _ := ts.do(t, "POST", "/v1/auth/login", "", bad)
		if resp.StatusCode == http.StatusTooManyRequests {
			t.Fatalf("request %d rejected early, want rate limit at 4th", i+1)
		}
	}
	resp, js := ts.do(t, "POST", "/v1/auth/login", "", bad)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("4th login status = %d body=%v, want 429", resp.StatusCode, js)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Fatal("429 must carry Retry-After")
	}
}

// TestAuthLockout proves lockout after N failed attempts: even well-formed
// requests are rejected while the lock holds.
func TestAuthLockout(t *testing.T) {
	ts := newTestServer(t)
	ts.svc.AuthRateLimitPerMin = 1000 // don't trip the volume limiter
	ts.svc.AuthLockoutThreshold = 2
	bad := map[string]string{"email": "nobody@example.com", "password": "wrong"}
	for i := 0; i < 2; i++ {
		resp, _ := ts.do(t, "POST", "/v1/auth/login", "", bad)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("bad login %d status = %d, want 401", i+1, resp.StatusCode)
		}
	}
	resp, js := ts.do(t, "POST", "/v1/auth/login", "", bad)
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("post-lockout login status = %d body=%v, want 429", resp.StatusCode, js)
	}
}

// TestAPIKeyQuota proves per-key quotas on the data plane: the N+1st request
// in a minute is rejected with 429.
func TestAPIKeyQuota(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	_, key := seedRowDB(t, ts, userDB.DSN,
		`CREATE TABLE t (id SERIAL PRIMARY KEY, v TEXT);`)
	ts.svc.APIKeyQuotaPerMin = 2

	get := func() int {
		req, _ := http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
		req.Header.Set("Authorization", "Bearer "+key)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		io.Copy(io.Discard, resp.Body)
		return resp.StatusCode
	}
	if s := get(); s != http.StatusOK {
		t.Fatalf("first tables status = %d, want 200", s)
	}
	if s := get(); s != http.StatusOK {
		t.Fatalf("second tables status = %d, want 200", s)
	}
	if s := get(); s != http.StatusTooManyRequests {
		t.Fatalf("third tables status = %d, want 429", s)
	}
}
