package server_test

import (
	"net/http"
	"testing"
)

// TestGetProject covers the single-project resolver: member reads, outsider
// is forbidden, missing is 404, anonymous is 401.
func TestGetProject(t *testing.T) {
	ts := newTestServer(t)
	tok, orgID, pID := ts.newProject(t)

	resp, js := ts.do(t, "GET", "/v1/projects/"+pID, tok, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get project status = %d body=%v", resp.StatusCode, js)
	}
	if js["id"] != pID {
		t.Fatalf("id = %v, want %s", js["id"], pID)
	}
	if js["organization_id"] != orgID {
		t.Fatalf("organization_id = %v, want %s", js["organization_id"], orgID)
	}
	if js["slug"] != "data-proj" {
		t.Fatalf("slug = %v, want data-proj", js["slug"])
	}

	outsider := ts.register(t, "outsider@example.com", "pw123456")
	resp, _ = ts.do(t, "GET", "/v1/projects/"+pID, outsider, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("outsider status = %d, want 403", resp.StatusCode)
	}

	resp, _ = ts.do(t, "GET", "/v1/projects/00000000-0000-0000-0000-000000000000", tok, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", resp.StatusCode)
	}

	resp, _ = ts.do(t, "GET", "/v1/projects/"+pID, "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", resp.StatusCode)
	}
}
