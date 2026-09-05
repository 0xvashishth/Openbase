"use client";

import { EmptyState } from "@/components/ui/feedback";
import { PageShell } from "@/components/layout/PageShell";

export default function OrgMembersPage() {
  return (
    <PageShell>
      <EmptyState
        title="Member management coming soon"
        hint="Organization roles (owner / admin / member) are enforced by the API. A full invite UI lands after the project flow refactor."
      />
    </PageShell>
  );
}
