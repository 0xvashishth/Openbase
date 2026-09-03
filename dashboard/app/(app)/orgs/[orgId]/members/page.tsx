"use client";

import { EmptyState, PageHeader } from "@/components/ui/feedback";

export default function OrgMembersPage() {
  return (
    <div>
      <PageHeader title="Members" subtitle="People with access to this organization." />
      <div className="mx-auto max-w-5xl px-6 py-6">
        <EmptyState
          title="Member management coming soon"
          hint="Organization roles (owner / admin / member) are enforced by the API. A full invite UI lands after the project flow refactor."
        />
      </div>
    </div>
  );
}
