package server_test

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/openbase/openbase/internal/testutil"
)

func TestAPIKeyCRUD(t *testing.T) {
	ts := newTestServer(t)
	tok, _, pID := ts.newProject(t)

	// List keys (empty).
	resp, js := ts.do(t, "GET", "/v1/projects/"+pID+"/api-keys", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list keys status = %d body=%v", resp.StatusCode, js)
	}

	// Create a key.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "test-key"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d body=%v", resp.StatusCode, js)
	}
	plaintext, _ := js["plaintext"].(string)
	keyID, _ := js["id"].(string)
	if plaintext == "" || keyID == "" {
		t.Fatalf("expected plaintext and id: %v", js)
	}

	// List keys (1 key).
	resp, js = ts.do(t, "GET", "/v1/projects/"+pID+"/api-keys", tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list keys status = %d body=%v", resp.StatusCode, js)
	}

	// Revoke the key.
	resp, js = ts.do(t, "DELETE", "/v1/projects/"+pID+"/api-keys/"+keyID, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revoke key status = %d body=%v", resp.StatusCode, js)
	}
}

func TestPublicAPIWithKey(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	// Connect user database as BYODB.
	resp, js := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save connection status = %d body=%v", resp.StatusCode, js)
	}
	if ok, _ := js["success"].(bool); !ok {
		t.Fatalf("save should succeed: %v", js)
	}

	// Create a table in the user database.
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatalf("user db connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
		CREATE TABLE items (
			id SERIAL PRIMARY KEY,
			name TEXT NOT NULL,
			price INT NOT NULL DEFAULT 0
		);
		INSERT INTO items (name, price) VALUES ('Widget', 99), ('Gadget', 149);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Create API key.
	resp, js = ts.do(t, "POST", "/v1/projects/"+pID+"/api-keys", tok, map[string]string{"name": "public"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create key status = %d body=%v", resp.StatusCode, js)
	}
	plaintext, _ := js["plaintext"].(string)
	if plaintext == "" {
		t.Fatal("expected plaintext key")
	}

	// Use API key to list tables.
	req, _ := http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("api tables status = %d", resp2.StatusCode)
	}

	// Use API key to query rows.
	req, _ = http.NewRequest("GET", ts.url+"/v1/api/items?limit=1", nil)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	resp3, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("api query status = %d", resp3.StatusCode)
	}

	// Use API key to insert a row.
	body := strings.NewReader(`{"name": "Doohickey", "price": 42}`)
	req, _ = http.NewRequest("POST", ts.url+"/v1/api/items", body)
	req.Header.Set("Authorization", "Bearer "+plaintext)
	req.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp4.Body.Close()
	if resp4.StatusCode != http.StatusCreated {
		t.Fatalf("api insert status = %d", resp4.StatusCode)
	}

	// Invalid API key -> 401.
	req, _ = http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
	req.Header.Set("Authorization", "Bearer ob_badkey")
	resp5, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp5.Body.Close()
	if resp5.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad key status = %d, want 401", resp5.StatusCode)
	}

	// No key -> 401.
	req, _ = http.NewRequest("GET", ts.url+"/v1/api/tables", nil)
	resp6, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp6.Body.Close()
	if resp6.StatusCode != http.StatusUnauthorized {
		t.Fatalf("no key status = %d, want 401", resp6.StatusCode)
	}
}

func TestFullSchemaEndpoint(t *testing.T) {
	ts := newTestServer(t)
	userDB := testutil.StartPostgres(t)
	tok, _, pID := ts.newProject(t)

	resp, _ := ts.do(t, "POST", "/v1/projects/"+pID+"/connections", tok,
		map[string]string{"connection_string": userDB.DSN})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("connect status = %d", resp.StatusCode)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, userDB.DSN)
	if err != nil {
		t.Fatalf("connect user db: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `
		CREATE TABLE articles (
			id SERIAL PRIMARY KEY,
			title TEXT NOT NULL
		);
	`); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp2, js := ts.do(t, "GET", "/v1/projects/"+pID+"/schema", tok, nil)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("full schema status = %d body=%v", resp2.StatusCode, js)
	}
	collections, ok := js["collections"].([]any)
	if !ok || len(collections) == 0 {
		t.Fatalf("expected collections in schema: %v", js)
	}
}
