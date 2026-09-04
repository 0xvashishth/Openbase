"use client";

import { EmptyState } from "@/components/ui/feedback";

export default function OrgSettingsPage() {
  return (
    <div className="mx-auto max-w-5xl px-6 py-6">
      <EmptyState
        title="Organization settings coming soon"
        hint="Renaming and deleting organizations is available via the API. UI controls land after the project flow refactor."
      />
    </div>
  );
}
