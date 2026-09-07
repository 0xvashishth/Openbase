package server

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/openbase/openbase/migrations"
)

// ---- Liveness / readiness ----

// healthz is the liveness probe: it answers without touching any dependency.
// Point load balancers, container healthchecks and the dashboard status badge
// here — never at an authenticated endpoint.
func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// dbPinger is the subset of the metadata store readiness needs. Real stores
// implement it; test fakes may not, in which case readyz reports degraded.
type dbPinger interface {
	Ping(ctx context.Context) error
}

// migrationReporter exposes the applied schema_migrations ledger.
type migrationReporter interface {
	AppliedMigrations(ctx context.Context) ([]string, error)
}

// readyz reports whether the API can serve traffic: the metadata database is
// reachable and its schema matches this binary's expected migrations.
// Returns 200 {"status":"ready"} or 503 {"status":"...","reason":"..."}.
func (s *Server) readyz(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pinger, ok := s.svc.Store.(dbPinger)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "degraded",
			"reason": "metadata store does not support health checks",
		})
		return
	}
	if err := pinger.Ping(ctx); err != nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"status": "not-ready",
			"reason": "metadata database unreachable",
		})
		return
	}
	if reporter, ok := s.svc.Store.(migrationReporter); ok {
		applied, err := reporter.AppliedMigrations(ctx)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "not-ready",
				"reason": "cannot read migration ledger",
			})
			return
		}
		have := map[string]bool{}
		for _, v := range applied {
			have[v] = true
		}
		var missing []string
		for _, v := range migrations.ExpectedVersions() {
			if !have[v] {
				missing = append(missing, v)
			}
		}
		if len(missing) > 0 {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "not-ready",
				"reason": "pending migrations: " + strings.Join(missing, ","),
			})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ready"})
}

// ---- Metrics (dependency-free Prometheus exposition) ----

// metricsRegistry holds process-wide HTTP counters. It lives on Server (not
// package-global) so tests never share series.
type metricsRegistry struct {
	requestsTotal   atomic.Uint64
	requestsByCode  sync.Map // string(status) -> *atomic.Uint64
	requestsByRoute sync.Map // "METHOD /pattern" -> *atomic.Uint64 (bounded, see record)
	durationNanos   atomic.Uint64
}

func newMetricsRegistry() *metricsRegistry { return &metricsRegistry{} }

func counterFor(m *sync.Map, key string) *atomic.Uint64 {
	if v, ok := m.Load(key); ok {
		return v.(*atomic.Uint64)
	}
	c := &atomic.Uint64{}
	actual, _ := m.LoadOrStore(key, c)
	return actual.(*atomic.Uint64)
}

// maxRouteSeries bounds the per-route series so crafted paths (UUIDs, table
// names) cannot grow memory without limit; overflow lands in "other".
const maxRouteSeries = 512

func (m *metricsRegistry) record(method, route string, status int, d time.Duration) {
	m.requestsTotal.Add(1)
	m.durationNanos.Add(uint64(d.Nanoseconds()))
	counterFor(&m.requestsByCode, strconv.Itoa(status)).Add(1)
	key := method + " " + route
	if _, ok := m.requestsByRoute.Load(key); !ok {
		n := 0
		m.requestsByRoute.Range(func(_, _ any) bool { n++; return true })
		if n >= maxRouteSeries {
			key = method + " other"
		}
	}
	counterFor(&m.requestsByRoute, key).Add(1)
}

// routePattern collapses a request path to its mux pattern shape so table
// names and UUIDs do not explode series cardinality. It mirrors the route
// table in New: only known fixed prefixes keep their tail.
func routePattern(p string) string {
	switch {
	case p == "/healthz" || p == "/readyz" || p == "/metrics":
		return p
	case strings.HasPrefix(p, "/v1/auth/"):
		return "/v1/auth/*"
	case p == "/v1/me" || p == "/v1/orgs" || p == "/v1/projects":
		return p
	case strings.HasPrefix(p, "/v1/api/"):
		return "/v1/api/*"
	case p == "/v1/realtime":
		return p
	case strings.HasPrefix(p, "/v1/orgs/"):
		return "/v1/orgs/*"
	case strings.HasPrefix(p, "/v1/projects/"):
		return "/v1/projects/*"
	default:
		return "other"
	}
}

