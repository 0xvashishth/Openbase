"use client";

import { MembersPanel } from "@/components/orgs/MembersPanel";
import { PageShell } from "@/components/layout/PageShell";

export default function OrgMembersPage() {
  return (
    <PageShell>
      <MembersPanel />
    </PageShell>
  );
}
