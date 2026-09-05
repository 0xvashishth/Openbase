package server_test

import (
	"net/http"
	"testing"

	"github.com/openbase/openbase/internal/metadata"
)

func TestListMembers(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")

	t.Run("member can list and rows carry email", func(t *testing.T) {
		arr := mustDecodeArray(t, ts, memberTok, "/v1/orgs/"+orgID+"/members")
		if len(arr) != 3 {
			t.Fatalf("expected 3 members, got %d: %v", len(arr), arr)
		}
		emails := map[string]string{}
		for _, raw := range arr {
			m, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("member row is not an object: %v", raw)
			}
			email, _ := m["email"].(string)
			if email == "" {
				// Without email the UI has nothing to render but an opaque uuid.
				t.Fatalf("member row missing email: %v", m)
			}
			if _, leaked := m["password_hash"]; leaked {
				t.Fatalf("password_hash must never be returned: %v", m)
			}
			role, _ := m["role"].(string)
			emails[email] = role
		}
		if emails["owner@example.com"] != string(metadata.RoleOwner) {
			t.Fatalf("owner role = %q", emails["owner@example.com"])
		}
		if emails["admin@example.com"] != string(metadata.RoleAdmin) {
			t.Fatalf("admin role = %q", emails["admin@example.com"])
		}
	})

	t.Run("ordered by role rank descending", func(t *testing.T) {
		arr := mustDecodeArray(t, ts, ownerTok, "/v1/orgs/"+orgID+"/members")
		rank := map[string]int{"owner": 3, "admin": 2, "member": 1}
		last := 4
		for _, raw := range arr {
			role, _ := raw.(map[string]any)["role"].(string)
			if rank[role] > last {
				t.Fatalf("members not ordered by role rank desc: %v", arr)
			}
			last = rank[role]
		}
	})

	t.Run("outsider forbidden", func(t *testing.T) {
		resp, _ := ts.do(t, "GET", "/v1/orgs/"+orgID+"/members", outsiderTok, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})
}

