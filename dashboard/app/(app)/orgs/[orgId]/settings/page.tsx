"use client";

import { EmptyState } from "@/components/ui/feedback";
import { PageShell } from "@/components/layout/PageShell";

export default function OrgSettingsPage() {
  return (
    <PageShell>
      <EmptyState
        title="Organization settings coming soon"
        hint="Renaming and deleting organizations is available via the API. UI controls land after the project flow refactor."
      />
    </PageShell>
  );
}
