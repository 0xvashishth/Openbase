"use client";

import { EmptyState } from "@/components/ui/feedback";

export default function OrgMembersPage() {
  return (
    <div className="mx-auto max-w-5xl px-6 py-6">
      <EmptyState
        title="Member management coming soon"
        hint="Organization roles (owner / admin / member) are enforced by the API. A full invite UI lands after the project flow refactor."
      />
    </div>
  );
}
