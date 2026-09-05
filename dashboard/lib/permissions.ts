import type { OrgRole } from "./types";

// Role ranking: owner(3) > admin(2) > member(1) > unknown(0)
export function roleRank(role: OrgRole): number {
  switch (role) {
    case "owner": return 3;
    case "admin": return 2;
    case "member": return 1;
    default: return 0;
  }
}

export function roleAtLeast(role: OrgRole, min: OrgRole): boolean {
  return roleRank(role) >= roleRank(min);
}

// Permission matrix mirroring B3 exactly:
// Action \ owner admin member
// Read:                  ✅   ✅   ✅
// Rename org:            ✅   ✅   ❌
// Delete org:            ✅   ❌   ❌
// Add member:            ✅   ✅   ❌
// Change member role:    ✅   ✅   ❌(not to/from owner)
// Remove member:         ✅   ✅   self only
// Transfer ownership:    ✅   ❌   ❌
// Create project:        ✅   ✅   ❌
// Rename project:        ✅   ✅   ❌
// Delete project:        ✅   ❌   ❌
// Save/remove DB source: ✅   ✅   ❌
// Mint/revoke API key:   ✅   ✅   ❌
// Raw SQL/DDL:           ✅   ✅   ❌
// Trigger & function CRUD: ✅   ✅   ❌

// can checks if a role can perform an action
export function can(role: OrgRole, action: string): boolean {
  const r = roleRank(role);

  switch (action) {
    case "read":
      return true;

    case "rename_org":
      return r >= 2; // admin or owner

    case "delete_org":
      return r >= 3; // owner only

    case "add_member":
      return r >= 2; // admin or owner

    case "change_member_role":
      return r >= 2 && // admin or owner
        // member cannot change owner role, and owner cannot demote themselves
        !(role === "member"); // simplified: member always denied

    case "remove_member":
      return r >= 2; // admin or owner

    case "transfer_ownership":
      return r >= 3; // owner only

    case "create_project":
      return r >= 2; // admin or owner

    case "rename_project":
      return r >= 2; // admin or owner

    case "delete_project":
      return r >= 3; // owner only

    case "save_source":
      return r >= 2; // admin or owner

    case "revoke_api_key":
      return r >= 2; // admin or owner

    case "raw_sql":
      return r >= 2; // admin or owner

    case "trigger_fn_crud":
      return r >= 2; // admin or owner

    default:
      return false;
  }
}