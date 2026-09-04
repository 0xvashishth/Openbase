package server

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

// tlsStub marks a request as TLS-terminated without needing a real handshake.
var tlsStub = tls.ConnectionState{}

func TestPublicBaseURL(t *testing.T) {
	tests := []struct {
		name       string
		configured string
		host       string
		tls        bool
		headers    map[string]string
		want       string
	}{
		{
			name:       "configured wins and loses trailing slash",
			configured: "https://api.example.com/",
			host:       "internal:8080",
			want:       "https://api.example.com",
		},
		{
			name: "derived from plain request",
			host: "localhost:8080",
			want: "http://localhost:8080",
		},
		{
			name: "derived from TLS request",
			host: "api.example.com",
			tls:  true,
			want: "https://api.example.com",
		},
		{
			name:    "honours reverse-proxy forwarded headers",
			host:    "api-internal:8080",
			headers: map[string]string{"X-Forwarded-Proto": "https", "X-Forwarded-Host": "api.example.com"},
			want:    "https://api.example.com",
		},
		{
			name:    "takes the left-most forwarded value",
			host:    "api-internal:8080",
			headers: map[string]string{"X-Forwarded-Proto": "https, http", "X-Forwarded-Host": "api.example.com, inner"},
			want:    "https://api.example.com",
		},
		{
			name:       "blank configuration falls back to the request",
			configured: "   ",
			host:       "localhost:8080",
			want:       "http://localhost:8080",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/v1/projects/p1/connect-info", nil)
			r.Host = tc.host
			if tc.tls {
				r.TLS = &tlsStub
			}
			for k, v := range tc.headers {
				r.Header.Set(k, v)
			}
			if got := publicBaseURL(r, tc.configured); got != tc.want {
				t.Fatalf("publicBaseURL = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestWebsocketURL(t *testing.T) {
	cases := map[string]string{
		"https://api.example.com": "wss://api.example.com/v1/realtime",
		"http://localhost:8080":   "ws://localhost:8080/v1/realtime",
		// Unexpected scheme: append rather than silently produce a broken URL.
		"api.example.com": "api.example.com/v1/realtime",
	}
	for in, want := range cases {
		if got := websocketURL(in); got != want {
			t.Fatalf("websocketURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestConnectEndpointPathsCoverPublicAPI(t *testing.T) {
	e := connectEndpointPaths()
	for _, want := range []string{
		"GET /v1/api/tables",
		"POST /v1/api/{collection}",
		"PUT /v1/api/{collection}/{id}",
		"DELETE /v1/api/{collection}/{id}",
		"GET /v1/api/{collection}/_schema",
		"GET /v1/realtime",
	} {
		found := false
		for _, got := range []string{
			e.ListTables, e.QueryRows, e.TableSchema,
			e.InsertRow, e.UpdateRow, e.DeleteRow, e.RealtimePath,
		} {
			if got == want || (want == "GET /v1/api/{collection}?limit=&offset=&order_by=&order=" && got == want) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("connect endpoints missing %q (got %+v)", want, e)
		}
	}
	if e.QueryRows != "GET /v1/api/{collection}?limit=&offset=&order_by=&order=" {
		t.Fatalf("query rows endpoint = %q", e.QueryRows)
	}
}