// metrics exposes counters in Prometheus text exposition format.
func (s *Server) metrics(w http.ResponseWriter, r *http.Request) {
	m := s.opMetrics()
	var b strings.Builder
	b.WriteString("# HELP openbase_http_requests_total Total HTTP requests served.\n")
	b.WriteString("# TYPE openbase_http_requests_total counter\n")
	b.WriteString(fmt.Sprintf("openbase_http_requests_total %d\n", m.requestsTotal.Load()))
	m.requestsByCode.Range(func(k, v any) bool {
		fmt.Fprintf(&b, "openbase_http_requests_total{status=%q} %d\n", k, v.(*atomic.Uint64).Load())
		return true
	})
	b.WriteString("# HELP openbase_http_requests_by_route Total HTTP requests by method and route pattern.\n")
	b.WriteString("# TYPE openbase_http_requests_by_route counter\n")
	m.requestsByRoute.Range(func(k, v any) bool {
		parts := strings.SplitN(k.(string), " ", 2)
		method, route := parts[0], "other"
		if len(parts) == 2 {
			route = parts[1]
		}
		fmt.Fprintf(&b, "openbase_http_requests_by_route{method=%q,route=%q} %d\n", method, route, v.(*atomic.Uint64).Load())
		return true
	})
	b.WriteString("# HELP openbase_http_request_duration_nanoseconds_sum Total time spent serving HTTP requests.\n")
	b.WriteString("# TYPE openbase_http_request_duration_nanoseconds_sum counter\n")
	fmt.Fprintf(&b, "openbase_http_request_duration_nanoseconds_sum %d\n", m.durationNanos.Load())
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	_, _ = w.Write([]byte(b.String()))
}

// ---- Request ID + status/bytes logging ----

type ctxKeyRequestID struct{}

// requestID generates a random hex request identifier.
func requestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// statusWriter captures the status code and bytes written for logging.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Hijack forwards connection hijacking to the underlying writer. Without
// this, wrapping the ResponseWriter would break the WebSocket upgrade on
// /v1/realtime (the handshake takes over the TCP connection via Hijacker).
func (w *statusWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := w.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, errors.New("operability: underlying writer does not support hijacking")
}

// withRequestID assigns every request an ID (or propagates the caller's
// X-Request-ID), exposes it on the response, and upgrades the access log with
// request id, status code and bytes — the old log line had none of those.
func (s *Server) withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = requestID()
		}
		w.Header().Set("X-Request-ID", id)
		sw := &statusWriter{ResponseWriter: w}
		start := time.Now()
		next.ServeHTTP(sw, r.WithContext(context.WithValue(r.Context(), ctxKeyRequestID{}, id)))
		status := sw.status
		if status == 0 {
			// The handler hijacked the connection (WebSocket upgrade on
			// /v1/realtime): the 101 went over the raw conn, invisible to
			// this wrapper. Attribute it rather than recording a bogus 0.
			status = http.StatusSwitchingProtocols
		}
		s.opMetrics().record(r.Method, routePattern(r.URL.Path), status, time.Since(start))
		s.svc.Log.Info("http",
			"request_id", id,
			"method", r.Method,
			"path", r.URL.Path,
			"status", status,
			"bytes", sw.bytes,
			"duration", time.Since(start).String(),
		)
	})
}

// ---- Auth rate limiting with lockout ----

const (
	defaultAuthPerMin        = 60
	defaultAuthLockoutAfter  = 10
	defaultAuthLockoutFor    = 10 * time.Minute
	defaultAPIKeyQuotaPerMin = 1000
)

// authBucket tracks one IP's auth attempt window and failure lockout.
type authBucket struct {
	windowStart time.Time
	count       int
	failures    int
	lockedUntil time.Time
}

// rateState holds the limiter maps. It lives on Server so tests are isolated.
type rateState struct {
	mu       sync.Mutex
	authIPs  map[string]*authBucket
	keyQuota map[string]*quotaBucket
}

type quotaBucket struct {
	windowStart time.Time
	count       int
}

func newRateState() *rateState {
	return &rateState{authIPs: map[string]*authBucket{}, keyQuota: map[string]*quotaBucket{}}
}

