import { describe, expect, it } from "vitest";
import { can, roleAtLeast, roleRank } from "./permissions";
import type { OrgRole } from "./types";

describe("roleRank", () => {
  it("orders owner > admin > member", () => {
    expect(roleRank("owner")).toBeGreaterThan(roleRank("admin"));
    expect(roleRank("admin")).toBeGreaterThan(roleRank("member"));
    expect(roleRank("member")).toBeGreaterThan(0);
  });
});

describe("roleAtLeast", () => {
  it("owner satisfies every minimum", () => {
    expect(roleAtLeast("owner", "owner")).toBe(true);
    expect(roleAtLeast("owner", "admin")).toBe(true);
    expect(roleAtLeast("owner", "member")).toBe(true);
  });

  it("admin satisfies admin and member but not owner", () => {
    expect(roleAtLeast("admin", "owner")).toBe(false);
    expect(roleAtLeast("admin", "admin")).toBe(true);
    expect(roleAtLeast("admin", "member")).toBe(true);
  });

  it("member satisfies only member", () => {
    expect(roleAtLeast("member", "owner")).toBe(false);
    expect(roleAtLeast("member", "admin")).toBe(false);
    expect(roleAtLeast("member", "member")).toBe(true);
  });
});

// Line-by-line parity check against the Go matrix in SCHEMA.md / middleware.go.
// Any divergence here means the UI will offer an action the server rejects
// (or hide one it would allow).
const MATRIX: Array<{
  action: string;
  owner: boolean;
  admin: boolean;
  member: boolean;
}> = [
  { action: "read", owner: true, admin: true, member: true },
  { action: "rename_org", owner: true, admin: true, member: false },
  { action: "delete_org", owner: true, admin: false, member: false },
  { action: "add_member", owner: true, admin: true, member: false },
  { action: "change_member_role", owner: true, admin: true, member: false },
  { action: "remove_member", owner: true, admin: true, member: false },
  { action: "transfer_ownership", owner: true, admin: false, member: false },
  { action: "create_project", owner: true, admin: true, member: false },
  { action: "rename_project", owner: true, admin: true, member: false },
  { action: "delete_project", owner: true, admin: false, member: false },
  { action: "save_source", owner: true, admin: true, member: false },
  { action: "revoke_api_key", owner: true, admin: true, member: false },
  { action: "raw_sql", owner: true, admin: true, member: false },
  { action: "trigger_fn_crud", owner: true, admin: true, member: false },
];

describe("can — parity with the Go RBAC matrix", () => {
  for (const row of MATRIX) {
    it(`${row.action}: owner=${row.owner} admin=${row.admin} member=${row.member}`, () => {
      expect(can("owner", row.action)).toBe(row.owner);
      expect(can("admin", row.action)).toBe(row.admin);
      expect(can("member", row.action)).toBe(row.member);
    });
  }

  it("denies unknown actions for every role", () => {
    const roles: OrgRole[] = ["owner", "admin", "member"];
    for (const role of roles) {
      expect(can(role, "not_a_real_action")).toBe(false);
    }
  });
});