func TestAddMember(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, _ := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, _ := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)

	// Accounts that exist but are not yet members.
	ts.register(t, "newbie@example.com", "pw123456")
	ts.register(t, "second@example.com", "pw123456")
	ts.register(t, "third@example.com", "pw123456")

	t.Run("admin adds an existing account", func(t *testing.T) {
		resp, js := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", adminTok,
			map[string]string{"email": "newbie@example.com", "role": "member"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d body=%v, want 201", resp.StatusCode, js)
		}
		if js["email"] != "newbie@example.com" {
			t.Fatalf("email = %v", js["email"])
		}
	})

	// There is no mailer yet (PHASES 9.2), so an unknown address cannot be
	// invited. 404 is the honest answer; a 201 would imply an invite went out.
	t.Run("unknown email is 404, not a silent invite", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", adminTok,
			map[string]string{"email": "ghost@example.com", "role": "member"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("duplicate conflicts", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", adminTok,
			map[string]string{"email": "member@example.com", "role": "member"})
		if resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 409", resp.StatusCode)
		}
	})

	t.Run("member cannot add", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", memberTok,
			map[string]string{"email": "second@example.com", "role": "member"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("admin cannot grant owner", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", adminTok,
			map[string]string{"email": "second@example.com", "role": "owner"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("owner can grant owner", func(t *testing.T) {
		resp, js := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", ownerTok,
			map[string]string{"email": "second@example.com", "role": "owner"})
		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("status = %d body=%v, want 201", resp.StatusCode, js)
		}
	})

	t.Run("invalid role rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", ownerTok,
			map[string]string{"email": "third@example.com", "role": "superuser"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("missing email rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/members", ownerTok,
			map[string]string{"role": "member"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})
}

func TestUpdateMemberRole(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	ownerID := ts.userID(t, ownerTok)
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, adminID := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	_, memberID := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)

	t.Run("admin promotes member to admin", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+memberID, adminTok,
			map[string]string{"role": "admin"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		if js["role"] != "admin" {
			t.Fatalf("role = %v, want admin", js["role"])
		}
	})

	t.Run("admin cannot grant owner", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+memberID, adminTok,
			map[string]string{"role": "owner"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("admin cannot modify an owner row", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+ownerID, adminTok,
			map[string]string{"role": "admin"})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	// A state conflict, not a permission problem: the owner has the right to
	// demote, there is simply no other owner to take over.
	t.Run("last owner cannot be demoted", func(t *testing.T) {
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+ownerID, ownerTok,
			map[string]string{"role": "admin"})
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d body=%v, want 403 or 409", resp.StatusCode, js)
		}
		// Whatever the status, the owner must still be an owner.
		m, err := ts.store.GetMembership(t.Context(), orgID, ownerID)
		if err != nil {
			t.Fatalf("get membership: %v", err)
		}
		if m.Role != metadata.RoleOwner {
			t.Fatalf("owner was demoted to %s despite being the last owner", m.Role)
		}
	})

	t.Run("demote works once a second owner exists", func(t *testing.T) {
		if err := ts.store.UpdateMemberRole(t.Context(), orgID, adminID, metadata.RoleOwner); err != nil {
			t.Fatalf("seed second owner: %v", err)
		}
		resp, js := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+adminID, ownerTok,
			map[string]string{"role": "admin"})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
	})

	t.Run("unknown member is 404", func(t *testing.T) {
		// A well-formed uuid that is not a member — not a malformed one, which
		// would fail at the driver rather than exercising the handler.
		const ghost = "00000000-0000-4000-8000-000000000000"
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+ghost, ownerTok,
			map[string]string{"role": "admin"})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("invalid role rejected", func(t *testing.T) {
		resp, _ := ts.do(t, "PATCH", "/v1/orgs/"+orgID+"/members/"+memberID, ownerTok,
			map[string]string{"role": "root"})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})
}

func TestRemoveMember(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	ownerID := ts.userID(t, ownerTok)
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, adminID := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	memberTok, memberID := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)

	t.Run("admin cannot remove an owner", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/orgs/"+orgID+"/members/"+ownerID, adminTok, nil)
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("last owner cannot leave", func(t *testing.T) {
		resp, _ := ts.do(t, "DELETE", "/v1/orgs/"+orgID+"/members/"+ownerID, ownerTok, nil)
		if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusConflict {
			t.Fatalf("status = %d, want 403 or 409", resp.StatusCode)
		}
	})

	// Leaving is self-service: a member does not need admin rights to remove
	// their own row.
	t.Run("member leaves, then loses read access", func(t *testing.T) {
		resp, js := ts.do(t, "DELETE", "/v1/orgs/"+orgID+"/members/"+memberID, memberTok, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		resp2, _ := ts.do(t, "GET", "/v1/orgs/"+orgID, memberTok, nil)
		if resp2.StatusCode != http.StatusForbidden {
			t.Fatalf("org read after leaving = %d, want 403", resp2.StatusCode)
		}
	})

	t.Run("owner removes an admin", func(t *testing.T) {
		resp, js := ts.do(t, "DELETE", "/v1/orgs/"+orgID+"/members/"+adminID, ownerTok, nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		if _, err := ts.store.GetMembership(t.Context(), orgID, adminID); err == nil {
			t.Fatal("membership should be gone")
		}
	})
}

func TestTransferOwnership(t *testing.T) {
	ts := newTestServer(t)

	ownerTok := ts.register(t, "owner@example.com", "pw123456")
	ownerID := ts.userID(t, ownerTok)
	orgID := ts.createOrg(t, ownerTok, "Acme", "acme")
	adminTok, adminID := ts.registerMember(t, orgID, "admin@example.com", metadata.RoleAdmin)
	_, memberID := ts.registerMember(t, orgID, "member@example.com", metadata.RoleMember)
	outsiderTok := ts.register(t, "outsider@example.com", "pw123456")
	outsiderID := ts.userID(t, outsiderTok)

	t.Run("admin cannot transfer", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", adminTok,
			map[string]any{"user_id": memberID})
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("status = %d, want 403", resp.StatusCode)
		}
	})

	t.Run("target must be a member", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", ownerTok,
			map[string]any{"user_id": outsiderID})
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("status = %d, want 404", resp.StatusCode)
		}
	})

	t.Run("cannot transfer to yourself", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", ownerTok,
			map[string]any{"user_id": ownerID})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	t.Run("user_id required", func(t *testing.T) {
		resp, _ := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", ownerTok, map[string]any{})
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", resp.StatusCode)
		}
	})

	// Staying a co-owner is legitimate, so demote_self is opt-in.
	t.Run("without demote_self both are owners", func(t *testing.T) {
		resp, js := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", ownerTok,
			map[string]any{"user_id": memberID, "demote_self": false})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		for _, id := range []string{ownerID, memberID} {
			m, err := ts.store.GetMembership(t.Context(), orgID, id)
			if err != nil {
				t.Fatalf("get membership %s: %v", id, err)
			}
			if m.Role != metadata.RoleOwner {
				t.Fatalf("user %s role = %s, want owner", id, m.Role)
			}
		}
	})

	t.Run("with demote_self the caller becomes admin", func(t *testing.T) {
		resp, js := ts.do(t, "POST", "/v1/orgs/"+orgID+"/transfer-ownership", ownerTok,
			map[string]any{"user_id": adminID, "demote_self": true})
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d body=%v, want 200", resp.StatusCode, js)
		}
		newOwner, err := ts.store.GetMembership(t.Context(), orgID, adminID)
		if err != nil {
			t.Fatalf("get new owner: %v", err)
		}
		if newOwner.Role != metadata.RoleOwner {
			t.Fatalf("target role = %s, want owner", newOwner.Role)
		}
		caller, err := ts.store.GetMembership(t.Context(), orgID, ownerID)
		if err != nil {
			t.Fatalf("get caller: %v", err)
		}
		if caller.Role != metadata.RoleAdmin {
			t.Fatalf("caller role = %s, want admin after demote_self", caller.Role)
		}
		// The invariant that matters: an org always keeps at least one owner.
		owners, err := ts.store.CountOwners(t.Context(), orgID)
		if err != nil {
			t.Fatalf("count owners: %v", err)
		}
		if owners < 1 {
			t.Fatal("org left with zero owners")
		}
	})
}