// clientIP prefers the leftmost X-Forwarded-For entry (self-host behind a
// proxy) and falls back to the connection's remote address.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		if first, _, _ := strings.Cut(xff, ","); strings.TrimSpace(first) != "" {
			return strings.TrimSpace(first)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) authPerMin() int {
	if s.svc.AuthRateLimitPerMin > 0 {
		return s.svc.AuthRateLimitPerMin
	}
	return defaultAuthPerMin
}

func (s *Server) authLockoutAfter() int {
	if s.svc.AuthLockoutThreshold > 0 {
		return s.svc.AuthLockoutThreshold
	}
	return defaultAuthLockoutAfter
}

// limitAuth wraps the /v1/auth/* routes: per-IP fixed-window rate limiting
// plus lockout after N failed attempts (observed via 4xx responses).
func (s *Server) limitAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		st := s.opRates()
		ip := clientIP(r)
		now := time.Now()

		st.mu.Lock()
		b, ok := st.authIPs[ip]
		if !ok {
			b = &authBucket{windowStart: now}
			st.authIPs[ip] = b
		}
		if now.Sub(b.windowStart) >= time.Minute {
			b.windowStart, b.count = now, 0
		}
		if now.Before(b.lockedUntil) {
			st.mu.Unlock()
			w.Header().Set("Retry-After", strconv.Itoa(int(time.Until(b.lockedUntil).Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "too many failed attempts; try again later")
			return
		}
		b.count++
		over := b.count > s.authPerMin()
		st.mu.Unlock()

		if over {
			w.Header().Set("Retry-After", "60")
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		sw := &statusWriter{ResponseWriter: w}
		next.ServeHTTP(sw, r)
		if sw.status == 0 {
			sw.status = http.StatusOK
		}

		// Observe the outcome for lockout accounting.
		st.mu.Lock()
		defer st.mu.Unlock()
		if sw.status >= 200 && sw.status < 300 {
			b.failures = 0
			b.lockedUntil = time.Time{}
		} else if sw.status == http.StatusUnauthorized || sw.status == http.StatusBadRequest {
			b.failures++
			if b.failures >= s.authLockoutAfter() {
				b.lockedUntil = now.Add(defaultAuthLockoutFor)
				b.failures = 0
			}
		}
	})
}

// ---- Per-API-key quotas ----

func (s *Server) apiKeyQuotaPerMin() int {
	if s.svc.APIKeyQuotaPerMin > 0 {
		return s.svc.APIKeyQuotaPerMin
	}
	return defaultAPIKeyQuotaPerMin
}

// checkAPIKeyQuota enforces a per-minute request quota per API key hash.
// Call after the key is resolved; returns false + 429 when exhausted.
func (s *Server) checkAPIKeyQuota(w http.ResponseWriter, r *http.Request, keyHash string) bool {
	st := s.opRates()
	now := time.Now()
	st.mu.Lock()
	b, ok := st.keyQuota[keyHash]
	if !ok {
		b = &quotaBucket{windowStart: now}
		st.keyQuota[keyHash] = b
	}
	if now.Sub(b.windowStart) >= time.Minute {
		b.windowStart, b.count = now, 0
	}
	// Opportunistic janitor: drop idle windows so the map cannot grow with
	// the number of keys ever seen.
	if len(st.keyQuota) > 4096 {
		for k, v := range st.keyQuota {
			if now.Sub(v.windowStart) > 5*time.Minute {
				delete(st.keyQuota, k)
			}
		}
	}
	b.count++
	over := b.count > s.apiKeyQuotaPerMin()
	st.mu.Unlock()
	if over {
		w.Header().Set("Retry-After", "60")
		writeError(w, http.StatusTooManyRequests, "API key quota exceeded")
		return false
	}
	return true
}

// opMetrics / opRates lazily initialize operability state so every Server
// construction path (including tests) gets isolated instances.
func (s *Server) opMetrics() *metricsRegistry {
	if s.metricsReg == nil {
		s.metricsReg = newMetricsRegistry()
	}
	return s.metricsReg
}

func (s *Server) opRates() *rateState {
	if s.rates == nil {
		s.rates = newRateState()
	}
	return s.rates
}
